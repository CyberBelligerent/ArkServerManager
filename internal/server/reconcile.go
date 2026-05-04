package server

import (
	"context"
	"log/slog"
)

// ReconcileOnStartup resets any non-terminal status to Stopped
func ReconcileOnStartup(ctx context.Context, r *Repo, log *slog.Logger) error {
	all, err := r.ListAll(ctx)
	if err != nil {
		return err
	}
	for _, s := range all {
		if isTerminalStatus(s.Status) {
			continue
		}
		if err := r.UpdateStatus(ctx, s.ID, StatusStopped); err != nil {
			return err
		}
		if log != nil {
			log.Info("reconciled stale server status",
				"server_id", s.ID, "name", s.Name,
				"previous", string(s.Status), "now", string(StatusStopped))
		}
	}
	return nil
}
