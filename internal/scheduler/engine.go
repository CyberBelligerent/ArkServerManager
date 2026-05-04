package scheduler

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/robfig/cron/v3"

	"asamanager/internal/backup"
	"asamanager/internal/clock"
	"asamanager/internal/cluster"
	"asamanager/internal/events"
	"asamanager/internal/preset"
	"asamanager/internal/rcon"
	"asamanager/internal/server"
)

// EngineDeps wires the engine to its collaborators.
type EngineDeps struct {
	Repo          *Repo
	Supervisor    server.Supervisor
	Coordinator   *server.Coordinator
	Backups       *backup.Manager
	Presets       *preset.Repo
	PresetManager *preset.Manager
	ServerRepo    *server.Repo
	ClusterRepo   *cluster.Repo
	Bus           *events.Bus
	Log           *slog.Logger
	Clock         clock.Clock
	PollInterval  time.Duration // default 10s
}

// Runs next-fire times, polls, dispatches actions, persists run history.
type Engine struct {
	deps   EngineDeps
	parser cron.Parser

	mu      sync.Mutex
	stop    chan struct{}
	stopped bool
}

func NewEngine(deps EngineDeps) *Engine {
	if deps.Clock == nil {
		deps.Clock = clock.System()
	}
	if deps.PollInterval == 0 {
		deps.PollInterval = 10 * time.Second
	}
	if deps.Log == nil {
		deps.Log = slog.Default()
	}
	return &Engine{
		deps:   deps,
		parser: cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow),
	}
}

// Start runs reconciliation against current state, then begins the poll loop
func (e *Engine) Start(ctx context.Context) func() {
	e.mu.Lock()
	e.stop = make(chan struct{})
	e.stopped = false
	e.mu.Unlock()

	if err := e.reconcile(ctx); err != nil {
		e.deps.Log.Error("scheduler reconcile", "err", err)
	}

	done := make(chan struct{})
	go e.run(ctx, done)

	return func() {
		e.mu.Lock()
		if e.stopped {
			e.mu.Unlock()
			return
		}
		e.stopped = true
		close(e.stop)
		e.mu.Unlock()
		<-done
	}
}

func (e *Engine) run(ctx context.Context, done chan struct{}) {
	defer close(done)
	ticker := time.NewTicker(e.deps.PollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-e.stop:
			return
		case <-ticker.C:
			e.tick(ctx)
		}
	}
}

func (e *Engine) tick(ctx context.Context) {
	tasks, err := e.deps.Repo.ListEnabled(ctx)
	if err != nil {
		e.deps.Log.Error("list enabled tasks", "err", err)
		return
	}
	now := e.deps.Clock.Now()
	var due, future, nilNext int
	for _, t := range tasks {
		switch {
		case t.NextFireAt == nil:
			nilNext++
		case t.NextFireAt.After(now):
			future++
		default:
			due++
		}
	}
	e.deps.Log.Info("scheduler tick",
		"enabled", len(tasks), "due", due, "future", future, "no_next_fire", nilNext,
		"now_utc", now.UTC().Format(time.RFC3339))
	for _, t := range tasks {
		if t.NextFireAt == nil {
			e.deps.Log.Warn("scheduler tick: skip (no next_fire)",
				"task_id", t.ID, "name", t.Name,
				"trigger", t.TriggerKind, "start_at_utc", t.StartAt.UTC().Format(time.RFC3339))
			continue
		}
		if t.NextFireAt.After(now) {
			e.deps.Log.Debug("scheduler tick: future",
				"task_id", t.ID, "name", t.Name,
				"next_fire_utc", t.NextFireAt.UTC().Format(time.RFC3339))
			continue
		}
		e.deps.Log.Info("scheduler tick: due, firing",
			"task_id", t.ID, "name", t.Name,
			"next_fire_utc", t.NextFireAt.UTC().Format(time.RFC3339),
			"now_utc", now.UTC().Format(time.RFC3339))
		e.fireTask(ctx, t)
	}
}

// RunOnce manually triggers task id, regardless of its schedule
func (e *Engine) RunOnce(ctx context.Context, id int64) error {
	t, err := e.deps.Repo.Get(ctx, id)
	if err != nil {
		return err
	}
	e.fireTask(ctx, t)
	return nil
}

