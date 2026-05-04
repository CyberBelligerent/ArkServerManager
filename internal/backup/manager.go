package backup

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"asamanager/internal/cluster"
	"asamanager/internal/events"
	"asamanager/internal/server"
)

// ErrBackupEmpty is returned by RestoreServer / RestoreCluster when
// the backup zip contained no restorable content (e.g., a cluster
// backup taken before ARK had ever populated the shared dir or any
// saves)
var ErrBackupEmpty = errors.New("backup contains no restorable content")

// ManagerDeps wires the manager to its collaborators.
type ManagerDeps struct {
	Repo     *Repo
	Servers  *server.Repo
	Clusters *cluster.Repo
	Bus      *events.Bus
	Log      *slog.Logger

	DestDir string		// Backup zip location
	KeepCount int		// Backups for server before rolling starts
}

type Manager struct {
	deps ManagerDeps
}

func NewManager(deps ManagerDeps) *Manager {
	if deps.KeepCount <= 0 {
		deps.KeepCount = 10
	}
	if deps.Log == nil {
		deps.Log = slog.Default()
	}
	return &Manager{deps: deps}
}

// BackupServer writes a server backup zip and records it in the DB.
// Emits BackupStarted/Completed/Failed on the bus
func (m *Manager) BackupServer(ctx context.Context, srv server.Server, kind string) (Backup, error) {
	scope := Scope{Type: "server", ID: srv.ID}
	m.publish(events.BackupStarted{Scope: scope.Type, ScopeID: scope.ID, At: time.Now()})

	dest := filepath.Join(m.deps.DestDir, "server-"+sanitizeArchiveName(srv.Name),
		fmt.Sprintf("server-%d-%s-%s.zip", srv.ID, sanitizeArchiveName(srv.Name), nowStamp()))
	size, err := WriteServerArchive(srv, dest)
	if err != nil {
		m.publishFail(scope, err)
		return Backup{}, err
	}
	saved, err := m.deps.Repo.Create(ctx, Backup{
		Scope: scope, Path: dest, SizeBytes: size, Kind: kindOrDefault(kind),
	})
	if err != nil {
		m.publishFail(scope, err)
		return Backup{}, err
	}
	m.publish(events.BackupCompleted{
		Scope: scope.Type, ScopeID: scope.ID, Path: dest, SizeBytes: size, At: time.Now(),
	})
	if err := m.Prune(ctx, scope); err != nil {
		m.deps.Log.Warn("prune backups", "scope_type", scope.Type, "scope_id", scope.ID, "err", err)
	}
	return saved, nil
}

// BackupCluster writes a cluster backup zip (cluster dir + every
// member server's saves+inis) and records it. Emits the same events
// as BackupServer.
func (m *Manager) BackupCluster(ctx context.Context, c cluster.Cluster, kind string) (Backup, error) {
	scope := Scope{Type: "cluster", ID: c.ID}
	m.publish(events.BackupStarted{Scope: scope.Type, ScopeID: scope.ID, At: time.Now()})

	members, err := m.deps.Servers.ListByCluster(ctx, c.ID)
	if err != nil {
		m.publishFail(scope, err)
		return Backup{}, err
	}
	if c.ClusterDir == "" || !dirExists(c.ClusterDir) {
		m.deps.Log.Warn("backup cluster: shared cluster dir does not exist on disk yet. Only per-server data will be archived",
			"cluster_id", c.ID, "cluster_dir", c.ClusterDir)
	}
	dest := filepath.Join(m.deps.DestDir, "cluster-"+sanitizeArchiveName(c.Name),
		fmt.Sprintf("cluster-%d-%s-%s.zip", c.ID, sanitizeArchiveName(c.Name), nowStamp()))
	size, err := WriteClusterArchive(c, members, dest)
	if err != nil {
		m.publishFail(scope, err)
		return Backup{}, err
	}
	saved, err := m.deps.Repo.Create(ctx, Backup{
		Scope: scope, Path: dest, SizeBytes: size, Kind: kindOrDefault(kind),
	})
	if err != nil {
		m.publishFail(scope, err)
		return Backup{}, err
	}
	m.publish(events.BackupCompleted{
		Scope: scope.Type, ScopeID: scope.ID, Path: dest, SizeBytes: size, At: time.Now(),
	})
	if err := m.Prune(ctx, scope); err != nil {
		m.deps.Log.Warn("prune backups", "scope_type", scope.Type, "scope_id", scope.ID, "err", err)
	}
	return saved, nil
}

// RestoreServer restores backup b into the given server's install dir
func (m *Manager) RestoreServer(ctx context.Context, b Backup, srv server.Server) error {
	if b.Scope.Type != "server" {
		return fmt.Errorf("backup #%d is not a server backup", b.ID)
	}
	dest := filepath.Join(srv.InstallDir, "ShooterGame", "Saved")
	if err := os.MkdirAll(dest, 0o755); err != nil {
		return err
	}
	count, err := restoreZipCounting(b.Path, dest)
	if err != nil {
		return fmt.Errorf("restore zip: %w", err)
	}
	if count == 0 {
		return ErrBackupEmpty
	}
	return nil
}

