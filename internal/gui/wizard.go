package gui

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"

	"asamanager/internal/cluster"
	"asamanager/internal/events"
	"asamanager/internal/i18n"
	"asamanager/internal/server"
	"asamanager/internal/settings"
	"asamanager/internal/steamcmd"
	"asamanager/pkg/asa/maps"
)

func (a *App) runWizard(onDone func()) {
	w := newWizard(a, onDone)
	w.show()
}

func (w *wizard) showError(err error)       { dialog.ShowError(err, w.win) }
func (w *wizard) showInfo(title, msg string) { dialog.ShowInformation(title, msg, w.win) }

type wizard struct {
	app    *App
	onDone func()

	win     fyne.Window
	body    *fyne.Container
	nextBtn *widget.Button
	backBtn *widget.Button

	step  int
	state wizardState
}

type wizardState struct {
	Locale string

	SteamCMDPath     string
	ServerInstallDir string
	DiscordWebhook   string

	ClusterName  string
	ClusterID    string
	PresetChoice string // "solo" / "normal" / "none"

	ServerName string
	ServerMap  string
}

const (
	presetChoiceSolo   = "solo"
	presetChoiceNormal = "normal"
	presetChoiceNone   = "none"
)

func newWizard(a *App, onDone func()) *wizard {
	loc := a.deps.Config.Locale
	if loc == "" {
		loc = a.deps.Bundle.Locale()
	}
	return &wizard{
		app: a, onDone: onDone,
		state: wizardState{
			Locale:           loc,
			ServerInstallDir: a.deps.Config.ServerInstallDir,
			PresetChoice:     presetChoiceSolo,
			ClusterName:      "Solo Cluster",
			ServerName:       "Island",
			ServerMap:        "TheIsland_WP",
		},
	}
}

func (w *wizard) show() {
	w.win = w.app.fyne.NewWindow(w.app.T("wizard.window_title"))
	w.win.Resize(fyne.NewSize(640, 480))
	w.body = container.NewStack()
	w.backBtn = widget.NewButton(w.app.T("common.back"), w.back)
	w.nextBtn = widget.NewButton(w.app.T("common.next"), w.next)
	bottom := container.NewBorder(nil, nil, w.backBtn, w.nextBtn)
	w.win.SetContent(container.NewBorder(nil, bottom, nil, nil, w.body))
	w.renderStep()
	w.win.Show()
}

func (w *wizard) renderStep() {
	var content fyne.CanvasObject
	switch w.step {
	case 0:
		content = w.stepLocale()
	case 1:
		content = w.stepSteamCMD()
	case 2:
		content = w.stepInstallDir()
	case 3:
		content = w.stepDiscord()
	case 4:
		content = w.stepCluster()
	default:
		content = w.stepServer()
	}
	w.body.Objects = []fyne.CanvasObject{content}
	w.body.Refresh()

	w.backBtn.Enable()
	if w.step == 0 {
		w.backBtn.Disable()
	}
	if w.step == wizardLastStep {
		w.nextBtn.SetText(w.app.T("common.finish"))
	} else {
		w.nextBtn.SetText(w.app.T("common.next"))
	}
}

const wizardLastStep = 5

func (w *wizard) back() {
	if w.step > 0 {
		w.step--
		w.renderStep()
	}
}

func (w *wizard) next() {
	if !w.validateCurrentStep() {
		return
	}
	if w.step < wizardLastStep {
		w.step++
		w.renderStep()
		return
	}
	w.finish()
}

func (w *wizard) validateCurrentStep() bool {
	if w.step == 2 && strings.TrimSpace(w.state.ServerInstallDir) == "" {
		w.showError(fmt.Errorf("%s", w.app.T("wizard.installdir.err_required")))
		return false
	}
	return true
}

