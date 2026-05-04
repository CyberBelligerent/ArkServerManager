package gui

import (
	"strconv"
	"strings"

	"asamanager/internal/server"
)

// parseModIDs turns a user-typed comma-separated list like
//   "123, 456,789"
// into a []server.ModRef with Enabled=true and empty Name/Version.
// Tokens that don't parse as positive integers are silently skipped —
// the field is a stop-gap for mod IDs until the CurseForge browser
// lands, and noisy validation errors on every keystroke aren't worth
// the friction. Order is preserved (matters for ARK's mod load order).
// Duplicates are dropped (first occurrence wins).
func parseModIDs(s string) []server.ModRef {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	seen := map[int]bool{}
	var out []server.ModRef
	for _, tok := range strings.Split(s, ",") {
		tok = strings.TrimSpace(tok)
		if tok == "" {
			continue
		}
		id, err := strconv.Atoi(tok)
		if err != nil || id <= 0 {
			continue
		}
		if seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, server.ModRef{CurseForgeID: id, Enabled: true})
	}
	return out
}

// formatModIDs renders a []ModRef as a comma-separated string of the
// IDs that are currently Enabled. Used to seed the text field with
// what's already saved on the server. Disabled mods are omitted so
// what the user sees and what gets passed via -mods= match.
func formatModIDs(refs []server.ModRef) string {
	if len(refs) == 0 {
		return ""
	}
	parts := make([]string, 0, len(refs))
	for _, r := range refs {
		if !r.Enabled {
			continue
		}
		parts = append(parts, strconv.Itoa(r.CurseForgeID))
	}
	return strings.Join(parts, ",")
}