// RestoreCluster restores any restorable content in backup b: the
// shared cluster directory (if the zip has cluster/) plus each member
// server's saves+inis
// Returns ErrBackupEmpty if neither portion was present.
func (m *Manager) RestoreCluster(ctx context.Context, b Backup, c cluster.Cluster) error {
	if b.Scope.Type != "cluster" {
		return fmt.Errorf("backup #%d is not a cluster backup", b.ID)
	}
	tmp, err := os.MkdirTemp("", "asamanager-restore-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	if err := RestoreZip(b.Path, tmp); err != nil {
		return err
	}

	restored := false

	src := filepath.Join(tmp, "cluster")
	if dirExists(src) {
		if c.ClusterDir == "" {
			m.deps.Log.Warn("restore cluster: backup has cluster/ but cluster has no ClusterDir set", "cluster_id", c.ID)
		} else {
			if err := os.RemoveAll(c.ClusterDir); err != nil {
				return fmt.Errorf("clear existing cluster dir: %w", err)
			}
			if err := os.MkdirAll(filepath.Dir(c.ClusterDir), 0o755); err != nil {
				return err
			}
			if err := os.Rename(src, c.ClusterDir); err != nil {
				return fmt.Errorf("move cluster dir: %w", err)
			}
			restored = true
		}
	}

	serversRoot := filepath.Join(tmp, "servers")
	if dirExists(serversRoot) {
		members, err := m.deps.Servers.ListByCluster(ctx, c.ID)
		if err != nil {
			return err
		}
		for _, srv := range members {
			srvSrc := filepath.Join(serversRoot, sanitizeArchiveName(srv.Name))
			if !dirExists(srvSrc) {
				continue
			}
			dest := filepath.Join(srv.InstallDir, "ShooterGame", "Saved")
			if err := os.MkdirAll(dest, 0o755); err != nil {
				return err
			}
			if err := mergeDir(srvSrc, dest); err != nil {
				return fmt.Errorf("restore server %s: %w", srv.Name, err)
			}
			restored = true
		}
	}

	if !restored {
		return ErrBackupEmpty
	}
	return nil
}

func dirExists(path string) bool {
	if path == "" {
		return false
	}
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

// mergeDir copies every entry in srcDir into destDir, creating
// directories as needed. Existing files are overwritten
func mergeDir(srcDir, destDir string) error {
	return filepath.WalkDir(srcDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(srcDir, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		target := filepath.Join(destDir, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		return copyFile(path, target)
	})
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}

// restoreZipCounting wraps RestoreZip and returns how many file entries were extracted
func restoreZipCounting(srcZip, destDir string) (int, error) {
	if err := RestoreZip(srcZip, destDir); err != nil {
		return 0, err
	}
	count := 0
	_ = filepath.WalkDir(destDir, func(_ string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if !d.IsDir() {
			count++
		}
		return nil
	})
	return count, nil
}

// Prune deletes oldest backups a server or cluster until at most KeepCount remain on disk + in the DB.
func (m *Manager) Prune(ctx context.Context, scope Scope) error {
	all, err := m.deps.Repo.List(ctx, scope)
	if err != nil {
		return err
	}
	if len(all) <= m.deps.KeepCount {
		return nil
	}

	for _, old := range all[m.deps.KeepCount:] {
		if old.Path != "" {
			if err := os.Remove(old.Path); err != nil && !os.IsNotExist(err) {
				m.deps.Log.Warn("remove old backup file", "path", old.Path, "err", err)
			}
		}
		if err := m.deps.Repo.Delete(ctx, old.ID); err != nil {
			m.deps.Log.Warn("remove old backup row", "id", old.ID, "err", err)
		}
	}
	return nil
}

func (m *Manager) DeleteBackup(ctx context.Context, b Backup) error {
	if b.Path != "" {
		if err := os.Remove(b.Path); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return m.deps.Repo.Delete(ctx, b.ID)
}

func (m *Manager) publish(e events.Event) {
	if m.deps.Bus == nil {
		return
	}
	m.deps.Bus.Publish(e)
}

func (m *Manager) publishFail(scope Scope, err error) {
	m.publish(events.BackupFailed{
		Scope: scope.Type, ScopeID: scope.ID, Err: err.Error(), At: time.Now(),
	})
	m.deps.Log.Error("backup failed", "scope_type", scope.Type, "scope_id", scope.ID, "err", err)
}

func kindOrDefault(k string) string {
	if k == "" {
		return "manual"
	}
	return k
}

func nowStamp() string {
	return time.Now().Format("20060102-150405")
}
