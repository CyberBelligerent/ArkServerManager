package server

import (
	"context"
	"sync"
	"testing"
	"time"

	"asamanager/internal/rcon"
)

type fakeSupervisor struct {
	mu     sync.Mutex
	starts []time.Time
	stops  []int64
	err    error
}

func (s *fakeSupervisor) Start(_ context.Context, id int64) error {
	s.mu.Lock()
	s.starts = append(s.starts, time.Now())
	s.mu.Unlock()
	return s.err
}

func (s *fakeSupervisor) Stop(_ context.Context, id int64, _ bool) error {
	s.mu.Lock()
	s.stops = append(s.stops, id)
	s.mu.Unlock()
	return nil
}

func (s *fakeSupervisor) Restart(_ context.Context, _ int64) error { return nil }
func (s *fakeSupervisor) Status(_ int64) Status                    { return StatusStopped }
func (s *fakeSupervisor) Logs(_ int64) (<-chan string, error)      { return nil, nil }
func (s *fakeSupervisor) RCONFor(_ int64) rcon.Client              { return nil }

func TestCoordinator_StartCluster_StaggersBetweenStarts(t *testing.T) {
	d := newTestDB(t)
	sr := NewRepo(d)
	ctx := context.Background()
	cid := seedCluster(t, d, "c1")

	ports := []PortTriple{
		DefaultBase,
		{Game: 7787, Query: 27025, RCON: 27030},
		{Game: 7797, Query: 27035, RCON: 27040},
	}
	for i, p := range ports {
		if _, err := sr.Create(ctx, Server{
			ClusterID: cid, Name: string(rune('A' + i)), Map: "x", InstallDir: "x", Ports: p,
		}); err != nil {
			t.Fatal(err)
		}
	}

	sup := &fakeSupervisor{}
	coord := &Coordinator{Sup: sup, Repo: sr, Stagger: 50 * time.Millisecond}

	start := time.Now()
	if err := coord.StartCluster(ctx, cid, 0); err != nil {
		t.Fatalf("StartCluster: %v", err)
	}
	elapsed := time.Since(start)

	if len(sup.starts) != 3 {
		t.Fatalf("got %d starts, want 3", len(sup.starts))
	}
	if elapsed < 100*time.Millisecond {
		t.Errorf("expected at least 100ms (2× 50ms stagger), got %v", elapsed)
	}
	gap1 := sup.starts[1].Sub(sup.starts[0])
	gap2 := sup.starts[2].Sub(sup.starts[1])
	if gap1 < 40*time.Millisecond {
		t.Errorf("gap1=%v too small", gap1)
	}
	if gap2 < 40*time.Millisecond {
		t.Errorf("gap2=%v too small", gap2)
	}
}

func TestCoordinator_StopCluster_AppliesToAll(t *testing.T) {
	d := newTestDB(t)
	sr := NewRepo(d)
	ctx := context.Background()
	cid := seedCluster(t, d, "c1")
	for i, p := range []PortTriple{DefaultBase, {Game: 7787, Query: 27025, RCON: 27030}} {
		if _, err := sr.Create(ctx, Server{ClusterID: cid, Name: string(rune('A' + i)), Map: "x", InstallDir: "x", Ports: p}); err != nil {
			t.Fatal(err)
		}
	}
	sup := &fakeSupervisor{}
	coord := &Coordinator{Sup: sup, Repo: sr, Stagger: 0}
	if err := coord.StopCluster(ctx, cid, true, 0); err != nil {
		t.Fatalf("StopCluster: %v", err)
	}
	if len(sup.stops) != 2 {
		t.Errorf("got %d stops, want 2", len(sup.stops))
	}
}

func TestCoordinator_StaggerOverride(t *testing.T) {
	d := newTestDB(t)
	sr := NewRepo(d)
	ctx := context.Background()
	cid := seedCluster(t, d, "c1")
	for i, p := range []PortTriple{DefaultBase, {Game: 7787, Query: 27025, RCON: 27030}} {
		if _, err := sr.Create(ctx, Server{ClusterID: cid, Name: string(rune('A' + i)), Map: "x", InstallDir: "x", Ports: p}); err != nil {
			t.Fatal(err)
		}
	}
	sup := &fakeSupervisor{}
	// Default would be 1 second between starts; override should bring it down to ~50ms.
	coord := &Coordinator{Sup: sup, Repo: sr, Stagger: 1 * time.Second}

	start := time.Now()
	if err := coord.StartCluster(ctx, cid, 50*time.Millisecond); err != nil {
		t.Fatalf("StartCluster: %v", err)
	}
	elapsed := time.Since(start)
	if elapsed > 500*time.Millisecond {
		t.Errorf("override ignored: elapsed=%v, expected <500ms", elapsed)
	}
}

func TestCoordinator_RespectsContextCancel(t *testing.T) {
	d := newTestDB(t)
	sr := NewRepo(d)
	ctx, cancel := context.WithCancel(context.Background())
	cid := seedCluster(t, d, "c1")
	for i, p := range []PortTriple{DefaultBase, {Game: 7787, Query: 27025, RCON: 27030}, {Game: 7797, Query: 27035, RCON: 27040}} {
		_, _ = sr.Create(ctx, Server{ClusterID: cid, Name: string(rune('A' + i)), Map: "x", InstallDir: "x", Ports: p})
	}
	sup := &fakeSupervisor{}
	coord := &Coordinator{Sup: sup, Repo: sr, Stagger: 200 * time.Millisecond}

	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	err := coord.StartCluster(ctx, cid, 0)
	if err == nil || err != context.Canceled {
		t.Errorf("expected context.Canceled, got %v", err)
	}
	if len(sup.starts) != 1 {
		t.Errorf("expected only first start before cancel, got %d", len(sup.starts))
	}
}
