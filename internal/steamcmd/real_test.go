package steamcmd

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestDetect_FindsExisting(t *testing.T) {
	tmp := t.TempDir()
	fakePath := filepath.Join(tmp, executableName())
	if err := os.WriteFile(fakePath, []byte{}, 0o755); err != nil {
		t.Fatal(err)
	}
	r := New()
	got, err := r.detect([]string{fakePath}, failingLookup)
	if err != nil {
		t.Fatal(err)
	}
	if got != fakePath {
		t.Errorf("got %q, want %q", got, fakePath)
	}
	if r.Path() != fakePath {
		t.Errorf("path not stored: %q", r.Path())
	}
}

func TestDetect_PrefersFirstHit(t *testing.T) {
	tmp := t.TempDir()
	a := filepath.Join(tmp, "a-"+executableName())
	b := filepath.Join(tmp, "b-"+executableName())
	if err := os.WriteFile(a, []byte{}, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(b, []byte{}, 0o755); err != nil {
		t.Fatal(err)
	}
	r := New()
	got, _ := r.detect([]string{a, b}, failingLookup)
	if got != a {
		t.Errorf("got %q, want %q (first candidate)", got, a)
	}
}

func TestDetect_FallsBackToPathLookup(t *testing.T) {
	r := New()
	tmp := t.TempDir()
	resolved := filepath.Join(tmp, executableName())
	if err := os.WriteFile(resolved, []byte{}, 0o755); err != nil {
		t.Fatal(err)
	}
	got, err := r.detect(nil, func(string) (string, error) { return resolved, nil })
	if err != nil {
		t.Fatal(err)
	}
	if got != resolved {
		t.Errorf("got %q, want %q", got, resolved)
	}
}

func TestDetect_NotFound(t *testing.T) {
	r := New()
	got, err := r.detect(nil, failingLookup)
	if err != nil {
		t.Fatal(err)
	}
	if got != "" {
		t.Errorf("expected empty, got %q", got)
	}
}

func TestInstall_SkipsIfPresent(t *testing.T) {
	tmp := t.TempDir()
	target := filepath.Join(tmp, executableName())
	if err := os.WriteFile(target, []byte("existing"), 0o755); err != nil {
		t.Fatal(err)
	}
	r := New()
	if err := r.Install(context.Background(), tmp); err != nil {
		t.Fatalf("Install: %v", err)
	}
	if r.Path() != target {
		t.Errorf("path %q != expected %q", r.Path(), target)
	}
	body, _ := os.ReadFile(target)
	if string(body) != "existing" {
		t.Errorf("file overwritten: %q", string(body))
	}
}

func TestInstall_DownloadsAndExtracts(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Install download path is Windows-only in v1")
	}
	zipBody := buildZip(t, map[string][]byte{
		executableName(): []byte("fake-steamcmd-binary"),
	})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/zip")
		_, _ = w.Write(zipBody)
	}))
	defer srv.Close()

	origURL := DownloadURL
	DownloadURL = srv.URL
	defer func() { DownloadURL = origURL }()

	tmp := t.TempDir()
	r := New()
	if err := r.Install(context.Background(), tmp); err != nil {
		t.Fatalf("Install: %v", err)
	}
	target := filepath.Join(tmp, executableName())
	body, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "fake-steamcmd-binary" {
		t.Errorf("body = %q", string(body))
	}
	if r.Path() != target {
		t.Errorf("path = %q, want %q", r.Path(), target)
	}
}

func TestInstall_RejectsBadHTTPStatus(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Install download path is Windows-only in v1")
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "nope", http.StatusInternalServerError)
	}))
	defer srv.Close()
	origURL := DownloadURL
	DownloadURL = srv.URL
	defer func() { DownloadURL = origURL }()

	r := New()
	err := r.Install(context.Background(), t.TempDir())
	if err == nil {
		t.Fatal("expected error from non-200 status")
	}
}

func TestExtractZip_BasicFlow(t *testing.T) {
	zipBody := buildZip(t, map[string][]byte{
		"a.txt":     []byte("alpha"),
		"sub/b.txt": []byte("bravo"),
	})
	tmp := t.TempDir()
	if err := extractZip(zipBody, tmp); err != nil {
		t.Fatalf("extract: %v", err)
	}
	a, err := os.ReadFile(filepath.Join(tmp, "a.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(a) != "alpha" {
		t.Errorf("a.txt = %q", string(a))
	}
	b, err := os.ReadFile(filepath.Join(tmp, "sub", "b.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != "bravo" {
		t.Errorf("sub/b.txt = %q", string(b))
	}
}

func TestExtractZip_RejectsZipSlip(t *testing.T) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	f, err := zw.Create("../escape.exe")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Write([]byte("evil")); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := extractZip(buf.Bytes(), t.TempDir()); err == nil {
		t.Fatal("expected zip-slip rejection")
	}
}

func TestDefaultInstallDir(t *testing.T) {
	p, err := DefaultInstallDir()
	if err != nil {
		t.Fatal(err)
	}
	if p == "" {
		t.Error("empty install dir")
	}
	if !filepath.IsAbs(p) {
		t.Errorf("expected absolute path, got %q", p)
	}
}

func TestSelfUpdate_ErrorsWithoutPath(t *testing.T) {
	r := New()
	if err := r.SelfUpdate(context.Background()); err == nil {
		t.Fatal("expected error when path is unset")
	}
}

func TestInstallApp_ErrorsWithoutPath(t *testing.T) {
	r := New()
	_, err := r.InstallApp(context.Background(), AppIDARKAscended, t.TempDir(), false)
	if err == nil {
		t.Fatal("expected error when path is unset")
	}
}

// failingLookup is a stand-in for exec.LookPath that always fails
func failingLookup(string) (string, error) {
	return "", errors.New("not on PATH")
}

func buildZip(t *testing.T, files map[string][]byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, body := range files {
		f, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := f.Write(body); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}
