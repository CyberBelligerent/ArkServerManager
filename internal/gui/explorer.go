package gui

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

func openInFileExplorer(path string) error {
	if path == "" {
		return errors.New("path is empty")
	}
	target := path
	if _, err := os.Stat(target); err != nil {
		parent := filepath.Dir(target)
		if _, err := os.Stat(parent); err != nil {
			return fmt.Errorf("path does not exist: %s", path)
		}
		target = parent
	}

	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("explorer", target)
	case "darwin":
		cmd = exec.Command("open", target)
	case "linux":
		cmd = exec.Command("xdg-open", target)
	default:
		return fmt.Errorf("open in file explorer: unsupported platform %s", runtime.GOOS)
	}
	return cmd.Start()
}
