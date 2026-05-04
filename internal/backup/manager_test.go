package backup

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"asamanager/internal/cluster"
	"asamanager/internal/events"
	"asamanager/internal/server"
)

func silentLog() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func setupManager(t *testing.T) (*Manager, *server.Repo, *cluster.Repo, *events.Bus, chan events.Event) {
	t.Helper()
	d := newTestDB(t)
	bus := events.NewBus(64)
	bus.Start()
	t.Cleanup(bus.Stop)
	got := make(chan events.Event, 16)
	bus.SubscribeAll(func(e events.Event) {
		select {
		case got <- e:
		default:
		}
	})
	srvRepo := server.NewRepo(d)
	clRepo := cluster.NewRepo(d)
	mgr := NewManager(ManagerDeps{
		Repo: NewRepo(d), Servers: srvRepo, Clusters: clRepo, Bus: bus,
		Log: silentLog(), DestDir: t.TempDir(), KeepCount: 2,
	})
	return mgr, srvRepo, clRepo, bus, got
}

func TestManager_BackupServer_EmitsEventsAndPrunes(t *testing.T) {
	mgr, srvRepo, clRepo, _, got := setupManager(t)
	ctx := context.Background()

	c, err := clRepo.Create(ctx, cluster.Cluster{Name: "c", ClusterID: "c1", ClusterDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	install := filepath.Join(t.TempDir(), "Island")
	writeFile(t, filepath.Join(install, savedArksRel, "save.bin"), "x")
	writeFile(t, filepath.Join(install, configRel, "Game.ini"), "x")
	srv, err := srvRepo.Create(ctx, server.Server{
		ClusterID: c.ID, Name: "Island", Map: "x", InstallDir: install, Ports: server.DefaultBase,
	})
	if err != nil {
		t.Fatal(err)
	}

	// Three backups; KeepCount=2
	for i := 0; i < 3; i++ {
		if _, err := mgr.BackupServer(ctx, srv, "manual"); err != nil {
			t.Fatalf("BackupServer #%d: %v", i, err)
		}
		time.Sleep(1100 * time.Millisecond)
	}

	scope := Scope{Type: "server", ID: srv.ID}
	all, _ := mgr.deps.Repo.List(ctx, scope)
	if len(all) != 2 {
		t.Errorf("expected 2 rows after prune, got %d", len(all))
	}
	for _, b := range all {
		if _, err := os.Stat(b.Path); err != nil {
			t.Errorf("expected %s on disk: %v", b.Path, err)
		}
	}

	// Should have 3 started and 3 completed events
	var startedCount, completedCount int
	deadline := time.After(time.Second)
loop:
	for {
		select {
		case e := <-got:
			switch e.(type) {
			case events.BackupStarted:
				startedCount++
			case events.BackupCompleted:
				completedCount++
			}
		case <-deadline:
			break loop
		}
	}
	if startedCount < 3 || completedCount < 3 {
		t.Errorf("events: started=%d, completed=%d, want >=3 each", startedCount, completedCount)
	}
}

func TestManager_DeleteBackup_RemovesFileAndRow(t *testing.T) {
	mgr, srvRepo, clRepo, _, _ := setupManager(t)
	ctx := context.Background()
	c, _ := clRepo.Create(ctx, cluster.Cluster{Name: "c", ClusterID: "c", ClusterDir: t.TempDir()})
	install := filepath.Join(t.TempDir(), "Srv")
	writeFile(t, filepath.Join(install, savedArksRel, "save.bin"), "x")
	srv, _ := srvRepo.Create(ctx, server.Server{ClusterID: c.ID, Name: "Srv", Map: "x", InstallDir: install, Ports: server.DefaultBase})

	b, err := mgr.BackupServer(ctx, srv, "manual")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(b.Path); err != nil {
		t.Fatal(err)
	}
	if err := mgr.DeleteBackup(ctx, b); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(b.Path); !os.IsNotExist(err) {
		t.Errorf("file not removed: err=%v", err)
	}
}
