package gui

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"asamanager/internal/asb"
	"asamanager/internal/cluster"
	"asamanager/internal/events"
	"asamanager/internal/server"
	"asamanager/internal/settings"
)

type detailTabs struct {
	app       *App
	container *fyne.Container

	tabs        *container.AppTabs
	placeholder fyne.CanvasObject

	mu       sync.Mutex
	selCl    *cluster.Cluster
	selSrv   *server.Server
	logsStop func()
}

func newDetailTabs(app *App) *detailTabs {
	t := &detailTabs{app: app}
	t.placeholder = container.NewCenter(widget.NewLabelWithStyle(
		app.T("window.placeholder_select"),
		fyne.TextAlignCenter, fyne.TextStyle{Italic: true}))
	t.container = container.NewStack(t.placeholder)
	return t
}

func (t *detailTabs) Container() fyne.CanvasObject { return t.container }

// Clear restores the placeholder pane
func (t *detailTabs) Clear() {
	t.cancelLogs()
	t.mu.Lock()
	t.selCl = nil
	t.selSrv = nil
	t.mu.Unlock()
	t.container.Objects = []fyne.CanvasObject{t.placeholder}
	t.container.Refresh()
}

// RefreshIfShowing re-fetches and re-renders the current selection if it matches serverID
func (t *detailTabs) RefreshIfShowing(serverID int64) {
	t.mu.Lock()
	sel := t.selSrv
	cl := t.selCl
	t.mu.Unlock()
	if sel == nil || cl == nil || sel.ID != serverID {
		return
	}
	fresh, err := t.app.deps.Servers.Get(t.app.ctx(), serverID)
	if err != nil {
		t.app.deps.Log.Error("refresh server", "server_id", serverID, "err", err)
		return
	}
	t.ShowServer(*cl, fresh)
}

func (t *detailTabs) ShowCluster(c cluster.Cluster) {
	t.cancelLogs()
	t.mu.Lock()
	t.selCl = &c
	t.selSrv = nil
	t.mu.Unlock()

	t.tabs = container.NewAppTabs(
		container.NewTabItem(t.app.T("cluster.tabs.overview"), t.clusterOverview(c)),
		container.NewTabItem(t.app.T("cluster.tabs.settings"), t.clusterSettings(c)),
		container.NewTabItem(t.app.T("cluster.tabs.schedule"), t.scheduleTab(scheduleScope{
			Type: "cluster", ID: c.ID, DisplayName: c.Name,
		})),
	)
	t.container.Objects = []fyne.CanvasObject{t.tabs}
	t.container.Refresh()
}

func (t *detailTabs) ShowServer(c cluster.Cluster, s server.Server) {
	t.cancelLogs()
	t.mu.Lock()
	t.selCl = &c
	t.selSrv = &s
	t.mu.Unlock()

	logsContent, stopLogs := t.serverLogs(s)
	t.mu.Lock()
	t.logsStop = stopLogs
	t.mu.Unlock()

	t.tabs = container.NewAppTabs(
		container.NewTabItem(t.app.T("server.tabs.overview"), t.serverOverview(c, s)),
		container.NewTabItem(t.app.T("server.tabs.settings"), t.serverSettings(c, s)),
		container.NewTabItem(t.app.T("server.tabs.players"), t.serverPlayers(s)),
		container.NewTabItem(t.app.T("server.tabs.logs"), logsContent),
		container.NewTabItem(t.app.T("server.tabs.schedule"), t.scheduleTab(scheduleScope{
			Type: "server", ID: s.ID, DisplayName: s.Name,
		})),
	)
	t.container.Objects = []fyne.CanvasObject{t.tabs}
	t.container.Refresh()
}

func (t *detailTabs) cancelLogs() {
	t.mu.Lock()
	stop := t.logsStop
	t.logsStop = nil
	t.mu.Unlock()
	if stop != nil {
		stop()
	}
}

