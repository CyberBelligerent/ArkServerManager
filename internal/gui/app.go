package gui

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"log/slog"
	"os"

	"fyne.io/fyne/v2"
	fyneapp "fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"asamanager/internal/asb"
	"asamanager/internal/backup"
	"asamanager/internal/cluster"
	"asamanager/internal/config"
	"asamanager/internal/discord/webhook"
	"asamanager/internal/events"
	"asamanager/internal/i18n"
	"asamanager/internal/players"
	"asamanager/internal/preset"
	"asamanager/internal/scheduler"
	"asamanager/internal/server"
	"asamanager/internal/steamcmd"
)

// statusEventNames lists every bus event that should trigger a tree + detail-tab refresh
var statusEventNames = []string{
	events.ServerStarting{}.EventName(),
	events.ServerStarted{}.EventName(),
	events.ServerStopped{}.EventName(),
	events.ServerCrashed{}.EventName(),
}

// Deps groups everything the GUI needs from main.
type Deps struct {
	Config     config.Config
	SaveConfig func(config.Config) error
	DataDir    string

	DB  *sql.DB
	Bus *events.Bus
	Log *slog.Logger

	Clusters *cluster.Repo
	Servers  *server.Repo
	Players  *players.Repo

	Supervisor  server.Supervisor
	Coordinator *server.Coordinator
	Tracker     *players.Tracker
	Actions     *players.Actions
	SteamCMD    *steamcmd.RealRunner

	Webhooks          *webhook.Repo
	WebhookDispatcher *webhook.Dispatcher

	BackupRepo    *backup.Repo
	BackupManager *backup.Manager

	SchedulerRepo   *scheduler.Repo
	SchedulerEngine *scheduler.Engine

	Presets       *preset.Repo
	PresetManager *preset.Manager

	// ASBDir is where ARK Smart Breeding Multipliers.ini files are
	ASBDir string

	// LogCloser is the slog file handle
	LogCloser io.Closer

	// Bundle is the loaded i18n bundle for the active locale
	Bundle *i18n.Bundle

	// LanguageDir is the absolute path to the folder where translator .toml files live
	LanguageDir string
}

type App struct {
	deps Deps

	fyne   fyne.App
	window fyne.Window

	tree     *clusterTree
	tabs     *detailTabs
	activity *activityFeed
}

func New(deps Deps) *App {
	a := &App{deps: deps}
	a.fyne = fyneapp.NewWithID("dev.asamanager")
	a.window = a.fyne.NewWindow(deps.Bundle.T("window.main_title"))
	a.window.Resize(fyne.NewSize(1200, 800))
	return a
}

func (a *App) T(key string, args ...any) string {
	return a.deps.Bundle.T(key, args...)
}

// Run blocks until the application's last window closes. On first run
// the wizard is the only window shown
func (a *App) Run() {
	a.deps.Log.Info("gui starting", "first_run", !a.deps.Config.FirstRunDone)
	if !a.deps.Config.FirstRunDone {
		a.runWizard(func() {
			a.buildMainShell()
			a.window.Show()
		})
		a.fyne.Run()
	} else {
		a.buildMainShell()
		a.window.ShowAndRun()
	}
	a.deps.Log.Info("gui exited")
}

// buildMainShell wires the tree + tabs + activity feed into the window
func (a *App) buildMainShell() {
	a.tree = newClusterTree(a)
	a.tabs = newDetailTabs(a)
	a.activity = newActivityFeed(a)

	for _, name := range statusEventNames {
		a.deps.Bus.Subscribe(name, a.onServerStateEvent)
	}

	toolbar := a.buildToolbar()

	split := container.NewHSplit(
		container.NewVScroll(a.tree.Container()),
		a.tabs.Container(),
	)
	split.SetOffset(0.28)

	main := container.NewBorder(
		toolbar,
		a.activity.Container(),
		nil, nil,
		split,
	)
	a.window.SetContent(main)

	a.tree.Refresh()
}

