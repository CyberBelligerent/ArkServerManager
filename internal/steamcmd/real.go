package steamcmd

import (
	"archive/zip"
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)


const AppIDARKAscended = 2430930

var DownloadURL = "https://steamcdn-a.akamaihd.net/client/installer/steamcmd.zip"

var _ Runner = (*RealRunner)(nil)

type RealRunner struct {
	mu          sync.Mutex
	path        string
	httpClient  *http.Client
	bootstrapFn func(ctx context.Context) error
	progressFn  func(string)
}

func New() *RealRunner {
	r := &RealRunner{httpClient: &http.Client{Timeout: 5 * time.Minute}}
	r.bootstrapFn = r.bootstrap
	return r
}

func NewWithPath(path string) *RealRunner {
	r := New()
	r.path = path
	return r
}

func (r *RealRunner) Path() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.path
}

func (r *RealRunner) SetPath(p string) {
	r.mu.Lock()
	r.path = p
	r.mu.Unlock()
}

func (r *RealRunner) SetProgress(fn func(string)) {
	r.mu.Lock()
	r.progressFn = fn
	r.mu.Unlock()
}

func (r *RealRunner) progress(msg string) {
	r.mu.Lock()
	fn := r.progressFn
	r.mu.Unlock()
	if fn != nil {
		fn(msg)
	}
}

func (r *RealRunner) Detect(_ context.Context) (string, error) {
	return r.detect(r.detectCandidates(), exec.LookPath)
}

func (r *RealRunner) detect(candidates []string, lookup func(string) (string, error)) (string, error) {
	for _, c := range candidates {
		if c == "" {
			continue
		}
		if fileExists(c) {
			r.SetPath(c)
			return c, nil
		}
	}
	if p, err := lookup(executableName()); err == nil {
		r.SetPath(p)
		return p, nil
	}
	return "", nil
}

func (r *RealRunner) detectCandidates() []string {
	out := []string{r.Path()}
	out = append(out, defaultDetectPaths()...)
	return out
}

func (r *RealRunner) Install(ctx context.Context, installDir string) error {
	if installDir == "" {
		return errors.New("installDir is required")
	}
	if err := os.MkdirAll(installDir, 0o755); err != nil {
		return fmt.Errorf("create install dir: %w", err)
	}
	target := filepath.Join(installDir, executableName())
	if fileExists(target) {
		r.SetPath(target)
		if r.isBootstrapped() {
			return nil
		}
		
		if err := r.bootstrapFn(ctx); err != nil {
			return fmt.Errorf("bootstrap steamcmd: %w", err)
		}
		return nil
	}
	if runtime.GOOS != "windows" {
		return errors.New("steamcmd auto-install is implemented for Windows only; install steamcmd manually on this platform")
	}
	r.progress("Downloading SteamCMD…")
	body, err := r.download(ctx, DownloadURL)
	if err != nil {
		return err
	}
	r.progress("Extracting SteamCMD…")
	if err := extractZip(body, installDir); err != nil {
		return fmt.Errorf("extract steamcmd: %w", err)
	}
	if !fileExists(target) {
		return fmt.Errorf("steamcmd executable missing after extract: %s", target)
	}
	r.SetPath(target)

	if err := r.bootstrapFn(ctx); err != nil {
		return fmt.Errorf("bootstrap steamcmd: %w", err)
	}
	return nil
}

// bootstrap brings a freshly-extracted SteamCMD to a usable state
// runs +quit and then a second pass to warm up configuration
func (r *RealRunner) bootstrap(ctx context.Context) error {
	runQuit := func() {
		cmd := exec.CommandContext(ctx, r.Path(), "+quit")
		_ = cmd.Run()
	}
	r.progress("First-run setup, pass 1/2 (this may take a minute)…")
	runQuit()
	if !r.isBootstrapped() {
		r.progress("First-run setup, pass 2/2…")
		runQuit()
	}
	if !r.isBootstrapped() {
		return fmt.Errorf("steamcmd bootstrap did not complete: missing %s after self-update (run steamcmd manually to diagnose)", r.bootstrapMarkerPath())
	}

	r.progress("Warming SteamCMD app cache…")
	cmd := exec.CommandContext(ctx, r.Path(),
		"+login", "anonymous",
		"+app_info_update", "1",
		"+quit")
	if err := cmd.Run(); err != nil && !isBenignExit(err) {
		r.progress(fmt.Sprintf("Cache warm-up returned %v (continuing).", err))
	}
	r.progress("SteamCMD ready.")
	return nil
}

// isBootstrapped reports whether SteamCMD has finished downloading its support files
func (r *RealRunner) isBootstrapped() bool {
	marker := r.bootstrapMarkerPath()
	return marker != "" && fileExists(marker)
}

