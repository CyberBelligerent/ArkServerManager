package gui

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"asamanager/internal/events"
)

const activityMaxLines = 200

// activityFeed is the bottom-pane log of bus events.
type activityFeed struct {
	app    *App
	body   *widget.TextGrid
	scroll *container.Scroll

	mu    sync.Mutex
	lines []string
	unsub func()
}

func newActivityFeed(app *App) *activityFeed {
	f := &activityFeed{app: app}
	f.body = widget.NewTextGrid()
	f.scroll = container.NewScroll(f.body)
	f.scroll.SetMinSize(fyne.NewSize(0, 120))
	f.unsub = app.deps.Bus.SubscribeAll(f.handle)
	return f
}

func (f *activityFeed) Container() fyne.CanvasObject {
	header := widget.NewLabelWithStyle(f.app.T("activity.header"), fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	return container.NewBorder(header, nil, nil, nil, withTextPaneBackground(f.scroll))
}

func (f *activityFeed) handle(e events.Event) {
	desc := formatEvent(f.app, e)
	if desc == "" {
		desc = e.EventName()
	}
	line := fmt.Sprintf("%s  %-20s  %s",
		time.Now().Local().Format("15:04:05"),
		e.EventName(),
		desc)

	f.mu.Lock()
	f.lines = append(f.lines, line)
	if len(f.lines) > activityMaxLines {
		f.lines = f.lines[len(f.lines)-activityMaxLines:]
	}
	snapshot := strings.Join(f.lines, "\n")
	f.mu.Unlock()

	fyne.Do(func() {
		f.body.SetText(snapshot)
		f.scroll.ScrollToBottom()
	})
}

func formatEvent(a *App, e events.Event) string {
	switch v := e.(type) {
	case events.ServerStarting:
		return a.T("activity.event.server_starting", v.ServerID, v.Name)
	case events.ServerStarted:
		return a.T("activity.event.server_started", v.ServerID, v.Name)
	case events.ServerStopped:
		return a.T("activity.event.server_stopped", v.ServerID, v.Name)
	case events.ServerCrashed:
		return a.T("activity.event.server_crashed", v.ServerID, v.Name, v.ExitCode)
	case events.ServerSaved:
		return a.T("activity.event.server_saved", v.ServerID, v.Name)
	case events.PlayerJoined:
		return a.T("activity.event.player_joined", v.ServerID, v.Name, v.SteamID)
	case events.PlayerLeft:
		return a.T("activity.event.player_left", v.ServerID, v.SteamID)
	case events.PlayerBanned:
		return a.T("activity.event.player_banned", v.ServerID, v.SteamID, v.Reason)
	case events.BackupCompleted:
		return a.T("activity.event.backup_completed", v.Path, v.SizeBytes)
	case events.BackupFailed:
		return a.T("activity.event.backup_failed", v.Err)
	case events.ServerBackupRestored:
		return a.T("activity.event.server_backup_restored", v.ServerID, v.Name, v.Path)
	case events.ClusterBackupRestored:
		return a.T("activity.event.cluster_backup_restored", v.ClusterID, v.Name, v.Path)
	case events.ServerCreated:
		return a.T("activity.event.server_created", v.ServerID, v.Name, v.Map)
	case events.ServerDeleted:
		return a.T("activity.event.server_deleted", v.ServerID, v.Name)
	case events.ServerInstallUpdate:
		return a.T("activity.event.server_install_started", v.ServerID, v.Name)
	case events.ClusterCreated:
		return a.T("activity.event.cluster_created", v.ClusterID, v.Name, v.ARKID)
	case events.ClusterDeleted:
		return a.T("activity.event.cluster_deleted", v.ClusterID, v.Name)
	case events.ClusterSettingsChanged:
		return a.T("activity.event.cluster_settings_changed", v.ClusterID, v.Name, v.Count)
	case events.ClusterSettingsSaved:
		return a.T("activity.event.cluster_settings_saved", v.ClusterID, v.Name, v.Count)
	case events.ClusterSettingsApplied:
		return a.T("activity.event.cluster_settings_applied", v.ClusterID, v.Name)
	case events.ClusterInstallUpdateAll:
		return a.T("activity.event.cluster_install_all", v.ClusterID, v.Name, v.Count)
	case events.ClusterInstallUpdateAllFinished:
		return a.T("activity.event.cluster_install_finished", v.ClusterID, v.Name, v.SuccessCount, v.FailedCount)
	case events.ServerInstallUpdateFinished:
		if v.Success {
			return a.T("activity.event.server_install_done", v.ServerID, v.Name)
		}
		return a.T("activity.event.server_install_failed", v.ServerID, v.Name, v.Err)
	case events.ServerSettingsChanged:
		return a.T("activity.event.server_settings_changed", v.ServerID, v.Name, v.Count)
	case events.ServerSettingsSaved:
		return a.T("activity.event.server_settings_saved", v.ServerID, v.Name, v.Count)
	default:
		return ""
	}
}
