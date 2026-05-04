package server

import (
	"bufio"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// tailARKLog opens <installDir>/ShooterGame/Saved/Logs/ShooterGame.log
// once it appears, then streams every line into hub
func tailARKLog(ctx context.Context, done <-chan struct{}, hub *lineHub, installDir string) {
	logPath := filepath.Join(installDir, "ShooterGame", "Saved", "Logs", "ShooterGame.log")

	var f *os.File
	for f == nil {
		select {
		case <-ctx.Done():
			return
		case <-done:
			return
		case <-time.After(500 * time.Millisecond):
		}
		if file, err := os.Open(logPath); err == nil {
			f = file
		}
	}
	defer f.Close()

	reader := bufio.NewReader(f)
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()

	for {
		// Drain whatever the reader has buffered.
		for {
			line, err := reader.ReadString('\n')
			if line != "" {
				hub.publish(strings.TrimRight(line, "\r\n"))
			}
			if err == io.EOF {
				break
			}
			if err != nil {
				return
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-done:
			for {
				line, err := reader.ReadString('\n')
				if line != "" {
					hub.publish(strings.TrimRight(line, "\r\n"))
				}
				if err != nil {
					return
				}
			}
		case <-ticker.C:
		}
	}
}
