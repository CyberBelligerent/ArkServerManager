package cluster

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"asamanager/internal/inifile"
	"asamanager/internal/server"
	"asamanager/internal/settings"
)

func TestWriteServerINIs_WritesParseableFiles(t *testing.T) {
	root := t.TempDir()
	s := server.Server{
		ID:         1,
		Name:       "Island",
		InstallDir: filepath.Join(root, "Island"),
	}
	values := map[string]settings.Value{
		"DifficultyOffset":           settings.FloatVal(1.0),  // GameUserSettings
		"OverrideOfficialDifficulty": settings.FloatVal(5.0),  // GameUserSettings
		"MatingIntervalMultiplier":   settings.FloatVal(0.4),  // Game.ini
		"PreventDownloadSurvivors":   settings.BoolVal(false), // Game.ini
	}
	if err := WriteServerINIs(s, values, nil); err != nil {
		t.Fatalf("WriteServerINIs: %v", err)
	}

	dir := ConfigDir(s)
	gameBytes, err := os.ReadFile(filepath.Join(dir, "Game.ini"))
	if err != nil {
		t.Fatalf("read Game.ini: %v", err)
	}
	gusBytes, err := os.ReadFile(filepath.Join(dir, "GameUserSettings.ini"))
	if err != nil {
		t.Fatalf("read GameUserSettings.ini: %v", err)
	}

	gameF, err := inifile.Parse(bytes.NewReader(gameBytes))
	if err != nil {
		t.Fatalf("parse Game.ini: %v", err)
	}
	if v, ok := gameF.Get(settings.SectionShooterGameMode, "MatingIntervalMultiplier"); !ok || v != "0.4" {
		t.Errorf("Game.ini MatingIntervalMultiplier = %q,%v; want 0.4,true", v, ok)
	}
	if v, ok := gameF.Get(settings.SectionShooterGameMode, "PreventDownloadSurvivors"); !ok || v != "False" {
		t.Errorf("Game.ini PreventDownloadSurvivors = %q,%v; want False,true", v, ok)
	}
	// GameUserSettings keys must NOT have leaked into Game.ini.
	if _, ok := gameF.Get(settings.SectionServerSettings, "DifficultyOffset"); ok {
		t.Error("DifficultyOffset leaked into Game.ini")
	}

	gusF, err := inifile.Parse(bytes.NewReader(gusBytes))
	if err != nil {
		t.Fatalf("parse GameUserSettings.ini: %v", err)
	}
	if v, ok := gusF.Get(settings.SectionServerSettings, "DifficultyOffset"); !ok || v != "1.0" {
		t.Errorf("GUS DifficultyOffset = %q,%v; want 1.0,true", v, ok)
	}
}

func TestWriteServerINIs_PreservesNonManagedKeys(t *testing.T) {
	root := t.TempDir()
	s := server.Server{InstallDir: filepath.Join(root, "Island"), Name: "Island"}
	dir := ConfigDir(s)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	preexisting := "[ServerSettings]\r\n" +
		"; user comment that must survive\r\n" +
		"CustomModSetting=42\r\n" +
		"DifficultyOffset=0.2\r\n"
	if err := os.WriteFile(filepath.Join(dir, "GameUserSettings.ini"), []byte(preexisting), 0o644); err != nil {
		t.Fatal(err)
	}

	values := map[string]settings.Value{
		"DifficultyOffset": settings.FloatVal(1.0),
	}
	if err := WriteServerINIs(s, values, nil); err != nil {
		t.Fatalf("WriteServerINIs: %v", err)
	}

	body, err := os.ReadFile(filepath.Join(dir, "GameUserSettings.ini"))
	if err != nil {
		t.Fatal(err)
	}
	got := string(body)
	if !bytes.Contains(body, []byte("CustomModSetting=42")) {
		t.Errorf("CustomModSetting was wiped:\n%s", got)
	}
	if !bytes.Contains(body, []byte("user comment that must survive")) {
		t.Errorf("user comment was wiped:\n%s", got)
	}
	if !bytes.Contains(body, []byte("DifficultyOffset=1.0")) {
		t.Errorf("managed key not updated:\n%s", got)
	}
}

func TestPropagateToServers_WritesEffectiveSettings(t *testing.T) {
	d := newTestDB(t)
	cr := NewRepo(d)
	sr := server.NewRepo(d)
	ctx := context.Background()

	c, err := cr.Create(ctx, Cluster{
		Name: "C", ClusterID: "prop-test", ClusterDir: "x",
		Settings: map[string]settings.Value{
			"DifficultyOffset": settings.FloatVal(1.0),
			"XPMultiplier":     settings.FloatVal(2.0),
		},
	})
	if err != nil {
		t.Fatalf("create cluster: %v", err)
	}

	root := t.TempDir()
	s, err := sr.Create(ctx, server.Server{
		ClusterID:  c.ID,
		Name:       "Island",
		Map:        "TheIsland_WP",
		InstallDir: filepath.Join(root, "Island"),
		Ports:      server.DefaultBase,
		SettingOverrides: map[string]settings.Value{
			"XPMultiplier": settings.FloatVal(5.0), // override beats cluster
		},
	})
	if err != nil {
		t.Fatalf("create server: %v", err)
	}

	if err := cr.PropagateToServers(ctx, sr, c.ID); err != nil {
		t.Fatalf("propagate: %v", err)
	}

	body, err := os.ReadFile(filepath.Join(ConfigDir(s), "GameUserSettings.ini"))
	if err != nil {
		t.Fatalf("read gus: %v", err)
	}
	f, err := inifile.Parse(bytes.NewReader(body))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if v, _ := f.Get(settings.SectionServerSettings, "XPMultiplier"); v != "5.0" {
		t.Errorf("override should win: XPMultiplier=%q, want 5.0", v)
	}
	if v, _ := f.Get(settings.SectionServerSettings, "DifficultyOffset"); v != "1.0" {
		t.Errorf("cluster value should land: DifficultyOffset=%q, want 1.0", v)
	}
}