func (t *detailTabs) clusterOverview(c cluster.Cluster) fyne.CanvasObject {
	title := widget.NewLabelWithStyle(c.Name, fyne.TextAlignLeading, fyne.TextStyle{Bold: true})

	addServer := widget.NewButton(t.app.T("cluster.overview.button_add_server"), func() {
		t.app.showNewServerDialog(&c)
	})
	installAll := widget.NewButton(t.app.T("cluster.overview.button_install_all"), func() {
		servers, err := t.app.deps.Servers.ListByCluster(t.app.ctx(), c.ID)
		if err != nil {
			t.app.showError(err)
			return
		}
		for _, srv := range servers {
			st := t.app.deps.Supervisor.Status(srv.ID)
			if st != server.StatusStopped && st != server.StatusCrashed {
				t.app.showError(fmt.Errorf(t.app.T("cluster.overview.err_busy_for_install"), srv.Name, st))
				return
			}
		}
		t.app.publish(events.ClusterInstallUpdateAll{
			ClusterID: c.ID, Name: c.Name, Count: len(servers), At: time.Now(),
		})
		clusterCopy := c
		t.app.runInstall(installPlan{servers: servers, clusterCtx: &clusterCopy})
	})
	startAll := widget.NewButton(t.app.T("cluster.overview.button_start_all"), func() {
		go t.run(func() error { return t.app.deps.Coordinator.StartCluster(t.app.ctx(), c.ID, 0) })
	})
	stopAll := widget.NewButton(t.app.T("cluster.overview.button_stop_all"), func() {
		t.app.confirm(t.app.T("cluster.overview.confirm_stop_all_title"), t.app.T("cluster.overview.confirm_stop_all_body"), func() {
			go t.run(func() error { return t.app.deps.Coordinator.StopCluster(t.app.ctx(), c.ID, true, 0) })
		})
	})
	restartAll := widget.NewButton(t.app.T("cluster.overview.button_restart_all"), func() {
		t.app.confirm(t.app.T("cluster.overview.confirm_restart_all_title"), t.app.T("cluster.overview.confirm_restart_all_body"), func() {
			go t.run(func() error { return t.app.deps.Coordinator.RestartCluster(t.app.ctx(), c.ID, 0) })
		})
	})
	backupNow := widget.NewButton(t.app.T("cluster.overview.button_backup_now"), func() {
		go func() {
			if _, err := t.app.deps.BackupManager.BackupCluster(t.app.ctx(), c, "manual"); err != nil {
				fyne.Do(func() { t.app.showError(err) })
				return
			}
			fyne.Do(func() { t.app.showInfo(t.app.T("cluster.overview.backup_done_title"), t.app.T("cluster.overview.backup_done_body")) })
		}()
	})
	manageBackups := widget.NewButton(t.app.T("cluster.overview.button_manage_backups"), func() {
		t.app.showClusterBackups(c)
	})
	openFolder := widget.NewButton(t.app.T("cluster.overview.button_open_explorer"), func() {
		if c.ClusterDir == "" {
			t.app.showError(fmt.Errorf("%s", t.app.T("cluster.overview.err_no_dir")))
			return
		}
		if _, err := os.Stat(c.ClusterDir); err == nil {
			if err := openInFileExplorer(c.ClusterDir); err != nil {
				t.app.showError(err)
			}
			return
		}
		t.app.confirm(
			t.app.T("cluster.overview.confirm_dir_create_title"),
			t.app.T("cluster.overview.confirm_dir_create_body", c.ClusterDir),
			func() {
				if err := os.MkdirAll(c.ClusterDir, 0o755); err != nil {
					t.app.showError(err)
					return
				}
				if err := openInFileExplorer(c.ClusterDir); err != nil {
					t.app.showError(err)
				}
			})
	})
	propagate := widget.NewButton(t.app.T("cluster.overview.button_propagate"), func() {
		t.app.confirm(t.app.T("cluster.overview.confirm_apply_title"),
			t.app.T("cluster.overview.confirm_apply_body"),
			func() {
				go func() {
					if err := t.app.deps.Clusters.PropagateToServers(t.app.ctx(), t.app.deps.Servers, c.ID); err != nil {
						t.app.showError(err)
						return
					}
					t.app.publish(events.ClusterSettingsApplied{ClusterID: c.ID, Name: c.Name, At: time.Now()})
					fyne.Do(func() { t.app.showInfo(t.app.T("cluster.overview.apply_done_title"), t.app.T("cluster.overview.apply_done_body")) })
				}()
			})
	})
	deleteBtn := widget.NewButton(t.app.T("cluster.overview.button_delete"), func() {
		servers, err := t.app.deps.Servers.ListByCluster(t.app.ctx(), c.ID)
		if err != nil {
			t.app.showError(err)
			return
		}
		for _, srv := range servers {
			st := t.app.deps.Supervisor.Status(srv.ID)
			if st != server.StatusStopped && st != server.StatusCrashed {
				t.app.showError(fmt.Errorf(t.app.T("cluster.overview.err_busy_for_delete"), srv.Name, st))
				return
			}
		}
		var paths []string
		for _, srv := range servers {
			paths = append(paths, "    • "+srv.InstallDir)
		}
		listed := strings.Join(paths, "\n")
		if listed == "" {
			listed = t.app.T("cluster.overview.delete_no_servers")
		}
		msg := t.app.T("cluster.overview.confirm_delete_body", c.Name, len(servers), c.ClusterDir, listed)
		t.app.confirm(t.app.T("cluster.overview.confirm_delete_title"), msg, func() {
			logDir := filepath.Join(t.app.deps.DataDir, "logs")
			if err := cluster.DestroyCluster(t.app.ctx(), t.app.deps.Clusters, t.app.deps.Servers, c.ID, logDir, t.app.deps.Log); err != nil {
				t.app.showError(err)
				return
			}
			if t.app.deps.ASBDir != "" {
				if err := asb.DeleteForCluster(c, t.app.deps.ASBDir); err != nil {
					t.app.deps.Log.Warn("delete ASB cluster file", "cluster_id", c.ID, "err", err)
				}

				for _, srv := range servers {
					if err := asb.DeleteForServer(srv, t.app.deps.ASBDir); err != nil {
						t.app.deps.Log.Warn("delete ASB server file", "server_id", srv.ID, "err", err)
					}
				}
			}
			t.app.publish(events.ClusterDeleted{ClusterID: c.ID, Name: c.Name, At: time.Now()})
			t.app.refreshTree()
			t.Clear()
		})
	})

	rows := container.NewVBox(
		formatRowItem(t.app.T("cluster.overview.row_cluster_id"), c.ClusterID),
		formatRowItem(t.app.T("cluster.overview.row_cluster_dir"), c.ClusterDir),
		formatRowItem(t.app.T("cluster.overview.row_settings_overridden"), t.app.T("cluster.overview.row_settings_count", len(c.Settings))),
	)

	return container.NewVScroll(container.NewVBox(
		title,
		widget.NewSeparator(),
		rows,
		widget.NewSeparator(),
		container.NewHBox(addServer, installAll, startAll, stopAll, restartAll),
		propagate,
		container.NewHBox(backupNow, manageBackups, openFolder),
		widget.NewSeparator(),
		deleteBtn,
	))
}

