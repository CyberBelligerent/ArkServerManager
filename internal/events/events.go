package events

import "time"

// Concrete event types. Add new ones here so subscribers have a single
// list to discover them. Each must implement Event.

type ServerStarting struct {
	ServerID int64
	Name     string
	At       time.Time
}

func (ServerStarting) EventName() string { return "server.starting" }

type ServerStarted struct {
	ServerID int64
	Name     string
	At       time.Time
}

func (ServerStarted) EventName() string { return "server.started" }

type ServerStopped struct {
	ServerID int64
	Name     string
	At       time.Time
}

func (ServerStopped) EventName() string { return "server.stopped" }

type ServerCrashed struct {
	ServerID int64
	Name     string
	ExitCode int
	At       time.Time
}

func (ServerCrashed) EventName() string { return "server.crashed" }

type ServerSaved struct {
	ServerID int64
	Name     string
	At       time.Time
}

func (ServerSaved) EventName() string { return "server.saved" }

type BackupStarted struct {
	Scope    string
	ScopeID  int64
	At       time.Time
}

func (BackupStarted) EventName() string { return "backup.started" }

type BackupCompleted struct {
	Scope    string
	ScopeID  int64
	Path     string
	SizeBytes int64
	At       time.Time
}

func (BackupCompleted) EventName() string { return "backup.completed" }

type BackupFailed struct {
	Scope    string
	ScopeID  int64
	Err      string
	At       time.Time
}

func (BackupFailed) EventName() string { return "backup.failed" }

type ServerBackupRestored struct {
	ServerID  int64
	ClusterID int64
	Name      string
	Path      string
	At        time.Time
}

func (ServerBackupRestored) EventName() string { return "server.backup_restored" }

type ClusterBackupRestored struct {
	ClusterID int64
	Name      string
	Path      string
	At        time.Time
}

func (ClusterBackupRestored) EventName() string { return "cluster.backup_restored" }

type PlayerJoined struct {
	ServerID int64
	SteamID  string
	Name     string
	At       time.Time
}

func (PlayerJoined) EventName() string { return "player.joined" }

type PlayerLeft struct {
	ServerID int64
	SteamID  string
	Name     string
	At       time.Time
}

func (PlayerLeft) EventName() string { return "player.left" }

type PlayerBanned struct {
	ServerID int64
	SteamID  string
	Reason   string
	At       time.Time
}

func (PlayerBanned) EventName() string { return "player.banned" }

type ScheduledTaskFired struct {
	TaskID   int64
	TaskName string
	Action   string
	At       time.Time
}

func (ScheduledTaskFired) EventName() string { return "schedule.fired" }

type PresetApplied struct {
	PresetID   int64
	PresetName string
	ScopeType  string // "cluster" or "server"
	ScopeID    int64
	ScopeName  string
}

func (PresetApplied) EventName() string { return "preset.applied" }

type ActiveEventApplied struct {
	Scope    string
	ScopeID  int64
	ARKEvent string
	At       time.Time
}

func (ActiveEventApplied) EventName() string { return "schedule.active_event_applied" }

type ActiveEventCleared struct {
	Scope    string
	ScopeID  int64
	At       time.Time
}

func (ActiveEventCleared) EventName() string { return "schedule.active_event_cleared" }

// RestartChurnWarning fires when two or more restart-causing scheduled
// actions on the same server fall within a 24-hour window
type RestartChurnWarning struct {
	ServerID int64
	Count    int
	WindowHours int
	At       time.Time
}

func (RestartChurnWarning) EventName() string { return "schedule.churn_warning" }

type ModUpdateAvailable struct {
	ServerID  int64
	ModID     int
	NewVersion string
	At        time.Time
}

func (ModUpdateAvailable) EventName() string { return "mod.update_available" }

// CRUD bound events that we pull manually

type ServerCreated struct {
	ServerID  int64
	ClusterID int64
	Name      string
	Map       string
	At        time.Time
}

func (ServerCreated) EventName() string { return "server.created" }

type ServerDeleted struct {
	ServerID int64
	Name     string
	At       time.Time
}

func (ServerDeleted) EventName() string { return "server.deleted" }

type ServerInstallUpdate struct {
	ServerID  int64
	ClusterID int64
	Name      string
	At        time.Time
}

func (ServerInstallUpdate) EventName() string { return "server.install_update" }

type ServerInstallUpdateFinished struct {
	ServerID  int64
	ClusterID int64
	Name      string
	Success   bool
	Err       string
	At        time.Time
}

func (ServerInstallUpdateFinished) EventName() string { return "server.install_update_finished" }

type ServerSettingsChanged struct {
	ServerID  int64
	ClusterID int64
	Name      string
	Count     int
	At        time.Time
}

func (ServerSettingsChanged) EventName() string { return "server.settings_changed" }

type ServerSettingsSaved struct {
	ServerID  int64
	ClusterID int64
	Name      string
	Count     int
	At        time.Time
}

func (ServerSettingsSaved) EventName() string { return "server.settings_saved" }

type ClusterCreated struct {
	ClusterID int64
	Name      string
	ARKID     string
	At        time.Time
}

func (ClusterCreated) EventName() string { return "cluster.created" }

type ClusterDeleted struct {
	ClusterID int64
	Name      string
	At        time.Time
}

func (ClusterDeleted) EventName() string { return "cluster.deleted" }

type ClusterSettingsChanged struct {
	ClusterID int64
	Name      string
	Count     int
	At        time.Time
}

func (ClusterSettingsChanged) EventName() string { return "cluster.settings_changed" }

type ClusterSettingsSaved struct {
	ClusterID int64
	Name      string
	Count     int
	At        time.Time
}

func (ClusterSettingsSaved) EventName() string { return "cluster.settings_saved" }

type ClusterSettingsApplied struct {
	ClusterID int64
	Name      string
	At        time.Time
}

func (ClusterSettingsApplied) EventName() string { return "cluster.settings_applied" }

type ClusterInstallUpdateAll struct {
	ClusterID int64
	Name      string
	Count     int
	At        time.Time
}

func (ClusterInstallUpdateAll) EventName() string { return "cluster.install_update_all" }

type ClusterInstallUpdateAllFinished struct {
	ClusterID    int64
	Name         string
	Count        int
	SuccessCount int
	FailedCount  int
	At           time.Time
}

func (ClusterInstallUpdateAllFinished) EventName() string {
	return "cluster.install_update_all_finished"
}
