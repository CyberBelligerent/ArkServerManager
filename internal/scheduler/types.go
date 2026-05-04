package scheduler

import (
	"encoding/json"
	"time"
)

const (
	TriggerOneshot         = "oneshot"
	TriggerCron            = "cron"
	TriggerWindow          = "window"
	TriggerRecurringWindow = "recurring_window"
)

const (
	ActionStartServer    = "start_server"
	ActionStopServer     = "stop_server"
	ActionRestartServer  = "restart_server"
	ActionStartCluster   = "start_cluster"
	ActionStopCluster    = "stop_cluster"
	ActionRestartCluster = "restart_cluster"
	ActionBackup         = "backup"
	ActionRCONBroadcast  = "rcon_broadcast"
	ActionApplyPreset    = "apply_preset"
)

const (
	MissedSkip       = "skip"        // never run for past misses
	MissedRunOnce    = "run_once"    // fire once if any misses queued
	MissedApplyState = "apply_state" // for windows; apply current state regardless
)

const (
	StatusEnabled  = "enabled"
	StatusDisabled = "disabled"
)

const (
	RunResultSuccess = "success"
	RunResultError   = "error"
	RunResultSkipped = "skipped"
)

// Task is one row in scheduled_tasks. ScopeType is "global" for
// global-scoped tasks (e.g., a backup-everything cron), "cluster" for
// cluster-scoped, or "server" for per-server.
type Task struct {
	ID            int64
	Name          string
	ScopeType     string
	ScopeID       int64
	TriggerKind   string
	TriggerCron   string    // populated for TriggerCron
	StartAt       time.Time // populated for TriggerOneshot/Window
	EndAt         time.Time // populated for Window
	ActionKind    string
	ActionPayload json.RawMessage
	MissedPolicy  string
	Status        string
	LastFiredAt   *time.Time
	NextFireAt    *time.Time
	CreatedAt     time.Time
}

type Run struct {
	ID      int64
	TaskID  int64
	FiredAt time.Time
	Result  string
	Message string
}

// StartServerPayload identifies one server to start
type StartServerPayload struct {
	ServerID int64 `json:"server_id"`
}

// StopServerPayload identifies one server to stop. Graceful=true uses
// RCON to request a clean shutdown
type StopServerPayload struct {
	ServerID int64 `json:"server_id"`
	Graceful bool  `json:"graceful"`
}

// RestartServerPayload identifies one server to restart
type RestartServerPayload struct {
	ServerID            int64 `json:"server_id"`
	DrainWarningMinutes int   `json:"drain_warning_minutes,omitempty"`
}

// StartClusterPayload identifies a cluster to start. StaggerSeconds=0
type StartClusterPayload struct {
	ClusterID      int64 `json:"cluster_id"`
	StaggerSeconds int   `json:"stagger_seconds,omitempty"`
}

// StopClusterPayload identifies a cluster to stop. StaggerSeconds=0
type StopClusterPayload struct {
	ClusterID      int64 `json:"cluster_id"`
	Graceful       bool  `json:"graceful"`
	StaggerSeconds int   `json:"stagger_seconds,omitempty"`
}

// RestartClusterPayload identifies a cluster to restart with optional stagger between servers
type RestartClusterPayload struct {
	ClusterID      int64 `json:"cluster_id"`
	StaggerSeconds int   `json:"stagger_seconds,omitempty"`
}

// BackupPayload picks the backup scope
type BackupPayload struct {
	Scope   string `json:"scope"` // "server" or "cluster"
	ScopeID int64  `json:"scope_id"`
}

// RCONBroadcastPayload sends one message to a list of servers
type RCONBroadcastPayload struct {
	Message   string  `json:"message"`
	ServerIDs []int64 `json:"server_ids,omitempty"`
}

// ApplyPresetPayload references a saved preset and tells the engine whether to restart afterwards
type ApplyPresetPayload struct {
	PresetID            int64 `json:"preset_id"`
	Restart             bool  `json:"restart"`
	StaggerSeconds      int   `json:"stagger_seconds,omitempty"`       // for cluster restarts
	DrainWarningMinutes int   `json:"drain_warning_minutes,omitempty"` // reserved
}