func (t *detailTabs) clusterSettings(c cluster.Cluster) fyne.CanvasObject {
	reg := settings.Default()
	var editors []settingEditorEntry

	clusterEventLabels, clusterEventValues := buildClusterEventOptions(t.app)
	clusterEventSel := widget.NewSelect(clusterEventLabels, nil)
	clusterEventSel.SetSelected(displayLabelForEvent(c.ActiveEvent, clusterEventLabels, clusterEventValues))

	clusterModsEntry := widget.NewEntry()
	clusterModsEntry.SetPlaceHolder(t.app.T("newserver.field_mods_placeholder"))
	clusterModsEntry.SetText(formatModIDs(c.ActiveMods))

	catTabs := container.NewAppTabs()
	for _, group := range reg.ByCategory() {
		if settings.IsPerStatCategory(group.Name) {
			continue
		}
		form := &widget.Form{}
		for _, sp := range group.Settings {
			cur, ok := c.Settings[sp.Key]
			if !ok || cur == (settings.Value{}) {
				cur = sp.DefaultValue()
			}
			ed := buildEditorFor(sp, cur)
			editors = append(editors, settingEditorEntry{setting: sp, read: ed.read})
			form.Items = append(form.Items, &widget.FormItem{
				Text:     sp.Key,
				Widget:   ed.widget,
				HintText: sp.Tooltip,
			})
		}
		catTabs.Append(container.NewTabItem(group.Name, container.NewVScroll(form)))
	}

	perStatTab, perStatEditors := buildPerStatGrid(t.app, c.Settings, reg)
	editors = append(editors, perStatEditors...)
	catTabs.Append(container.NewTabItem(t.app.T("cluster.settings.per_stat_tab"), perStatTab))

	workingLines := append([]cluster.CustomINIEntry(nil), c.CustomLines...)
	linesBox := container.NewVBox()
	var rebuildLines func()
	rebuildLines = func() {
		linesBox.Objects = nil
		if len(workingLines) == 0 {
			linesBox.Add(widget.NewLabelWithStyle(
				t.app.T("cluster.settings.empty_custom_lines"),
				fyne.TextAlignCenter, fyne.TextStyle{Italic: true}))
			linesBox.Refresh()
			return
		}
		for i, line := range workingLines {
			i := i
			file := line.File
			if file == "" {
				file = "Game.ini"
			}
			meta := widget.NewLabel(t.app.T("cluster.settings.custom_line_meta", file, line.Section, line.Key, line.Value))
			delBtn := widget.NewButton(t.app.T("common.delete"), func() {
				workingLines = append(workingLines[:i], workingLines[i+1:]...)
				rebuildLines()
			})
			linesBox.Add(container.NewBorder(nil, nil, nil, delBtn, meta))
			linesBox.Add(widget.NewSeparator())
		}
		linesBox.Refresh()
	}
	rebuildLines()

	addLineBtn := widget.NewButton(t.app.T("cluster.settings.button_add_line"), func() {
		fileSel := widget.NewSelect([]string{"Game.ini", "GameUserSettings.ini"}, nil)
		fileSel.SetSelected("Game.ini")
		sectionEntry := widget.NewEntry()
		sectionEntry.SetPlaceHolder(t.app.T("cluster.settings.field_section_placeholder"))
		keyEntry := widget.NewEntry()
		keyEntry.SetPlaceHolder(t.app.T("cluster.settings.field_key_placeholder"))
		valEntry := widget.NewEntry()
		valEntry.SetPlaceHolder(t.app.T("cluster.settings.field_value_placeholder"))

		form := widget.NewForm(
			widget.NewFormItem(t.app.T("cluster.settings.field_file"), fileSel),
			widget.NewFormItem(t.app.T("cluster.settings.field_section"), sectionEntry),
			widget.NewFormItem(t.app.T("cluster.settings.field_key"), keyEntry),
			widget.NewFormItem(t.app.T("cluster.settings.field_value"), valEntry),
		)
		t.app.showFormDialog(t.app.T("cluster.settings.add_line_title"), t.app.T("cluster.settings.add_line_confirm"), form,
			fyne.NewSize(560, 360),
			func() error {
				sec := strings.TrimSpace(sectionEntry.Text)
				key := strings.TrimSpace(keyEntry.Text)
				if sec == "" || key == "" {
					return fmt.Errorf("%s", t.app.T("cluster.settings.err_section_key_required"))
				}
				workingLines = append(workingLines, cluster.CustomINIEntry{
					File: fileSel.Selected, Section: sec, Key: key, Value: valEntry.Text,
				})
				rebuildLines()
				return nil
			})
	})

	customTab := container.NewBorder(
		container.NewVBox(
			widget.NewLabelWithStyle(
				t.app.T("cluster.settings.custom_lines_intro"),
				fyne.TextAlignLeading, fyne.TextStyle{Italic: true}),
			addLineBtn,
		),
		nil, nil, nil,
		container.NewVScroll(linesBox),
	)
	catTabs.Append(container.NewTabItem(t.app.T("cluster.settings.custom_lines_tab"), customTab))

	saveBtn := widget.NewButton(t.app.T("cluster.settings.button_save"), func() {
		newSettings := map[string]settings.Value{}
		for _, ed := range editors {
			v, err := ed.read()
			if err != nil {
				t.app.showError(fmt.Errorf(t.app.T("server.settings.err_setting_invalid"), ed.setting.Key, err))
				return
			}
			if err := ed.setting.Validate(v); err != nil {
				t.app.showError(err)
				return
			}

			if v != ed.setting.DefaultValue() {
				newSettings[ed.setting.Key] = v
			}
		}
		c.Settings = newSettings
		c.CustomLines = workingLines
		c.ActiveEvent = eventValueForLabel(clusterEventSel.Selected, clusterEventLabels, clusterEventValues)
		c.ActiveMods = parseModIDs(clusterModsEntry.Text)
		if err := t.app.deps.Clusters.Update(t.app.ctx(), c); err != nil {
			t.app.showError(err)
			return
		}
		now := time.Now()
		t.app.publish(events.ClusterSettingsChanged{ClusterID: c.ID, Name: c.Name, Count: len(newSettings), At: now})
		t.app.publish(events.ClusterSettingsSaved{ClusterID: c.ID, Name: c.Name, Count: len(newSettings), At: now})
		t.app.exportASBClusterAndAllServers(c)
		t.app.showInfo(t.app.T("cluster.settings.saved_title"),
			t.app.T("cluster.settings.saved_body", len(newSettings), len(workingLines)))
		t.app.refreshTree()
		t.ShowCluster(c)
	})

	applyBtn := widget.NewButton(t.app.T("cluster.settings.button_apply_all"), func() {
		t.app.confirm(t.app.T("cluster.overview.confirm_apply_title"),
			t.app.T("cluster.settings.confirm_apply_settings_body"),
			func() {
				go func() {
					if err := t.app.deps.Clusters.PropagateToServers(t.app.ctx(), t.app.deps.Servers, c.ID); err != nil {
						t.app.showError(err)
						return
					}
					fyne.Do(func() {
						t.app.showInfo(t.app.T("cluster.overview.apply_done_title"), t.app.T("cluster.overview.apply_done_body"))
					})
				}()
			})
	})

	scope := clusterPresetScope(c)
	savePresetBtn := widget.NewButton(t.app.T("cluster.settings.button_save_preset"), func() {
		t.app.showSavePresetDialog(scope, nil)
	})
	applyPresetBtn := widget.NewButton(t.app.T("cluster.settings.button_apply_preset"), func() {
		t.app.showApplyPresetDialog(scope, func() { t.ShowCluster(c) })
	})
	managePresetsBtn := widget.NewButton(t.app.T("cluster.settings.button_manage_presets"), func() {
		t.app.showManagePresetsDialog(scope, nil)
	})

	header := widget.NewLabelWithStyle(
		t.app.T("cluster.settings.header", c.Name, len(c.Settings)),
		fyne.TextAlignLeading, fyne.TextStyle{Italic: true})

	eventRow := container.NewBorder(nil, nil,
		widget.NewLabelWithStyle(t.app.T("cluster.settings.active_event_label"), fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		nil,
		clusterEventSel)

	clusterModsRow := container.NewBorder(nil,
		widget.NewLabelWithStyle(t.app.T("cluster.settings.mods_hint"), fyne.TextAlignLeading, fyne.TextStyle{Italic: true}),
		widget.NewLabelWithStyle(t.app.T("cluster.settings.mods_label"), fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		nil,
		clusterModsEntry)

	top := container.NewVBox(header, eventRow, clusterModsRow, widget.NewSeparator())

	return container.NewBorder(
		top,
		container.NewHBox(saveBtn, applyBtn, savePresetBtn, applyPresetBtn, managePresetsBtn),
		nil, nil,
		catTabs,
	)
}

func (t *detailTabs) serverOverview(c cluster.Cluster, s server.Server) fyne.CanvasObject {
	title := widget.NewLabelWithStyle(s.Name, fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	statusLabel := widget.NewLabel(t.app.T("server.overview.status_label", statusBadge(t.app, s.Status), string(s.Status)))

	installBtn := widget.NewButton(t.app.T("server.overview.button_install"), func() {
		st := t.app.deps.Supervisor.Status(s.ID)
		if st != server.StatusStopped && st != server.StatusCrashed {
			t.app.showError(fmt.Errorf(t.app.T("server.overview.err_busy_for_install"), st))
			return
		}
		t.app.publish(events.ServerInstallUpdate{
			ServerID: s.ID, ClusterID: s.ClusterID, Name: s.Name, At: time.Now(),
		})
		t.app.runInstall(installPlan{servers: []server.Server{s}})
	})

	runAndRefresh := func(fn func() error) {
		go func() {
			if err := fn(); err != nil {
				fyne.Do(func() { t.app.showError(err) })
			}
			fyne.Do(func() {
				t.app.refreshTree()
				t.RefreshIfShowing(s.ID)
			})
		}()
	}
	startBtn := widget.NewButton(t.app.T("server.overview.button_start"), func() {
		runAndRefresh(func() error { return t.app.deps.Supervisor.Start(t.app.ctx(), s.ID) })
	})
	stopBtn := widget.NewButton(t.app.T("server.overview.button_stop"), func() {
		t.app.confirm(t.app.T("server.overview.confirm_stop_title"), t.app.T("server.overview.confirm_stop_body", s.Name), func() {
			runAndRefresh(func() error { return t.app.deps.Supervisor.Stop(t.app.ctx(), s.ID, true) })
		})
	})
	killBtn := widget.NewButton(t.app.T("server.overview.button_kill"), func() {
		t.app.confirm(t.app.T("server.overview.confirm_kill_title"), t.app.T("server.overview.confirm_kill_body", s.Name), func() {
			runAndRefresh(func() error { return t.app.deps.Supervisor.Stop(t.app.ctx(), s.ID, false) })
		})
	})
	restartBtn := widget.NewButton(t.app.T("server.overview.button_restart"), func() {
		t.app.confirm(t.app.T("server.overview.confirm_restart_title"), t.app.T("server.overview.confirm_restart_body", s.Name), func() {
			runAndRefresh(func() error { return t.app.deps.Supervisor.Restart(t.app.ctx(), s.ID) })
		})
	})
	applyINIs := widget.NewButton(t.app.T("server.overview.button_write_inis"), func() {
		go func() {
			values := cluster.MergeSettings(c.Settings, s.SettingOverrides)
			if err := cluster.WriteServerINIs(s, values, c.CustomLines); err != nil {
				t.app.showError(err)
				return
			}
			fyne.Do(func() { t.app.showInfo(t.app.T("server.overview.inis_done_title"), t.app.T("server.overview.inis_done_body")) })
		}()
	})
	writeBat := widget.NewButton(t.app.T("server.overview.button_write_bat"), func() {
		opts := server.LaunchOptions{ClusterID: c.ClusterID, ClusterDir: c.ClusterDir, Mods: modIDs(s.ActiveMods)}
		cmd := server.BuildLaunchCommand(s, opts)
		path, err := server.WriteBatchFile(s, cmd)
		if err != nil {
			t.app.showError(err)
			return
		}
		t.app.showInfo(t.app.T("server.overview.bat_done_title"), t.app.T("server.overview.bat_done_body", path))
	})
	backupNow := widget.NewButton(t.app.T("server.overview.button_backup_now"), func() {
		go func() {
			if _, err := t.app.deps.BackupManager.BackupServer(t.app.ctx(), s, "manual"); err != nil {
				fyne.Do(func() { t.app.showError(err) })
				return
			}
			fyne.Do(func() { t.app.showInfo(t.app.T("server.overview.backup_done_title"), t.app.T("server.overview.backup_done_body")) })
		}()
	})
	manageBackups := widget.NewButton(t.app.T("server.overview.button_manage_backups"), func() {
		t.app.showServerBackups(s)
	})
	openFolder := widget.NewButton(t.app.T("server.overview.button_open_explorer"), func() {
		if s.InstallDir == "" {
			t.app.showError(fmt.Errorf("%s", t.app.T("server.overview.err_no_install_dir")))
			return
		}
		if _, err := os.Stat(s.InstallDir); err == nil {
			if err := openInFileExplorer(s.InstallDir); err != nil {
				t.app.showError(err)
			}
			return
		}
		t.app.confirm(
			t.app.T("server.overview.confirm_dir_create_title"),
			t.app.T("server.overview.confirm_dir_create_body", s.InstallDir),
			func() {
				if err := os.MkdirAll(s.InstallDir, 0o755); err != nil {
					t.app.showError(err)
					return
				}
				if err := openInFileExplorer(s.InstallDir); err != nil {
					t.app.showError(err)
				}
			})
	})
	deleteBtn := widget.NewButton(t.app.T("server.overview.button_delete"), func() {
		st := t.app.deps.Supervisor.Status(s.ID)
		if st != server.StatusStopped && st != server.StatusCrashed {
			t.app.showError(fmt.Errorf(t.app.T("server.overview.err_busy_for_delete"), st))
			return
		}
		msg := t.app.T("server.overview.confirm_delete_body", s.Name, s.InstallDir)
		t.app.confirm(t.app.T("server.overview.confirm_delete_title"), msg, func() {
			logDir := filepath.Join(t.app.deps.DataDir, "logs")
			if err := cluster.DestroyServer(t.app.ctx(), t.app.deps.Servers, s, logDir, t.app.deps.Log); err != nil {
				t.app.showError(err)
				return
			}
			if t.app.deps.ASBDir != "" {
				if err := asb.DeleteForServer(s, t.app.deps.ASBDir); err != nil {
					t.app.deps.Log.Warn("delete ASB server file", "server_id", s.ID, "err", err)
				}
			}
			t.app.publish(events.ServerDeleted{ServerID: s.ID, Name: s.Name, At: time.Now()})
			t.app.refreshTree()
			t.Clear()
		})
	})

	clusterRowValue := t.app.T("server.overview.row_cluster_value", c.Name, c.ClusterID)
	if s.ClusterID == 0 {
		clusterRowValue = t.app.T("server.overview.row_cluster_standalone")
	}
	rows := container.NewVBox(
		formatRowItem(t.app.T("server.overview.row_cluster"), clusterRowValue),
		formatRowItem(t.app.T("server.overview.row_map"), s.Map),
		formatRowItem(t.app.T("server.overview.row_install_dir"), s.InstallDir),
		formatRowItem(t.app.T("server.overview.row_game_port"), strconv.Itoa(s.Ports.Game)),
		formatRowItem(t.app.T("server.overview.row_query_port"), strconv.Itoa(s.Ports.Query)),
		formatRowItem(t.app.T("server.overview.row_rcon_port"), strconv.Itoa(s.Ports.RCON)),
	)

	return container.NewVScroll(container.NewVBox(
		title,
		statusLabel,
		widget.NewSeparator(),
		container.NewHBox(installBtn, startBtn, stopBtn, killBtn, restartBtn),
		widget.NewSeparator(),
		rows,
		widget.NewSeparator(),
		container.NewHBox(applyINIs, writeBat, backupNow, manageBackups, openFolder),
		widget.NewSeparator(),
		deleteBtn,
	))
}

func (t *detailTabs) serverSettings(c cluster.Cluster, s server.Server) fyne.CanvasObject {
	reg := settings.Default()
	values := cluster.MergeSettings(c.Settings, s.SettingOverrides)
	var editors []settingEditorEntry

	serverEventLabels, serverEventValues := buildServerEventOptions(t.app, c.ActiveEvent)
	serverEventSel := widget.NewSelect(serverEventLabels, nil)
	serverEventSel.SetSelected(displayLabelForEvent(s.ActiveEvent, serverEventLabels, serverEventValues))

	anticheatCheck := widget.NewCheck(t.app.T("newserver.field_anticheat"), nil)
	anticheatCheck.SetChecked(s.AnticheatEnabled)

	modsEntry := widget.NewEntry()
	modsEntry.SetPlaceHolder(t.app.T("newserver.field_mods_placeholder"))
	modsEntry.SetText(formatModIDs(s.ActiveMods))
	modsHint := t.app.T("server.settings.mods_hint")
	if clusterMods := formatModIDs(c.ActiveMods); clusterMods != "" {
		modsHint = t.app.T("server.settings.mods_hint_with_cluster", clusterMods)
	}

	catTabs := container.NewAppTabs()
	for _, group := range reg.ByCategory() {
		if settings.IsPerStatCategory(group.Name) {
			continue
		}
		form := &widget.Form{}
		for _, sp := range group.Settings {
			cur := values[sp.Key]
			if (cur == settings.Value{}) {
				cur = sp.DefaultValue()
			}
			ed := buildEditorFor(sp, cur)
			editors = append(editors, settingEditorEntry{setting: sp, read: ed.read})
			form.Items = append(form.Items, &widget.FormItem{
				Text:     sp.Key,
				Widget:   ed.widget,
				HintText: sp.Tooltip,
			})
		}
		catTabs.Append(container.NewTabItem(group.Name, container.NewVScroll(form)))
	}

	perStatTab, perStatEditors := buildPerStatGrid(t.app, values, reg)
	editors = append(editors, perStatEditors...)
	catTabs.Append(container.NewTabItem(t.app.T("cluster.settings.per_stat_tab"), perStatTab))

	saveBtn := widget.NewButton(t.app.T("server.settings.button_save"), func() {
		newOverrides := map[string]settings.Value{}
		for _, ed := range editors {
			v, err := ed.read()
			if err != nil {
				t.app.showError(fmt.Errorf(t.app.T("server.settings.err_setting_invalid"), ed.setting.Key, err))
				return
			}
			if err := ed.setting.Validate(v); err != nil {
				t.app.showError(err)
				return
			}

			base, hasBase := c.Settings[ed.setting.Key]
			if !hasBase {
				base = ed.setting.DefaultValue()
			}
			if v != base {
				newOverrides[ed.setting.Key] = v
			}
		}
		s.SettingOverrides = newOverrides
		s.ActiveEvent = eventValueForLabel(serverEventSel.Selected, serverEventLabels, serverEventValues)
		s.AnticheatEnabled = anticheatCheck.Checked
		s.ActiveMods = parseModIDs(modsEntry.Text)
		if err := t.app.deps.Servers.Update(t.app.ctx(), s); err != nil {
			t.app.showError(err)
			return
		}
		now := time.Now()
		t.app.publish(events.ServerSettingsChanged{
			ServerID: s.ID, ClusterID: s.ClusterID, Name: s.Name,
			Count: len(newOverrides), At: now,
		})
		t.app.publish(events.ServerSettingsSaved{
			ServerID: s.ID, ClusterID: s.ClusterID, Name: s.Name,
			Count: len(newOverrides), At: now,
		})
		t.app.exportASBServer(c, s)
		t.app.showInfo(t.app.T("server.settings.saved_title"), t.app.T("server.settings.saved_body", len(newOverrides)))
		t.app.refreshTree()
	})

	resetBtn := widget.NewButton(t.app.T("server.settings.button_clear"), func() {
		t.app.confirm(t.app.T("server.settings.confirm_clear_title"), t.app.T("server.settings.confirm_clear_body"), func() {
			s.SettingOverrides = map[string]settings.Value{}
			if err := t.app.deps.Servers.Update(t.app.ctx(), s); err != nil {
				t.app.showError(err)
				return
			}
			t.app.refreshTree()
			t.ShowServer(c, s)
		})
	})

	scope := serverPresetScope(s)
	savePresetBtn := widget.NewButton(t.app.T("server.settings.button_save_preset"), func() {
		t.app.showSavePresetDialog(scope, nil)
	})
	applyPresetBtn := widget.NewButton(t.app.T("server.settings.button_apply_preset"), func() {
		t.app.showApplyPresetDialog(scope, func() { t.ShowServer(c, s) })
	})
	managePresetsBtn := widget.NewButton(t.app.T("server.settings.button_manage_presets"), func() {
		t.app.showManagePresetsDialog(scope, nil)
	})

	header := widget.NewLabelWithStyle(
		t.app.T("server.settings.header", s.Name, len(c.Settings), len(s.SettingOverrides)),
		fyne.TextAlignLeading, fyne.TextStyle{Italic: true})

	eventRow := container.NewBorder(nil, nil,
		widget.NewLabelWithStyle(t.app.T("server.settings.active_event_label"), fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		nil,
		serverEventSel)

	modsRow := container.NewBorder(nil,
		widget.NewLabelWithStyle(modsHint, fyne.TextAlignLeading, fyne.TextStyle{Italic: true}),
		widget.NewLabelWithStyle(t.app.T("server.settings.mods_label"), fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		nil,
		modsEntry)

	top := container.NewVBox(header, eventRow, anticheatCheck, modsRow, widget.NewSeparator())

	return container.NewBorder(
		top,
		container.NewHBox(saveBtn, resetBtn, savePresetBtn, applyPresetBtn, managePresetsBtn),
		nil, nil,
		catTabs,
	)
}

type settingEditor struct {
	widget fyne.CanvasObject
	read   func() (settings.Value, error)
}

func buildEditorFor(s settings.Setting, current settings.Value) settingEditor {
	switch s.Type {
	case settings.TypeBool:
		c := widget.NewCheck("", nil)
		c.SetChecked(current.Bool)
		return settingEditor{
			widget: c,
			read:   func() (settings.Value, error) { return settings.BoolVal(c.Checked), nil },
		}
	case settings.TypeInt:
		e := widget.NewEntry()
		e.SetText(strconv.Itoa(current.Int))
		return settingEditor{
			widget: e,
			read: func() (settings.Value, error) {
				n, err := strconv.Atoi(strings.TrimSpace(e.Text))
				if err != nil {
					return settings.Value{}, fmt.Errorf("invalid integer %q", e.Text)
				}
				return settings.IntVal(n), nil
			},
		}
	case settings.TypeString:
		e := widget.NewEntry()
		e.SetText(current.String)
		return settingEditor{
			widget: e,
			read:   func() (settings.Value, error) { return settings.StringVal(e.Text), nil },
		}
	default: // TypeFloat
		e := widget.NewEntry()
		e.SetText(current.Format(s.Type))
		return settingEditor{
			widget: e,
			read: func() (settings.Value, error) {
				f, err := strconv.ParseFloat(strings.TrimSpace(e.Text), 64)
				if err != nil {
					return settings.Value{}, fmt.Errorf("invalid number %q", e.Text)
				}
				return settings.FloatVal(f), nil
			},
		}
	}
}

func (t *detailTabs) serverPlayers(s server.Server) fyne.CanvasObject {
	header := widget.NewLabelWithStyle(
		t.app.T("server.players.header"),
		fyne.TextAlignLeading, fyne.TextStyle{Italic: true})

	rows := []playerRow{}
	tableBox := container.NewVBox()

	rebuild := func() {
		sessions, err := t.app.deps.Players.ListOpenSessions(t.app.ctx(), s.ID)
		if err != nil {
			t.app.showError(err)
			return
		}
		rows = rows[:0]
		for _, sess := range sessions {
			p, _ := t.app.deps.Players.GetPlayer(t.app.ctx(), sess.SteamID)
			rows = append(rows, playerRow{
				steamID: sess.SteamID, name: p.LastKnownName, joined: sess.JoinedAt,
			})
		}
		tableBox.Objects = nil
		tableBox.Add(playerHeaderRow(t.app))
		for _, r := range rows {
			tableBox.Add(r.toRow())
		}
		if len(rows) == 0 {
			tableBox.Add(widget.NewLabelWithStyle(t.app.T("server.players.empty"), fyne.TextAlignCenter, fyne.TextStyle{Italic: true}))
		}
		tableBox.Refresh()
	}
	rebuild()

	refresh := widget.NewButton(t.app.T("common.refresh"), rebuild)
	pollNow := widget.NewButton(t.app.T("server.players.button_poll_now"), func() {
		go func() {
			ctx, cancel := context.WithTimeout(t.app.ctx(), 5*time.Second)
			defer cancel()
			if err := t.app.deps.Tracker.PollOnce(ctx, s.ID); err != nil {
				t.app.showError(err)
				return
			}
			fyne.Do(rebuild)
		}()
	})

	return container.NewBorder(header, container.NewHBox(refresh, pollNow), nil, nil,
		container.NewVScroll(tableBox))
}

type playerRow struct {
	steamID string
	name    string
	joined  time.Time
}

func playerHeaderRow(a *App) fyne.CanvasObject {
	bold := func(s string) *widget.Label {
		l := widget.NewLabel(s)
		l.TextStyle = fyne.TextStyle{Bold: true}
		return l
	}
	return container.NewGridWithColumns(3,
		bold(a.T("server.players.header_name")),
		bold(a.T("server.players.header_steam_id")),
		bold(a.T("server.players.header_joined")))
}

func (r playerRow) toRow() fyne.CanvasObject {
	return container.NewGridWithColumns(3,
		widget.NewLabel(r.name),
		widget.NewLabel(r.steamID),
		widget.NewLabel(r.joined.Local().Format("15:04:05")),
	)
}

func (t *detailTabs) serverLogs(s server.Server) (fyne.CanvasObject, func()) {
	const maxLines = 500

	body := widget.NewTextGrid()
	bodyScroll := container.NewScroll(body)
	bodyScroll.SetMinSize(fyne.NewSize(0, 420))

	logPath := filepath.Join(t.app.deps.DataDir, "logs", fmt.Sprintf("server-%d.log", s.ID))
	var lines []string
	if data, err := tailFile(logPath, 64*1024); err == nil && len(data) > 0 {
		lines = strings.Split(string(data), "\n")
		if len(lines) > maxLines {
			lines = lines[len(lines)-maxLines:]
		}
		body.SetText(strings.Join(lines, "\n"))
	}

	var (
		mu    sync.Mutex
		dirty bool
	)
	flush := func() {
		mu.Lock()
		if !dirty {
			mu.Unlock()
			return
		}
		snap := strings.Join(lines, "\n")
		dirty = false
		mu.Unlock()
		fyne.Do(func() {
			body.SetText(snap)
			bodyScroll.ScrollToBottom()
		})
	}

	stop := make(chan struct{})
	stopOnce := sync.Once{}
	cancel := func() { stopOnce.Do(func() { close(stop) }) }

	if t.app.deps.Supervisor.Status(s.ID) == server.StatusRunning ||
		t.app.deps.Supervisor.Status(s.ID) == server.StatusStarting {
		ch, err := t.app.deps.Supervisor.Logs(s.ID)
		if err == nil && ch != nil {
			go func() {
				for {
					select {
					case <-stop:
						return
					case line, ok := <-ch:
						if !ok {
							return
						}
						mu.Lock()
						lines = append(lines, line)
						if len(lines) > maxLines {
							lines = lines[len(lines)-maxLines:]
						}
						dirty = true
						mu.Unlock()
					}
				}
			}()
			
			go func() {
				ticker := time.NewTicker(200 * time.Millisecond)
				defer ticker.Stop()
				for {
					select {
					case <-stop:
						flush()
						return
					case <-ticker.C:
						flush()
					}
				}
			}()
		}
	}

	header := widget.NewLabelWithStyle(
		t.app.T("server.logs.header"),
		fyne.TextAlignLeading, fyne.TextStyle{Italic: true})

	return container.NewBorder(header, nil, nil, nil, withTextPaneBackground(bodyScroll)), cancel
}

func (t *detailTabs) run(fn func() error) {
	if err := fn(); err != nil {
		fyne.Do(func() { t.app.showError(err) })
	}
}

func modIDs(refs []server.ModRef) []int {
	out := make([]int, 0, len(refs))
	for _, r := range refs {
		if r.Enabled {
			out = append(out, r.CurseForgeID)
		}
	}
	return out
}

func tailFile(path string, n int64) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	stat, err := f.Stat()
	if err != nil {
		return nil, err
	}
	size := stat.Size()
	offset := size - n
	if offset < 0 {
		offset = 0
	}
	if _, err := f.Seek(offset, 0); err != nil {
		return nil, err
	}
	buf := make([]byte, size-offset)
	if _, err := f.Read(buf); err != nil {
		return nil, err
	}
	return buf, nil
}