// PreviewNext returns the next fire time t would have starting from 'from'
func (e *Engine) PreviewNext(t Task, from time.Time) (*time.Time, error) {
	next, _, err := e.computeNextFire(t, from)
	return next, err
}

func (e *Engine) fireTask(ctx context.Context, t Task) {
	now := e.deps.Clock.Now()
	e.deps.Log.Info("firing scheduled task",
		"task_id", t.ID, "name", t.Name, "action", t.ActionKind)

	msg, err := e.executeAction(ctx, t)
	result := RunResultSuccess
	if err != nil {
		result = RunResultError
		msg = err.Error()
	}
	if rerr := e.deps.Repo.RecordRun(ctx, t.ID, now, result, msg); rerr != nil {
		e.deps.Log.Error("record run", "task_id", t.ID, "err", rerr)
	}
	if e.deps.Bus != nil {
		e.deps.Bus.Publish(events.ScheduledTaskFired{
			TaskID: t.ID, TaskName: t.Name, Action: t.ActionKind, At: now,
		})
	}

	next, terminal, err := e.computeNextFire(t, now)
	if err != nil {
		e.deps.Log.Warn("compute next fire", "task_id", t.ID, "err", err)
		terminal = true
	}
	fired := now
	if terminal {
		if err := e.deps.Repo.SetStatus(ctx, t.ID, StatusDisabled); err != nil {
			e.deps.Log.Error("disable oneshot", "task_id", t.ID, "err", err)
		}
		if err := e.deps.Repo.SetNextFire(ctx, t.ID, &fired, nil); err != nil {
			e.deps.Log.Error("clear next_fire after oneshot", "task_id", t.ID, "err", err)
		}
		return
	}
	if err := e.deps.Repo.SetNextFire(ctx, t.ID, &fired, next); err != nil {
		e.deps.Log.Error("set next fire", "task_id", t.ID, "err", err)
	}
}

// secondsDuration converts a positive seconds count into a Duration
func secondsDuration(s int) time.Duration {
	if s <= 0 {
		return 0
	}
	return time.Duration(s) * time.Second
}

// computeNextFire returns the next fire time strictly after `from`
func (e *Engine) computeNextFire(t Task, from time.Time) (*time.Time, bool, error) {
	switch t.TriggerKind {
	case TriggerOneshot:
		return nil, true, nil
	case TriggerCron:
		if t.TriggerCron == "" {
			return nil, true, fmt.Errorf("cron task with empty expression")
		}
		sched, err := e.parser.Parse(t.TriggerCron)
		if err != nil {
			return nil, true, fmt.Errorf("parse cron %q: %w", t.TriggerCron, err)
		}
		n := sched.Next(from)
		return &n, false, nil
	}
	return nil, true, fmt.Errorf("unsupported trigger kind %q", t.TriggerKind)
}

