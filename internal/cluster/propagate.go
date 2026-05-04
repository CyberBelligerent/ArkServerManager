package cluster

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"asamanager/internal/inifile"
	"asamanager/internal/server"
	"asamanager/internal/settings"
)

func ConfigDir(s server.Server) string {
	return filepath.Join(s.InstallDir, "ShooterGame", "Saved", "Config", "WindowsServer")
}

// WriteServerINIs writes Game.ini and GameUserSettings.ini for one
// server using values. Existing files are parsed first so any
// non-managed keys (custom mod settings, manual edits) survive — only
// keys we own in the registry are overwritten. customLines (typically
// from cluster.CustomLines) are appended afterwards via Set so each
// (file, section, key) is single-valued and re-runs idempotent.
func WriteServerINIs(s server.Server, values map[string]settings.Value, customLines []CustomINIEntry) error {
	dir := ConfigDir(s)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}
	reg := settings.Default()
	if err := writeINI(
		filepath.Join(dir, "Game.ini"), reg, values, settings.FileGame,
		filterCustomLines(customLines, "Game.ini"),
	); err != nil {
		return err
	}
	if err := writeINI(
		filepath.Join(dir, "GameUserSettings.ini"), reg, values, settings.FileGameUserSettings,
		filterCustomLines(customLines, "GameUserSettings.ini"),
	); err != nil {
		return err
	}
	return nil
}

func (r *Repo) PropagateToServers(ctx context.Context, sr *server.Repo, clusterID int64) error {
	c, err := r.Get(ctx, clusterID)
	if err != nil {
		return err
	}
	servers, err := sr.ListByCluster(ctx, clusterID)
	if err != nil {
		return err
	}
	for _, s := range servers {
		effective := MergeSettings(c.Settings, s.SettingOverrides)
		if err := WriteServerINIs(s, effective, c.CustomLines); err != nil {
			return fmt.Errorf("server %d (%s): %w", s.ID, s.Name, err)
		}
	}
	return nil
}

func writeINI(path string, reg *settings.Registry, all map[string]settings.Value, file settings.File, customLines []CustomINIEntry) error {
	f := inifile.New()
	existing, err := os.ReadFile(path)
	switch {
	case err == nil:
		parsed, perr := inifile.Parse(bytes.NewReader(existing))
		if perr != nil {
			return fmt.Errorf("parse %s: %w", path, perr)
		}
		f = parsed
	case errors.Is(err, os.ErrNotExist):
		// fall through with empty file
	default:
		return fmt.Errorf("read %s: %w", path, err)
	}
	settings.ApplyTo(reg, f, settings.FilterByFile(reg, all, file))
	for _, line := range customLines {
		if line.Section == "" || line.Key == "" {
			continue
		}
		f.Set(line.Section, line.Key, line.Value)
	}
	body, err := f.Marshal()
	if err != nil {
		return err
	}
	return os.WriteFile(path, body, 0o644)
}

func filterCustomLines(lines []CustomINIEntry, name string) []CustomINIEntry {
	var out []CustomINIEntry
	for _, l := range lines {
		f := l.File
		if f == "" {
			f = "Game.ini"
		}
		if f == name {
			out = append(out, l)
		}
	}
	return out
}