func (w *wizard) finish() {
	cfg := w.app.deps.Config
	cfg.Locale = w.state.Locale
	if w.state.SteamCMDPath != "" {
		cfg.SteamCMDPath = w.state.SteamCMDPath
	}
	cfg.ServerInstallDir = w.state.ServerInstallDir
	cfg.Discord.WebhookEnabled = strings.TrimSpace(w.state.DiscordWebhook) != ""
	cfg.FirstRunDone = true
	if err := w.app.deps.SaveConfig(cfg); err != nil {
		w.showError(err)
		return
	}
	w.app.deps.Config = cfg

	if wh := strings.TrimSpace(w.state.DiscordWebhook); wh != "" {
		if _, err := w.app.deps.DB.Exec(
			`INSERT INTO discord_webhooks(name, url, scope_type, scope_id, event_mask)
			 VALUES('default', ?, 'global', NULL, '[]')`,
			wh); err != nil {
			w.app.deps.Log.Warn("save discord webhook", "err", err)
		}
	}

	clusterName := strings.TrimSpace(w.state.ClusterName)
	serverName := strings.TrimSpace(w.state.ServerName)
	serverMap := strings.TrimSpace(w.state.ServerMap)

	if clusterName == "" {
		w.win.Close()
		if w.onDone != nil {
			w.onDone()
		}
		return
	}

	cid := strings.TrimSpace(w.state.ClusterID)
	if cid == "" {
		cid = cluster.SuggestClusterID(clusterName)
	}
	c := cluster.Cluster{
		Name:       clusterName,
		ClusterID:  cid,
		ClusterDir: filepath.Join(cfg.DataDir, "clusters", cid),
	}
	switch w.state.PresetChoice {
	case presetChoiceSolo:
		c.Settings = settings.SoloRecommendedPreset()
	case presetChoiceNormal:
		c.Settings = settings.NormalPreset()
	}
	created, err := w.app.deps.Clusters.Create(context.Background(), c)
	if err != nil {
		w.showError(err)
		return
	}
	w.app.exportASBCluster(created)
	w.app.publish(events.ClusterCreated{
		ClusterID: created.ID, Name: created.Name, ARKID: created.ClusterID, At: time.Now(),
	})

	if serverName != "" && serverMap != "" {
		suggested, err := w.app.deps.Servers.SuggestPortsFromDB(context.Background())
		if err != nil {
			w.showError(err)
			return
		}
		s := server.Server{
			ClusterID:  created.ID,
			Name:       serverName,
			Map:        serverMap,
			InstallDir: filepath.Join(cfg.ServerInstallDir, serverName),
			Ports:      suggested,
		}
		createdServer, err := w.app.deps.Servers.Create(context.Background(), s)
		if err != nil {
			w.showError(err)
			return
		}

		effective := cluster.MergeSettings(created.Settings, createdServer.SettingOverrides)
		if err := cluster.WriteServerINIs(createdServer, effective, created.CustomLines); err != nil {
			w.app.deps.Log.Warn("write initial inis", "server_id", createdServer.ID, "err", err)
		}
		w.app.exportASBServer(created, createdServer)
		w.app.publish(events.ServerCreated{
			ServerID: createdServer.ID, ClusterID: createdServer.ClusterID,
			Name: createdServer.Name, Map: createdServer.Map, At: time.Now(),
		})
	}

	w.win.Close()
	if w.onDone != nil {
		w.onDone()
	}
}

