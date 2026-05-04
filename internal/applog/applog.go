// Package applog wires the slog logger to a rotating file plus a fan-out hook
package applog

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
)

// Setup creates the log file under dataDir and returns a slog.Logger that
// writes JSON to both the file and stderr. The returned closer flushes
// the file on shutdown.
func Setup(dataDir string) (*slog.Logger, io.Closer, error) {
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return nil, nil, err
	}
	path := filepath.Join(dataDir, "asamanager.log")
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, nil, err
	}
	sink := &fanout{writers: []io.Writer{f, os.Stderr}}
	h := slog.NewJSONHandler(sink, &slog.HandlerOptions{Level: slog.LevelInfo})
	return slog.New(h), f, nil
}

// fanout is an io.Writer that duplicates writes across N writers. Used so
// the GUI can later attach a writer to receive log lines live.
type fanout struct {
	mu      sync.Mutex
	writers []io.Writer
}

// Attach adds w to the fanout and returns a detach func.
func (f *fanout) Attach(w io.Writer) func() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.writers = append(f.writers, w)
	idx := len(f.writers) - 1
	return func() {
		f.mu.Lock()
		defer f.mu.Unlock()
		f.writers[idx] = io.Discard
	}
}

func (f *fanout) Write(p []byte) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, w := range f.writers {
		_, _ = w.Write(p)
	}
	return len(p), nil
}
