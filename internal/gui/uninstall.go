package gui

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"

	"asamanager/internal/steamcmd"
)

func (a *App) showUninstallDialog() {
	plan := a.collectUninstallPlan()

	intro := widget.NewLabelWithStyle(
		a.T("uninstall.intro"),
		fyne.TextAlignLeading, fyne.TextStyle{Bold: true},
	)
	body := widget.NewTextGrid()
	body.SetText(planSummary(a, plan))
	bodyScroll := container.NewScroll(body)
	bodyScroll.SetMinSize(fyne.NewSize(620, 320))

	content := container.NewBorder(intro, nil, nil, nil, withTextPaneBackground(bodyScroll))

	dlg := dialog.NewCustomConfirm(
		a.T("uninstall.title"),
		a.T("uninstall.confirm_first"),
		a.T("common.cancel"),
		content,
		func(yes bool) {
			if !yes {
				return
			}

			a.confirm(
				a.T("uninstall.confirm_final_title"),
				a.T("uninstall.confirm_final_body"),
				func() { a.runUninstall(plan) },
			)
		},
		a.window,
	)
	dlg.Resize(fyne.NewSize(720, 480))
	dlg.Show()
}

type uninstallPlan struct {
	dataDir     string
	steamCMDDir string
	appHomeDir  string // <home>/ASAManager parent
	clusterDirs []string
	serverDirs  []string
}

func (a *App) collectUninstallPlan() uninstallPlan {
	plan := uninstallPlan{dataDir: a.deps.DataDir}
	if a.deps.SteamCMD != nil && a.deps.SteamCMD.Path() != "" {
		plan.steamCMDDir = filepath.Dir(a.deps.SteamCMD.Path())
		if def, err := steamcmd.DefaultInstallDir(); err == nil {
			if filepath.Clean(plan.steamCMDDir) == filepath.Clean(def) {
				plan.appHomeDir = filepath.Dir(def)
			}
		}
	}
	clusters, err := a.deps.Clusters.List(a.ctx())
	if err != nil {
		a.deps.Log.Error("uninstall: list clusters", "err", err)
		return plan
	}
	for _, c := range clusters {
		if c.ClusterDir != "" {
			plan.clusterDirs = append(plan.clusterDirs, c.ClusterDir)
		}
		servers, err := a.deps.Servers.ListByCluster(a.ctx(), c.ID)
		if err != nil {
			a.deps.Log.Error("uninstall: list servers", "cluster_id", c.ID, "err", err)
			continue
		}
		for _, s := range servers {
			if s.InstallDir != "" {
				plan.serverDirs = append(plan.serverDirs, s.InstallDir)
			}
		}
	}
	return plan
}

func planSummary(a *App, p uninstallPlan) string {
	var b strings.Builder
	if p.dataDir != "" {
		b.WriteString(a.T("uninstall.plan_data_dir", p.dataDir))
	}
	if p.steamCMDDir != "" {
		b.WriteString(a.T("uninstall.plan_steamcmd", p.steamCMDDir))
	}
	if p.appHomeDir != "" {
		b.WriteString(a.T("uninstall.plan_app_home", p.appHomeDir))
	}
	if len(p.clusterDirs) > 0 {
		b.WriteString(a.T("uninstall.plan_cluster_dirs"))
		for _, d := range p.clusterDirs {
			fmt.Fprintf(&b, "  %s\n", d)
		}
		b.WriteString("\n")
	}
	if len(p.serverDirs) > 0 {
		b.WriteString(a.T("uninstall.plan_server_dirs"))
		for _, d := range p.serverDirs {
			fmt.Fprintf(&b, "  %s\n", d)
		}
	}
	return b.String()
}

func (a *App) runUninstall(plan uninstallPlan) {
	a.deps.Log.Info("uninstall: starting", "clusters", len(plan.clusterDirs), "servers", len(plan.serverDirs))

	if a.deps.Supervisor != nil {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		if rs, ok := a.deps.Supervisor.(interface {
			Shutdown(context.Context, bool)
		}); ok {
			rs.Shutdown(shutdownCtx, false)
		}
		cancel()
	}

	time.Sleep(1 * time.Second)


	if a.deps.DB != nil {
		_ = a.deps.DB.Close()
	}

	if a.deps.LogCloser != nil {
		_ = a.deps.LogCloser.Close()
	}

	var paths []string
	paths = append(paths, plan.serverDirs...)
	paths = append(paths, plan.clusterDirs...)
	if plan.steamCMDDir != "" {
		paths = append(paths, plan.steamCMDDir)
	}
	if plan.appHomeDir != "" {
		paths = append(paths, plan.appHomeDir)
	}
	if plan.dataDir != "" {
		paths = append(paths, plan.dataDir)
	}

	var failed []string
	for _, p := range paths {
		if err := os.RemoveAll(p); err != nil {
			failed = append(failed, fmt.Sprintf("%s: %v", p, err))
		}
	}
	if len(failed) > 0 && a.deps.Log != nil {
		a.deps.Log.Warn("uninstall: some paths failed", "errors", failed)
	}

	a.fyne.Quit()
}
