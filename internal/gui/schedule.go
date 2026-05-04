package gui

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"asamanager/internal/preset"
	"asamanager/internal/scheduler"
)

type scheduleScope struct {
	Type        string // "cluster" or "server"
	ID          int64
	DisplayName string
}

// scheduleTab returns the Schedule pane for a cluster or server
func (t *detailTabs) scheduleTab(scope scheduleScope) fyne.CanvasObject {
	listBox := container.NewVBox()
	var refresh func()
	refresh = func() {
		tasks, err := t.app.deps.SchedulerRepo.List(t.app.ctx(), scope.Type, scope.ID)
		if err != nil {
			t.app.showError(err)
			return
		}
		listBox.Objects = nil
		if len(tasks) == 0 {
			listBox.Add(widget.NewLabelWithStyle(
				t.app.T("scheduler.empty"),
				fyne.TextAlignCenter, fyne.TextStyle{Italic: true}))
			listBox.Refresh()
			return
		}
		for _, task := range tasks {
			task := task
			listBox.Add(t.buildScheduleRow(task, refresh, scope))
			listBox.Add(widget.NewSeparator())
		}
		listBox.Refresh()
	}

	addBtn := widget.NewButton(t.app.T("scheduler.button_add_task"), func() {
		t.app.showTaskDialog(scope, nil, refresh)
	})
	scheduleEventBtn := widget.NewButton(t.app.T("scheduler.button_schedule_event"), func() {
		t.app.showScheduleEventDialog(scope, refresh)
	})
	refreshBtn := widget.NewButton(t.app.T("common.refresh"), refresh)
	debugBtn := widget.NewButton(t.app.T("scheduler.button_debug_engine"), func() {
		t.app.showSchedulerDebug()
	})
	logsBtn := widget.NewButton(t.app.T("scheduler.button_open_logs_folder"), func() {
		if err := openInFileExplorer(t.app.deps.DataDir); err != nil {
			t.app.showError(err)
		}
	})

	header := widget.NewLabelWithStyle(
		t.app.T("scheduler.tab_header", scope.Type, scope.DisplayName),
		fyne.TextAlignLeading, fyne.TextStyle{Italic: true})

	refresh()
	return container.NewBorder(
		container.NewVBox(header, container.NewHBox(addBtn, scheduleEventBtn, refreshBtn, debugBtn, logsBtn)),
		nil, nil, nil,
		container.NewVScroll(listBox),
	)
}

