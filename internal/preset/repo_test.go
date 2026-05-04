package preset

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
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = d.Close() })
	return d
}

func TestRepo_CRUDRoundTrip(t *testing.T) {
	r := NewRepo(newTestDB(t))
	ctx := context.Background()

	ev := "Summer"
	in := Preset{
		ScopeType: ScopeCluster,
		ScopeID:   42,
		Name:      "Summer Bash",
		Description: "Crank rates and flip on the seasonal event.",
		Payload: Payload{
			Settings: map[string]settings.Value{
				"TamingSpeedMultiplier": settings.FloatVal(5.0),
				"XPMultiplier":          settings.FloatVal(2.0),
			},
			ActiveEvent: &ev,
		},
	}
	created, err := r.Create(ctx, in)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if created.ID == 0 {
		t.Fatal("expected ID populated")
	}
	if created.Payload.ActiveEvent == nil || *created.Payload.ActiveEvent != "Summer" {
		t.Errorf("ActiveEvent didn't round-trip: %v", created.Payload.ActiveEvent)
	}
	if got := created.Payload.Settings["TamingSpeedMultiplier"].Float; got != 5.0 {
		t.Errorf("TamingSpeedMultiplier=%v, want 5.0", got)
	}

	// Update
	created.Description = "edited"
	if err := r.Update(ctx, created); err != nil {
		t.Fatal(err)
	}
	got, _ := r.Get(ctx, created.ID)
	if got.Description != "edited" {
		t.Errorf("description didn't update")
	}

	// Delete
	if err := r.Delete(ctx, created.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Get(ctx, created.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound after delete, got %v", err)
	}
}

func TestRepo_NameUniquePerScope(t *testing.T) {
	r := NewRepo(newTestDB(t))
	ctx := context.Background()
	mk := func(scope string, id int64, name string) error {
		_, err := r.Create(ctx, Preset{ScopeType: scope, ScopeID: id, Name: name})
		return err
	}
	if err := mk(ScopeCluster, 1, "Summer"); err != nil {
		t.Fatal(err)
	}
	if err := mk(ScopeCluster, 1, "Summer"); !errors.Is(err, ErrNameDuplicate) {
		t.Errorf("expected ErrNameDuplicate, got %v", err)
	}
	// Same name, different scope id => allowed.
	if err := mk(ScopeCluster, 2, "Summer"); err != nil {
		t.Errorf("same name in different scope should succeed, got %v", err)
	}
}

func TestRepo_List_OrderedByName(t *testing.T) {
	r := NewRepo(newTestDB(t))
	ctx := context.Background()
	for _, name := range []string{"Zeta", "Alpha", "Mike"} {
		if _, err := r.Create(ctx, Preset{ScopeType: ScopeCluster, ScopeID: 1, Name: name}); err != nil {
			t.Fatal(err)
		}
	}
	got, _ := r.List(ctx, ScopeCluster, 1)
	if len(got) != 3 {
		t.Fatalf("got %d, want 3", len(got))
	}
	want := []string{"Alpha", "Mike", "Zeta"}
	for i, p := range got {
		if p.Name != want[i] {
			t.Errorf("[%d] = %q, want %q", i, p.Name, want[i])
		}
	}
}
