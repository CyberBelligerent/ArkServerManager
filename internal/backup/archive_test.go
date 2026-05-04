package backup

import (
	"archive/zip"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"asamanager/internal/server"
)

func writeMaliciousZip(path, entryName, body string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	zw := zip.NewWriter(f)
	w, err := zw.Create(entryName)
	if err != nil {
		return err
	}
	if _, err := w.Write([]byte(body)); err != nil {
		return err
	}
	return zw.Close()
}

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestWriteServerArchive_RoundTrip(t *testing.T) {
	root := t.TempDir()
	install := filepath.Join(root, "Island")

	writeFile(t, filepath.Join(install, savedArksRel, "Island.ark"), "saveblob")
	writeFile(t, filepath.Join(install, savedArksRel, "Sub", "Island.profile"), "profile")
	writeFile(t, filepath.Join(install, configRel, "Game.ini"), "[/script/...]\nMatingIntervalMultiplier=0.4\n")
	writeFile(t, filepath.Join(install, configRel, "GameUserSettings.ini"), "[ServerSettings]\nXPMultiplier=2.0\n")

	srv := server.Server{ID: 1, Name: "Island", InstallDir: install}
	dest := filepath.Join(root, "out", "server-1.zip")
	size, err := WriteServerArchive(srv, dest)
	if err != nil {
		t.Fatalf("WriteServerArchive: %v", err)
	}
	if size <= 0 {
		t.Fatalf("expected non-zero size, got %d", size)
	}

	restoreInto := filepath.Join(root, "restored")
	if err := RestoreZip(dest, restoreInto); err != nil {
		t.Fatalf("RestoreZip: %v", err)
	}

	for _, p := range []string{
		filepath.Join("SavedArks", "Island.ark"),
		filepath.Join("SavedArks", "Sub", "Island.profile"),
		"Game.ini",
		"GameUserSettings.ini",
	} {
		if _, err := os.Stat(filepath.Join(restoreInto, p)); err != nil {
			t.Errorf("expected %s to be restored: %v", p, err)
		}
	}
}

func TestWriteServerArchive_MissingPiecesAreOK(t *testing.T) {
	root := t.TempDir()
	install := filepath.Join(root, "Phantom")
	// Only create Game.ini; SavedArks and GameUserSettings missing.
	writeFile(t, filepath.Join(install, configRel, "Game.ini"), "x")

	srv := server.Server{ID: 1, Name: "Phantom", InstallDir: install}
	dest := filepath.Join(root, "out.zip")
	if _, err := WriteServerArchive(srv, dest); err != nil {
		t.Fatalf("expected silent skip of missing pieces, got %v", err)
	}
}

func TestRestoreZip_RejectsZipSlip(t *testing.T) {
	root := t.TempDir()
	zipPath := filepath.Join(root, "evil.zip")
	if err := writeMaliciousZip(zipPath, "../escape.txt", "evil"); err != nil {
		t.Fatal(err)
	}
	if err := RestoreZip(zipPath, t.TempDir()); err == nil || !errStr(err, "escapes dest") {
		t.Errorf("expected zip-slip rejection, got %v", err)
	}
}

func errStr(err error, sub string) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, nil) {
		return false
	}
	return contains(err.Error(), sub)
}

func contains(haystack, needle string) bool {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}
