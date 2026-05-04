package backup

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

func TestRepo_CRUDRoundTrip(t *testing.T) {
	d := newTestDB(t)
	r := NewRepo(d)
	ctx := context.Background()

	in := Backup{
		Scope: Scope{Type: "server", ID: 1},
		Path:  `C:\backups\server-1.zip`,
		SizeBytes: 12345,
		Kind:  "manual",
	}
	created, err := r.Create(ctx, in)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if created.ID == 0 {
		t.Fatal("expected ID populated")
	}
	if created.CreatedAt.IsZero() {
		t.Error("expected CreatedAt populated")
	}

	got, err := r.Get(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Path != in.Path || got.SizeBytes != in.SizeBytes {
		t.Errorf("got %+v", got)
	}

	if err := r.Delete(ctx, got.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Get(ctx, got.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound after delete, got %v", err)
	}
}

func TestRepo_ListScopedNewestFirst(t *testing.T) {
	d := newTestDB(t)
	r := NewRepo(d)
	ctx := context.Background()
	scope := Scope{Type: "server", ID: 5}

	now := time.Now().UTC().Truncate(time.Second)
	for i := 0; i < 3; i++ {
		_, err := r.Create(ctx, Backup{
			Scope: scope, Path: "p", Kind: "manual",
			CreatedAt: now.Add(time.Duration(i) * time.Hour),
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	list, err := r.List(ctx, scope)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 3 {
		t.Fatalf("got %d, want 3", len(list))
	}
	if !list[0].CreatedAt.After(list[2].CreatedAt) {
		t.Errorf("expected newest first; got %v then %v", list[0].CreatedAt, list[2].CreatedAt)
	}
}

func TestRepo_ListIsolatedByScope(t *testing.T) {
	d := newTestDB(t)
	r := NewRepo(d)
	ctx := context.Background()

	if _, err := r.Create(ctx, Backup{Scope: Scope{Type: "server", ID: 1}, Path: "a", Kind: "manual"}); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Create(ctx, Backup{Scope: Scope{Type: "server", ID: 2}, Path: "b", Kind: "manual"}); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Create(ctx, Backup{Scope: Scope{Type: "cluster", ID: 1}, Path: "c", Kind: "manual"}); err != nil {
		t.Fatal(err)
	}

	srv1, _ := r.List(ctx, Scope{Type: "server", ID: 1})
	if len(srv1) != 1 {
		t.Errorf("server 1: got %d", len(srv1))
	}
	all, _ := r.ListAll(ctx)
	if len(all) != 3 {
		t.Errorf("listAll: got %d", len(all))
	}
}
