package gui

import (
	"fmt"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/storage"
	"fyne.io/fyne/v2/widget"

	"asamanager/internal/cluster"
	"asamanager/internal/events"
	"asamanager/internal/server"
	"asamanager/internal/settings"
	"asamanager/pkg/asa/maps"
)

func (a *App) showError(err error) {
	if err == nil {
		return
	}
	dialog.ShowError(err, a.window)
}

func (a *App) showInfo(title, msg string) {
	dialog.ShowInformation(title, msg, a.window)
}

func (a *App) confirm(title, msg string, onYes func()) {
	dialog.NewConfirm(title, msg, func(yes bool) {
		if yes {
			onYes()
		}
	}, a.window).Show()
}

func (a *App) showFormDialog(title, confirmText string, content fyne.CanvasObject, size fyne.Size, onConfirm func() error) {
	var dlg dialog.Dialog
	confirmBtn := widget.NewButton(confirmText, func() {
		if err := onConfirm(); err != nil {
			dialog.ShowError(err, a.window)
			return
		}
		dlg.Hide()
	})
	confirmBtn.Importance = widget.HighImportance
	cancelBtn := widget.NewButton(a.T("common.cancel"), func() { dlg.Hide() })

	buttonRow := container.NewBorder(nil, nil, nil, container.NewHBox(cancelBtn, confirmBtn), nil)
	body := container.NewBorder(nil, buttonRow, nil, nil, content)

	dlg = dialog.NewCustomWithoutButtons(title, body, a.window)
	dlg.Resize(size)
	dlg.Show()
}

func (a *App) showNewClusterDialog() {
	nameEntry := widget.NewEntry()
	nameEntry.SetPlaceHolder(a.T("newcluster.name_placeholder"))

	idEntry := widget.NewEntry()
	idEntry.SetPlaceHolder(a.T("newcluster.id_placeholder"))

	dirEntry := widget.NewEntry()
	defaultDir := filepath.Join(a.deps.Config.DataDir, "clusters")
	dirEntry.SetPlaceHolder(defaultDir)
	dirRow := entryWithBrowse(dirEntry, widget.NewButton(a.T("common.browse"), func() {
		showFolderPicker(a.window, func(p string) { dirEntry.SetText(p) })
	}))

	presetLabels, presetByLabel := buildPresetChoiceOptions(a)
	presetSel := widget.NewSelect(presetLabels, nil)
	presetSel.SetSelected(presetLabels[0]) // Solo is the default

	form := &widget.Form{
		Items: []*widget.FormItem{
			{Text: a.T("newcluster.field_name"), Widget: nameEntry},
			{Text: a.T("newcluster.field_id"), Widget: idEntry, HintText: a.T("newcluster.field_id_hint")},
			{Text: a.T("newcluster.field_dir"), Widget: dirRow, HintText: a.T("newcluster.field_dir_hint")},
			{Text: a.T("newcluster.field_preset"), Widget: presetSel},
		},
	}

	a.showFormDialog(a.T("newcluster.title"), a.T("common.create"), form, fyne.NewSize(560, 360), func() error {
		name := strings.TrimSpace(nameEntry.Text)
		if name == "" {
			return fmt.Errorf("%s", a.T("newcluster.err_name_required"))
		}
		clusterID := strings.TrimSpace(idEntry.Text)
		if clusterID == "" {
			clusterID = cluster.SuggestClusterID(name)
		}
		existing, err := a.deps.Clusters.ListClusterIDs(a.ctx())
		if err != nil {
			return err
		}
		if err := cluster.ValidateClusterID(clusterID, existing); err != nil {
			return err
		}
		dir := strings.TrimSpace(dirEntry.Text)
		if dir == "" {
			dir = filepath.Join(defaultDir, clusterID)
		}
		c := cluster.Cluster{Name: name, ClusterID: clusterID, ClusterDir: dir}
		switch presetByLabel[presetSel.Selected] {
		case presetChoiceSolo:
			c.Settings = settings.SoloRecommendedPreset()
		case presetChoiceNormal:
			c.Settings = settings.NormalPreset()
		}
		created, err := a.deps.Clusters.Create(a.ctx(), c)
		if err != nil {
			return err
		}
		a.exportASBCluster(created)
		a.publish(events.ClusterCreated{
			ClusterID: created.ID, Name: created.Name, ARKID: created.ClusterID, At: time.Now(),
		})
		a.refreshTree()
		return nil
	})
}

