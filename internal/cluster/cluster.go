// Package cluster owns Cluster CRUD, settings inheritance, and
// propagation of cluster templates to member servers.
package cluster

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"asamanager/internal/server"
	"asamanager/internal/settings"
)

// Cluster groups one or more servers under a shared ARK cluster ID and a
// shared cluster directory.
type Cluster struct {
	ID          int64
	Name        string                    	// human-friendly label
	ClusterID   string                    	// value passed to -clusterid=
	ClusterDir  string                    	// path passed to -ClusterDirOverride=
	Settings    map[string]settings.Value 	// cluster-level setting values
	CustomLines []CustomINIEntry          	// free-form lines appended to each server's .ini files
	ActiveEvent string						// ActiveEvent is the cluster-wide default for -ActiveEvent=
	ActiveMods  []server.ModRef
	CreatedAt   time.Time
}

// Mainly for mods or things I missed in the registery
type CustomINIEntry struct {
	File    string 	// "Game.ini" or "GameUserSettings.ini"
	Section string 	// section name without brackets, e.g. "ServerSettings"
	Key     string 	// INI key
	Value   string 	// INI value (everything after =)
}

// Template is a saved cluster preset
type Template struct {
	ID         int64
	Name       string
	Settings   map[string]settings.Value
	ActiveMods []int
	CreatedAt  time.Time
}

// enforces a filesystem-safe cluster ID
var clusterIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{2,63}$`)

// Validation errors returned by ValidateClusterID.
var (
	ErrClusterIDEmpty     = errors.New("cluster id is required")
	ErrClusterIDFormat    = errors.New("cluster id must be 3-64 chars, alphanumeric/_/- and start with a letter or digit")
	ErrClusterIDDuplicate = errors.New("cluster id is already in use")
)

func ValidateClusterID(id string, existing []string) error {
	if strings.TrimSpace(id) == "" {
		return ErrClusterIDEmpty
	}
	if !clusterIDPattern.MatchString(id) {
		return ErrClusterIDFormat
	}
	for _, e := range existing {
		if strings.EqualFold(e, id) {
			return fmt.Errorf("%w: %q", ErrClusterIDDuplicate, id)
		}
	}
	return nil
}

func SuggestClusterID(base string) string {
	sanitized := sanitizeBase(base)
	if sanitized == "" {
		sanitized = "cluster"
	}
	return sanitized + "-" + randomSuffix(4)
}

func sanitizeBase(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '_' || r == '-':
			b.WriteRune(r)
		}
	}
	out := strings.TrimLeft(b.String(), "-_")
	if len(out) > 32 {
		out = out[:32]
	}
	return out
}

func randomSuffix(nBytes int) string {
	b := make([]byte, nBytes)
	if _, err := rand.Read(b); err != nil {
		return "0000"
	}
	return hex.EncodeToString(b)
}
