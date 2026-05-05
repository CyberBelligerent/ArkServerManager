package server

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildLaunchCommand_BasicChain(t *testing.T) {
	s := Server{
		Name:        "Island",
		Map:         "TheIsland_WP",
		InstallDir:  `C:\servers\Island`,
		Ports:       DefaultBase,
		RCONEnabled: true,
	}
	cmd := BuildLaunchCommand(s, LaunchOptions{
		ClusterID:  "solo-001",
		ClusterDir: `C:\ASAManager\cluster\solo-001`,
	})

	wantExeSuffix := filepath.Join("ShooterGame", "Binaries", "Win64", "ArkAscendedServer.exe")
	if !strings.HasSuffix(cmd.Exe, wantExeSuffix) {
		t.Errorf("Exe=%q, want suffix %q", cmd.Exe, wantExeSuffix)
	}
	chain := cmd.Args[0]
	for _, want := range []string{
		"TheIsland_WP",
		"?listen",
		"?SessionName=Island",
		"?Port=7777",
		"?QueryPort=27015",
		"?RCONPort=27020",
		"?RCONEnabled=True",
	} {
		if !strings.Contains(chain, want) {
			t.Errorf("chain missing %q: %q", want, chain)
		}
	}
	joined := strings.Join(cmd.Args, " ")
	if !strings.Contains(joined, "-server") {
		t.Errorf("flags missing -server: %q", joined)
	}
	if strings.Contains(joined, "-log") {
		t.Errorf("-log should be omitted (causes a visible console on Windows): %q", joined)
	}
	if !strings.Contains(joined, "-clusterid=solo-001") {
		t.Errorf("missing -clusterid: %q", joined)
	}
}

func TestBuildLaunchCommand_QuotesClusterDirWithSpaces(t *testing.T) {
	s := Server{Name: "X", Map: "M", InstallDir: "D", Ports: DefaultBase}
	cmd := BuildLaunchCommand(s, LaunchOptions{
		ClusterDir: `C:\Path With Space\cluster`,
	})
	joined := strings.Join(cmd.Args, " ")
	if !strings.Contains(joined, `-ClusterDirOverride="C:\Path With Space\cluster"`) {
		t.Errorf("ClusterDir not quoted: %q", joined)
	}
}

func TestBuildLaunchCommand_ModsAndPasswords(t *testing.T) {
	s := Server{
		Name:         "X",
		Map:          "M",
		InstallDir:   "D",
		Ports:        DefaultBase,
		RCONEnabled:  true,
		RCONPassword: "rcon-fallback",
	}
	cmd := BuildLaunchCommand(s, LaunchOptions{
		ServerPassword: "playerpw",
		Mods:           []int{100, 200, 300},
	})
	joined := strings.Join(cmd.Args, " ")
	if !strings.Contains(joined, "-mods=100,200,300") {
		t.Errorf("mods missing or malformed: %q", joined)
	}
	if !strings.Contains(cmd.Args[0], "?ServerPassword=playerpw") {
		t.Errorf("ServerPassword missing from chain: %q", cmd.Args[0])
	}
	// ServerAdminPassword should fall back to Server.RCONPassword.
	if !strings.Contains(cmd.Args[0], "?ServerAdminPassword=rcon-fallback") {
		t.Errorf("ServerAdminPassword should fall back to RCONPassword: %q", cmd.Args[0])
	}
}

// TestMergeMods covers the cluster→server mod cascade
func TestMergeMods(t *testing.T) {
	cl := []ModRef{
		{CurseForgeID: 100, Enabled: true},
		{CurseForgeID: 200, Enabled: true},
		{CurseForgeID: 999, Enabled: false}, // disabled, should be gone
	}
	sv := []ModRef{
		{CurseForgeID: 200, Enabled: true},  // dup with cluster, should be gone
		{CurseForgeID: 300, Enabled: true},
		{CurseForgeID: 0, Enabled: true},    // zero ID, should be gone
	}
	got := MergeMods(cl, sv)
	want := []int{100, 200, 300}
	if len(got) != len(want) {
		t.Fatalf("got %d mods, want %d: %+v", len(got), len(want), got)
	}
	for i, w := range want {
		if got[i].CurseForgeID != w {
			t.Errorf("[%d] got %d, want %d", i, got[i].CurseForgeID, w)
		}
	}

	// Standalone path: nil cluster, server's mods stand alone.
	if got := MergeMods(nil, sv); len(got) != 2 || got[0].CurseForgeID != 200 || got[1].CurseForgeID != 300 {
		t.Errorf("standalone merge wrong: %+v", got)
	}
	// Both nil → nil.
	if got := MergeMods(nil, nil); len(got) != 0 {
		t.Errorf("nil/nil merge should be empty, got %+v", got)
	}
}