func (a *App) buildToolbar() *fyne.Container {
	addCluster := widget.NewButtonWithIcon(a.T("toolbar.new_cluster"), theme.ContentAddIcon(), func() {
		a.showNewClusterDialog()
	})
	addServer := widget.NewButtonWithIcon(a.T("toolbar.new_server"), theme.DocumentCreateIcon(), func() {
		a.showNewServerDialog(nil)
	})
	wizardBtn := widget.NewButtonWithIcon(a.T("toolbar.run_wizard"), theme.SettingsIcon(), func() {
		a.runWizard(func() { a.refreshTree() })
	})
	webhooksBtn := widget.NewButtonWithIcon(a.T("toolbar.webhooks"), theme.MailComposeIcon(), func() {
		a.showWebhooksManager()
	})
	asbBtn := widget.NewButtonWithIcon(a.T("toolbar.asb_folder"), theme.FolderIcon(), func() {
		if a.deps.ASBDir == "" {
			a.showError(fmt.Errorf("%s", a.T("toolbar.err_no_asb")))
			return
		}

		if err := os.MkdirAll(a.deps.ASBDir, 0o755); err != nil {
			a.showError(err)
			return
		}
		if err := openInFileExplorer(a.deps.ASBDir); err != nil {
			a.showError(err)
		}
	})
	prefsBtn := widget.NewButtonWithIcon(a.T("toolbar.preferences"), theme.SettingsIcon(), func() {
		a.showAppPreferencesDialog()
	})
	uninstallBtn := widget.NewButtonWithIcon(a.T("toolbar.uninstall"), theme.DeleteIcon(), func() {
		a.showUninstallDialog()
	})
	uninstallBtn.Importance = widget.DangerImportance
	return container.NewHBox(addCluster, addServer, webhooksBtn, asbBtn, wizardBtn, prefsBtn, uninstallBtn)
}

// refreshTree reloads the cluster/server data from the DB and redraws.
func (a *App) refreshTree() {
	if a.tree != nil {
		a.tree.Refresh()
	}
}

// onServerStateEvent is the bus subscriber that pulls the latest
// server row from the DB and refreshes the tree + currently-shown
// detail tabs whenever a lifecycle event fires.
func (a *App) onServerStateEvent(e events.Event) {
	var serverID int64
	switch v := e.(type) {
	case events.ServerStarting:
		serverID = v.ServerID
	case events.ServerStarted:
		serverID = v.ServerID
	case events.ServerStopped:
		serverID = v.ServerID
	case events.ServerCrashed:
		serverID = v.ServerID
	default:
		return
	}
	fyne.Do(func() {
		a.refreshTree()
		if a.tabs != nil {
			a.tabs.RefreshIfShowing(serverID)
		}
	})
}

// ctx returns a fresh background context
func (a *App) ctx() context.Context { return context.Background() }

// publish forwards e onto the bus when one is wired. Safe to call from any goroutine.
func (a *App) publish(e events.Event) {
	if a.deps.Bus == nil {
		return
	}
	a.deps.Bus.Publish(e)
}

func (a *App) exportASBCluster(c cluster.Cluster) {
	if a.deps.ASBDir == "" {
		return
	}
	if _, err := asb.ExportCluster(c, a.deps.ASBDir); err != nil {
		a.deps.Log.Warn("export ASB cluster file", "cluster_id", c.ID, "err", err)
	}
}

func (a *App) exportASBServer(c cluster.Cluster, s server.Server) {
	if a.deps.ASBDir == "" {
		return
	}
	if _, err := asb.ExportServer(c, s, a.deps.ASBDir); err != nil {
		a.deps.Log.Warn("export ASB server file", "server_id", s.ID, "err", err)
	}
}

func (a *App) exportASBClusterAndAllServers(c cluster.Cluster) {
	a.exportASBCluster(c)
	if a.deps.Servers == nil {
		return
	}
	servers, err := a.deps.Servers.ListByCluster(a.ctx(), c.ID)
	if err != nil {
		a.deps.Log.Warn("list servers for ASB re-export", "cluster_id", c.ID, "err", err)
		return
	}
	for _, s := range servers {
		a.exportASBServer(c, s)
	}
}
