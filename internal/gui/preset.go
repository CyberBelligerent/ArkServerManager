package gui

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"asamanager/internal/cluster"
	"asamanager/internal/preset"
	"asamanager/internal/scheduler"
	"asamanager/internal/server"
	"asamanager/internal/settings"
)

type presetScope struct {
	Type      string // "cluster" or "server"
	ID        int64
	Name      string
	Snapshot  func() preset.Payload // capture current saved state into a Payload
}

func clusterPresetScope(c cluster.Cluster) presetScope {
	return presetScope{
		Type: preset.ScopeCluster, ID: c.ID, Name: c.Name,
		Snapshot: func() preset.Payload {
			ev := c.ActiveEvent
			return preset.Payload{
				Settings:    cloneSettings(c.Settings),
				ActiveEvent: &ev,
			}
		},
	}
}

func serverPresetScope(s server.Server) presetScope {
	return presetScope{
		Type: preset.ScopeServer, ID: s.ID, Name: s.Name,
		Snapshot: func() preset.Payload {
			ev := s.ActiveEvent
			return preset.Payload{
				Settings:    cloneSettings(s.SettingOverrides),
				ActiveEvent: &ev,
			}
		},
	}
}

func cloneSettings(in map[string]settings.Value) map[string]settings.Value {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]settings.Value, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

// showSavePresetDialog snapshots the scope's currently-saved state into a new preset
func (a *App) showSavePresetDialog(scope presetScope, onSaved func()) {
	nameEntry := widget.NewEntry()
	nameEntry.SetPlaceHolder(a.T("preset.name_placeholder"))
	descEntry := widget.NewMultiLineEntry()
	descEntry.SetPlaceHolder(a.T("preset.desc_placeholder"))

	payload := scope.Snapshot()
	previewLines := []string{
		a.T("preset.save.preview_header", scope.Type, scope.Name),
		a.T("preset.save.preview_settings", len(payload.Settings)),
	}
	if payload.ActiveEvent != nil {
		ev := *payload.ActiveEvent
		if ev == "" {
			ev = a.T("preset.event_empty_inherit")
		}
		previewLines = append(previewLines, a.T("preset.save.preview_event", ev))
	}
	preview := widget.NewLabelWithStyle(strings.Join(previewLines, "\n"),
		fyne.TextAlignLeading, fyne.TextStyle{Italic: true})

	form := widget.NewForm(
		widget.NewFormItem(a.T("preset.save.field_name"), nameEntry),
		widget.NewFormItem(a.T("preset.save.field_description"), descEntry),
		widget.NewFormItem("", preview),
	)

	a.showFormDialog(a.T("preset.save.title"), a.T("common.save"), form, fyne.NewSize(560, 420), func() error {
		name := strings.TrimSpace(nameEntry.Text)
		if name == "" {
			return fmt.Errorf("%s", a.T("preset.save.err_name_required"))
		}

		if len(payload.Settings) == 0 && (payload.ActiveEvent == nil || *payload.ActiveEvent == "") {
			return fmt.Errorf("%s", a.T("preset.save.err_empty_payload"))
		}
		_, err := a.deps.Presets.Create(a.ctx(), preset.Preset{
			ScopeType: scope.Type, ScopeID: scope.ID,
			Name: name, Description: descEntry.Text,
			Payload: payload,
		})
		if err != nil {
			return err
		}
		if onSaved != nil {
			onSaved()
		}
		return nil
	})
}

func (a *App) showApplyPresetDialog(scope presetScope, onApplied func()) {
	presets, err := a.deps.Presets.List(a.ctx(), scope.Type, scope.ID)
	if err != nil {
		a.showError(err)
		return
	}
	if len(presets) == 0 {
		a.showInfo(a.T("preset.apply.no_presets_title"),
			a.T("preset.apply.no_presets_body", scope.Type, scope.Name))
		return
	}
	labels, idByLabel := presetLabels(a, presets)
	sel := widget.NewSelect(labels, nil)
	sel.SetSelected(labels[0])
	restartCheck := widget.NewCheck(a.T("preset.apply.field_restart"), nil)

	descLbl := widget.NewLabel(presetDescription(a, presets, idByLabel[sel.Selected]))
	sel.OnChanged = func(s string) {
		descLbl.SetText(presetDescription(a, presets, idByLabel[s]))
	}

	form := widget.NewForm(
		widget.NewFormItem(a.T("preset.apply.field_preset"), sel),
		widget.NewFormItem("", descLbl),
		widget.NewFormItem("", restartCheck),
	)

	a.showFormDialog(
		a.T("preset.apply.title", scope.Type, scope.Name),
		a.T("preset.apply.confirm"), form, fyne.NewSize(560, 380),
		func() error {
			id, ok := idByLabel[sel.Selected]
			if !ok {
				return fmt.Errorf("%s", a.T("preset.apply.err_pick"))
			}
			ps, _, err := a.deps.PresetManager.ApplyByID(a.ctx(), id)
			if err != nil {
				return err
			}
			if restartCheck.Checked {
				switch scope.Type {
				case preset.ScopeCluster:
					go func() {
						if err := a.deps.Coordinator.RestartCluster(a.ctx(), ps.ScopeID, 0); err != nil {
							fyne.Do(func() { a.showError(err) })
						}
					}()
				case preset.ScopeServer:
					go func() {
						if err := a.deps.Supervisor.Restart(a.ctx(), ps.ScopeID); err != nil {
							fyne.Do(func() { a.showError(err) })
						}
					}()
				}
			}
			if onApplied != nil {
				onApplied()
			}
			return nil
		})
}

