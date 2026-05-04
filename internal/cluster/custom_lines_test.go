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

func TestRepo_CustomLinesRoundTrip(t *testing.T) {
	d := newTestDB(t)
	r := NewRepo(d)
	ctx := context.Background()

	in := Cluster{
		Name: "c", ClusterID: "c1", ClusterDir: "x",
		CustomLines: []CustomINIEntry{
			{File: "Game.ini", Section: "/script/shootergame.shootergamemode", Key: "ModSettingA", Value: "42"},
			{File: "GameUserSettings.ini", Section: "ServerSettings", Key: "ModSettingB", Value: "true"},
		},
	}
	created, err := r.Create(ctx, in)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	got, err := r.Get(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.CustomLines) != 2 {
		t.Fatalf("got %d custom lines, want 2", len(got.CustomLines))
	}
	for i, want := range in.CustomLines {
		if got.CustomLines[i] != want {
			t.Errorf("[%d] got %+v, want %+v", i, got.CustomLines[i], want)
		}
	}
}

func TestWriteServerINIs_AppendsCustomLinesToCorrectFile(t *testing.T) {
	root := t.TempDir()
	srv := server.Server{ID: 1, Name: "Island", InstallDir: filepath.Join(root, "Island")}

	// One registered key plus two custom lines split across files.
	values := map[string]settings.Value{
		"DifficultyOffset": settings.FloatVal(1.0),
	}
	custom := []CustomINIEntry{
		{File: "Game.ini", Section: "/script/shootergame.shootergamemode", Key: "ModFoo", Value: "bar"},
		{File: "GameUserSettings.ini", Section: "ServerSettings", Key: "ModBaz", Value: "qux"},
		{File: "", Section: "ignored", Key: "", Value: "empty key skipped"},
	}
	if err := WriteServerINIs(srv, values, custom); err != nil {
		t.Fatalf("WriteServerINIs: %v", err)
	}
	dir := ConfigDir(srv)

	gameBytes, _ := os.ReadFile(filepath.Join(dir, "Game.ini"))
	gameF, err := inifile.Parse(bytes.NewReader(gameBytes))
	if err != nil {
		t.Fatal(err)
	}
	if v, _ := gameF.Get("/script/shootergame.shootergamemode", "ModFoo"); v != "bar" {
		t.Errorf("Game.ini ModFoo = %q, want bar", v)
	}
	if _, ok := gameF.Get("ServerSettings", "ModBaz"); ok {
		t.Errorf("GameUserSettings line leaked into Game.ini")
	}

	gusBytes, _ := os.ReadFile(filepath.Join(dir, "GameUserSettings.ini"))
	gusF, err := inifile.Parse(bytes.NewReader(gusBytes))
	if err != nil {
		t.Fatal(err)
	}
	if v, _ := gusF.Get("ServerSettings", "ModBaz"); v != "qux" {
		t.Errorf("GUS ModBaz = %q, want qux", v)
	}
}
