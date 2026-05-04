package gui

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"asamanager/internal/cluster"
	"asamanager/internal/events"
	"asamanager/internal/server"
	"asamanager/internal/steamcmd"
)

type installPlan struct {
	servers    []server.Server
	validate   bool
	clusterCtx *cluster.Cluster
}

// runInstall opens a non-modal window that streams SteamCMD output
func (a *App) runInstall(plan installPlan) {
	if a.deps.SteamCMD == nil || a.deps.SteamCMD.Path() == "" {
		a.showError(fmt.Errorf("%s", a.T("install.err_no_steamcmd")))
		return
	}
	if len(plan.servers) == 0 {
		a.showInfo(a.T("install.empty_title"), a.T("install.empty_body"))
		return
	}

	titleText := titleFor(a, plan.servers)
	winTitle := a.T("install.window_title_many")
	if len(plan.servers) == 1 {
		winTitle = a.T("install.window_title_one", plan.servers[0].Name)
	}

	win := a.fyne.NewWindow(winTitle)
	win.Resize(fyne.NewSize(900, 560))

	title := widget.NewLabelWithStyle(titleText, fyne.TextAlignLeading, fyne.TextStyle{Bold: true})

	body := widget.NewTextGrid()
	bodyScroll := container.NewScroll(body)
	bodyScroll.SetMinSize(fyne.NewSize(880, 460))

	cancelBtn := widget.NewButton(a.T("common.cancel"), nil)
	closeBtn := widget.NewButton(a.T("common.close"), func() { win.Close() })

	win.SetContent(container.NewBorder(
		title,
		container.NewHBox(cancelBtn, closeBtn),
		nil, nil,
		withTextPaneBackground(bodyScroll),
	))

	const maxLines = 1000
	var (
		mu       sync.Mutex
		lines    []string
		dirty    bool
		lastSeen = time.Now()
	)
	appendLine := func(s string) {
		mu.Lock()
		lines = append(lines, s)
		if len(lines) > maxLines {
			lines = lines[len(lines)-maxLines:]
		}
		dirty = true
		lastSeen = time.Now()
		mu.Unlock()
	}

	appendHeartbeat := func(s string) {
		mu.Lock()
		lines = append(lines, s)
		if len(lines) > maxLines {
			lines = lines[len(lines)-maxLines:]
		}
		dirty = true
		mu.Unlock()
	}
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

	ctx, cancel := context.WithCancel(context.Background())
	cancelBtn.OnTapped = func() {
		appendLine(a.T("install.log_cancel_requested"))
		cancel()
	}
	win.SetCloseIntercept(func() {
		cancel()
		win.Close()
	})

	win.Show()

	flushStop := make(chan struct{})
	go func() {
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-flushStop:
				flush()
				return
			case <-ticker.C:
				flush()
			}
		}
	}()

	heartbeatStop := make(chan struct{})
	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-heartbeatStop:
				return
			case now := <-ticker.C:
				mu.Lock()
				since := now.Sub(lastSeen)
				mu.Unlock()
				if since >= 5*time.Second {
					appendHeartbeat(a.T("install.heartbeat_silent", int(since.Seconds())))
				}
			}
		}
	}()

	go func() {
		defer close(flushStop)
		defer close(heartbeatStop)
		defer fyne.Do(func() { cancelBtn.Disable() })
		successes := 0
		failures := 0
		for i, s := range plan.servers {
			if ctx.Err() != nil {
				appendLine(a.T("install.log_cancelled"))
				return
			}
			appendLine(a.T("install.log_section",
				i+1, len(plan.servers), s.Name, steamcmd.AppIDARKAscended, s.InstallDir))
			appendLine(a.T("install.log_launching"))
			ch, err := a.deps.SteamCMD.InstallApp(ctx, steamcmd.AppIDARKAscended, s.InstallDir, plan.validate)
			if err != nil {
				appendLine(a.T("install.log_error", err))
				a.deps.Log.Error("steamcmd install", "server_id", s.ID, "err", err)
				failures++
				a.publish(events.ServerInstallUpdateFinished{
					ServerID: s.ID, ClusterID: s.ClusterID, Name: s.Name,
					Success: false, Err: err.Error(), At: time.Now(),
				})
				continue
			}
			sawError := false
			for line := range ch {
				appendLine(line)
				if strings.HasPrefix(line, "[exit]") || strings.HasPrefix(line, "[error]") {
					sawError = true
				}
			}
			appendLine(a.T("install.log_done_section", i+1, len(plan.servers), s.Name))
			if sawError {
				failures++
				a.publish(events.ServerInstallUpdateFinished{
					ServerID: s.ID, ClusterID: s.ClusterID, Name: s.Name,
					Success: false, Err: a.T("install.err_steamcmd_failed"), At: time.Now(),
				})
			} else {
				successes++
				a.publish(events.ServerInstallUpdateFinished{
					ServerID: s.ID, ClusterID: s.ClusterID, Name: s.Name,
					Success: true, At: time.Now(),
				})
			}
		}
		appendLine(a.T("install.log_all_done"))
		if plan.clusterCtx != nil {
			a.publish(events.ClusterInstallUpdateAllFinished{
				ClusterID: plan.clusterCtx.ID, Name: plan.clusterCtx.Name,
				Count: len(plan.servers), SuccessCount: successes, FailedCount: failures,
				At: time.Now(),
			})
		}
	}()
}

func titleFor(a *App, servers []server.Server) string {
	if len(servers) == 1 {
		return a.T("install.title_one", steamcmd.AppIDARKAscended, servers[0].Name)
	}
	names := make([]string, 0, len(servers))
	for _, s := range servers {
		names = append(names, s.Name)
	}
	return a.T("install.title_many", steamcmd.AppIDARKAscended, len(servers), strings.Join(names, ", "))
}
