package server

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

// seedCluster inserts a minimal clusters row so server inserts don't
// fail the cluster_id foreign-key constraint.
func seedCluster(t *testing.T, d *sql.DB, clusterID string) int64 {
	t.Helper()
	res, err := d.Exec(
		`INSERT INTO clusters(name, cluster_id, cluster_dir) VALUES(?,?,?)`,
		clusterID, clusterID, "x")
	if err != nil {
		t.Fatalf("seed cluster: %v", err)
	}
	id, _ := res.LastInsertId()
	return id
}

func TestServerRepo_CRUDRoundTrip(t *testing.T) {
	d := newTestDB(t)
	r := NewRepo(d)
	ctx := context.Background()
	cid := seedCluster(t, d, "c1")

	in := Server{
		ClusterID:    cid,
		Name:         "Island",
		Map:          "TheIsland_WP",
		InstallDir:   `C:\servers\Island`,
		Ports:        DefaultBase,
		RCONPassword: "secret",
		SettingOverrides: map[string]settings.Value{
			"XPMultiplier": settings.FloatVal(2.0),
		},
		ActiveMods: []ModRef{
			{CurseForgeID: 100, Name: "Awesome", Version: "1.0", Enabled: true},
		},
	}
	created, err := r.Create(ctx, in)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if created.ID == 0 || created.Status != StatusStopped {
		t.Errorf("expected populated row, got %+v", created)
	}

	got, err := r.Get(ctx, created.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Name != in.Name || got.Map != in.Map || got.RCONPassword != "secret" {
		t.Errorf("Get returned %+v", got)
	}
	if got.Ports != DefaultBase {
		t.Errorf("Ports = %+v", got.Ports)
	}
	if v := got.SettingOverrides["XPMultiplier"]; v != settings.FloatVal(2.0) {
		t.Errorf("override XPMultiplier=%+v", v)
	}
	if len(got.ActiveMods) != 1 || got.ActiveMods[0].CurseForgeID != 100 {
		t.Errorf("ActiveMods=%+v", got.ActiveMods)
	}

	got.Map = "ScorchedEarth_WP"
	if err := r.Update(ctx, got); err != nil {
		t.Fatalf("update: %v", err)
	}
	if err := r.UpdateStatus(ctx, got.ID, StatusRunning); err != nil {
		t.Fatalf("update status: %v", err)
	}
	reread, _ := r.Get(ctx, got.ID)
	if reread.Map != "ScorchedEarth_WP" {
		t.Errorf("Map not updated: %q", reread.Map)
	}
	if reread.Status != StatusRunning {
		t.Errorf("Status not updated: %q", reread.Status)
	}

	if err := r.Delete(ctx, got.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := r.Get(ctx, got.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestServerRepo_NameUniqueWithinCluster(t *testing.T) {
	d := newTestDB(t)
	r := NewRepo(d)
	ctx := context.Background()
	cid := seedCluster(t, d, "c1")
	cid2 := seedCluster(t, d, "c2")

	if _, err := r.Create(ctx, Server{ClusterID: cid, Name: "Island", Map: "x", InstallDir: "x", Ports: DefaultBase}); err != nil {
		t.Fatalf("first: %v", err)
	}
	// Same name in same cluster should fail.
	if _, err := r.Create(ctx, Server{ClusterID: cid, Name: "Island", Map: "y", InstallDir: "y", Ports: PortTriple{Game: 7900, Query: 27200, RCON: 27500}}); !errors.Is(err, ErrNameDuplicate) {
		t.Errorf("expected ErrNameDuplicate, got %v", err)
	}
	// Same name in a different cluster should be fine.
	if _, err := r.Create(ctx, Server{ClusterID: cid2, Name: "Island", Map: "y", InstallDir: "y", Ports: PortTriple{Game: 7900, Query: 27200, RCON: 27500}}); err != nil {
		t.Errorf("different-cluster duplicate name should be allowed: %v", err)
	}
}

func TestServerRepo_ListByClusterAndAll(t *testing.T) {
	d := newTestDB(t)
	r := NewRepo(d)
	ctx := context.Background()
	c1 := seedCluster(t, d, "c1")
	c2 := seedCluster(t, d, "c2")
	mk := func(cid int64, name string, p PortTriple) Server {
		return Server{ClusterID: cid, Name: name, Map: "x", InstallDir: "x", Ports: p}
	}
	if _, err := r.Create(ctx, mk(c1, "A", DefaultBase)); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Create(ctx, mk(c1, "B", PortTriple{Game: 7787, Query: 27025, RCON: 27030})); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Create(ctx, mk(c2, "C", PortTriple{Game: 7900, Query: 27200, RCON: 27500})); err != nil {
		t.Fatal(err)
	}

	c1Servers, _ := r.ListByCluster(ctx, c1)
	if len(c1Servers) != 2 {
		t.Errorf("ListByCluster(c1) len=%d, want 2", len(c1Servers))
	}
	all, _ := r.ListAll(ctx)
	if len(all) != 3 {
		t.Errorf("ListAll len=%d, want 3", len(all))
	}
}

// TestServerRepo_StandaloneServer covers the no-cluster path: ClusterID
// 0 must round-trip through the nullable column without violating the
// FK constraint, and ListStandalone must find it while ListByCluster
// must NOT.
func TestServerRepo_StandaloneServer(t *testing.T) {
	d := newTestDB(t)
	r := NewRepo(d)
	ctx := context.Background()

	in := Server{
		ClusterID:  0, // standalone — no cluster
		Name:       "OneOff",
		Map:        "TheIsland_WP",
		InstallDir: `C:\servers\OneOff`,
		Ports:      DefaultBase,
	}
	created, err := r.Create(ctx, in)
	if err != nil {
		t.Fatalf("create standalone: %v", err)
	}
	if created.ClusterID != 0 {
		t.Errorf("ClusterID = %d, want 0 (standalone)", created.ClusterID)
	}

	got, _ := r.Get(ctx, created.ID)
	if got.ClusterID != 0 {
		t.Errorf("Get reported ClusterID=%d, want 0", got.ClusterID)
	}

	standalone, err := r.ListStandalone(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(standalone) != 1 || standalone[0].ID != created.ID {
		t.Errorf("ListStandalone returned %v, want [#%d]", standalone, created.ID)
	}

	// A cluster's list must not include standalone servers.
	bogus, _ := r.ListByCluster(ctx, 1)
	for _, s := range bogus {
		if s.ID == created.ID {
			t.Error("standalone server leaked into ListByCluster")
		}
	}
}

func TestServerRepo_SuggestPortsFromDB_CrossCluster(t *testing.T) {
	d := newTestDB(t)
	r := NewRepo(d)
	ctx := context.Background()
	c1 := seedCluster(t, d, "c1")
	c2 := seedCluster(t, d, "c2")
	if _, err := r.Create(ctx, Server{ClusterID: c1, Name: "A", Map: "x", InstallDir: "x", Ports: DefaultBase}); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Create(ctx, Server{ClusterID: c2, Name: "B", Map: "x", InstallDir: "x", Ports: PortTriple{Game: 7787, Query: 27025, RCON: 27030}}); err != nil {
		t.Fatal(err)
	}
	got, err := r.SuggestPortsFromDB(ctx)
	if err != nil {
		t.Fatalf("suggest: %v", err)
	}
	want := PortTriple{Game: 7797, Query: 27035, RCON: 27040}
	if got != want {
		t.Errorf("got %+v, want %+v (must skip cross-cluster collision)", got, want)
	}
}