// showSchedulerDebug dumps every enabled task with the exact NextFireAt the engine reads from the DB
func (a *App) showSchedulerDebug() {
	win := a.fyne.NewWindow(a.T("scheduler.debug.window_title"))
	win.Resize(fyne.NewSize(900, 520))

	now := time.Now()
	zoneName, offsetSec := now.Zone()
	offsetH := offsetSec / 3600
	offsetM := (offsetSec % 3600) / 60
	sign := "+"
	if offsetSec < 0 {
		sign = "-"
		offsetH = -offsetH
		offsetM = -offsetM
	}
	var sb strings.Builder
	sb.WriteString(a.T("scheduler.debug.now_local", now.Format("2006-01-02 15:04:05 -0700 MST")) + "\n")
	sb.WriteString(a.T("scheduler.debug.now_utc", now.UTC().Format("2006-01-02 15:04:05 MST")) + "\n")
	sb.WriteString(a.T("scheduler.debug.tz", zoneName, sign, offsetH, offsetM) + "\n\n")

	tasks, err := a.deps.SchedulerRepo.ListAll(a.ctx())
	if err != nil {
		sb.WriteString(a.T("scheduler.history.error", err.Error()))
	} else if len(tasks) == 0 {
		sb.WriteString(a.T("scheduler.debug.no_tasks") + "\n")
	} else {
		sb.WriteString(a.T("scheduler.debug.total_count", len(tasks)) + "\n\n")
		for _, task := range tasks {
			sb.WriteString(a.T("scheduler.debug.task_header",
				task.ID, task.Name, task.Status, task.ScopeType, task.ScopeID,
				task.TriggerKind, task.ActionKind, task.MissedPolicy) + "\n")
			if task.TriggerKind == scheduler.TriggerCron {
				sb.WriteString(a.T("scheduler.debug.task_cron", task.TriggerCron) + "\n")
			}
			if !task.StartAt.IsZero() {
				sb.WriteString(a.T("scheduler.debug.task_start_at",
					task.StartAt.Local().Format("2006-01-02 15:04:05 MST"),
					task.StartAt.UTC().Format("2006-01-02 15:04:05")) + "\n")
			}
			if task.NextFireAt != nil {
				delta := time.Until(*task.NextFireAt).Round(time.Second)
				dueWord := a.T("scheduler.debug.status_future")
				if delta <= 0 {
					dueWord = a.T("scheduler.debug.status_due_now", (-delta).String())
				}
				sb.WriteString(a.T("scheduler.debug.task_next_fire",
					task.NextFireAt.Local().Format("2006-01-02 15:04:05 MST"),
					task.NextFireAt.UTC().Format("2006-01-02 15:04:05"),
					dueWord, delta) + "\n")
			} else {
				sb.WriteString(a.T("scheduler.debug.task_next_fire_null") + "\n")
			}
			if task.LastFiredAt != nil {
				sb.WriteString(a.T("scheduler.debug.task_last_fired",
					task.LastFiredAt.Local().Format("2006-01-02 15:04:05")) + "\n")
			}
			sb.WriteString("\n")
		}
	}

	body := widget.NewTextGrid()
	body.SetText(sb.String())
	scroll := container.NewScroll(body)
	scroll.SetMinSize(fyne.NewSize(880, 460))

	win.SetContent(container.NewBorder(
		widget.NewLabelWithStyle(
			a.T("scheduler.debug.header"),
			fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		nil, nil, nil,
		withTextPaneBackground(scroll),
	))
	win.Show()
}

func (t *detailTabs) buildScheduleRow(task scheduler.Task, refresh func(), scope scheduleScope) fyne.CanvasObject {
	statusBadge := t.app.T("scheduler.row_status_off")
	if task.Status == scheduler.StatusEnabled {
		statusBadge = t.app.T("scheduler.row_status_on")
	}
	trigger := task.TriggerKind
	if task.TriggerKind == scheduler.TriggerCron {
		trigger = t.app.T("scheduler.row_cron", task.TriggerCron)
	} else if task.TriggerKind == scheduler.TriggerOneshot && !task.StartAt.IsZero() {
		trigger = t.app.T("scheduler.row_oneshot_at", task.StartAt.Local().Format("2006-01-02 15:04:05"))
	}
	nextLine := t.app.T("scheduler.row_next_none")
	if task.NextFireAt != nil {
		nextLine = t.app.T("scheduler.row_next_at", task.NextFireAt.Local().Format("2006-01-02 15:04:05"))
	}

	meta := widget.NewLabel(t.app.T("scheduler.row_meta",
		statusBadge, task.Name, task.ActionKind, trigger, nextLine))

	runNowBtn := widget.NewButton(t.app.T("scheduler.button_run_now"), func() {
		go func() {
			if err := t.app.deps.SchedulerEngine.RunOnce(t.app.ctx(), task.ID); err != nil {
				fyne.Do(func() { t.app.showError(err) })
				return
			}
			fyne.Do(refresh)
		}()
	})
	editBtn := widget.NewButton(t.app.T("common.edit"), func() {
		t.app.showTaskDialog(scope, &task, refresh)
	})
	toggleLabel := t.app.T("scheduler.button_disable")
	if task.Status == scheduler.StatusDisabled {
		toggleLabel = t.app.T("scheduler.button_enable")
	}
	toggleBtn := widget.NewButton(toggleLabel, func() {
		newStatus := scheduler.StatusDisabled
		if task.Status == scheduler.StatusDisabled {
			newStatus = scheduler.StatusEnabled
		}
		if err := t.app.deps.SchedulerRepo.SetStatus(t.app.ctx(), task.ID, newStatus); err != nil {
			t.app.showError(err)
			return
		}
		refresh()
	})
	historyBtn := widget.NewButton(t.app.T("scheduler.button_history"), func() {
		t.app.showTaskRuns(task)
	})
	delBtn := widget.NewButton(t.app.T("common.delete"), func() {
		t.app.confirm(t.app.T("scheduler.delete_confirm_title"),
			t.app.T("scheduler.delete_confirm_body", task.Name),
			func() {
				if err := t.app.deps.SchedulerRepo.Delete(t.app.ctx(), task.ID); err != nil {
					t.app.showError(err)
					return
				}
				refresh()
			})
	})

	return container.NewBorder(nil, nil, nil,
		container.NewHBox(runNowBtn, editBtn, toggleBtn, historyBtn, delBtn),
		meta)
}

// showTaskRuns opens a window listing the most recent runs for the task
func (a *App) showTaskRuns(task scheduler.Task) {
	win := a.fyne.NewWindow(a.T("scheduler.history.window_title", task.Name))
	win.Resize(fyne.NewSize(720, 420))

	body := widget.NewTextGrid()
	scroll := container.NewScroll(body)
	scroll.SetMinSize(fyne.NewSize(700, 360))

	runs, err := a.deps.SchedulerRepo.ListRuns(a.ctx(), task.ID, 50)
	if err != nil {
		body.SetText(a.T("scheduler.history.error", err.Error()))
	} else if len(runs) == 0 {
		body.SetText(a.T("scheduler.history.empty"))
	} else {
		var sb strings.Builder
		for _, r := range runs {
			fmt.Fprintf(&sb, "%s  [%s]  %s\n",
				r.FiredAt.Local().Format("2006-01-02 15:04:05"),
				r.Result, r.Message)
		}
		body.SetText(sb.String())
	}

	win.SetContent(container.NewBorder(
		widget.NewLabelWithStyle(a.T("scheduler.history.header"), fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		nil, nil, nil,
		withTextPaneBackground(scroll),
	))
	win.Show()
}

// showTaskDialog opens the task editor scoped to a cluster or server
func (a *App) showTaskDialog(scope scheduleScope, existing *scheduler.Task, onSaved func()) {
	title, confirmText := a.T("scheduler.dialog.title_create"), a.T("scheduler.dialog.confirm_create")
	if existing != nil {
		title, confirmText = a.T("scheduler.dialog.title_edit"), a.T("scheduler.dialog.confirm_save")
	}

	nameEntry := widget.NewEntry()
	nameEntry.SetPlaceHolder(a.T("scheduler.dialog.field_name_placeholder"))

	triggerSel := widget.NewSelect([]string{"oneshot", "cron"}, nil)
	triggerSel.SetSelected("cron")

	oneshotEntry := widget.NewEntry()
	oneshotEntry.SetPlaceHolder(a.T("scheduler.dialog.field_oneshot_placeholder"))
	oneshotEntry.SetText(time.Now().Add(5 * time.Minute).Local().Format("2006-01-02 15:04:05"))

	cronEntry := widget.NewEntry()
	cronEntry.SetPlaceHolder(a.T("scheduler.dialog.field_cron_placeholder"))
	cronEntry.SetText("0 4 * * *")
	cronHint := widget.NewLabelWithStyle(
		a.T("scheduler.dialog.field_cron_hint"),
		fyne.TextAlignLeading, fyne.TextStyle{Italic: true})

	missedSel := widget.NewSelect(
		[]string{scheduler.MissedSkip, scheduler.MissedRunOnce},
		nil)
	missedSel.SetSelected(scheduler.MissedRunOnce)

	var actions []string
	if scope.Type == "cluster" {
		actions = []string{
			scheduler.ActionStartCluster,
			scheduler.ActionStopCluster,
			scheduler.ActionRestartCluster,
			scheduler.ActionBackup,
			scheduler.ActionRCONBroadcast,
			scheduler.ActionApplyPreset,
		}
	} else {
		actions = []string{
			scheduler.ActionStartServer,
			scheduler.ActionStopServer,
			scheduler.ActionRestartServer,
			scheduler.ActionBackup,
			scheduler.ActionRCONBroadcast,
			scheduler.ActionApplyPreset,
		}
	}
	actionSel := widget.NewSelect(actions, nil)
	actionSel.SetSelected(actions[0])

	staggerEntry := widget.NewEntry()
	staggerEntry.SetText("30")
	drainEntry := widget.NewEntry()
	drainEntry.SetText("10")
	broadcastEntry := widget.NewMultiLineEntry()
	broadcastEntry.SetPlaceHolder(a.T("scheduler.dialog.field_broadcast_placeholder"))
	gracefulCheck := widget.NewCheck(a.T("scheduler.dialog.field_graceful"), nil)
	gracefulCheck.SetChecked(true)

	presets, _ := a.deps.Presets.List(a.ctx(), scope.Type, scope.ID)
	presetLbls, presetByLbl := presetLabels(a, presets)
	presetSel := widget.NewSelect(presetLbls, nil)
	if len(presetLbls) > 0 {
		presetSel.SetSelected(presetLbls[0])
	}
	presetRestart := widget.NewCheck(a.T("scheduler.dialog.field_preset_restart"), nil)
	presetRestart.SetChecked(true)

	previewLbl := widget.NewLabelWithStyle(a.T("scheduler.dialog.preview_initial"),
		fyne.TextAlignLeading, fyne.TextStyle{Italic: true})

	updatePreview := func() {
		switch triggerSel.Selected {
		case scheduler.TriggerOneshot:
			when, err := parseLocalScheduleTime(oneshotEntry.Text)
			if err != nil {
				previewLbl.SetText(a.T("scheduler.dialog.preview_error", err.Error()))
				return
			}
			previewLbl.SetText(a.T("scheduler.dialog.preview_oneshot",
				when.Local().Format("2006-01-02 15:04:05"),
				humanizeUntil(a, time.Until(when))))
		case scheduler.TriggerCron:
			expr := strings.TrimSpace(cronEntry.Text)
			tmp := scheduler.Task{TriggerKind: scheduler.TriggerCron, TriggerCron: expr}
			next, err := a.deps.SchedulerEngine.PreviewNext(tmp, time.Now())
			if err != nil || next == nil {
				previewLbl.SetText(a.T("scheduler.dialog.preview_invalid_cron"))
				return
			}
			previewLbl.SetText(a.T("scheduler.dialog.preview_cron",
				next.Local().Format("2006-01-02 15:04:05"),
				humanizeUntil(a, time.Until(*next))))
		}
	}
	triggerSel.OnChanged = func(string) { updatePreview() }
	oneshotEntry.OnChanged = func(string) { updatePreview() }
	cronEntry.OnChanged = func(string) { updatePreview() }

	if existing != nil {
		nameEntry.SetText(existing.Name)
		triggerSel.SetSelected(existing.TriggerKind)
		if !existing.StartAt.IsZero() {
			oneshotEntry.SetText(existing.StartAt.Local().Format("2006-01-02 15:04:05"))
		}
		if existing.TriggerCron != "" {
			cronEntry.SetText(existing.TriggerCron)
		}
		if existing.MissedPolicy != "" {
			missedSel.SetSelected(existing.MissedPolicy)
		}
		actionSel.SetSelected(existing.ActionKind)
		switch existing.ActionKind {
		case scheduler.ActionRestartCluster:
			var p scheduler.RestartClusterPayload
			_ = json.Unmarshal(existing.ActionPayload, &p)
			if p.StaggerSeconds > 0 {
				staggerEntry.SetText(strconv.Itoa(p.StaggerSeconds))
			}
		case scheduler.ActionRestartServer:
			var p scheduler.RestartServerPayload
			_ = json.Unmarshal(existing.ActionPayload, &p)
			if p.DrainWarningMinutes > 0 {
				drainEntry.SetText(strconv.Itoa(p.DrainWarningMinutes))
			}
		case scheduler.ActionRCONBroadcast:
			var p scheduler.RCONBroadcastPayload
			_ = json.Unmarshal(existing.ActionPayload, &p)
			broadcastEntry.SetText(p.Message)
		case scheduler.ActionApplyPreset:
			var p scheduler.ApplyPresetPayload
			_ = json.Unmarshal(existing.ActionPayload, &p)
			for lbl, id := range presetByLbl {
				if id == p.PresetID {
					presetSel.SetSelected(lbl)
					break
				}
			}
			presetRestart.SetChecked(p.Restart)
			if p.StaggerSeconds > 0 {
				staggerEntry.SetText(strconv.Itoa(p.StaggerSeconds))
			}
		case scheduler.ActionStopServer:
			var p scheduler.StopServerPayload
			_ = json.Unmarshal(existing.ActionPayload, &p)
			gracefulCheck.SetChecked(p.Graceful)
		case scheduler.ActionStopCluster:
			var p scheduler.StopClusterPayload
			_ = json.Unmarshal(existing.ActionPayload, &p)
			gracefulCheck.SetChecked(p.Graceful)
		}
	}
	updatePreview()

	form := widget.NewForm(
		widget.NewFormItem(a.T("scheduler.dialog.field_name"), nameEntry),
		widget.NewFormItem(a.T("scheduler.dialog.field_trigger"), triggerSel),
		widget.NewFormItem(a.T("scheduler.dialog.field_oneshot_when"), oneshotEntry),
		widget.NewFormItem(a.T("scheduler.dialog.field_cron"), cronEntry),
		widget.NewFormItem("", cronHint),
		widget.NewFormItem(a.T("scheduler.dialog.field_missed_policy"), missedSel),
		widget.NewFormItem(a.T("scheduler.dialog.field_action"), actionSel),
		widget.NewFormItem(a.T("scheduler.dialog.field_stagger"), staggerEntry),
		widget.NewFormItem(a.T("scheduler.dialog.field_drain"), drainEntry),
		widget.NewFormItem(a.T("scheduler.dialog.field_broadcast"), broadcastEntry),
		widget.NewFormItem(a.T("scheduler.dialog.field_preset"), presetSel),
		widget.NewFormItem("", presetRestart),
		widget.NewFormItem(a.T("scheduler.dialog.field_stop_options"), gracefulCheck),
		widget.NewFormItem("", previewLbl),
	)

	a.showFormDialog(title, confirmText, form, fyne.NewSize(640, 640), func() error {
		name := strings.TrimSpace(nameEntry.Text)
		if name == "" {
			return fmt.Errorf("%s", a.T("scheduler.dialog.err_name_required"))
		}
		task := scheduler.Task{
			Name:         name,
			ScopeType:    scope.Type,
			ScopeID:      scope.ID,
			TriggerKind:  triggerSel.Selected,
			ActionKind:   actionSel.Selected,
			MissedPolicy: missedSel.Selected,
			Status:       scheduler.StatusEnabled,
		}
		if existing != nil {
			task.ID = existing.ID
			task.CreatedAt = existing.CreatedAt
			task.LastFiredAt = existing.LastFiredAt
		}
		switch task.TriggerKind {
		case scheduler.TriggerOneshot:
			when, err := parseLocalScheduleTime(oneshotEntry.Text)
			if err != nil {
				return err
			}
			task.StartAt = when.UTC()
			start := task.StartAt
			task.NextFireAt = &start
		case scheduler.TriggerCron:
			expr := strings.TrimSpace(cronEntry.Text)
			if expr == "" {
				return fmt.Errorf("%s", a.T("scheduler.dialog.err_cron_required"))
			}
			task.TriggerCron = expr
			next, err := a.deps.SchedulerEngine.PreviewNext(task, time.Now())
			if err != nil {
				return fmt.Errorf(a.T("scheduler.dialog.err_cron_parse"), err)
			}
			task.NextFireAt = next
		default:
			return fmt.Errorf(a.T("scheduler.dialog.err_unsupported_trigger"), task.TriggerKind)
		}

		var presetID int64
		if actionSel.Selected == scheduler.ActionApplyPreset {
			id, ok := presetByLbl[presetSel.Selected]
			if !ok {
				return fmt.Errorf("%s", a.T("scheduler.dialog.err_apply_preset_pick"))
			}
			presetID = id
		}
		payload, err := buildActionPayload(a, scope, actionSel.Selected,
			staggerEntry.Text, drainEntry.Text, broadcastEntry.Text,
			presetID, presetRestart.Checked, gracefulCheck.Checked)
		if err != nil {
			return err
		}
		task.ActionPayload = payload

		if existing != nil {
			if err := a.deps.SchedulerRepo.Update(a.ctx(), task); err != nil {
				return err
			}
		} else {
			if _, err := a.deps.SchedulerRepo.Create(a.ctx(), task); err != nil {
				return err
			}
		}
		if onSaved != nil {
			onSaved()
		}
		return nil
	})
}

// parseLocalScheduleTime accepts either YYYY-MM-DD HH:MM or
// YYYY-MM-DD HH:MM:SS in the user's local timezone
func parseLocalScheduleTime(s string) (time.Time, error) {
	s = strings.TrimSpace(s)
	if t, err := time.ParseInLocation("2006-01-02 15:04:05", s, time.Local); err == nil {
		return t, nil
	}
	if t, err := time.ParseInLocation("2006-01-02 15:04", s, time.Local); err == nil {
		return t, nil
	}
	return time.Time{}, fmt.Errorf("oneshot time %q: expected YYYY-MM-DD HH:MM or YYYY-MM-DD HH:MM:SS", s)
}

func humanizeUntil(a *App, d time.Duration) string {
	if d < 0 {
		return a.T("scheduler.debug.duration_past")
	}
	if d < time.Minute {
		return a.T("scheduler.debug.duration_seconds", int(d.Seconds()))
	}
	if d < time.Hour {
		return a.T("scheduler.debug.duration_minutes", int(d.Minutes()))
	}
	if d < 24*time.Hour {
		h := int(d.Hours())
		m := int(d.Minutes()) % 60
		return a.T("scheduler.debug.duration_hours", h, m)
	}
	days := int(d.Hours()) / 24
	hours := int(d.Hours()) % 24
	return a.T("scheduler.debug.duration_days", days, hours)
}

func buildActionPayload(a *App, scope scheduleScope, action, staggerStr, drainStr, broadcast string, presetID int64, presetRestart, graceful bool) ([]byte, error) {
	stagger, _ := strconv.Atoi(strings.TrimSpace(staggerStr))
	switch action {
	case scheduler.ActionStartServer:
		return json.Marshal(scheduler.StartServerPayload{ServerID: scope.ID})
	case scheduler.ActionStopServer:
		return json.Marshal(scheduler.StopServerPayload{ServerID: scope.ID, Graceful: graceful})
	case scheduler.ActionStartCluster:
		return json.Marshal(scheduler.StartClusterPayload{ClusterID: scope.ID, StaggerSeconds: stagger})
	case scheduler.ActionStopCluster:
		return json.Marshal(scheduler.StopClusterPayload{ClusterID: scope.ID, Graceful: graceful, StaggerSeconds: stagger})
	case scheduler.ActionRestartCluster:
		return json.Marshal(scheduler.RestartClusterPayload{
			ClusterID: scope.ID, StaggerSeconds: stagger,
		})
	case scheduler.ActionRestartServer:
		drain, _ := strconv.Atoi(strings.TrimSpace(drainStr))
		return json.Marshal(scheduler.RestartServerPayload{
			ServerID: scope.ID, DrainWarningMinutes: drain,
		})
	case scheduler.ActionBackup:
		return json.Marshal(scheduler.BackupPayload{
			Scope: scope.Type, ScopeID: scope.ID,
		})
	case scheduler.ActionRCONBroadcast:
		msg := strings.TrimSpace(broadcast)
		if msg == "" {
			return nil, fmt.Errorf("%s", a.T("scheduler.dialog.err_broadcast_required"))
		}
		var ids []int64
		if scope.Type == "server" {
			ids = []int64{scope.ID}
		}
		_ = context.Background()
		return json.Marshal(scheduler.RCONBroadcastPayload{
			Message:   msg,
			ServerIDs: ids,
		})
	case scheduler.ActionApplyPreset:
		if presetID == 0 {
			return nil, fmt.Errorf("%s", a.T("scheduler.dialog.err_apply_preset_missing"))
		}
		return json.Marshal(scheduler.ApplyPresetPayload{
			PresetID:       presetID,
			Restart:        presetRestart,
			StaggerSeconds: stagger,
		})
	}
	return nil, fmt.Errorf(a.T("scheduler.dialog.err_unknown_action"), action)
}

func (a *App) presetIDsForScope(scope scheduleScope) []preset.Preset {
	out, _ := a.deps.Presets.List(a.ctx(), scope.Type, scope.ID)
	return out
}

// showScheduleEventDialog opens the "Schedule Event" wizard
func (a *App) showScheduleEventDialog(scope scheduleScope, onCreated func()) {
	presets := a.presetIDsForScope(scope)
	if len(presets) < 2 {
		a.showInfo(a.T("scheduler.event.title"),
			a.T("scheduler.event.need_two_presets", scope.Type, scope.DisplayName))
		return
	}
	labels, idByLabel := presetLabels(a, presets)

	nameEntry := widget.NewEntry()
	nameEntry.SetPlaceHolder(a.T("scheduler.event.field_event_placeholder"))

	startEntry := widget.NewEntry()
	startEntry.SetPlaceHolder(a.T("scheduler.dialog.field_oneshot_placeholder"))
	startEntry.SetText(time.Now().Add(time.Hour).Local().Format("2006-01-02 15:04:00"))

	endEntry := widget.NewEntry()
	endEntry.SetPlaceHolder(a.T("scheduler.dialog.field_oneshot_placeholder"))
	endEntry.SetText(time.Now().Add(8 * 24 * time.Hour).Local().Format("2006-01-02 15:04:00"))

	startPresetSel := widget.NewSelect(labels, nil)
	startPresetSel.SetSelected(labels[0])
	endPresetSel := widget.NewSelect(labels, nil)
	endPresetSel.SetSelected(labels[1])

	restartCheck := widget.NewCheck(a.T("scheduler.event.field_restart_each"), nil)
	restartCheck.SetChecked(true)

	staggerEntry := widget.NewEntry()
	staggerEntry.SetText("30")

	missedSel := widget.NewSelect(
		[]string{scheduler.MissedSkip, scheduler.MissedRunOnce},
		nil)
	missedSel.SetSelected(scheduler.MissedRunOnce)

	form := widget.NewForm(
		widget.NewFormItem(a.T("scheduler.event.field_event_name"), nameEntry),
		widget.NewFormItem(a.T("scheduler.event.field_starts_at"), startEntry),
		widget.NewFormItem(a.T("scheduler.event.field_ends_at"), endEntry),
		widget.NewFormItem(a.T("scheduler.event.field_on_start_preset"), startPresetSel),
		widget.NewFormItem(a.T("scheduler.event.field_on_end_preset"), endPresetSel),
		widget.NewFormItem("", restartCheck),
		widget.NewFormItem(a.T("scheduler.event.field_stagger_cluster"), staggerEntry),
		widget.NewFormItem(a.T("scheduler.dialog.field_missed_policy"), missedSel),
	)

	a.showFormDialog(
		a.T("scheduler.event.title"),
		a.T("scheduler.event.confirm"),
		form,
		fyne.NewSize(680, 520),
		func() error {
			name := strings.TrimSpace(nameEntry.Text)
			if name == "" {
				return fmt.Errorf("%s", a.T("scheduler.event.err_event_name_required"))
			}
			startAt, err := parseLocalScheduleTime(startEntry.Text)
			if err != nil {
				return fmt.Errorf(a.T("scheduler.event.err_start_parse"), err)
			}
			endAt, err := parseLocalScheduleTime(endEntry.Text)
			if err != nil {
				return fmt.Errorf(a.T("scheduler.event.err_end_parse"), err)
			}
			if !endAt.After(startAt) {
				return fmt.Errorf("%s", a.T("scheduler.event.err_end_before_start"))
			}
			startPresetID, ok := idByLabel[startPresetSel.Selected]
			if !ok {
				return fmt.Errorf("%s", a.T("scheduler.event.err_pick_start_preset"))
			}
			endPresetID, ok := idByLabel[endPresetSel.Selected]
			if !ok {
				return fmt.Errorf("%s", a.T("scheduler.event.err_pick_end_preset"))
			}
			if startPresetID == endPresetID {
				return fmt.Errorf("%s", a.T("scheduler.event.err_presets_must_differ"))
			}
			stagger, _ := strconv.Atoi(strings.TrimSpace(staggerEntry.Text))

			mk := func(suffix string, when time.Time, presetID int64) error {
				payload, err := json.Marshal(scheduler.ApplyPresetPayload{
					PresetID: presetID, Restart: restartCheck.Checked, StaggerSeconds: stagger,
				})
				if err != nil {
					return err
				}
				task := scheduler.Task{
					Name:          name + " — " + suffix,
					ScopeType:     scope.Type,
					ScopeID:       scope.ID,
					TriggerKind:   scheduler.TriggerOneshot,
					ActionKind:    scheduler.ActionApplyPreset,
					ActionPayload: payload,
					MissedPolicy:  missedSel.Selected,
					Status:        scheduler.StatusEnabled,
					StartAt:       when.UTC(),
				}
				start := task.StartAt
				task.NextFireAt = &start
				_, err = a.deps.SchedulerRepo.Create(a.ctx(), task)
				return err
			}
			if err := mk("start", startAt, startPresetID); err != nil {
				return fmt.Errorf(a.T("scheduler.event.err_create_start"), err)
			}
			if err := mk("end", endAt, endPresetID); err != nil {
				return fmt.Errorf(a.T("scheduler.event.err_create_end"), err)
			}
			if onCreated != nil {
				onCreated()
			}
			a.showInfo(a.T("scheduler.event.title"),
				a.T("scheduler.event.created_info", name))
			return nil
		})
}