func (a *App) showManagePresetsDialog(scope presetScope, onChange func()) {
	win := a.fyne.NewWindow(a.T("preset.manage.window_title", scope.Type, scope.Name))
	win.Resize(fyne.NewSize(720, 420))

	listBox := container.NewVBox()
	var refresh func()
	refresh = func() {
		listBox.Objects = nil
		presets, err := a.deps.Presets.List(a.ctx(), scope.Type, scope.ID)
		if err != nil {
			listBox.Add(widget.NewLabel(a.T("preset.manage.err_load", err.Error())))
			listBox.Refresh()
			return
		}
		if len(presets) == 0 {
			listBox.Add(widget.NewLabelWithStyle(a.T("preset.manage.empty"),
				fyne.TextAlignCenter, fyne.TextStyle{Italic: true}))
			listBox.Refresh()
			return
		}
		for _, p := range presets {
			p := p
			meta := widget.NewLabel(a.T("preset.manage.meta",
				p.Name, len(p.Payload.Settings), formatPresetEvent(a, p.Payload.ActiveEvent)))
			desc := widget.NewLabelWithStyle(p.Description,
				fyne.TextAlignLeading, fyne.TextStyle{Italic: true})
			delBtn := widget.NewButton(a.T("common.delete"), func() {
				refs, err := a.scheduledTasksReferencingPreset(p.ID)
				if err != nil {
					a.showError(err)
					return
				}
				if len(refs) > 0 {
					a.showError(fmt.Errorf(a.T("preset.manage.delete_in_use"),
						p.Name, len(refs), strings.Join(refs, ", ")))
					return
				}
				a.confirm(a.T("preset.manage.confirm_delete_title"), a.T("preset.manage.confirm_delete_body", p.Name), func() {
					if err := a.deps.Presets.Delete(a.ctx(), p.ID); err != nil {
						a.showError(err)
						return
					}
					refresh()
					if onChange != nil {
						onChange()
					}
				})
			})
			row := container.NewBorder(nil, desc, nil, delBtn, meta)
			listBox.Add(row)
			listBox.Add(widget.NewSeparator())
		}
		listBox.Refresh()
	}
	refresh()

	win.SetContent(container.NewBorder(
		widget.NewLabelWithStyle(
			a.T("preset.manage.intro"),
			fyne.TextAlignLeading, fyne.TextStyle{Italic: true}),
		nil, nil, nil,
		container.NewVScroll(listBox),
	))
	win.Show()
}

func presetLabels(a *App, ps []preset.Preset) ([]string, map[string]int64) {
	sort.Slice(ps, func(i, j int) bool { return strings.ToLower(ps[i].Name) < strings.ToLower(ps[j].Name) })
	labels := make([]string, 0, len(ps))
	byLabel := make(map[string]int64, len(ps))
	for _, p := range ps {
		label := a.T("preset.apply.label",
			p.Name, len(p.Payload.Settings), formatPresetEvent(a, p.Payload.ActiveEvent))
		labels = append(labels, label)
		byLabel[label] = p.ID
	}
	return labels, byLabel
}

func presetDescription(a *App, ps []preset.Preset, id int64) string {
	for _, p := range ps {
		if p.ID == id {
			if p.Description == "" {
				return a.T("preset.no_description")
			}
			return p.Description
		}
	}
	return ""
}

func formatPresetEvent(a *App, p *string) string {
	if p == nil {
		return a.T("preset.event_unchanged")
	}
	if *p == "" {
		return a.T("preset.event_inherit")
	}
	return *p
}

func (a *App) scheduledTasksReferencingPreset(id int64) ([]string, error) {
	tasks, err := a.deps.SchedulerRepo.ListAll(a.ctx())
	if err != nil {
		return nil, err
	}
	var names []string
	for _, t := range tasks {
		if t.ActionKind != scheduler.ActionApplyPreset {
			continue
		}
		var p scheduler.ApplyPresetPayload
		if err := json.Unmarshal(t.ActionPayload, &p); err != nil {
			continue
		}
		if p.PresetID == id {
			names = append(names, fmt.Sprintf("#%d %q", t.ID, t.Name))
		}
	}
	return names, nil
}