func (r *RealRunner) bootstrapMarkerPath() string {
	p := r.Path()
	if p == "" {
		return ""
	}
	dir := filepath.Dir(p)
	if runtime.GOOS == "windows" {
		return filepath.Join(dir, "steamerrorreporter.exe")
	}
	return filepath.Join(dir, "linux32", "steamcmd")
}

func (r *RealRunner) download(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := r.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("download steamcmd: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download steamcmd: HTTP %d", resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}

func (r *RealRunner) SelfUpdate(ctx context.Context) error {
	path := r.Path()
	if path == "" {
		return errors.New("steamcmd path not set; call Detect or Install first")
	}
	cmd := exec.CommandContext(ctx, path, "+quit")
	if err := cmd.Run(); err != nil && !isBenignExit(err) {
		return err
	}
	return nil
}

func (r *RealRunner) InstallApp(ctx context.Context, appID int, installDir string, validate bool) (<-chan string, error) {
	if r.Path() == "" {
		return nil, errors.New("steamcmd path not set; call Detect or Install first")
	}
	if installDir == "" {
		return nil, errors.New("installDir is required")
	}
	if err := os.MkdirAll(installDir, 0o755); err != nil {
		return nil, fmt.Errorf("create install dir: %w", err)
	}
	if runtime.GOOS == "windows" {
		if ch, err := r.installAppPTY(ctx, appID, installDir, validate); err == nil {
			return ch, nil
		}
	}
	return r.installAppPipe(ctx, appID, installDir, validate)
}

func (r *RealRunner) installAppPipe(ctx context.Context, appID int, installDir string, validate bool) (<-chan string, error) {
	path := r.Path()
	args := []string{
		"+force_install_dir", installDir,
		"+login", "anonymous",
		"+app_info_update", "1",
		"+app_update", strconv.Itoa(appID),
	}
	if validate {
		args = append(args, "validate")
	}
	args = append(args, "+quit")

	cmd := exec.CommandContext(ctx, path, args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}

	out := make(chan string, 64)
	var wg sync.WaitGroup
	wg.Add(2)
	go pumpLines(stdout, out, &wg)
	go pumpLines(stderr, out, &wg)
	go func() {
		wg.Wait()
		if err := cmd.Wait(); err != nil {
			if isBenignExit(err) {
				out <- "[note] SteamCMD self-update finished (non-zero exit is normal on Windows)."
			} else {
				out <- fmt.Sprintf("[exit] %v", err)
			}
		}
		close(out)
	}()
	return out, nil
}

func DefaultInstallDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	if runtime.GOOS == "windows" {
		return filepath.Join(home, "ASAManager", "steamcmd"), nil
	}
	return filepath.Join(home, ".local", "share", "asamanager", "steamcmd"), nil
}

func defaultDetectPaths() []string {
	var paths []string
	if home, err := os.UserHomeDir(); err == nil {
		paths = append(paths,
			filepath.Join(home, "ASAManager", "steamcmd", executableName()),
			filepath.Join(home, "steamcmd", executableName()),
		)
	}
	if runtime.GOOS == "windows" {
		paths = append(paths, `C:\steamcmd\`+executableName())
	}
	return paths
}

func executableName() string {
	if runtime.GOOS == "windows" {
		return "steamcmd.exe"
	}
	return "steamcmd.sh"
}

func fileExists(path string) bool {
	if path == "" {
		return false
	}
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return !info.IsDir()
}

func pumpLines(r io.Reader, out chan<- string, wg *sync.WaitGroup) {
	defer wg.Done()
	s := bufio.NewScanner(r)
	s.Buffer(make([]byte, 64*1024), 1024*1024)
	s.Split(scanLinesAndCR)
	for s.Scan() {
		line := s.Text()
		if line == "" {
			continue
		}
		out <- line
	}
}

func scanLinesAndCR(data []byte, atEOF bool) (advance int, token []byte, err error) {
	if atEOF && len(data) == 0 {
		return 0, nil, nil
	}
	for i, b := range data {
		if b == '\n' || b == '\r' {
			return i + 1, data[:i], nil
		}
	}
	if atEOF {
		return len(data), data, nil
	}
	return 0, nil, nil
}

func extractZip(body []byte, destDir string) error {
	zr, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
	if err != nil {
		return err
	}
	cleanDest := filepath.Clean(destDir)
	for _, zf := range zr.File {
		target := filepath.Join(cleanDest, zf.Name)
		rel, err := filepath.Rel(cleanDest, target)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return fmt.Errorf("zip entry escapes dest: %s", zf.Name)
		}
		if zf.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		if err := writeZipEntry(zf, target); err != nil {
			return err
		}
	}
	return nil
}

func writeZipEntry(zf *zip.File, target string) error {
	rc, err := zf.Open()
	if err != nil {
		return err
	}
	defer rc.Close()
	f, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o755)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(f, rc)
	return err
}

func isBenignExit(err error) bool {
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		switch ee.ExitCode() {
		case 7, 8:
			return true
		}
	}
	return false
}
