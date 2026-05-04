package gui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"

	"asamanager/internal/discord/webhook"
)

func (a *App) showWebhooksManager() {
	m := &webhooksManager{app: a}
	m.win = a.fyne.NewWindow(a.T("webhooks.window_title"))
	m.win.Resize(fyne.NewSize(900, 640))

	m.listBox = container.NewVBox()
	addBtn := widget.NewButton(a.T("webhooks.button_add"), func() { m.showEditDialog(nil) })
	refreshBtn := widget.NewButton(a.T("common.refresh"), m.refresh)

	m.win.SetContent(container.NewBorder(
		container.NewHBox(addBtn, refreshBtn),
		nil, nil, nil,
		container.NewVScroll(m.listBox),
	))

	m.refresh()
	m.win.Show()
}

type webhooksManager struct {
	app     *App
	win     fyne.Window
	listBox *fyne.Container
}

func (m *webhooksManager) refresh() {
	list, err := m.app.deps.Webhooks.List(m.app.ctx())
	if err != nil {
		dialog.ShowError(err, m.win)
		return
	}
	m.listBox.Objects = nil

	if len(list) == 0 {
		m.listBox.Add(widget.NewLabelWithStyle(
			m.app.T("webhooks.empty"),
			fyne.TextAlignCenter, fyne.TextStyle{Italic: true}))
		m.listBox.Refresh()
		return
	}

	for _, wh := range list {
		wh := wh
		scopeText := wh.Scope.Type
		if wh.Scope.Type != "global" {
			scopeText = m.app.T("webhooks.scope_with_id", wh.Scope.Type, wh.Scope.ID)
		}
		enabledBadge := m.app.T("webhooks.status_off")
		if wh.Enabled {
			enabledBadge = m.app.T("webhooks.status_on")
		}
		eventBadge := m.app.T("webhooks.events_count", len(wh.EventMask))
		for _, e := range wh.EventMask {
			if e == webhook.AllEventsWildcard {
				eventBadge = m.app.T("webhooks.events_all")
				break
			}
		}
		nameLabel := widget.NewLabelWithStyle(wh.Name, fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
		meta := widget.NewLabel(m.app.T("webhooks.meta_line", enabledBadge, scopeText, eventBadge))

		editBtn := widget.NewButton(m.app.T("common.edit"), func() { m.showEditDialog(&wh) })
		testBtn := widget.NewButton(m.app.T("webhooks.button_send_test"), func() { m.sendTest(wh) })
		delBtn := widget.NewButton(m.app.T("common.delete"), func() {
			dialog.NewConfirm(m.app.T("webhooks.confirm_delete_title"),
				m.app.T("webhooks.confirm_delete_body", wh.Name),
				func(yes bool) {
					if !yes {
						return
					}
					if err := m.app.deps.Webhooks.Delete(m.app.ctx(), wh.ID); err != nil {
						dialog.ShowError(err, m.win)
						return
					}
					m.refresh()
				}, m.win).Show()
		})

		row := container.NewVBox(
			container.NewBorder(nil, nil, nil,
				container.NewHBox(editBtn, testBtn, delBtn),
				container.NewVBox(nameLabel, meta),
			),
			widget.NewSeparator(),
		)
		m.listBox.Add(row)
	}
	m.listBox.Refresh()
}

func (m *webhooksManager) sendTest(w webhook.Webhook) {
	go func() {
		ctx, cancel := context.WithTimeout(m.app.ctx(), 30*time.Second)
		defer cancel()
		err := m.app.deps.WebhookDispatcher.SendTest(ctx, w)
		fyne.Do(func() {
			if err != nil {
				dialog.ShowError(err, m.win)
				return
			}
			dialog.ShowInformation(m.app.T("webhooks.test_sent_title"),
				m.app.T("webhooks.test_sent_body", w.Name), m.win)
		})
	}()
}

func (m *webhooksManager) showEditDialog(existing *webhook.Webhook) {
	nameEntry := widget.NewEntry()
	urlEntry := widget.NewPasswordEntry()
	enabledCheck := widget.NewCheck(m.app.T("webhooks.dialog.field_enabled"), nil)
	enabledCheck.SetChecked(true)

	scopeSelect := widget.NewSelect([]string{"global", "cluster", "server"}, nil)
	scopeIDSelect := widget.NewSelect(nil, nil)
	scopeIDSelect.PlaceHolder = m.app.T("webhooks.dialog.scope_target_placeholder")

	clusterLabels, clusterIDByLabel := m.loadClusterOptions()
	serverLabels, serverIDByLabel := m.loadServerOptions()

	scopeSelect.OnChanged = func(s string) {
		switch s {
		case "global":
			scopeIDSelect.Options = nil
			scopeIDSelect.ClearSelected()
			scopeIDSelect.Disable()
		case "cluster":
			scopeIDSelect.Options = clusterLabels
			scopeIDSelect.Enable()
			if len(clusterLabels) > 0 {
				scopeIDSelect.SetSelected(clusterLabels[0])
			}
		case "server":
			scopeIDSelect.Options = serverLabels
			scopeIDSelect.Enable()
			if len(serverLabels) > 0 {
				scopeIDSelect.SetSelected(serverLabels[0])
			}
		}
		scopeIDSelect.Refresh()
	}
	scopeSelect.SetSelected("global")

	allCheck := widget.NewCheck(m.app.T("webhooks.dialog.all_events_check"), nil)
	eventChecks := make(map[string]*widget.Check)
	sectionsBox := container.NewVBox()
	for _, cat := range webhook.EventCategories {
		cat := cat
		header := widget.NewLabelWithStyle(cat.Name, fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
		sectionToggle := widget.NewCheck(m.app.T("webhooks.dialog.section_toggle_label"), nil)
		sectionEventChecks := make([]*widget.Check, 0, len(cat.Events))
		eventsBox := container.NewVBox()
		for _, ev := range cat.Events {
			ev := ev
			c := widget.NewCheck(ev, nil)
			eventChecks[ev] = c
			sectionEventChecks = append(sectionEventChecks, c)
			eventsBox.Add(c)
		}
		sectionToggle.OnChanged = func(checked bool) {
			for _, c := range sectionEventChecks {
				if !c.Disabled() {
					c.SetChecked(checked)
				}
			}
		}
		sectionsBox.Add(container.NewBorder(nil, nil, nil, sectionToggle, header))
		sectionsBox.Add(eventsBox)
		sectionsBox.Add(widget.NewSeparator())
	}
	checksScroll := container.NewVScroll(sectionsBox)
	checksScroll.SetMinSize(fyne.NewSize(0, 380))
	allCheck.OnChanged = func(checked bool) {
		for _, c := range eventChecks {
			if checked {
				c.SetChecked(false)
				c.Disable()
			} else {
				c.Enable()
			}
		}
	}

	// Pre-populate when editing.
	if existing != nil {
		nameEntry.SetText(existing.Name)
		urlEntry.SetText(existing.URL)
		enabledCheck.SetChecked(existing.Enabled)
		scopeSelect.SetSelected(existing.Scope.Type)
		switch existing.Scope.Type {
		case "cluster":
			for label, id := range clusterIDByLabel {
				if id == existing.Scope.ID {
					scopeIDSelect.SetSelected(label)
				}
			}
		case "server":
			for label, id := range serverIDByLabel {
				if id == existing.Scope.ID {
					scopeIDSelect.SetSelected(label)
				}
			}
		}
		hasWildcard := false
		for _, ev := range existing.EventMask {
			if ev == webhook.AllEventsWildcard {
				hasWildcard = true
			} else if c, ok := eventChecks[ev]; ok {
				c.SetChecked(true)
			}
		}
		allCheck.SetChecked(hasWildcard)
	}

	title := m.app.T("webhooks.dialog.title_add")
	if existing != nil {
		title = m.app.T("webhooks.dialog.title_edit")
	}

	form := container.NewVBox(
		widget.NewForm(
			widget.NewFormItem(m.app.T("webhooks.dialog.field_name"), nameEntry),
			widget.NewFormItem(m.app.T("webhooks.dialog.field_url"), urlEntry),
			widget.NewFormItem(m.app.T("webhooks.dialog.field_enabled"), enabledCheck),
			widget.NewFormItem(m.app.T("webhooks.dialog.field_scope"), scopeSelect),
			widget.NewFormItem(m.app.T("webhooks.dialog.field_scope_target"), scopeIDSelect),
		),
		widget.NewSeparator(),
		widget.NewLabelWithStyle(m.app.T("webhooks.dialog.events_section_title"), fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		allCheck,
		widget.NewSeparator(),
		checksScroll,
	)

	dlg := dialog.NewCustomConfirm(title, m.app.T("common.save"), m.app.T("common.cancel"), form, func(ok bool) {
		if !ok {
			return
		}
		name := strings.TrimSpace(nameEntry.Text)
		url := strings.TrimSpace(urlEntry.Text)
		if name == "" || url == "" {
			dialog.ShowError(fmt.Errorf("%s", m.app.T("webhooks.dialog.err_name_url_required")), m.win)
			return
		}
		scope := webhook.Scope{Type: scopeSelect.Selected}
		switch scope.Type {
		case "cluster":
			scope.ID = clusterIDByLabel[scopeIDSelect.Selected]
		case "server":
			scope.ID = serverIDByLabel[scopeIDSelect.Selected]
		}
		if (scope.Type == "cluster" || scope.Type == "server") && scope.ID == 0 {
			dialog.ShowError(fmt.Errorf(m.app.T("webhooks.dialog.err_pick_scope_target"), scope.Type), m.win)
			return
		}

		var mask []string
		if allCheck.Checked {
			mask = []string{webhook.AllEventsWildcard}
		} else {
			for _, ev := range webhook.SubscribableEvents {
				if eventChecks[ev].Checked {
					mask = append(mask, ev)
				}
			}
		}
		if len(mask) == 0 {
			dialog.ShowError(fmt.Errorf("%s", m.app.T("webhooks.dialog.err_no_events")), m.win)
			return
		}

		wh := webhook.Webhook{
			Name:      name,
			URL:       url,
			Scope:     scope,
			EventMask: mask,
			Enabled:   enabledCheck.Checked,
		}
		if existing != nil {
			wh.ID = existing.ID
			wh.Templates = existing.Templates
			if err := m.app.deps.Webhooks.Update(m.app.ctx(), wh); err != nil {
				dialog.ShowError(err, m.win)
				return
			}
		} else {
			if _, err := m.app.deps.Webhooks.Create(m.app.ctx(), wh); err != nil {
				dialog.ShowError(err, m.win)
				return
			}
		}
		m.refresh()
	}, m.win)
	dlg.Resize(fyne.NewSize(640, 620))
	dlg.Show()
}

func (m *webhooksManager) loadClusterOptions() (labels []string, byLabel map[string]int64) {
	byLabel = map[string]int64{}
	list, err := m.app.deps.Clusters.List(m.app.ctx())
	if err != nil {
		return nil, byLabel
	}
	for _, c := range list {
		label := fmt.Sprintf("%s (#%d)", c.Name, c.ID)
		labels = append(labels, label)
		byLabel[label] = c.ID
	}
	return labels, byLabel
}

func (m *webhooksManager) loadServerOptions() (labels []string, byLabel map[string]int64) {
	byLabel = map[string]int64{}
	list, err := m.app.deps.Servers.ListAll(m.app.ctx())
	if err != nil {
		return nil, byLabel
	}
	for _, s := range list {
		label := fmt.Sprintf("%s (#%d)", s.Name, s.ID)
		labels = append(labels, label)
		byLabel[label] = s.ID
	}
	return labels, byLabel
}