func (a *App) showNewServerDialog(preselectedCluster *cluster.Cluster) {
	standaloneLabel := a.T("newserver.cluster_standalone")
	clusterOptions := append([]string{standaloneLabel}, a.tree.ClusterIDs()...)

	clusterSelect := widget.NewSelect(clusterOptions, nil)
	if preselectedCluster != nil {
		clusterSelect.SetSelected(preselectedCluster.ClusterID)
	} else if len(clusterOptions) > 1 {
		clusterSelect.SetSelected(clusterOptions[1])
	} else {
		clusterSelect.SetSelected(standaloneLabel)
	}

	nameEntry := widget.NewEntry()
	nameEntry.SetPlaceHolder(a.T("newserver.name_placeholder"))

	mapOptions := make([]string, 0, len(maps.Known)+1)
	levelByLabel := map[string]string{}
	for _, m := range maps.Known {
		label := fmt.Sprintf("%s  (%s)", m.DisplayName, m.LevelName)
		mapOptions = append(mapOptions, label)
		levelByLabel[label] = m.LevelName
	}
	customLabel := a.T("newserver.custom_level_label")
	mapOptions = append(mapOptions, customLabel)

	levelEntry := widget.NewEntry()
	levelEntry.SetText(maps.Known[0].LevelName)
	mapSelect := widget.NewSelect(mapOptions, func(s string) {
		if s == customLabel {
			levelEntry.SetText("")
			return
		}
		if lvl, ok := levelByLabel[s]; ok {
			levelEntry.SetText(lvl)
		}
	})
	mapSelect.SetSelected(mapOptions[0])

	suggested, err := a.deps.Servers.SuggestPortsFromDB(a.ctx())
	if err != nil {
		a.showError(err)
		return
	}
	gameEntry := widget.NewEntry()
	gameEntry.SetText(fmt.Sprintf("%d", suggested.Game))
	queryEntry := widget.NewEntry()
	queryEntry.SetText(fmt.Sprintf("%d", suggested.Query))
	rconEntry := widget.NewEntry()
	rconEntry.SetText(fmt.Sprintf("%d", suggested.RCON))

	rconPwdEntry := widget.NewPasswordEntry()
	rconPwdEntry.SetPlaceHolder(a.T("newserver.rcon_placeholder"))

	anticheatCheck := widget.NewCheck("", nil)
	anticheatCheck.SetChecked(true)

	modsEntry := widget.NewEntry()
	modsEntry.SetPlaceHolder(a.T("newserver.field_mods_placeholder"))

	installDir := widget.NewEntry()
	installDir.SetPlaceHolder(filepath.Join(a.deps.Config.ServerInstallDir, "Island"))
	installDirRow := entryWithBrowse(installDir, widget.NewButton(a.T("common.browse"), func() {
		showFolderPicker(a.window, func(p string) { installDir.SetText(p) })
	}))

	form := &widget.Form{
		Items: []*widget.FormItem{
			{Text: a.T("newserver.field_cluster"), Widget: clusterSelect},
			{Text: a.T("newserver.field_name"), Widget: nameEntry, HintText: a.T("newserver.field_name_hint")},
			{Text: a.T("newserver.field_map"), Widget: mapSelect},
			{Text: a.T("newserver.field_level"), Widget: levelEntry, HintText: a.T("newserver.field_level_hint")},
			{Text: a.T("newserver.field_install_dir"), Widget: installDirRow, HintText: a.T("newserver.field_install_dir_hint")},
			{Text: a.T("newserver.field_game_port"), Widget: gameEntry},
			{Text: a.T("newserver.field_query_port"), Widget: queryEntry},
			{Text: a.T("newserver.field_rcon_port"), Widget: rconEntry},
			{Text: a.T("newserver.field_rcon_pass"), Widget: rconPwdEntry},
			{Text: a.T("newserver.field_anticheat"), Widget: anticheatCheck, HintText: a.T("newserver.field_anticheat_hint")},
			{Text: a.T("newserver.field_mods"), Widget: modsEntry, HintText: a.T("newserver.field_mods_hint")},
		},
	}

	a.showFormDialog(a.T("newserver.title"), a.T("common.create"), form, fyne.NewSize(600, 560), func() error {
		var (
			c        cluster.Cluster
			isStandalone = clusterSelect.Selected == standaloneLabel
		)
		if !isStandalone {
			var found bool
			c, found = a.tree.ClusterByClusterID(clusterSelect.Selected)
			if !found {
				return fmt.Errorf(a.T("newserver.err_cluster_not_found"), clusterSelect.Selected)
			}
		}
		name := strings.TrimSpace(nameEntry.Text)
		level := strings.TrimSpace(levelEntry.Text)
		if name == "" || level == "" {
			return fmt.Errorf("%s", a.T("newserver.err_name_level_required"))
		}
		dir := strings.TrimSpace(installDir.Text)
		if dir == "" {
			dir = filepath.Join(a.deps.Config.ServerInstallDir, name)
		}
		ports, err := parsePorts(gameEntry.Text, queryEntry.Text, rconEntry.Text)
		if err != nil {
			return err
		}
		existing, err := a.deps.Servers.CollectAllPorts(a.ctx())
		if err != nil {
			return err
		}
		if err := server.CheckPortConflict(ports, existing); err != nil {
			return err
		}
		s := server.Server{
			ClusterID:        c.ID, // 0 if standalone
			Name:             name,
			Map:              level,
			InstallDir:       dir,
			Ports:            ports,
			RCONPassword:     rconPwdEntry.Text,
			AnticheatEnabled: anticheatCheck.Checked,
			ActiveMods:       parseModIDs(modsEntry.Text),
		}
		created, err := a.deps.Servers.Create(a.ctx(), s)
		if err != nil {
			return err
		}
		// Sync settings to disk immediately
		effective := cluster.MergeSettings(c.Settings, created.SettingOverrides)
		if err := cluster.WriteServerINIs(created, effective, c.CustomLines); err != nil {
			a.deps.Log.Warn("write initial inis", "server_id", created.ID, "err", err)
		}

		a.exportASBServer(c, created)
		a.publish(events.ServerCreated{
			ServerID: created.ID, ClusterID: created.ClusterID, Name: created.Name, Map: created.Map, At: time.Now(),
		})
		a.refreshTree()
		return nil
	})
}

