package webhook

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"

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

	in := Webhook{
		Name:      "alerts",
		URL:       "https://discord.com/api/webhooks/123/abc",
		Scope:     Scope{Type: "global"},
		EventMask: []string{"server.started", "server.crashed"},
		Templates: map[string]string{"server.started": "**custom**: {{.Name}}"},
		Enabled:   true,
	}
	created, err := r.Create(ctx, in)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if created.ID == 0 {
		t.Fatal("expected ID populated")
	}

	got, err := r.Get(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != in.Name || got.URL != in.URL || !got.Enabled {
		t.Errorf("got %+v", got)
	}
	if len(got.EventMask) != 2 || got.EventMask[0] != "server.started" {
		t.Errorf("EventMask = %v", got.EventMask)
	}
	if got.Templates["server.started"] != "**custom**: {{.Name}}" {
		t.Errorf("Templates = %+v", got.Templates)
	}

	got.Name = "renamed"
	got.EventMask = []string{"*"}
	if err := r.Update(ctx, got); err != nil {
		t.Fatal(err)
	}
	reread, _ := r.Get(ctx, got.ID)
	if reread.Name != "renamed" || len(reread.EventMask) != 1 || reread.EventMask[0] != "*" {
		t.Errorf("post-update: %+v", reread)
	}

	if err := r.Delete(ctx, got.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Get(ctx, got.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound after delete, got %v", err)
	}
}

func TestRepo_ScopeIDNullForGlobal(t *testing.T) {
	d := newTestDB(t)
	r := NewRepo(d)
	ctx := context.Background()
	created, _ := r.Create(ctx, Webhook{Name: "g", URL: "u", Scope: Scope{Type: "global"}, Enabled: true})
	got, _ := r.Get(ctx, created.ID)
	if got.Scope.ID != 0 {
		t.Errorf("global webhook scope.ID = %d, want 0", got.Scope.ID)
	}
}

func TestRepo_ListReturnsAll(t *testing.T) {
	d := newTestDB(t)
	r := NewRepo(d)
	ctx := context.Background()
	for _, n := range []string{"a", "b", "c"} {
		if _, err := r.Create(ctx, Webhook{Name: n, URL: "u", Scope: Scope{Type: "global"}, Enabled: true}); err != nil {
			t.Fatal(err)
		}
	}
	all, err := r.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 3 {
		t.Errorf("List len = %d, want 3", len(all))
	}
}
