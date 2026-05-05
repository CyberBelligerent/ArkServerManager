package rcon

import (
	"context"
	"net"
	"strings"
	"testing"
	"time"
)

// fakeServer answers a single RCON connection with the supplied handler
// and returns its TCP address. Cleanup tears down the listener.
func fakeServer(t *testing.T, handle func(net.Conn)) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		handle(conn)
	}()
	return ln.Addr().String()
}

func splitHostPort(t *testing.T, addr string) (string, int) {
	t.Helper()
	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		t.Fatal(err)
	}
	tcp, err := net.ResolveTCPAddr("tcp", "127.0.0.1:"+portStr)
	if err != nil {
		t.Fatal(err)
	}
	return host, tcp.Port
}

func TestRCON_AuthAndExec(t *testing.T) {
	addr := fakeServer(t, func(conn net.Conn) {
		for {
			p, err := readPacket(conn)
			if err != nil {
				return
			}
			switch p.Type {
			case typeAuth:
				_ = (packet{ID: p.ID, Type: typeResponseValue}).encode(conn)
				if p.Body == "secret" {
					_ = (packet{ID: p.ID, Type: typeAuthResponse}).encode(conn)
				} else {
					_ = (packet{ID: -1, Type: typeAuthResponse}).encode(conn)
				}
			case typeExecCommand:
				_ = (packet{ID: p.ID, Type: typeResponseValue, Body: "hello "}).encode(conn)
				_ = (packet{ID: p.ID, Type: typeResponseValue, Body: "world"}).encode(conn)
			case typeResponseValue:
				_ = (packet{ID: p.ID, Type: typeResponseValue}).encode(conn)
			}
		}
	})
	host, port := splitHostPort(t, addr)
	c := New()
	c.SetTimeout(2 * time.Second)
	if err := c.Dial(context.Background(), host, port, "secret"); err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer c.Close()
	out, err := c.Exec(context.Background(), "say hi")
	if err != nil {
		t.Fatalf("Exec: %v", err)
	}
	if out != "hello world" {
		t.Errorf("Exec body = %q, want %q", out, "hello world")
	}
}

func TestRCON_BadPassword(t *testing.T) {
	addr := fakeServer(t, func(conn net.Conn) {
		p, err := readPacket(conn)
		if err != nil || p.Type != typeAuth {
			return
		}
		_ = (packet{ID: -1, Type: typeAuthResponse}).encode(conn)
	})
	host, port := splitHostPort(t, addr)
	c := New()
	c.SetTimeout(2 * time.Second)
	err := c.Dial(context.Background(), host, port, "wrong")
	if err == nil || !strings.Contains(err.Error(), "bad password") {
		t.Errorf("expected bad password error, got %v", err)
	}
}

func TestRCON_ASAExec_NoMarkerEcho(t *testing.T) {
	addr := fakeServer(t, func(conn net.Conn) {
		for {
			p, err := readPacket(conn)
			if err != nil {
				return
			}
			switch p.Type {
			case typeAuth:
				_ = (packet{ID: p.ID, Type: typeAuthResponse}).encode(conn)
			case typeExecCommand:
				_ = (packet{ID: p.ID, Type: typeResponseValue, Body: "0. Alice, 76561198000000001\n"}).encode(conn)
				// Intentionally do NOT echo the trailing RESPONSE_VALUE
			}
		}
	})
	host, port := splitHostPort(t, addr)
	c := New()
	c.SetTimeout(5 * time.Second) // longer than tailReadTimeout
	if err := c.Dial(context.Background(), host, port, "secret"); err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer c.Close()
	start := time.Now()
	out, err := c.Exec(context.Background(), "listplayers")
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("Exec: %v", err)
	}
	if !strings.Contains(out, "Alice") {
		t.Errorf("Exec body = %q, want it to contain Alice", out)
	}
	// Should return shortly after tailReadTimeout, not 5 seconds later
	if elapsed > 2*time.Second {
		t.Errorf("Exec took too long without marker echo: %s", elapsed)
	}
}

func TestRCON_ASAExec_MismatchedID(t *testing.T) {
	addr := fakeServer(t, func(conn net.Conn) {
		for {
			p, err := readPacket(conn)
			if err != nil {
				return
			}
			switch p.Type {
			case typeAuth:
				_ = (packet{ID: p.ID, Type: typeAuthResponse}).encode(conn)
			case typeExecCommand:
				// Reply with an unrelated ID
				_ = (packet{ID: 9999, Type: typeResponseValue, Body: "No Players Connected\n"}).encode(conn)
			}
		}
	})
	host, port := splitHostPort(t, addr)
	c := New()
	c.SetTimeout(5 * time.Second)
	if err := c.Dial(context.Background(), host, port, "secret"); err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer c.Close()
	out, err := c.Exec(context.Background(), "listplayers")
	if err != nil {
		t.Fatalf("Exec: %v", err)
	}
	if !strings.Contains(out, "No Players Connected") {
		t.Errorf("Exec body = %q, want it to contain 'No Players Connected'", out)
	}
}

func TestRCON_Exec_RedialsOnStaleConn(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })

	connNum := 0
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			connNum++
			n := connNum
			go func(conn net.Conn, n int) {
				defer conn.Close()
				for {
					p, err := readPacket(conn)
					if err != nil {
						return
					}
					switch p.Type {
					case typeAuth:
						_ = (packet{ID: p.ID, Type: typeAuthResponse}).encode(conn)
						if n == 1 {
							// Simulate ASA idle-closing the first conn
							return
						}
					case typeExecCommand:
						_ = (packet{ID: p.ID, Type: typeResponseValue, Body: "after-redial"}).encode(conn)
					}
				}
			}(conn, n)
		}
	}()

	host, port := splitHostPort(t, ln.Addr().String())
	c := New()
	c.SetTimeout(3 * time.Second)
	if err := c.Dial(context.Background(), host, port, "secret"); err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer c.Close()

	// Drop a connection
	time.Sleep(100 * time.Millisecond)

	out, err := c.Exec(context.Background(), "listplayers")
	if err != nil {
		t.Fatalf("Exec: %v", err)
	}
	if !strings.Contains(out, "after-redial") {
		t.Errorf("Exec body = %q, want it to contain 'after-redial'", out)
	}
	if connNum < 2 {
		t.Errorf("expected redial (connNum >= 2), got %d", connNum)
	}
}

func TestRCON_ExecRequiresConnection(t *testing.T) {
	c := New()
	if _, err := c.Exec(context.Background(), "saveworld"); err == nil {
		t.Error("expected error on Exec without Dial")
	}
}
