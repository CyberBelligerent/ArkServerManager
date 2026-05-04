package players

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"asamanager/internal/db"
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

// seedServer inserts a minimal cluster + server pair
func seedServer(t *testing.T, d *sql.DB) int64 {
	t.Helper()
	cres, err := d.Exec(`INSERT INTO clusters(name, cluster_id, cluster_dir) VALUES('c','c','x')`)
	if err != nil {
		t.Fatal(err)
	}
	cid, _ := cres.LastInsertId()
	sres, err := d.Exec(`
		INSERT INTO servers(cluster_id, name, map, install_dir, port, query_port, rcon_port)
		VALUES(?, 'srv', 'm', 'd', 7777, 27015, 27020)`, cid)
	if err != nil {
		t.Fatal(err)
	}
	sid, _ := sres.LastInsertId()
	return sid
}

func TestRepo_UpsertSeen_InsertsAndUpdatesNameHistory(t *testing.T) {
	d := newTestDB(t)
	r := NewRepo(d)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)

	if err := r.UpsertSeen(ctx, "76561198000000001", "Alice", now); err != nil {
		t.Fatalf("first upsert: %v", err)
	}
	p, err := r.GetPlayer(ctx, "76561198000000001")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if p.LastKnownName != "Alice" {
		t.Errorf("LastKnownName = %q", p.LastKnownName)
	}
	if !p.FirstSeen.Equal(now) {
		t.Errorf("FirstSeen = %v, want %v", p.FirstSeen, now)
	}

	later := now.Add(2 * time.Hour)
	if err := r.UpsertSeen(ctx, "76561198000000001", "Alice2", later); err != nil {
		t.Fatalf("rename upsert: %v", err)
	}
	p2, _ := r.GetPlayer(ctx, "76561198000000001")
	if p2.LastKnownName != "Alice2" {
		t.Errorf("LastKnownName not updated: %q", p2.LastKnownName)
	}
	if !p2.LastSeen.Equal(later) {
		t.Errorf("LastSeen = %v, want %v", p2.LastSeen, later)
	}

	var nameHistoryCount int
	if err := d.QueryRow(`SELECT COUNT(*) FROM player_name_history WHERE steam_id = ?`, "76561198000000001").Scan(&nameHistoryCount); err != nil {
		t.Fatal(err)
	}
	if nameHistoryCount != 2 {
		t.Errorf("name history rows = %d, want 2", nameHistoryCount)
	}
}

func TestRepo_SessionLifecycle(t *testing.T) {
	d := newTestDB(t)
	r := NewRepo(d)
	ctx := context.Background()
	sid := seedServer(t, d)

	now := time.Now().UTC().Truncate(time.Second)
	if err := r.UpsertSeen(ctx, "S1", "Alice", now); err != nil {
		t.Fatal(err)
	}
	sessID, err := r.StartSession(ctx, "S1", sid, now)
	if err != nil {
		t.Fatalf("StartSession: %v", err)
	}

	open, err := r.ListOpenSessions(ctx, sid)
	if err != nil {
		t.Fatal(err)
	}
	if len(open) != 1 || open[0].ID != sessID {
		t.Errorf("ListOpenSessions = %+v, want one with id %d", open, sessID)
	}

	left := now.Add(45 * time.Minute)
	if err := r.EndSession(ctx, sessID, left); err != nil {
		t.Fatalf("EndSession: %v", err)
	}
	open, _ = r.ListOpenSessions(ctx, sid)
	if len(open) != 0 {
		t.Errorf("expected no open sessions after EndSession, got %+v", open)
	}
	p, _ := r.GetPlayer(ctx, "S1")
	if p.TotalMinutes != 45 {
		t.Errorf("TotalMinutes = %d, want 45", p.TotalMinutes)
	}
}