func (w *wizard) stepLocale() fyne.CanvasObject {
	title := widget.NewLabelWithStyle(w.app.T("wizard.locale.title"),
		fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	msg := widget.NewLabel(w.app.T("wizard.locale.description"))
	msg.Wrapping = fyne.TextWrapWord

	available := i18n.AvailableLocales(w.app.deps.LanguageDir)
	sel := widget.NewSelect(available, func(s string) { w.state.Locale = s })
	if w.state.Locale == "" {
		w.state.Locale = i18n.DefaultLocale
	}
	sel.SetSelected(w.state.Locale)

	form := widget.NewForm(widget.NewFormItem(w.app.T("wizard.locale.picker_label"), sel))
	return container.NewVBox(title, msg, form)
}

func (w *wizard) stepSteamCMD() fyne.CanvasObject {
	title := widget.NewLabelWithStyle(w.app.T("wizard.steamcmd.title"), fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	msg := widget.NewLabel(w.app.T("wizard.steamcmd.description"))

	pathEntry := widget.NewEntry()
	pathEntry.SetPlaceHolder(w.app.T("wizard.steamcmd.path_placeholder"))
	pathEntry.OnChanged = func(s string) { w.state.SteamCMDPath = s }
	if cur, _ := w.app.deps.SteamCMD.Detect(context.Background()); cur != "" {
		pathEntry.SetText(cur)
		w.state.SteamCMDPath = cur
	}
	browseBtn := widget.NewButton(w.app.T("common.browse"), func() {
		showFilePicker(w.win, []string{".exe"}, func(p string) {
			pathEntry.SetText(p)
			w.state.SteamCMDPath = p
		})
	})
	pathRow := entryWithBrowse(pathEntry, browseBtn)

	detectBtn := widget.NewButton(w.app.T("common.detect"), func() {
		if cur, _ := w.app.deps.SteamCMD.Detect(context.Background()); cur != "" {
			pathEntry.SetText(cur)
			w.state.SteamCMDPath = cur
			w.showInfo(w.app.T("wizard.steamcmd.detect_title"), w.app.T("wizard.steamcmd.detect_found", cur))
		} else {
			w.showInfo(w.app.T("wizard.steamcmd.detect_title"), w.app.T("wizard.steamcmd.detect_none"))
		}
	})
	progressLabel := widget.NewLabel("")
	progressLabel.Wrapping = fyne.TextWrapWord

	installBtn := widget.NewButton(w.app.T("wizard.steamcmd.button_install"), nil)
	installBtn.OnTapped = func() {
		dir, _ := steamcmd.DefaultInstallDir()
		installBtn.Disable()
		detectBtn.Disable()
		fyne.Do(func() { progressLabel.SetText(w.app.T("wizard.steamcmd.progress_starting")) })
		go func() {
			defer fyne.Do(func() {
				installBtn.Enable()
				detectBtn.Enable()
			})
			w.app.deps.SteamCMD.SetProgress(func(msg string) {
				fyne.Do(func() { progressLabel.SetText(msg) })
			})
			defer w.app.deps.SteamCMD.SetProgress(nil)
			if err := w.app.deps.SteamCMD.Install(context.Background(), dir); err != nil {
				fyne.Do(func() { progressLabel.SetText("") })
				w.showError(err)
				return
			}
			fyne.Do(func() {
				progressLabel.SetText("")
				pathEntry.SetText(w.app.deps.SteamCMD.Path())
				w.state.SteamCMDPath = w.app.deps.SteamCMD.Path()
				w.showInfo(w.app.T("wizard.steamcmd.install_done_title"), w.app.T("wizard.steamcmd.install_done_body", dir))
			})
		}()
	}

	return container.NewVBox(title, msg, pathRow, container.NewHBox(detectBtn, installBtn), progressLabel)
}

func (w *wizard) stepInstallDir() fyne.CanvasObject {
	title := widget.NewLabelWithStyle(w.app.T("wizard.installdir.title"), fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	msg := widget.NewLabel(w.app.T("wizard.installdir.description"))
	entry := widget.NewEntry()
	entry.SetText(w.state.ServerInstallDir)
	entry.OnChanged = func(s string) { w.state.ServerInstallDir = s }
	browse := widget.NewButton(w.app.T("common.browse"), func() {
		showFolderPicker(w.win, func(p string) {
			entry.SetText(p)
			w.state.ServerInstallDir = p
		})
	})
	return container.NewVBox(title, msg, entryWithBrowse(entry, browse))
}

func (w *wizard) stepDiscord() fyne.CanvasObject {
	title := widget.NewLabelWithStyle(w.app.T("wizard.discord.title"), fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	msg := widget.NewLabel(w.app.T("wizard.discord.description"))
	entry := widget.NewPasswordEntry()
	entry.SetText(w.state.DiscordWebhook)
	entry.OnChanged = func(s string) { w.state.DiscordWebhook = s }
	return container.NewVBox(title, msg, entry)
}

func (w *wizard) stepCluster() fyne.CanvasObject {
	title := widget.NewLabelWithStyle(w.app.T("wizard.cluster.title"), fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	msg := widget.NewLabel(w.app.T("wizard.cluster.description"))

	nameEntry := widget.NewEntry()
	nameEntry.SetText(w.state.ClusterName)
	nameEntry.OnChanged = func(s string) { w.state.ClusterName = s }

	idEntry := widget.NewEntry()
	idEntry.SetPlaceHolder(w.app.T("wizard.cluster.field_id_placeholder"))
	idEntry.SetText(w.state.ClusterID)
	idEntry.OnChanged = func(s string) { w.state.ClusterID = s }

	presetLabels, presetByLabel := buildPresetChoiceOptions(w.app)
	presetSel := widget.NewSelect(presetLabels, func(s string) {
		w.state.PresetChoice = presetByLabel[s]
	})
	for label, value := range presetByLabel {
		if value == w.state.PresetChoice {
			presetSel.SetSelected(label)
			break
		}
	}

	form := widget.NewForm(
		widget.NewFormItem(w.app.T("wizard.cluster.field_name"), nameEntry),
		widget.NewFormItem(w.app.T("wizard.cluster.field_id"), idEntry),
		widget.NewFormItem(w.app.T("wizard.cluster.field_preset"), presetSel),
	)
	return container.NewVBox(title, msg, form)
}

func buildPresetChoiceOptions(a *App) ([]string, map[string]string) {
	labels := []string{
		a.T("preset_choice.solo"),
		a.T("preset_choice.normal"),
		a.T("preset_choice.none"),
	}
	values := map[string]string{
		labels[0]: presetChoiceSolo,
		labels[1]: presetChoiceNormal,
		labels[2]: presetChoiceNone,
	}
	return labels, values
}

func (w *wizard) stepServer() fyne.CanvasObject {
	title := widget.NewLabelWithStyle(w.app.T("wizard.server.title"), fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	msg := widget.NewLabel(w.app.T("wizard.server.description"))

	nameEntry := widget.NewEntry()
	nameEntry.SetText(w.state.ServerName)
	nameEntry.OnChanged = func(s string) { w.state.ServerName = s }

	mapOptions := make([]string, 0, len(maps.Known))
	levelByLabel := map[string]string{}
	for _, m := range maps.Known {
		label := m.DisplayName + " (" + m.LevelName + ")"
		mapOptions = append(mapOptions, label)
		levelByLabel[label] = m.LevelName
	}
	levelEntry := widget.NewEntry()
	levelEntry.SetText(w.state.ServerMap)
	levelEntry.OnChanged = func(s string) { w.state.ServerMap = s }
	mapSelect := widget.NewSelect(mapOptions, func(s string) {
		if lvl, ok := levelByLabel[s]; ok {
			levelEntry.SetText(lvl)
			w.state.ServerMap = lvl
		}
	})
	mapSelect.SetSelected(mapOptions[0])

	form := widget.NewForm(
		widget.NewFormItem(w.app.T("wizard.server.field_name"), nameEntry),
		widget.NewFormItem(w.app.T("wizard.server.field_map"), mapSelect),
		widget.NewFormItem(w.app.T("wizard.server.field_level"), levelEntry),
	)
	return container.NewVBox(title, msg, form)
}
