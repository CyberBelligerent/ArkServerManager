package cluster

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"

	"asamanager/internal/db"
	"asamanager/internal/settings"
)

func newTestDB(t *testing.T) *sql.DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.db")
	d, err := db.Open(path)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })
	return d
}

func TestClusterRepo_CRUDRoundTrip(t *testing.T) {
	d := newTestDB(t)
	r := NewRepo(d)
	ctx := context.Background()

	in := Cluster{
		Name:       "Solo Cluster",
		ClusterID:  "solo-001",
		ClusterDir: `C:\ASAManager\cluster\solo-001`,
		Settings: map[string]settings.Value{
			"DifficultyOffset":             settings.FloatVal(1.0),
			"OverrideOfficialDifficulty":   settings.FloatVal(5.0),
			"PreventDownloadSurvivors":     settings.BoolVal(false),
		},
	}
	created, err := r.Create(ctx, in)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if created.ID == 0 {
		t.Fatal("expected ID to be populated")
	}
	if created.CreatedAt.IsZero() {
		t.Error("expected CreatedAt to be populated")
	}

	got, err := r.Get(ctx, created.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.ClusterID != in.ClusterID || got.Name != in.Name {
		t.Errorf("Get returned %+v, want fields matching %+v", got, in)
	}
	for k, want := range in.Settings {
		if got.Settings[k] != want {
			t.Errorf("setting %s: got %+v, want %+v", k, got.Settings[k], want)
		}
	}

	got.Name = "Renamed"
	got.Settings["DifficultyOffset"] = settings.FloatVal(0.5)
	if err := r.Update(ctx, got); err != nil {
		t.Fatalf("update: %v", err)
	}

	reread, err := r.Get(ctx, got.ID)
	if err != nil {
		t.Fatalf("re-get: %v", err)
	}
	if reread.Name != "Renamed" {
		t.Errorf("Name not updated: %q", reread.Name)
	}
	if reread.Settings["DifficultyOffset"] != settings.FloatVal(0.5) {
		t.Errorf("Settings not updated: %+v", reread.Settings["DifficultyOffset"])
	}

	if err := r.Delete(ctx, got.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := r.Get(ctx, got.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound after delete, got %v", err)
	}
}

func TestClusterRepo_DuplicateID(t *testing.T) {
	d := newTestDB(t)
	r := NewRepo(d)
	ctx := context.Background()
	c := Cluster{Name: "A", ClusterID: "dup-1", ClusterDir: "x"}
	if _, err := r.Create(ctx, c); err != nil {
		t.Fatalf("first create: %v", err)
	}
	if _, err := r.Create(ctx, c); !errors.Is(err, ErrClusterIDDuplicate) {
		t.Errorf("expected ErrClusterIDDuplicate, got %v", err)
	}
}

func TestClusterRepo_ListAndListClusterIDs(t *testing.T) {
	d := newTestDB(t)
	r := NewRepo(d)
	ctx := context.Background()
	for _, id := range []string{"alpha", "bravo", "charlie"} {
		if _, err := r.Create(ctx, Cluster{Name: id, ClusterID: id, ClusterDir: id}); err != nil {
			t.Fatalf("create %s: %v", id, err)
		}
	}
	all, err := r.List(ctx)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(all) != 3 {
		t.Errorf("List len=%d, want 3", len(all))
	}
	ids, err := r.ListClusterIDs(ctx)
	if err != nil {
		t.Fatalf("ListClusterIDs: %v", err)
	}
	want := []string{"alpha", "bravo", "charlie"}
	for i := range want {
		if ids[i] != want[i] {
			t.Errorf("ids[%d]=%q, want %q", i, ids[i], want[i])
		}
	}
}

func TestClusterRepo_GetByClusterID(t *testing.T) {
	d := newTestDB(t)
	r := NewRepo(d)
	ctx := context.Background()
	if _, err := r.Create(ctx, Cluster{Name: "n", ClusterID: "find-me", ClusterDir: "x"}); err != nil {
		t.Fatalf("create: %v", err)
	}
	got, err := r.GetByClusterID(ctx, "find-me")
	if err != nil {
		t.Fatalf("GetByClusterID: %v", err)
	}
	if got.ClusterID != "find-me" {
		t.Errorf("got %q", got.ClusterID)
	}
	if _, err := r.GetByClusterID(ctx, "missing"); !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}
