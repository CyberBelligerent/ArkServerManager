package cluster

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"asamanager/internal/server"
)

// Both deletes remove from disk and DB. If disk fails, db is still usually fine. Just manually delte.
func DestroyServer(ctx context.Context, sr *server.Repo, s server.Server, logDir string, log *slog.Logger) error {
	removeServerFiles(s, logDir, log)
	return sr.Delete(ctx, s.ID)
}

func DestroyCluster(ctx context.Context, cr *Repo, sr *server.Repo, clusterID int64, logDir string, log *slog.Logger) error {
	c, err := cr.Get(ctx, clusterID)
	if err != nil {
		return err
	}
	servers, err := sr.ListByCluster(ctx, clusterID)
	if err != nil {
		return err
	}
	for _, s := range servers {
		removeServerFiles(s, logDir, log)
	}
	if c.ClusterDir != "" {
		if err := os.RemoveAll(c.ClusterDir); err != nil && log != nil {
			log.Error("remove cluster dir", "path", c.ClusterDir, "err", err)
		}
	}
	return cr.Delete(ctx, clusterID)
}

func removeServerFiles(s server.Server, logDir string, log *slog.Logger) {
	if s.InstallDir != "" {
		if err := os.RemoveAll(s.InstallDir); err != nil && log != nil {
			log.Error("remove install dir", "server_id", s.ID, "path", s.InstallDir, "err", err)
		}
	}
	if logDir != "" {
		logPath := filepath.Join(logDir, fmt.Sprintf("server-%d.log", s.ID))
		if err := os.RemoveAll(logPath); err != nil && log != nil {
			log.Error("remove server log", "server_id", s.ID, "path", logPath, "err", err)
		}
	}
}