func parsePorts(game, query, rcon string) (server.PortTriple, error) {
	g, err := atoiTrim(game)
	if err != nil {
		return server.PortTriple{}, fmt.Errorf("game port: %w", err)
	}
	q, err := atoiTrim(query)
	if err != nil {
		return server.PortTriple{}, fmt.Errorf("query port: %w", err)
	}
	r, err := atoiTrim(rcon)
	if err != nil {
		return server.PortTriple{}, fmt.Errorf("rcon port: %w", err)
	}
	pt := server.PortTriple{Game: g, Query: q, RCON: r}
	if err := pt.Validate(); err != nil {
		return server.PortTriple{}, err
	}
	return pt, nil
}

func atoiTrim(s string) (int, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, fmt.Errorf("required")
	}
	var n int
	if _, err := fmt.Sscanf(s, "%d", &n); err != nil {
		return 0, fmt.Errorf("invalid number %q", s)
	}
	return n, nil
}

func formatRowItem(label, value string) *fyne.Container {
	return container.NewBorder(nil, nil,
		widget.NewLabelWithStyle(label, fyne.TextAlignTrailing, fyne.TextStyle{Bold: true}),
		nil,
		widget.NewLabel(value),
	)
}

func showFolderPicker(parent fyne.Window, onSelect func(string)) {
	dialog.ShowFolderOpen(func(uri fyne.ListableURI, err error) {
		if err != nil {
			dialog.ShowError(err, parent)
			return
		}
		if uri == nil {
			return
		}
		onSelect(normalizePickedPath(uri.Path()))
	}, parent)
}

func showFilePicker(parent fyne.Window, exts []string, onSelect func(string)) {
	fd := dialog.NewFileOpen(func(rc fyne.URIReadCloser, err error) {
		if err != nil {
			dialog.ShowError(err, parent)
			return
		}
		if rc == nil {
			return
		}
		defer rc.Close()
		onSelect(normalizePickedPath(rc.URI().Path()))
	}, parent)
	if len(exts) > 0 {
		fd.SetFilter(storage.NewExtensionFileFilter(exts))
	}
	fd.Show()
}

func normalizePickedPath(p string) string {
	if runtime.GOOS == "windows" && len(p) >= 3 && p[0] == '/' && p[2] == ':' {
		p = p[1:]
	}
	return filepath.Clean(p)
}

func entryWithBrowse(entry *widget.Entry, browse *widget.Button) *fyne.Container {
	return container.NewBorder(nil, nil, nil, browse, entry)
}
