package scheduler

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
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = d.Close() })
	return d
}

func TestRepo_CRUDRoundTrip(t *testing.T) {
	r := NewRepo(newTestDB(t))
	ctx := context.Background()

	now := time.Now().UTC().Truncate(time.Second)
	in := Task{
		Name:          "Nightly restart",
		ScopeType:     "cluster",
		ScopeID:       7,
		TriggerKind:   TriggerCron,
		TriggerCron:   "0 4 * * *",
		ActionKind:    ActionRestartCluster,
		ActionPayload: []byte(`{"cluster_id":7,"stagger_seconds":30}`),
		MissedPolicy:  MissedSkip,
		Status:        StatusEnabled,
		NextFireAt:    &now,
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
	if got.Name != in.Name || got.ScopeID != in.ScopeID || got.TriggerCron != in.TriggerCron {
		t.Errorf("got %+v", got)
	}
	if got.NextFireAt == nil || !got.NextFireAt.Equal(now) {
		t.Errorf("NextFireAt = %v, want %v", got.NextFireAt, now)
	}

	got.Name = "renamed"
	if err := r.Update(ctx, got); err != nil {
		t.Fatal(err)
	}
	rer, _ := r.Get(ctx, got.ID)
	if rer.Name != "renamed" {
		t.Errorf("update did not stick")
	}

	if err := r.Delete(ctx, got.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Get(ctx, got.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound after delete, got %v", err)
	}
}

func TestRepo_ListByScope(t *testing.T) {
	r := NewRepo(newTestDB(t))
	ctx := context.Background()

	mk := func(scope string, id int64, name string) {
		t.Helper()
		_, err := r.Create(ctx, Task{
			Name: name, ScopeType: scope, ScopeID: id,
			TriggerKind: TriggerOneshot, ActionKind: ActionBackup,
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	mk("global", 0, "g1")
	mk("cluster", 1, "c1a")
	mk("cluster", 1, "c1b")
	mk("cluster", 2, "c2")
	mk("server", 9, "s9")

	cluster1, _ := r.List(ctx, "cluster", 1)
	if len(cluster1) != 2 {
		t.Errorf("cluster 1 list = %d, want 2", len(cluster1))
	}
	global, _ := r.List(ctx, "global", 0)
	if len(global) != 1 {
		t.Errorf("global list = %d, want 1", len(global))
	}
}

func TestRepo_TimezoneInstantPreserved(t *testing.T) {
	r := NewRepo(newTestDB(t))
	when := time.Date(2026, 5, 3, 20, 15, 0, 0, time.UTC)
	in := Task{
		Name: "tz", ScopeType: "global",
		TriggerKind: TriggerOneshot, ActionKind: ActionBackup,
		StartAt: when, NextFireAt: &when,
		ActionPayload: []byte(`{}`),
	}
	created, err := r.Create(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	if !created.StartAt.Equal(when) {
		t.Fatalf("StartAt instant drift: got %v (unix %d), want %v (unix %d)",
			created.StartAt, created.StartAt.Unix(), when, when.Unix())
	}
	if created.NextFireAt == nil || !created.NextFireAt.Equal(when) {
		t.Fatalf("NextFireAt instant drift: got %v, want %v", created.NextFireAt, when)
	}
}

func TestRepo_RecordRun_AndListRuns(t *testing.T) {
	r := NewRepo(newTestDB(t))
	ctx := context.Background()
	created, err := r.Create(ctx, Task{
		Name: "x", ScopeType: "global", TriggerKind: TriggerOneshot, ActionKind: ActionBackup,
	})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Second)
	for i := 0; i < 3; i++ {
		if err := r.RecordRun(ctx, created.ID, now.Add(time.Duration(i)*time.Second), RunResultSuccess, "ok"); err != nil {
			t.Fatal(err)
		}
	}
	runs, err := r.ListRuns(ctx, created.ID, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 3 {
		t.Errorf("got %d runs, want 3", len(runs))
	}
}
