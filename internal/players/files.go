package players

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
)

// ReadSteamIDList reads a plaintext file of Steam IDs, one per line
func ReadSteamIDList(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return []string{}, nil
		}
		return nil, err
	}
	defer f.Close()

	var out []string
	seen := map[string]bool{}
	s := bufio.NewScanner(f)
	for s.Scan() {
		line := strings.TrimSpace(s.Text())
		if line == "" || line[0] == '#' || line[0] == ';' {
			continue
		}
		// Tolerate inline comments after whitespace.
		if i := strings.IndexAny(line, " \t#;"); i >= 0 {
			line = strings.TrimSpace(line[:i])
		}
		if line == "" || seen[line] {
			continue
		}
		seen[line] = true
		out = append(out, line)
	}
	if err := s.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// WriteSteamIDList writes ids to path, one per line, sorted for stable diffs
func WriteSteamIDList(path string, ids []string) error {
	dir := dirOf(path)
	if dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("create parent dir: %w", err)
		}
	}
	if _, err := os.Stat(path); err == nil {
		bak := path + ".bak"
		if err := os.Rename(path, bak); err != nil {
			return fmt.Errorf("backup existing: %w", err)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}

	deduped := dedupeSorted(ids)
	var b strings.Builder
	b.Grow(len(deduped) * 18)
	for _, id := range deduped {
		b.WriteString(id)
		b.WriteString("\r\n")
	}
	return os.WriteFile(path, []byte(b.String()), 0o644)
}

// SyncFile rewrites path so its contents match ids exactly
func SyncFile(path string, ids []string) error {
	return WriteSteamIDList(path, ids)
}

func dedupeSorted(ids []string) []string {
	seen := make(map[string]bool, len(ids))
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}

func dirOf(path string) string {
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '/' || path[i] == '\\' {
			return path[:i]
		}
	}
	return ""
}
