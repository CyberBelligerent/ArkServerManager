package asb

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"asamanager/internal/cluster"
	"asamanager/internal/server"
	"asamanager/internal/settings"
)

func TestExportCluster_WritesFileWithExpectedKeys(t *testing.T) {
	dir := t.TempDir()
	c := cluster.Cluster{
		Name: "Solo Cluster",
		Settings: map[string]settings.Value{
			"OverrideOfficialDifficulty":             settings.FloatVal(5.0),
			"PerLevelStatsMultiplier_DinoTamed[0]":   settings.FloatVal(0.2),
			"PerLevelStatsMultiplier_DinoTamed[8]":   settings.FloatVal(0.17),
			"MatingIntervalMultiplier":               settings.FloatVal(0.4),
			"BabyImprintAmountMultiplier":            settings.FloatVal(2.0),
			"bUseSingleplayerSettings":               settings.BoolVal(false),
		},
	}
	path, err := ExportCluster(c, dir)
	if err != nil {
		t.Fatalf("ExportCluster: %v", err)
	}
	if filepath.Base(path) != "cluster-Solo_Cluster.ini" {
		t.Errorf("filename = %q, want cluster-Solo_Cluster.ini", filepath.Base(path))
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	s := string(body)

	checks := []string{
		"PerLevelStatsMultiplier_DinoTamed[0] = 0.2",
		"PerLevelStatsMultiplier_DinoTamed[8] = 0.17",
		"PerLevelStatsMultiplier_DinoWild[0] = 1",
		"MatingIntervalMultiplier = 0.4",
		"BabyImprintAmountMultiplier = 2",
		"bUseSingleplayerSettings = false",
		"ASBMaxWildLevels_Dinos = 150", // OverrideOfficialDifficulty(5) * 30
		"ASBMaxDomLevels_Dinos = 88",
		"ASBEvent_TamingSpeedMultiplier = 1",
	}
	for _, want := range checks {
		if !strings.Contains(s, want) {
			t.Errorf("output missing %q\n--- file ---\n%s", want, s)
		}
	}
}

func TestExportServer_OverridesWin(t *testing.T) {
	dir := t.TempDir()
	c := cluster.Cluster{
		Name: "C",
		Settings: map[string]settings.Value{
			"TamingSpeedMultiplier": settings.FloatVal(5.0),
		},
	}
	srv := server.Server{
		Name: "Island",
		SettingOverrides: map[string]settings.Value{
			"TamingSpeedMultiplier": settings.FloatVal(10.0), // server override wins
		},
	}
	path, err := ExportServer(c, srv, dir)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := os.ReadFile(path)
	if !strings.Contains(string(body), "TamingSpeedMultiplier = 10") {
		t.Errorf("server override should win in ASB output:\n%s", body)
	}
	if filepath.Base(path) != "server-Island.ini" {
		t.Errorf("filename = %q, want server-Island.ini", filepath.Base(path))
	}
}

func TestDeleteForCluster_RemovesFile(t *testing.T) {
	dir := t.TempDir()
	c := cluster.Cluster{Name: "X"}
	path, err := ExportCluster(c, dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal(err)
	}
	if err := DeleteForCluster(c, dir); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("file still exists after Delete: %v", err)
	}

	if err := DeleteForCluster(c, dir); err != nil {
		t.Errorf("second delete should be idempotent, got %v", err)
	}
}

func TestSanitizeFilename(t *testing.T) {
	cases := map[string]string{
		"Solo Cluster":     "Solo_Cluster",
		"My-Cluster_01":    "My-Cluster_01",
		"Bob's Server!?":   "Bob_s_Server__",
		"":                 "unnamed",
		"  trim me  ":      "trim_me",
	}
	for in, want := range cases {
		if got := sanitizeFilename(in); got != want {
			t.Errorf("sanitizeFilename(%q) = %q, want %q", in, got, want)
		}
	}
}