// reconcile computes initial NextFireAt for every enabled task. For
// oneshots whose StartAt has already passed, the missed-fire policy
// decides whether to fire-once or skip
func (e *Engine) reconcile(ctx context.Context) error {
	now := e.deps.Clock.Now()
	tasks, err := e.deps.Repo.ListEnabled(ctx)
	if err != nil {
		return err
	}
	for _, t := range tasks {
		switch t.TriggerKind {
		case TriggerOneshot:
			if t.StartAt.IsZero() {
				e.deps.Log.Warn("reconcile: oneshot with no StartAt", "task_id", t.ID, "name", t.Name)
				continue
			}
			e.deps.Log.Info("reconcile oneshot",
				"task_id", t.ID, "name", t.Name,
				"start_utc", t.StartAt.UTC().Format(time.RFC3339),
				"now_utc", now.UTC().Format(time.RFC3339),
				"in_future", t.StartAt.After(now),
				"missed_policy", t.MissedPolicy)
			if t.StartAt.After(now) {
				start := t.StartAt
				if err := e.deps.Repo.SetNextFire(ctx, t.ID, t.LastFiredAt, &start); err != nil {
					e.deps.Log.Error("reconcile: set next_fire", "task_id", t.ID, "err", err)
				}
				continue
			}
			// In the past.
			if t.MissedPolicy == MissedRunOnce {
				start := t.StartAt
				if err := e.deps.Repo.SetNextFire(ctx, t.ID, t.LastFiredAt, &start); err != nil {
					e.deps.Log.Error("reconcile: set next_fire (run_once)", "task_id", t.ID, "err", err)
				}
				continue
			}
			if err := e.deps.Repo.SetStatus(ctx, t.ID, StatusDisabled); err != nil {
				e.deps.Log.Error("reconcile: disable missed oneshot", "task_id", t.ID, "err", err)
			}
			if err := e.deps.Repo.RecordRun(ctx, t.ID, now, RunResultSkipped, "missed oneshot, policy=skip"); err != nil {
				e.deps.Log.Error("reconcile: record skipped run", "task_id", t.ID, "err", err)
			}
		case TriggerCron:
			if t.TriggerCron == "" {
				continue
			}
			sched, err := e.parser.Parse(t.TriggerCron)
			if err != nil {
				e.deps.Log.Warn("reconcile: bad cron expression",
					"task_id", t.ID, "expr", t.TriggerCron, "err", err)
				continue
			}
			n := sched.Next(now)
			if err := e.deps.Repo.SetNextFire(ctx, t.ID, t.LastFiredAt, &n); err != nil {
				e.deps.Log.Error("reconcile: set next_fire (cron)", "task_id", t.ID, "err", err)
			}
		}
	}
	return nil
}

