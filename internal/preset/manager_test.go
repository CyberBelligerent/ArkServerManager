package preset

import (
	"context"
	"path/filepath"
	"testing"

	"asamanager/internal/cluster"
	"asamanager/internal/server"
	"asamanager/internal/settings"
)

func TestManager_ApplyCluster_DiffMergesAndPreservesUnmentionedKeys(t *testing.T) {
	d := newTestDB(t)
	clusterRepo := cluster.NewRepo(d)
	serverRepo := server.NewRepo(d)
	presetRepo := NewRepo(d)

	// Cluster starts with two settings; preset only touches one of them
	// and adds a new one. The untouched key must survive.
	c, err := clusterRepo.Create(context.Background(), cluster.Cluster{
		Name: "C", ClusterID: "abc-001", ClusterDir: t.TempDir(),
		Settings: map[string]settings.Value{
			"TamingSpeedMultiplier": settings.FloatVal(2.0),
			"XPMultiplier":          settings.FloatVal(2.0),
		},
		ActiveEvent: "",
	})
	if err != nil {
		t.Fatal(err)
	}
	// One server in the cluster so PropagateToServers has somewhere to write.
	sv, err := serverRepo.Create(context.Background(), server.Server{
		ClusterID: c.ID, Name: "S", Map: "TheIsland_WP",
		InstallDir: filepath.Join(t.TempDir(), "srv"),
		Ports:      server.DefaultBase,
	})
	if err != nil {
		t.Fatal(err)
	}
	_ = sv

	ev := "Summer"
	p, err := presetRepo.Create(context.Background(), Preset{
		ScopeType: ScopeCluster, ScopeID: c.ID, Name: "Summer",
		Payload: Payload{
			Settings: map[string]settings.Value{
				"TamingSpeedMultiplier": settings.FloatVal(5.0),
				// Note: HarvestAmountMultiplier was NOT in cluster before; preset adds it.
				"HarvestAmountMultiplier": settings.FloatVal(3.0),
			},
			ActiveEvent: &ev,
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	mgr := NewManager(ManagerDeps{
		Presets: presetRepo, Servers: serverRepo, Clusters: clusterRepo,
	})
	if _, err := mgr.Apply(context.Background(), p); err != nil {
		t.Fatalf("apply: %v", err)
	}

	updated, _ := clusterRepo.Get(context.Background(), c.ID)
	if got := updated.Settings["TamingSpeedMultiplier"].Float; got != 5.0 {
		t.Errorf("TamingSpeed should be overridden to 5.0, got %v", got)
	}
	if got := updated.Settings["XPMultiplier"].Float; got != 2.0 {
		t.Errorf("XPMultiplier should NOT be touched, got %v", got)
	}
	if got := updated.Settings["HarvestAmountMultiplier"].Float; got != 3.0 {
		t.Errorf("HarvestAmount should be added by preset, got %v", got)
	}
	if updated.ActiveEvent != "Summer" {
		t.Errorf("ActiveEvent should be set to Summer, got %q", updated.ActiveEvent)
	}
}

func TestManager_ApplyServer_OnlyTouchesOverrides(t *testing.T) {
	d := newTestDB(t)
	clusterRepo := cluster.NewRepo(d)
	serverRepo := server.NewRepo(d)
	presetRepo := NewRepo(d)

	c, _ := clusterRepo.Create(context.Background(), cluster.Cluster{
		Name: "C", ClusterID: "abc-002", ClusterDir: t.TempDir(),
	})
	s, _ := serverRepo.Create(context.Background(), server.Server{
		ClusterID: c.ID, Name: "S", Map: "TheIsland_WP",
		InstallDir: filepath.Join(t.TempDir(), "srv"),
		Ports:      server.DefaultBase,
		SettingOverrides: map[string]settings.Value{
			"TamingSpeedMultiplier": settings.FloatVal(2.0),
		},
	})

	clear := "None"
	p, _ := presetRepo.Create(context.Background(), Preset{
		ScopeType: ScopeServer, ScopeID: s.ID, Name: "Reset",
		Payload: Payload{
			Settings: map[string]settings.Value{
				"XPMultiplier": settings.FloatVal(1.5),
			},
			ActiveEvent: &clear,
		},
	})

	mgr := NewManager(ManagerDeps{Presets: presetRepo, Servers: serverRepo, Clusters: clusterRepo})
	if _, err := mgr.Apply(context.Background(), p); err != nil {
		t.Fatalf("apply: %v", err)
	}

	updated, _ := serverRepo.Get(context.Background(), s.ID)
	if got := updated.SettingOverrides["TamingSpeedMultiplier"].Float; got != 2.0 {
		t.Errorf("existing override should survive, got %v", got)
	}
	if got := updated.SettingOverrides["XPMultiplier"].Float; got != 1.5 {
		t.Errorf("preset override missing, got %v", got)
	}
	if updated.ActiveEvent != "None" {
		t.Errorf("ActiveEvent should be 'None' (explicit clear), got %q", updated.ActiveEvent)
	}
}

func TestManager_Apply_ActiveEventNilDoesNotTouch(t *testing.T) {
	d := newTestDB(t)
	clusterRepo := cluster.NewRepo(d)
	serverRepo := server.NewRepo(d)
	presetRepo := NewRepo(d)

	c, _ := clusterRepo.Create(context.Background(), cluster.Cluster{
		Name: "C", ClusterID: "abc-003", ClusterDir: t.TempDir(),
		ActiveEvent: "Winter",
	})

	p, _ := presetRepo.Create(context.Background(), Preset{
		ScopeType: ScopeCluster, ScopeID: c.ID, Name: "Rates only",
		Payload: Payload{
			Settings: map[string]settings.Value{
				"XPMultiplier": settings.FloatVal(2.0),
			},
			// ActiveEvent intentionally nil.
		},
	})

	mgr := NewManager(ManagerDeps{Presets: presetRepo, Servers: serverRepo, Clusters: clusterRepo})
	if _, err := mgr.Apply(context.Background(), p); err != nil {
		t.Fatalf("apply: %v", err)
	}

	updated, _ := clusterRepo.Get(context.Background(), c.ID)
	if updated.ActiveEvent != "Winter" {
		t.Errorf("ActiveEvent must be untouched when preset's ActiveEvent is nil; got %q", updated.ActiveEvent)
	}
}
