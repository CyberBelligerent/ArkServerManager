package server

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	"asamanager/internal/settings"
)

func GenerateRCONPassword() string {
	var buf [12]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return fmt.Sprintf("rcon%016x", time.Now().UnixNano())
	}
	return hex.EncodeToString(buf[:])
}

type ModRef struct {
	CurseForgeID int
	Name         string
	Version      string
	Enabled      bool
}

// MergeMods returns the effective mod list for a server in a cluster
// Cluster mods then Server mods duplicates removed
func MergeMods(cluster, server []ModRef) []ModRef {
	seen := map[int]bool{}
	out := make([]ModRef, 0, len(cluster)+len(server))
	for _, src := range [][]ModRef{cluster, server} {
		for _, m := range src {
			if !m.Enabled || m.CurseForgeID == 0 {
				continue
			}
			if seen[m.CurseForgeID] {
				continue
			}
			seen[m.CurseForgeID] = true
			out = append(out, m)
		}
	}
	return out
}

// Server is one ARK dedicated server inside a cluster
type Server struct {
	ID               int64
	ClusterID        int64
	Name             string
	Map              string // ARK level name, e.g. TheIsland_WP
	InstallDir       string
	Ports            PortTriple
	RCONEnabled      bool   // when false, the manager loses player tracking, graceful stop, and the RCON tab
	RCONPassword     string
	ServerPassword   string // optional player join password; empty = open
	MaxPlayers       int    // 0 = leave at ASA default (currently 70)
	SettingOverrides map[string]settings.Value
	ActiveMods       []ModRef
	ActiveEvent      string
	AnticheatEnabled bool
	Status           Status
	CreatedAt        time.Time
}

type PortTriple struct {
	Game  int // GamePort (UDP)
	Query int // QueryPort (UDP)
	RCON  int // RCON (TCP)
}

func (p PortTriple) All() []int {
	return []int{p.Game, p.Game + 1, p.Query, p.RCON}
}

var DefaultBase = PortTriple{Game: 7777, Query: 27015, RCON: 27020}

const PortStep = 10

func (p PortTriple) Validate() error {
	for _, v := range p.All() {
		if v < 1 || v > 65535 {
			return fmt.Errorf("port %d outside 1-65535", v)
		}
	}
	seen := map[int]bool{}
	for _, v := range p.All() {
		if seen[v] {
			return fmt.Errorf("triple reuses port %d internally", v)
		}
		seen[v] = true
	}
	return nil
}

func SuggestPorts(existing []PortTriple) PortTriple {
	next := DefaultBase
	for _, e := range existing {
		if e.Game >= next.Game {
			next = PortTriple{
				Game:  e.Game + PortStep,
				Query: e.Query + PortStep,
				RCON:  e.RCON + PortStep,
			}
		}
	}
	for CheckPortConflict(next, existing) != nil {
		next = PortTriple{
			Game:  next.Game + PortStep,
			Query: next.Query + PortStep,
			RCON:  next.RCON + PortStep,
		}
		if next.Game > 65000 {
			return next
		}
	}
	return next
}

func CheckPortConflict(candidate PortTriple, existing []PortTriple) error {
	if err := candidate.Validate(); err != nil {
		return err
	}
	used := map[int]int{} // port -> 1-based index of the triple that owns it
	for i, e := range existing {
		for _, p := range e.All() {
			used[p] = i + 1
		}
	}
	for _, p := range candidate.All() {
		if owner, taken := used[p]; taken {
			return fmt.Errorf("port %d already used by server #%d", p, owner)
		}
	}
	return nil
}
