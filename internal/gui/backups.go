package gui

import (
	"errors"
	"fmt"
	"path/filepath"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"

	"asamanager/internal/backup"
	"asamanager/internal/cluster"
	"asamanager/internal/events"
	"asamanager/internal/server"
)

// showServerBackups opens the backups manager for one server
func (a *App) showServerBackups(srv server.Server) {
	a.showBackupsWindow(
		backup.Scope{Type: "server", ID: srv.ID},
		a.T("backup.window.title_server", srv.Name),
		func(b backup.Backup) error {
			if err := a.deps.BackupManager.RestoreServer(a.ctx(), b, srv); err != nil {
				return err
			}
			a.publish(events.ServerBackupRestored{
				ServerID: srv.ID, ClusterID: srv.ClusterID, Name: srv.Name,
				Path: b.Path, At: time.Now(),
			})
			return nil
		},
		func() error { _, err := a.deps.BackupManager.BackupServer(a.ctx(), srv, "manual"); return err },
	)
}

// showClusterBackups opens the backups manager for one cluster
func (a *App) showClusterBackups(c cluster.Cluster) {
	a.showBackupsWindow(
		backup.Scope{Type: "cluster", ID: c.ID},
		a.T("backup.window.title_cluster", c.Name),
		func(b backup.Backup) error {
			if err := a.deps.BackupManager.RestoreCluster(a.ctx(), b, c); err != nil {
				return err
			}
			a.publish(events.ClusterBackupRestored{
				ClusterID: c.ID, Name: c.Name, Path: b.Path, At: time.Now(),
			})
			return nil
		},
		func() error { _, err := a.deps.BackupManager.BackupCluster(a.ctx(), c, "manual"); return err },
	)
}

func (a *App) showBackupsWindow(scope backup.Scope, title string, onRestore func(backup.Backup) error, onBackup func() error) {
	win := a.fyne.NewWindow(a.T("backup.window.window_title", title))
	win.Resize(fyne.NewSize(880, 520))

	listBox := container.NewVBox()
	var refresh func()
	refresh = func() {
		list, err := a.deps.BackupRepo.List(a.ctx(), scope)
		if err != nil {
			dialog.ShowError(err, win)
			return
		}
		listBox.Objects = nil
		if len(list) == 0 {
			listBox.Add(widget.NewLabelWithStyle(a.T("backup.window.empty"),
				fyne.TextAlignCenter, fyne.TextStyle{Italic: true}))
			listBox.Refresh()
			return
		}
		for _, b := range list {
			b := b
			meta := widget.NewLabel(a.T("backup.window.meta",
				b.CreatedAt.Local().Format("2006-01-02 15:04:05"),
				humanizeBytes(b.SizeBytes),
				b.Kind,
				filepath.Base(b.Path)))
			restoreBtn := widget.NewButton(a.T("common.restore"), func() {
				dialog.NewConfirm(a.T("backup.window.confirm_restore_title"),
					a.T("backup.window.confirm_restore_body", filepath.Base(b.Path)),
					func(yes bool) {
						if !yes {
							return
						}
						go func() {
							err := onRestore(b)
							fyne.Do(func() {
								switch {
								case err == nil:
									dialog.ShowInformation(a.T("backup.window.restore_done_title"),
										a.T("backup.window.restore_done_body", filepath.Base(b.Path)), win)
								case errors.Is(err, backup.ErrBackupEmpty):
									dialog.ShowInformation(a.T("backup.window.restore_empty_title"),
										a.T("backup.window.restore_empty_body"), win)
								default:
									dialog.ShowError(err, win)
								}
							})
						}()
					}, win).Show()
			})
			deleteBtn := widget.NewButton(a.T("common.delete"), func() {
				dialog.NewConfirm(a.T("backup.window.confirm_delete_title"),
					a.T("backup.window.confirm_delete_body", filepath.Base(b.Path)),
					func(yes bool) {
						if !yes {
							return
						}
						if err := a.deps.BackupManager.DeleteBackup(a.ctx(), b); err != nil {
							dialog.ShowError(err, win)
							return
						}
						refresh()
					}, win).Show()
			})
			row := container.NewBorder(nil, nil, nil, container.NewHBox(restoreBtn, deleteBtn), meta)
			listBox.Add(row)
			listBox.Add(widget.NewSeparator())
		}
		listBox.Refresh()
	}

	backupBtn := widget.NewButton(a.T("backup.window.button_now"), func() {
		go func() {
			if err := onBackup(); err != nil {
				fyne.Do(func() { dialog.ShowError(err, win) })
				return
			}
			fyne.Do(refresh)
		}()
	})
	backupBtn.Importance = widget.HighImportance
	refreshBtn := widget.NewButton(a.T("common.refresh"), refresh)

	header := widget.NewLabelWithStyle(title, fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	win.SetContent(container.NewBorder(
		container.NewVBox(header, container.NewHBox(backupBtn, refreshBtn)),
		nil, nil, nil,
		container.NewVScroll(listBox),
	))
	refresh()
	win.Show()
}

func humanizeBytes(n int64) string {
	const k = 1024
	switch {
	case n < k:
		return fmt.Sprintf("%d B", n)
	case n < k*k:
		return fmt.Sprintf("%.1f KB", float64(n)/k)
	case n < k*k*k:
		return fmt.Sprintf("%.1f MB", float64(n)/(k*k))
	default:
		return fmt.Sprintf("%.2f GB", float64(n)/(k*k*k))
	}
}