func TestRepo_EndSessionIdempotent(t *testing.T) {
	d := newTestDB(t)
	r := NewRepo(d)
	ctx := context.Background()
	sid := seedServer(t, d)
	now := time.Now().UTC()
	_ = r.UpsertSeen(ctx, "S1", "Alice", now)
	id, _ := r.StartSession(ctx, "S1", sid, now)
	if err := r.EndSession(ctx, id, now.Add(10*time.Minute)); err != nil {
		t.Fatal(err)
	}
	// Calling again should be a no-op rather than double-counting.
	if err := r.EndSession(ctx, id, now.Add(20*time.Minute)); err != nil {
		t.Fatal(err)
	}
	p, _ := r.GetPlayer(ctx, "S1")
	if p.TotalMinutes != 10 {
		t.Errorf("TotalMinutes = %d, want 10 (no double-count)", p.TotalMinutes)
	}
}

func TestRepo_NotesAndWatched(t *testing.T) {
	d := newTestDB(t)
	r := NewRepo(d)
	ctx := context.Background()
	_ = r.UpsertSeen(ctx, "S1", "Alice", time.Now())

	if err := r.SetNotes(ctx, "S1", "suspicious"); err != nil {
		t.Fatal(err)
	}
	if err := r.SetWatched(ctx, "S1", true); err != nil {
		t.Fatal(err)
	}
	p, _ := r.GetPlayer(ctx, "S1")
	if p.Notes != "suspicious" || !p.IsWatched {
		t.Errorf("got %+v", p)
	}
}

func TestRepo_BansAddRemoveList(t *testing.T) {
	d := newTestDB(t)
	r := NewRepo(d)
	ctx := context.Background()
	scope := Scope{Type: "server", ID: 1}

	if _, err := r.AddBan(ctx, Ban{SteamID: "S1", Scope: scope, Reason: "griefing", BannedBy: "admin"}); err != nil {
		t.Fatal(err)
	}
	// Idempotent on conflict — second add updates instead of erroring.
	if _, err := r.AddBan(ctx, Ban{SteamID: "S1", Scope: scope, Reason: "updated", BannedBy: "admin2"}); err != nil {
		t.Fatal(err)
	}
	bans, err := r.ListBans(ctx, scope)
	if err != nil {
		t.Fatal(err)
	}
	if len(bans) != 1 {
		t.Fatalf("expected 1 ban after upsert, got %d", len(bans))
	}
	if bans[0].Reason != "updated" || bans[0].BannedBy != "admin2" {
		t.Errorf("upsert did not apply: %+v", bans[0])
	}

	removed, err := r.RemoveBan(ctx, "S1", scope)
	if err != nil {
		t.Fatal(err)
	}
	if !removed {
		t.Error("expected RemoveBan to report removal")
	}
	bans, _ = r.ListBans(ctx, scope)
	if len(bans) != 0 {
		t.Errorf("expected empty bans after remove, got %d", len(bans))
	}
}

func TestRepo_WhitelistAddRemoveList(t *testing.T) {
	d := newTestDB(t)
	r := NewRepo(d)
	ctx := context.Background()
	scope := Scope{Type: "cluster", ID: 5}

	if _, err := r.AddWhitelist(ctx, WhitelistEntry{SteamID: "S1", Scope: scope}); err != nil {
		t.Fatal(err)
	}
	// Idempotent.
	if _, err := r.AddWhitelist(ctx, WhitelistEntry{SteamID: "S1", Scope: scope}); err != nil {
		t.Fatal(err)
	}
	list, _ := r.ListWhitelist(ctx, scope)
	if len(list) != 1 {
		t.Errorf("expected 1 entry, got %d", len(list))
	}
	if removed, _ := r.RemoveWhitelist(ctx, "S1", scope); !removed {
		t.Error("expected removal")
	}
}

func TestRepo_GetPlayer_NotFound(t *testing.T) {
	d := newTestDB(t)
	r := NewRepo(d)
	if _, err := r.GetPlayer(context.Background(), "missing"); !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}
