package gui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"asamanager/internal/settings"
)

type settingEditorEntry struct {
	setting settings.Setting
	read    func() (settings.Value, error)
}

func buildPerStatGrid(a *App, values map[string]settings.Value, reg *settings.Registry) (fyne.CanvasObject, []settingEditorEntry) {
	groups := settings.PerStatGroupCatalog()
	labels := settings.PerStatLabels()

	cols := 1 + len(groups)
	grid := container.NewGridWithColumns(cols)

	bold := func(s string) *widget.Label {
		l := widget.NewLabel(s)
		l.TextStyle = fyne.TextStyle{Bold: true}
		return l
	}
	grid.Add(bold(a.T("settings.grid.header_stat")))
	for _, g := range groups {
		grid.Add(bold(g.Display))
	}

	editors := make([]settingEditorEntry, 0, len(labels)*len(groups))
	for i, label := range labels {
		grid.Add(widget.NewLabel(label))
		for _, g := range groups {
			key := settings.PerStatKey(g.Suffix, i)
			sp, ok := reg.Lookup(key)
			if !ok {
				grid.Add(widget.NewLabel("?"))
				continue
			}
			cur, has := values[key]
			if !has {
				cur = sp.DefaultValue()
			}
			ed := buildEditorFor(sp, cur)
			editors = append(editors, settingEditorEntry{setting: sp, read: ed.read})
			grid.Add(ed.widget)
		}
	}
	return container.NewVScroll(grid), editors
}
