package server

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestTailARKLog_SkipsHistoricalContent(t *testing.T) {
	installDir := t.TempDir()
	logDir := filepath.Join(installDir, "ShooterGame", "Saved", "Logs")
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		t.Fatal(err)
	}
	logPath := filepath.Join(logDir, "ShooterGame.log")

	// Simulate a previous run leaving a "Server ready" line on disk
	historical := "previous run boot\nServer ready: TheIsland_WP\n"
	if err := os.WriteFile(logPath, []byte(historical), 0o644); err != nil {
		t.Fatal(err)
	}
	baseSize := arkLogCurrentSize(installDir)
	if baseSize != int64(len(historical)) {
		t.Fatalf("baseSize = %d, want %d", baseSize, len(historical))
	}

	hub := newLineHub()
	sub := hub.subscribe()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go tailARKLog(ctx, done, hub, installDir, baseSize)

	time.Sleep(750 * time.Millisecond)

	f, err := os.OpenFile(logPath, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString("new run starting up\n"); err != nil {
		t.Fatal(err)
	}
	_ = f.Close()

	deadline := time.After(750 * time.Millisecond)
	var got []string
loop:
	for {
		select {
		case line, ok := <-sub:
			if !ok {
				break loop
			}
			got = append(got, line)
		case <-deadline:
			break loop
		}
	}

	for _, line := range got {
		if line == "Server ready: TheIsland_WP" {
			t.Errorf("tailer replayed historical 'Server ready' line; got lines=%v", got)
		}
		if line == "previous run boot" {
			t.Errorf("tailer replayed historical line; got lines=%v", got)
		}
	}
	sawNew := false
	for _, line := range got {
		if line == "new run starting up" {
			sawNew = true
		}
	}
	if !sawNew {
		t.Errorf("tailer missed the new run's line; got lines=%v", got)
	}
}

func TestTailARKLog_ResumesAfterTruncation(t *testing.T) {
	installDir := t.TempDir()
	logDir := filepath.Join(installDir, "ShooterGame", "Saved", "Logs")
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		t.Fatal(err)
	}
	logPath := filepath.Join(logDir, "ShooterGame.log")

	// Pretend the previous run left a large file behind
	if err := os.WriteFile(logPath, []byte("AAAAAAAAAAAAAAAAAAAAAAAA\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	baseSize := arkLogCurrentSize(installDir)

	if err := os.WriteFile(logPath, []byte("fresh\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	hub := newLineHub()
	sub := hub.subscribe()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go tailARKLog(ctx, done, hub, installDir, baseSize)

	deadline := time.After(2 * time.Second)
	for {
		select {
		case line := <-sub:
			if line == "fresh" {
				return // pass
			}
		case <-deadline:
			t.Fatal("did not see 'fresh' line within deadline")
		}
	}
}
