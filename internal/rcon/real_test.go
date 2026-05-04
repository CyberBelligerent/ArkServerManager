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

func TestRCON_ExecRequiresConnection(t *testing.T) {
	c := New()
	if _, err := c.Exec(context.Background(), "saveworld"); err == nil {
		t.Error("expected error on Exec without Dial")
	}
}
