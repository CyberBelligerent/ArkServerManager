package gui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"asamanager/internal/i18n"
)

// showAppPreferencesDialog opens the global app preferences
// Will add more in a future release
func (a *App) showAppPreferencesDialog() {
	available := i18n.AvailableLocales(a.deps.LanguageDir)

	localeSel := widget.NewSelect(available, nil)
	current := a.deps.Config.Locale
	if current == "" {
		current = a.deps.Bundle.Locale()
	}
	localeSel.SetSelected(current)

	restartNote := widget.NewLabelWithStyle(
		a.T("settings.locale.restart_note"),
		fyne.TextAlignLeading, fyne.TextStyle{Italic: true})

	form := widget.NewForm(
		widget.NewFormItem(
			a.T("settings.locale.section_header"),
			widget.NewLabelWithStyle("", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})),
		widget.NewFormItem(a.T("settings.locale.picker_label"), localeSel),
		widget.NewFormItem("", restartNote),
	)

	body := container.NewVBox(form)

	a.showFormDialog(
		a.T("preferences.window_title"),
		a.T("common.save"),
		body,
		fyne.NewSize(520, 320),
		func() error {
			cfg := a.deps.Config
			cfg.Locale = localeSel.Selected
			if err := a.deps.SaveConfig(cfg); err != nil {
				return err
			}
			a.deps.Config = cfg
			a.showInfo(
				a.T("preferences.window_title"),
				a.T("settings.locale.saved_info"))
			return nil
		})
}
