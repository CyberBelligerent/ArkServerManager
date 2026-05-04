package players

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadSteamIDList_MissingFileIsEmpty(t *testing.T) {
	got, err := ReadSteamIDList(filepath.Join(t.TempDir(), "absent.txt"))
	if err != nil {
		t.Fatalf("ReadSteamIDList: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected empty, got %v", got)
	}
}

func TestReadSteamIDList_SkipsBlanksAndComments(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ids.txt")
	body := "# header\n" +
		"  \n" +
		"76561198000000001\n" +
		"; comment line\n" +
		"76561198000000002 # inline comment\n" +
		"76561198000000001\n" + // duplicate, should be deduped
		"\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := ReadSteamIDList(path)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"76561198000000001", "76561198000000002"}
	if len(got) != len(want) {
		t.Fatalf("got %d, want %d (%v)", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("[%d] %q, want %q", i, got[i], want[i])
		}
	}
}

func TestWriteSteamIDList_RoundTripWithBackup(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ids.txt")

	if err := WriteSteamIDList(path, []string{"76561198000000003", "76561198000000001", "76561198000000002"}); err != nil {
		t.Fatal(err)
	}
	got, _ := ReadSteamIDList(path)
	want := []string{"76561198000000001", "76561198000000002", "76561198000000003"} // sorted
	if len(got) != 3 {
		t.Fatalf("got %d, want 3 (%v)", len(got), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("[%d] %q, want %q", i, got[i], want[i])
		}
	}

	// Second write should produce a .bak of the first.
	if err := WriteSteamIDList(path, []string{"76561198000000099"}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path + ".bak"); err != nil {
		t.Errorf(".bak file not created: %v", err)
	}
	now, _ := ReadSteamIDList(path)
	if len(now) != 1 || now[0] != "76561198000000099" {
		t.Errorf("after rewrite: %v", now)
	}
}

func TestWriteSteamIDList_DedupesAndCreatesParentDir(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nested", "ids.txt")
	if err := WriteSteamIDList(path, []string{"S1", "S2", "S1", "  ", ""}); err != nil {
		t.Fatal(err)
	}
	got, _ := ReadSteamIDList(path)
	if len(got) != 2 {
		t.Errorf("dedupe failed: %v", got)
	}
}
