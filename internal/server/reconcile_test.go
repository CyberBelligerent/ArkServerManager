package server

import (
	"context"
	"testing"
)

func TestReconcileOnStartup_ResetsNonTerminalStatuses(t *testing.T) {
	d := newTestDB(t)
	r := NewRepo(d)
	ctx := context.Background()
	cid := seedCluster(t, d, "c1")

	mk := func(name string, st Status, p PortTriple) Server {
		s, err := r.Create(ctx, Server{ClusterID: cid, Name: name, Map: "x", InstallDir: "x", Ports: p})
		if err != nil {
			t.Fatal(err)
		}
		if err := r.UpdateStatus(ctx, s.ID, st); err != nil {
			t.Fatal(err)
		}
		s.Status = st
		return s
	}

	mk("starting", StatusStarting, DefaultBase)
	mk("running", StatusRunning, PortTriple{Game: 7787, Query: 27025, RCON: 27030})
	mk("stopping", StatusStopping, PortTriple{Game: 7797, Query: 27035, RCON: 27040})
	mk("crashed", StatusCrashed, PortTriple{Game: 7807, Query: 27045, RCON: 27050})
	mk("stopped", StatusStopped, PortTriple{Game: 7817, Query: 27055, RCON: 27060})

	if err := ReconcileOnStartup(ctx, r, nil); err != nil {
		t.Fatalf("ReconcileOnStartup: %v", err)
	}

	all, _ := r.ListAll(ctx)
	for _, s := range all {
		switch s.Name {
		case "starting", "running", "stopping":
			if s.Status != StatusStopped {
				t.Errorf("%q: status=%q, want stopped (was non-terminal)", s.Name, s.Status)
			}
		case "crashed":
			if s.Status != StatusCrashed {
				t.Errorf("%q: status=%q, want crashed (terminal, untouched)", s.Name, s.Status)
			}
		case "stopped":
			if s.Status != StatusStopped {
				t.Errorf("%q: status=%q, want stopped (terminal, untouched)", s.Name, s.Status)
			}
		}
	}
}

func TestReconcileOnStartup_NoOpOnEmptyDB(t *testing.T) {
	d := newTestDB(t)
	if err := ReconcileOnStartup(context.Background(), NewRepo(d), nil); err != nil {
		t.Errorf("expected nil on empty DB, got %v", err)
	}
}