func TestBuildLaunchCommand_AnticheatToggle(t *testing.T) {
	base := Server{Name: "X", Map: "M", InstallDir: "D", Ports: DefaultBase}

	t.Run("enabled (default) omits -NoBattlEye", func(t *testing.T) {
		s := base
		s.AnticheatEnabled = true
		cmd := BuildLaunchCommand(s, LaunchOptions{})
		joined := strings.Join(cmd.Args, " ")
		if strings.Contains(joined, "-NoBattlEye") {
			t.Errorf("anticheat enabled must NOT emit -NoBattlEye: %q", joined)
		}
	})

	t.Run("disabled emits -NoBattlEye", func(t *testing.T) {
		s := base
		s.AnticheatEnabled = false
		cmd := BuildLaunchCommand(s, LaunchOptions{})
		joined := strings.Join(cmd.Args, " ")
		if !strings.Contains(joined, "-NoBattlEye") {
			t.Errorf("anticheat disabled must emit -NoBattlEye: %q", joined)
		}
	})
}

func TestBuildLaunchCommand_ActiveEvent(t *testing.T) {
	s := Server{Name: "X", Map: "M", InstallDir: "D", Ports: DefaultBase}

	cases := []struct {
		name      string
		event     string
		wantFlag  string // "" if -ActiveEvent should NOT appear
		mustOmit  bool
	}{
		{"named event emits flag", "Summer", "-ActiveEvent=Summer", false},
		{"empty event omits flag", "", "", true},
		{"None sentinel omits flag", "None", "", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cmd := BuildLaunchCommand(s, LaunchOptions{ActiveEvent: tc.event})
			joined := strings.Join(cmd.Args, " ")
			if tc.mustOmit {
				if strings.Contains(joined, "-ActiveEvent") {
					t.Errorf("expected no -ActiveEvent, got: %q", joined)
				}
			} else if !strings.Contains(joined, tc.wantFlag) {
				t.Errorf("expected %q in: %q", tc.wantFlag, joined)
			}
		})
	}
}

func TestResolveActiveEvent(t *testing.T) {
	cases := []struct {
		srv, cls, want string
	}{
		{"", "", ""},                        // both empty -> nothing
		{"", "Summer", "Summer"},            // server inherits cluster
		{"Winter", "Summer", "Winter"},      // server overrides cluster
		{"None", "Summer", ""},              // explicit clear beats inheritance
		{"Summer", "", "Summer"},            // server-only
		{"None", "", ""},                    // explicit clear with no cluster default
	}
	for _, tc := range cases {
		got := ResolveActiveEvent(tc.srv, tc.cls)
		if got != tc.want {
			t.Errorf("Resolve(srv=%q, cls=%q) = %q, want %q", tc.srv, tc.cls, got, tc.want)
		}
	}
}

func TestBuildLaunchCommand_AdminPasswordExplicitWins(t *testing.T) {
	s := Server{Name: "X", Map: "M", InstallDir: "D", Ports: DefaultBase, RCONEnabled: true, RCONPassword: "fallback"}
	cmd := BuildLaunchCommand(s, LaunchOptions{ServerAdminPassword: "explicit"})
	if !strings.Contains(cmd.Args[0], "?ServerAdminPassword=explicit") {
		t.Errorf("explicit admin password should win: %q", cmd.Args[0])
	}
	if strings.Contains(cmd.Args[0], "fallback") {
		t.Errorf("fallback leaked into chain: %q", cmd.Args[0])
	}
}

func TestBuildLaunchCommand_RCONDisabled(t *testing.T) {
	s := Server{
		Name:         "X",
		Map:          "M",
		InstallDir:   "D",
		Ports:        DefaultBase,
		RCONEnabled:  false,
		RCONPassword: "should-not-leak",
	}
	cmd := BuildLaunchCommand(s, LaunchOptions{ServerAdminPassword: "also-not"})
	chain := cmd.Args[0]
	for _, banned := range []string{
		"RCONEnabled=True",
		"RCONPort=",
		"ServerAdminPassword=",
		"should-not-leak",
		"also-not",
	} {
		if strings.Contains(chain, banned) {
			t.Errorf("disabled RCON should omit %q from chain: %q", banned, chain)
		}
	}
}

func TestSanitizeChainValue_StripsBreakingChars(t *testing.T) {
	if got := sanitizeChainValue("Hello World"); got != "Hello_World" {
		t.Errorf("space → underscore: got %q", got)
	}
	if got := sanitizeChainValue(`a?b=c"d`); got != "abcd" {
		t.Errorf("breaking chars stripped: got %q", got)
	}
}

func TestWriteBatchFile_WritesExpectedContent(t *testing.T) {
	dir := t.TempDir()
	s := Server{
		Name:       "My Server",
		Map:        "TheIsland_WP",
		InstallDir: dir,
		Ports:      DefaultBase,
	}
	cmd := BuildLaunchCommand(s, LaunchOptions{ClusterID: "c1"})
	path, err := WriteBatchFile(s, cmd)
	if err != nil {
		t.Fatalf("WriteBatchFile: %v", err)
	}
	wantName := "start-My_Server.bat"
	if filepath.Base(path) != wantName {
		t.Errorf("filename = %q, want %q", filepath.Base(path), wantName)
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(body), "@echo off\r\n") {
		t.Errorf("missing @echo off prefix")
	}
	if !strings.Contains(string(body), cmd.Line()) {
		t.Errorf("body missing launch line:\n%s", string(body))
	}
}
