package server

import (
	"context"
	"log/slog"
	"time"
)

type Coordinator struct {
	Sup     Supervisor
	Repo    *Repo
	Stagger time.Duration
	Log     *slog.Logger
}

func (c *Coordinator) StartCluster(ctx context.Context, clusterID int64, stagger time.Duration) error {
	return c.foreach(ctx, clusterID, "start", stagger, func(ctx context.Context, id int64) error {
		return c.Sup.Start(ctx, id)
	})
}

func (c *Coordinator) StopCluster(ctx context.Context, clusterID int64, graceful bool, stagger time.Duration) error {
	return c.foreach(ctx, clusterID, "stop", stagger, func(ctx context.Context, id int64) error {
		return c.Sup.Stop(ctx, id, graceful)
	})
}

func (c *Coordinator) RestartCluster(ctx context.Context, clusterID int64, stagger time.Duration) error {
	return c.foreach(ctx, clusterID, "restart", stagger, func(ctx context.Context, id int64) error {
		return c.Sup.Restart(ctx, id)
	})
}

func (c *Coordinator) foreach(ctx context.Context, clusterID int64, op string, stagger time.Duration, fn func(context.Context, int64) error) error {
	servers, err := c.Repo.ListByCluster(ctx, clusterID)
	if err != nil {
		return err
	}
	if stagger <= 0 {
		stagger = c.Stagger
	}
	var firstErr error
	for i, s := range servers {
		if i > 0 && stagger > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(stagger):
			}
		}
		if err := fn(ctx, s.ID); err != nil {
			if c.Log != nil {
				c.Log.Error("coordinator op failed", "op", op, "server_id", s.ID, "err", err)
			}
			if firstErr == nil {
				firstErr = err
			}
		}
	}
	return firstErr
}
