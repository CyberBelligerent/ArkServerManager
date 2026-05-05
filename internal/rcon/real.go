package rcon

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

var _ Client = (*RealClient)(nil)

// RealClient is a Source RCON client over TCP
type RealClient struct {
	mu      sync.Mutex
	conn    net.Conn
	nextID  atomic.Int32
	timeout time.Duration

	host string
	port int
	pwd  string
}

func New() *RealClient {
	return &RealClient{timeout: 10 * time.Second}
}

func (c *RealClient) SetTimeout(d time.Duration) {
	c.mu.Lock()
	c.timeout = d
	c.mu.Unlock()
}

func (c *RealClient) Dial(ctx context.Context, host string, port int, password string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.host = host
	c.port = port
	c.pwd = password
	return c.dialLocked(ctx)
}

// dialLocked opens the TCP connection and authenticates. The caller must hold c.mu.
func (c *RealClient) dialLocked(ctx context.Context) error {
	if c.conn != nil {
		_ = c.conn.Close()
		c.conn = nil
	}
	d := net.Dialer{Timeout: c.timeout}
	conn, err := d.DialContext(ctx, "tcp", net.JoinHostPort(c.host, strconv.Itoa(c.port)))
	if err != nil {
		return fmt.Errorf("rcon dial: %w", err)
	}
	
	// TCP keepalives
	if tcp, ok := conn.(*net.TCPConn); ok {
		_ = tcp.SetKeepAlive(true)
		_ = tcp.SetKeepAlivePeriod(15 * time.Second)
	}
	c.conn = conn
	if err := c.authLocked(ctx, c.pwd); err != nil {
		_ = conn.Close()
		c.conn = nil
		return err
	}
	return nil
}

func (c *RealClient) authLocked(ctx context.Context, password string) error {
	c.applyDeadline(ctx)
	id := c.nextID.Add(1)
	if err := (packet{ID: id, Type: typeAuth, Body: password}).encode(c.conn); err != nil {
		return fmt.Errorf("rcon auth send: %w", err)
	}
	// Read responses until we see the AUTH_RESPONSE
	for {
		resp, err := readPacket(c.conn)
		if err != nil {
			return fmt.Errorf("rcon auth recv: %w", err)
		}
		if resp.Type != typeAuthResponse {
			continue
		}
		if resp.ID == -1 {
			return errors.New("rcon auth failed: bad password")
		}
		if resp.ID != id {
			return fmt.Errorf("rcon auth: id mismatch (got %d, want %d)", resp.ID, id)
		}
		return nil
	}
}

const tailReadTimeout = 750 * time.Millisecond

// Exec runs cmd and returns the joined response body
func (c *RealClient) Exec(ctx context.Context, cmd string) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	out, err := c.execLocked(ctx, cmd)
	if err == nil {
		return out, nil
	}
	if c.host == "" {
		return "", err
	}
	// Drop the stale connection and try once with a fresh one.
	if c.conn != nil {
		_ = c.conn.Close()
		c.conn = nil
	}
	if dErr := c.dialLocked(ctx); dErr != nil {
		return "", fmt.Errorf("%w (reconnect failed: %v)", err, dErr)
	}
	return c.execLocked(ctx, cmd)
}

// execLocked sends cmd and reads the response. The caller must hold c.mu.
func (c *RealClient) execLocked(ctx context.Context, cmd string) (string, error) {
	if c.conn == nil {
		return "", errors.New("rcon: not connected")
	}
	c.applyDeadline(ctx)

	cmdID := c.nextID.Add(1)

	if err := (packet{ID: cmdID, Type: typeExecCommand, Body: cmd}).encode(c.conn); err != nil {
		return "", fmt.Errorf("rcon exec send: %w", err)
	}

	var body strings.Builder
	gotAny := false
	for {
		resp, err := readPacket(c.conn)
		if err != nil {
			if gotAny {
				var ne net.Error
				if errors.As(err, &ne) && ne.Timeout() {
					return body.String(), nil
				}
			}
			return "", fmt.Errorf("rcon exec recv: %w", err)
		}

		body.WriteString(resp.Body)
		gotAny = true
		tailDL := time.Now().Add(tailReadTimeout)
		if d, ok := ctx.Deadline(); ok && d.Before(tailDL) {
			tailDL = d
		}
		_ = c.conn.SetDeadline(tailDL)
	}
}

func (c *RealClient) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.conn == nil {
		return nil
	}
	err := c.conn.Close()
	c.conn = nil
	return err
}

func (c *RealClient) applyDeadline(ctx context.Context) {
	if c.conn == nil {
		return
	}
	if d, ok := ctx.Deadline(); ok {
		_ = c.conn.SetDeadline(d)
		return
	}
	_ = c.conn.SetDeadline(time.Now().Add(c.timeout))
}