// executeAction dispatches by t.ActionKind to the right handler.
func (e *Engine) executeAction(ctx context.Context, t Task) (string, error) {
	switch t.ActionKind {
	case ActionStartServer:
		var p StartServerPayload
		if err := json.Unmarshal(t.ActionPayload, &p); err != nil {
			return "", fmt.Errorf("parse payload: %w", err)
		}
		if err := e.deps.Supervisor.Start(ctx, p.ServerID); err != nil {
			return "", err
		}
		return fmt.Sprintf("started server #%d", p.ServerID), nil

	case ActionStopServer:
		var p StopServerPayload
		if err := json.Unmarshal(t.ActionPayload, &p); err != nil {
			return "", fmt.Errorf("parse payload: %w", err)
		}
		if err := e.deps.Supervisor.Stop(ctx, p.ServerID, p.Graceful); err != nil {
			return "", err
		}
		mode := "graceful"
		if !p.Graceful {
			mode = "hard"
		}
		return fmt.Sprintf("stopped server #%d (%s)", p.ServerID, mode), nil

	case ActionStartCluster:
		var p StartClusterPayload
		if err := json.Unmarshal(t.ActionPayload, &p); err != nil {
			return "", fmt.Errorf("parse payload: %w", err)
		}
		if err := e.deps.Coordinator.StartCluster(ctx, p.ClusterID, secondsDuration(p.StaggerSeconds)); err != nil {
			return "", err
		}
		return fmt.Sprintf("started cluster #%d", p.ClusterID), nil

	case ActionStopCluster:
		var p StopClusterPayload
		if err := json.Unmarshal(t.ActionPayload, &p); err != nil {
			return "", fmt.Errorf("parse payload: %w", err)
		}
		if err := e.deps.Coordinator.StopCluster(ctx, p.ClusterID, p.Graceful, secondsDuration(p.StaggerSeconds)); err != nil {
			return "", err
		}
		mode := "graceful"
		if !p.Graceful {
			mode = "hard"
		}
		return fmt.Sprintf("stopped cluster #%d (%s)", p.ClusterID, mode), nil

	case ActionRestartServer:
		var p RestartServerPayload
		if err := json.Unmarshal(t.ActionPayload, &p); err != nil {
			return "", fmt.Errorf("parse payload: %w", err)
		}
		if err := e.deps.Supervisor.Restart(ctx, p.ServerID); err != nil {
			return "", err
		}
		return fmt.Sprintf("restarted server #%d", p.ServerID), nil

	case ActionRestartCluster:
		var p RestartClusterPayload
		if err := json.Unmarshal(t.ActionPayload, &p); err != nil {
			return "", fmt.Errorf("parse payload: %w", err)
		}
		if err := e.deps.Coordinator.RestartCluster(ctx, p.ClusterID, secondsDuration(p.StaggerSeconds)); err != nil {
			return "", err
		}
		return fmt.Sprintf("restarted cluster #%d", p.ClusterID), nil

	case ActionBackup:
		var p BackupPayload
		if err := json.Unmarshal(t.ActionPayload, &p); err != nil {
			return "", fmt.Errorf("parse payload: %w", err)
		}
		switch p.Scope {
		case "server":
			s, err := e.deps.ServerRepo.Get(ctx, p.ScopeID)
			if err != nil {
				return "", fmt.Errorf("load server #%d: %w", p.ScopeID, err)
			}
			if _, err := e.deps.Backups.BackupServer(ctx, s, "scheduled"); err != nil {
				return "", err
			}
			return fmt.Sprintf("backup server #%d", p.ScopeID), nil
		case "cluster":
			c, err := e.deps.ClusterRepo.Get(ctx, p.ScopeID)
			if err != nil {
				return "", fmt.Errorf("load cluster #%d: %w", p.ScopeID, err)
			}
			if _, err := e.deps.Backups.BackupCluster(ctx, c, "scheduled"); err != nil {
				return "", err
			}
			return fmt.Sprintf("backup cluster #%d", p.ScopeID), nil
		}
		return "", fmt.Errorf("unknown backup scope %q", p.Scope)

	case ActionApplyPreset:
		var p ApplyPresetPayload
		if err := json.Unmarshal(t.ActionPayload, &p); err != nil {
			return "", fmt.Errorf("parse payload: %w", err)
		}
		if e.deps.PresetManager == nil {
			return "", fmt.Errorf("apply_preset action requires PresetManager dep")
		}
		// Pre-fetch the preset so we can validate scope before any state-changing operation
		preFetch, err := e.deps.Presets.Get(ctx, p.PresetID)
		if err != nil {
			return "", fmt.Errorf("load preset #%d: %w", p.PresetID, err)
		}
		if t.ScopeType != "global" && preFetch.ScopeType != t.ScopeType {
			return "", fmt.Errorf("preset scope mismatch: task is %s, preset is %s",
				t.ScopeType, preFetch.ScopeType)
		}
		ps, msg, err := e.deps.PresetManager.ApplyByID(ctx, p.PresetID)
		if err != nil {
			return "", err
		}
		if !p.Restart {
			return msg, nil
		}
		switch ps.ScopeType {
		case "cluster":
			if err := e.deps.Coordinator.RestartCluster(ctx, ps.ScopeID, secondsDuration(p.StaggerSeconds)); err != nil {
				return msg, fmt.Errorf("preset applied but cluster restart failed: %w", err)
			}
			return msg + "; cluster restarted", nil
		case "server":
			if err := e.deps.Supervisor.Restart(ctx, ps.ScopeID); err != nil {
				return msg, fmt.Errorf("preset applied but server restart failed: %w", err)
			}
			return msg + "; server restarted", nil
		}
		return msg, nil

	case ActionRCONBroadcast:
		var p RCONBroadcastPayload
		if err := json.Unmarshal(t.ActionPayload, &p); err != nil {
			return "", fmt.Errorf("parse payload: %w", err)
		}
		targets := p.ServerIDs
		if len(targets) == 0 {
			// All servers across all clusters.
			all, err := e.deps.ServerRepo.ListAll(ctx)
			if err != nil {
				return "", err
			}
			for _, s := range all {
				targets = append(targets, s.ID)
			}
		}
		var sent, skipped int
		for _, id := range targets {
			c := e.deps.Supervisor.RCONFor(id)
			if c == nil {
				skipped++
				continue
			}
			if _, err := c.Exec(ctx, rcon.CmdBroadcast+" "+p.Message); err != nil {
				e.deps.Log.Warn("scheduled broadcast", "server_id", id, "err", err)
				skipped++
				continue
			}
			sent++
		}
		return fmt.Sprintf("broadcast: %d sent, %d skipped", sent, skipped), nil
	}
	return "", fmt.Errorf("unknown action %q", t.ActionKind)
}
