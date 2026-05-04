package cluster

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"asamanager/internal/server"
)

func seedFile(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestDestroyServer_RemovesDBRowAndFiles(t *testing.T) {
	d := newTestDB(t)
	cr := NewRepo(d)
	sr := server.NewRepo(d)
	ctx := context.Background()

	c, _ := cr.Create(ctx, Cluster{Name: "c", ClusterID: "c", ClusterDir: ""})
	root := t.TempDir()
	installDir := filepath.Join(root, "Island")
	logDir := filepath.Join(root, "logs")

	srv, err := sr.Create(ctx, server.Server{
		ClusterID: c.ID, Name: "Island", Map: "x",
		InstallDir: installDir, Ports: server.DefaultBase,
	})
	if err != nil {
		t.Fatal(err)
	}
	seedFile(t, filepath.Join(installDir, "ShooterGame", "binary.dat"), "x")
	seedFile(t, filepath.Join(logDir, "server-"+itoa(srv.ID)+".log"), "log")

	if err := DestroyServer(ctx, sr, srv, logDir, nil); err != nil {
		t.Fatalf("DestroyServer: %v", err)
	}

	if _, err := os.Stat(installDir); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("install dir still exists: %v", err)
	}
	if _, err := os.Stat(filepath.Join(logDir, "server-"+itoa(srv.ID)+".log")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("log file still exists: %v", err)
	}
	if _, err := sr.Get(ctx, srv.ID); !errors.Is(err, server.ErrNotFound) {
		t.Errorf("DB row not deleted: %v", err)
	}
}

func TestDestroyServer_MissingPathsAreIgnored(t *testing.T) {
	d := newTestDB(t)
	cr := NewRepo(d)
	sr := server.NewRepo(d)
	ctx := context.Background()
	c, _ := cr.Create(ctx, Cluster{Name: "c", ClusterID: "c", ClusterDir: ""})
	srv, err := sr.Create(ctx, server.Server{
		ClusterID: c.ID, Name: "Phantom", Map: "x",
		InstallDir: filepath.Join(t.TempDir(), "never-created"),
		Ports:      server.DefaultBase,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := DestroyServer(ctx, sr, srv, t.TempDir(), nil); err != nil {
		t.Errorf("expected nil for missing paths, got %v", err)
	}
}

func TestDestroyCluster_RemovesEverything(t *testing.T) {
	d := newTestDB(t)
	cr := NewRepo(d)
	sr := server.NewRepo(d)
	ctx := context.Background()

	root := t.TempDir()
	clusterDir := filepath.Join(root, "cluster-shared")
	logDir := filepath.Join(root, "logs")
	seedFile(t, filepath.Join(clusterDir, "savedark.bin"), "save")

	c, _ := cr.Create(ctx, Cluster{Name: "c", ClusterID: "c1", ClusterDir: clusterDir})
	srvA, _ := sr.Create(ctx, server.Server{
		ClusterID: c.ID, Name: "A", Map: "x",
		InstallDir: filepath.Join(root, "A"),
		Ports:      server.DefaultBase,
	})
	srvB, _ := sr.Create(ctx, server.Server{
		ClusterID: c.ID, Name: "B", Map: "x",
		InstallDir: filepath.Join(root, "B"),
		Ports:      server.PortTriple{Game: 7787, Query: 27025, RCON: 27030},
	})
	seedFile(t, filepath.Join(srvA.InstallDir, "binary.dat"), "x")
	seedFile(t, filepath.Join(srvB.InstallDir, "binary.dat"), "x")
	seedFile(t, filepath.Join(logDir, "server-"+itoa(srvA.ID)+".log"), "x")
	seedFile(t, filepath.Join(logDir, "server-"+itoa(srvB.ID)+".log"), "x")

	if err := DestroyCluster(ctx, cr, sr, c.ID, logDir, nil); err != nil {
		t.Fatalf("DestroyCluster: %v", err)
	}

	for _, p := range []string{
		clusterDir,
		srvA.InstallDir,
		srvB.InstallDir,
		filepath.Join(logDir, "server-"+itoa(srvA.ID)+".log"),
		filepath.Join(logDir, "server-"+itoa(srvB.ID)+".log"),
	} {
		if _, err := os.Stat(p); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("expected %q removed, got err=%v", p, err)
		}
	}
	if _, err := cr.Get(ctx, c.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("cluster row not deleted: %v", err)
	}
}

func itoa(n int64) string { return formatInt(n) }

// formatInt avoids importing strconv just for tests.
func formatInt(n int64) string {
	if n == 0 {
		return "0"
	}
	negative := n < 0
	if negative {
		n = -n
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if negative {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}
