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

	if c.conn != nil {
		_ = c.conn.Close()
		c.conn = nil
	}
	d := net.Dialer{Timeout: c.timeout}
	conn, err := d.DialContext(ctx, "tcp", net.JoinHostPort(host, strconv.Itoa(port)))
	if err != nil {
		return fmt.Errorf("rcon dial: %w", err)
	}
	c.conn = conn
	if err := c.authLocked(ctx, password); err != nil {
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

// Exec runs cmd and returns the joined response body
func (c *RealClient) Exec(ctx context.Context, cmd string) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.conn == nil {
		return "", errors.New("rcon: not connected")
	}
	c.applyDeadline(ctx)

	cmdID := c.nextID.Add(1)
	markerID := c.nextID.Add(1)

	if err := (packet{ID: cmdID, Type: typeExecCommand, Body: cmd}).encode(c.conn); err != nil {
		return "", fmt.Errorf("rcon exec send: %w", err)
	}
	// The marker is a RESPONSE_VALUE packet the server can't actually process
	if err := (packet{ID: markerID, Type: typeResponseValue}).encode(c.conn); err != nil {
		return "", fmt.Errorf("rcon marker send: %w", err)
	}

	var body strings.Builder
	for {
		resp, err := readPacket(c.conn)
		if err != nil {
			return "", fmt.Errorf("rcon exec recv: %w", err)
		}
		switch resp.ID {
		case markerID:
			return body.String(), nil
		case cmdID:
			body.WriteString(resp.Body)
		}
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
