//go:build windows

package steamcmd

import (
	"bufio"
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/UserExistsError/conpty"
)

// installAppPTY runs SteamCMD via a Windows pseudo-console (ConPTY).
// SteamCMD's libc detects the PTY as a TTY and switches stdout to
// line-buffered mode, so progress lines appear in real time instead
// of being held until the underlying pipe buffer fills up. Without
// this, "downloading 35%\r" updates accumulate in the libc buffer and
// only flush at process exit, making the install look hung.
func (r *RealRunner) installAppPTY(ctx context.Context, appID int, installDir string, validate bool) (<-chan string, error) {
	path := r.Path()
	if path == "" {
		return nil, fmt.Errorf("steamcmd path not set")
	}
	args := []string{
		"+force_install_dir", `"` + installDir + `"`,
		"+login", "anonymous",
		"+app_info_update", "1",
		"+app_update", strconv.Itoa(appID),
	}
	if validate {
		args = append(args, "validate")
	}
	args = append(args, "+quit")
	cmdline := `"` + path + `" ` + strings.Join(args, " ")

	cpty, err := conpty.Start(cmdline)
	if err != nil {
		return nil, fmt.Errorf("conpty start: %w", err)
	}

	out := make(chan string, 64)
	scannerDone := make(chan struct{})
	go func() {
		defer close(scannerDone)
		s := bufio.NewScanner(cpty)
		s.Buffer(make([]byte, 64*1024), 1024*1024)
		s.Split(scanLinesAndCR)
		for s.Scan() {
			line := stripANSI(s.Text())
			if line == "" {
				continue
			}
			out <- line
		}
	}()
	go func() {
		code, waitErr := cpty.Wait(ctx)
		_ = cpty.Close()
		<-scannerDone
		switch {
		case waitErr != nil:
			out <- fmt.Sprintf("[exit] %v", waitErr)
		case code == 7 || code == 8:
			out <- "[note] SteamCMD self-update finished (non-zero exit is normal on Windows)."
		case code != 0:
			out <- fmt.Sprintf("[exit] code %d", code)
		}
		close(out)
	}()
	return out, nil
}

var ansiRE = regexp.MustCompile("\x1b\\[[0-9;?]*[A-Za-z]")

func stripANSI(s string) string {
	return ansiRE.ReplaceAllString(s, "")
}
