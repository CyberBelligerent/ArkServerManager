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

func arkLogCurrentSize(installDir string) int64 {
	logPath := filepath.Join(installDir, "ShooterGame", "Saved", "Logs", "ShooterGame.log")
	stat, err := os.Stat(logPath)
	if err != nil {
		return 0
	}
	return stat.Size()
}

// tailARKLog opens <installDir>/ShooterGame/Saved/Logs/ShooterGame.log
// once it appears, then streams every line written from baseSize
// onwards into hub.
func tailARKLog(ctx context.Context, done <-chan struct{}, hub *lineHub, installDir string, baseSize int64) {
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

	if stat, err := f.Stat(); err == nil {
		offset := baseSize
		if stat.Size() < baseSize {
			offset = 0
		}
		if _, err := f.Seek(offset, io.SeekStart); err != nil {
			return
		}
	}

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
