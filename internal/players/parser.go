// Package players tracks online players via RCON polling, edits the
// whitelist and ban-list files, and persists per-player session history
// and notes.
package players

import (
	"regexp"
	"strings"
)

// Snapshot is the parsed result of `listplayers`.
type Snapshot struct {
	Players []OnlinePlayer
}

// OnlinePlayer is one entry in a listplayers response.
type OnlinePlayer struct {
	Name    string
	SteamID string
}

// listLine matches the conventional ARK format: "<index>. <name>, <steamid>"
// where the steamid is a 17-digit Steam64 number. The leading index is
// allowed to be missing because some ARK builds emit just "<name>, <id>".
var listLine = regexp.MustCompile(`^\s*(?:\d+\.\s+)?(.+?)\s*,\s*(\d{15,20})\s*$`)

// ParseListPlayers parses the body of a `listplayers` RCON response.
// "No Players Connected" and empty bodies return an empty slice.
func ParseListPlayers(raw string) []OnlinePlayer {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	if strings.Contains(strings.ToLower(raw), "no players connected") {
		return nil
	}
	var out []OnlinePlayer
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimRight(line, "\r")
		m := listLine.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		name := strings.TrimSpace(m[1])
		id := m[2]
		if name == "" || id == "" {
			continue
		}
		out = append(out, OnlinePlayer{Name: name, SteamID: id})
	}
	return out
}
