package server

import (
	"bytes"
	"io"
	"sync"
)

// linePump is an io.Writer that splits incoming bytes on '\n', forwards
// each completed line to a lineHub, and also writes the raw bytes
// through to a downstream writer
type linePump struct {
	mu  sync.Mutex
	w   io.Writer
	hub *lineHub
	buf []byte
}

func newLinePump(w io.Writer, hub *lineHub) *linePump {
	return &linePump{w: w, hub: hub}
}

func (p *linePump) Write(b []byte) (int, error) {
	p.mu.Lock()
	p.buf = append(p.buf, b...)
	for {
		i := bytes.IndexByte(p.buf, '\n')
		if i < 0 {
			break
		}
		line := p.buf[:i]
		if n := len(line); n > 0 && line[n-1] == '\r' {
			line = line[:n-1]
		}
		p.hub.publish(string(line))
		p.buf = p.buf[i+1:]
	}
	p.mu.Unlock()
	if p.w != nil {
		return p.w.Write(b)
	}
	return len(b), nil
}

// lineHub is a tiny pub/sub for log lines
type lineHub struct {
	mu     sync.Mutex
	subs   []chan string
	closed bool
}

func newLineHub() *lineHub { return &lineHub{} }

func (h *lineHub) publish(line string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed {
		return
	}
	for _, ch := range h.subs {
		select {
		case ch <- line:
		default:
			// Subscriber not draining fast enough, just drop
		}
	}
}

func (h *lineHub) subscribe() <-chan string {
	h.mu.Lock()
	defer h.mu.Unlock()
	ch := make(chan string, 256)
	if h.closed {
		close(ch)
		return ch
	}
	h.subs = append(h.subs, ch)
	return ch
}

func (h *lineHub) close() {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed {
		return
	}
	h.closed = true
	for _, ch := range h.subs {
		close(ch)
	}
	h.subs = nil
}
