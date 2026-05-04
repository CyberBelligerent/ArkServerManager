package backup

import (
	"archive/zip"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"asamanager/internal/cluster"
	"asamanager/internal/server"
)

const (
	savedArksRel = "ShooterGame/Saved/SavedArks"
	configRel    = "ShooterGame/Saved/Config/WindowsServer"
	gameINI      = "Game.ini"
	gusINI       = "GameUserSettings.ini"
)

// Creates a zip containing the server's SavedArks directory and .ini files.
func WriteServerArchive(s server.Server, destZip string) (int64, error) {
	if err := os.MkdirAll(filepath.Dir(destZip), 0o755); err != nil {
		return 0, fmt.Errorf("create dest dir: %w", err)
	}
	f, err := os.Create(destZip)
	if err != nil {
		return 0, err
	}
	zw := zip.NewWriter(f)
	if err := addServerToZip(zw, s, ""); err != nil {
		_ = zw.Close()
		_ = f.Close()
		return 0, err
	}
	if err := zw.Close(); err != nil {
		_ = f.Close()
		return 0, err
	}
	
	// Close the OS file BEFORE Stat
	if err := f.Close(); err != nil {
		return 0, err
	}
	
	info, err := os.Stat(destZip)
	if err != nil {
		return 0, err
	}
	return info.Size(), nil
}

// WriteClusterArchive writes a zip containing the cluster's shared
// directory and all child servers ini's
func WriteClusterArchive(c cluster.Cluster, members []server.Server, destZip string) (int64, error) {
	if err := os.MkdirAll(filepath.Dir(destZip), 0o755); err != nil {
		return 0, fmt.Errorf("create dest dir: %w", err)
	}
	f, err := os.Create(destZip)
	if err != nil {
		return 0, err
	}
	zw := zip.NewWriter(f)
	if c.ClusterDir != "" {
		if err := addDirToZip(zw, c.ClusterDir, "cluster"); err != nil {
			_ = zw.Close()
			_ = f.Close()
			return 0, fmt.Errorf("add cluster dir: %w", err)
		}
	}
	for _, m := range members {
		prefix := "servers/" + sanitizeArchiveName(m.Name)
		if err := addServerToZip(zw, m, prefix); err != nil {
			_ = zw.Close()
			_ = f.Close()
			return 0, fmt.Errorf("add server %q: %w", m.Name, err)
		}
	}
	if err := zw.Close(); err != nil {
		_ = f.Close()
		return 0, err
	}

	if err := f.Close(); err != nil {
		return 0, err
	}
	
	info, err := os.Stat(destZip)
	if err != nil {
		return 0, err
	}
	return info.Size(), nil
}

// addServerToZip places SavedArks + inis under prefix in the zip
func addServerToZip(zw *zip.Writer, s server.Server, prefix string) error {
	saved := filepath.Join(s.InstallDir, savedArksRel)
	if err := addDirToZip(zw, saved, joinZip(prefix, "SavedArks")); err != nil {
		return fmt.Errorf("SavedArks: %w", err)
	}
	cfgDir := filepath.Join(s.InstallDir, configRel)
	for _, name := range []string{gameINI, gusINI} {
		src := filepath.Join(cfgDir, name)
		dst := joinZip(prefix, name)
		if err := addFileToZip(zw, src, dst); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("%s: %w", name, err)
		}
	}
	return nil
}

// addDirToZip walks srcDir and writes every file under prefix in the zip
func addDirToZip(zw *zip.Writer, srcDir, prefix string) error {
	if _, err := os.Stat(srcDir); errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	return filepath.WalkDir(srcDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(srcDir, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		zipPath := joinZip(prefix, filepath.ToSlash(rel))
		if d.IsDir() {
			_, err := zw.Create(zipPath + "/")
			return err
		}
		return copyFileIntoZip(zw, path, zipPath)
	})
}

// addFileToZip writes srcFile into the zip at zipPath. Returns fs.ErrNotExist
func addFileToZip(zw *zip.Writer, srcFile, zipPath string) error {
	if _, err := os.Stat(srcFile); err != nil {
		return err
	}
	return copyFileIntoZip(zw, srcFile, zipPath)
}

func copyFileIntoZip(zw *zip.Writer, srcFile, zipPath string) error {
	w, err := zw.Create(zipPath)
	if err != nil {
		return err
	}
	src, err := os.Open(srcFile)
	if err != nil {
		return err
	}
	defer src.Close()
	_, err = io.Copy(w, src)
	return err
}

// RestoreZip extracts srcZip into destDir. Defends against zip-slip.
// Existing files at the same paths are overwritten.
func RestoreZip(srcZip, destDir string) error {
	r, err := zip.OpenReader(srcZip)
	if err != nil {
		return err
	}
	defer r.Close()

	cleanDest := filepath.Clean(destDir)
	if err := os.MkdirAll(cleanDest, 0o755); err != nil {
		return err
	}
	for _, zf := range r.File {
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
	out, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, rc)
	return err
}

func joinZip(prefix, rest string) string {
	if prefix == "" {
		return rest
	}
	return prefix + "/" + rest
}

func sanitizeArchiveName(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return "unnamed"
	}
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z',
			r >= 'A' && r <= 'Z',
			r >= '0' && r <= '9',
			r == '-', r == '_':
			b.WriteRune(r)
		default:
			b.WriteRune('_')
		}
	}
	return b.String()
}
