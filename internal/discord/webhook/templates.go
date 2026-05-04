package webhook

import (
	"bytes"
	"fmt"
	"path/filepath"
	"text/template"
	"time"

	"asamanager/internal/events"
)

func baseName(p string) string { return filepath.Base(p) }

const AllEventsWildcard = "*"

const (
	colorSuccess  = 0x57F287 // green
	colorProgress = 0xFEE75C // yellow
	colorInfo     = 0x5865F2 // blurple
	colorWarning  = 0xFF8C00 // orange
	colorError    = 0xED4245 // red
)

type EventCategory struct {
	Name   string
	Events []string
}

var EventCategories = []EventCategory{
	{"Server Lifecycle", []string{
		events.ServerStarting{}.EventName(),
		events.ServerStarted{}.EventName(),
		events.ServerStopped{}.EventName(),
		events.ServerCrashed{}.EventName(),
		events.ServerSaved{}.EventName(),
	}},
	{"Server Management", []string{
		events.ServerCreated{}.EventName(),
		events.ServerDeleted{}.EventName(),
		events.ServerInstallUpdate{}.EventName(),
		events.ServerInstallUpdateFinished{}.EventName(),
		events.ServerSettingsChanged{}.EventName(),
		events.ServerSettingsSaved{}.EventName(),
	}},
	{"Cluster Management", []string{
		events.ClusterCreated{}.EventName(),
		events.ClusterDeleted{}.EventName(),
		events.ClusterInstallUpdateAll{}.EventName(),
		events.ClusterInstallUpdateAllFinished{}.EventName(),
	}},
	{"Cluster Settings", []string{
		events.ClusterSettingsChanged{}.EventName(),
		events.ClusterSettingsSaved{}.EventName(),
		events.ClusterSettingsApplied{}.EventName(),
	}},
	{"Players", []string{
		events.PlayerJoined{}.EventName(),
		events.PlayerLeft{}.EventName(),
		events.PlayerBanned{}.EventName(),
	}},
	{"Backups", []string{
		events.BackupStarted{}.EventName(),
		events.BackupCompleted{}.EventName(),
		events.BackupFailed{}.EventName(),
		events.ServerBackupRestored{}.EventName(),
		events.ClusterBackupRestored{}.EventName(),
	}},
	{"Schedule & Mods", []string{
		events.ScheduledTaskFired{}.EventName(),
		events.ActiveEventApplied{}.EventName(),
		events.ActiveEventCleared{}.EventName(),
		events.RestartChurnWarning{}.EventName(),
		events.ModUpdateAvailable{}.EventName(),
	}},
}

var SubscribableEvents = func() []string {
	var out []string
	for _, cat := range EventCategories {
		out = append(out, cat.Events...)
	}
	return out
}()

func SubscribesTo(mask []string, eventName string) bool {
	for _, m := range mask {
		if m == AllEventsWildcard || m == eventName {
			return true
		}
	}
	return false
}

func Render(eventName string, evt events.Event, overrides map[string]string) (Message, error) {
	embed, ok := embedFor(eventName, evt)
	if !ok {
		return Message{}, fmt.Errorf("no embed for %s", eventName)
	}
	if tmplStr, ok := overrides[eventName]; ok && tmplStr != "" {
		rendered, err := executeTemplate(eventName, tmplStr, evt)
		if err == nil {
			embed.Description = rendered
		}
	}
	return Message{Embeds: []Embed{embed}}, nil
}

// executeTemplate runs tmplStr against evt using text/template syntax.
func executeTemplate(name, tmplStr string, evt events.Event) (string, error) {
	tmpl, err := template.New(name).Parse(tmplStr)
	if err != nil {
		return "", fmt.Errorf("parse %s template: %w", name, err)
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, evt); err != nil {
		return "", fmt.Errorf("execute %s template: %w", name, err)
	}
	return buf.String(), nil
}

func serverFooter(serverID int64) string {
	if serverID == 0 {
		return ""
	}
	return fmt.Sprintf("Server #%d", serverID)
}

func clusterFooter(clusterID int64) string {
	if clusterID == 0 {
		return ""
	}
	return fmt.Sprintf("Cluster #%d", clusterID)
}

func embedFor(name string, evt events.Event) (Embed, bool) {
	switch v := evt.(type) {
	// Server lifecycle
	case events.ServerStarting:
		return Embed{Title: "Server Starting", Description: fmt.Sprintf("**%s** is starting up...", v.Name),
			Color: colorProgress, Footer: serverFooter(v.ServerID), Timestamp: v.At}, true
	case events.ServerStarted:
		return Embed{Title: "Server Ready", Description: fmt.Sprintf("**%s** is up and accepting joins.", v.Name),
			Color: colorSuccess, Footer: serverFooter(v.ServerID), Timestamp: v.At}, true
	case events.ServerStopped:
		return Embed{Title: "Server Stopped", Description: fmt.Sprintf("**%s** has stopped.", v.Name),
			Color: colorWarning, Footer: serverFooter(v.ServerID), Timestamp: v.At}, true
	case events.ServerCrashed:
		return Embed{Title: "Server Crashed", Description: fmt.Sprintf("**%s** crashed.", v.Name),
			Color: colorError, Footer: serverFooter(v.ServerID), Timestamp: v.At,
			Fields: []EmbedField{{Name: "Exit code", Value: fmt.Sprintf("`%d`", v.ExitCode), Inline: true}}}, true
	case events.ServerSaved:
		return Embed{Title: "World Saved", Description: fmt.Sprintf("**%s** saved world.", v.Name),
			Color: colorInfo, Footer: serverFooter(v.ServerID), Timestamp: v.At}, true

	// Server management
	case events.ServerCreated:
		return Embed{Title: "Server Created", Description: fmt.Sprintf("**%s** created.", v.Name),
			Color: colorSuccess, Footer: serverFooter(v.ServerID), Timestamp: v.At,
			Fields: []EmbedField{{Name: "Map", Value: v.Map, Inline: true}}}, true
	case events.ServerDeleted:
		return Embed{Title: "Server Deleted", Description: fmt.Sprintf("**%s** deleted.", v.Name),
			Color: colorError, Footer: serverFooter(v.ServerID), Timestamp: v.At}, true
	case events.ServerInstallUpdate:
		return Embed{Title: "Server Install/Update Started", Description: fmt.Sprintf("**%s** — install/update started via SteamCMD.", v.Name),
			Color: colorProgress, Footer: serverFooter(v.ServerID), Timestamp: v.At}, true
	case events.ServerInstallUpdateFinished:
		title := "Server Install/Update Finished"
		color := colorSuccess
		desc := fmt.Sprintf("**%s** — install/update completed.", v.Name)
		if !v.Success {
			color = colorError
			desc = fmt.Sprintf("**%s** — install/update FAILED.", v.Name)
		}
		em := Embed{Title: title, Description: desc, Color: color, Footer: serverFooter(v.ServerID), Timestamp: v.At}
		if !v.Success && v.Err != "" {
			em.Fields = []EmbedField{{Name: "Error", Value: "`" + v.Err + "`"}}
		}
		return em, true
	case events.ServerSettingsChanged:
		return Embed{Title: "Server Overrides Changed", Description: fmt.Sprintf("**%s** — settings overrides edited.", v.Name),
			Color: colorInfo, Footer: serverFooter(v.ServerID), Timestamp: v.At,
			Fields: []EmbedField{{Name: "Override count", Value: fmt.Sprintf("`%d`", v.Count), Inline: true}}}, true
	case events.ServerSettingsSaved:
		return Embed{Title: "Server Overrides Saved", Description: fmt.Sprintf("**%s** — settings overrides saved.", v.Name),
			Color: colorSuccess, Footer: serverFooter(v.ServerID), Timestamp: v.At,
			Fields: []EmbedField{{Name: "Override count", Value: fmt.Sprintf("`%d`", v.Count), Inline: true}}}, true

	// Cluster management
	case events.ClusterCreated:
		return Embed{Title: "Cluster Created", Description: fmt.Sprintf("**%s** created.", v.Name),
			Color: colorSuccess, Footer: clusterFooter(v.ClusterID), Timestamp: v.At,
			Fields: []EmbedField{{Name: "ARK cluster ID", Value: "`" + v.ARKID + "`", Inline: true}}}, true
	case events.ClusterDeleted:
		return Embed{Title: "Cluster Deleted", Description: fmt.Sprintf("**%s** deleted.", v.Name),
			Color: colorError, Footer: clusterFooter(v.ClusterID), Timestamp: v.At}, true
	case events.ClusterInstallUpdateAll:
		return Embed{Title: "Cluster Install/Update Started", Description: fmt.Sprintf("**%s** — bulk install/update started.", v.Name),
			Color: colorProgress, Footer: clusterFooter(v.ClusterID), Timestamp: v.At,
			Fields: []EmbedField{{Name: "Servers", Value: fmt.Sprintf("`%d`", v.Count), Inline: true}}}, true
	case events.ClusterInstallUpdateAllFinished:
		color := colorSuccess
		desc := fmt.Sprintf("**%s** — bulk install/update completed.", v.Name)
		if v.FailedCount > 0 {
			color = colorWarning
			desc = fmt.Sprintf("**%s** — bulk install/update completed with errors.", v.Name)
		}
		return Embed{Title: "Cluster Install/Update Finished", Description: desc, Color: color,
			Footer: clusterFooter(v.ClusterID), Timestamp: v.At,
			Fields: []EmbedField{
				{Name: "Succeeded", Value: fmt.Sprintf("`%d`", v.SuccessCount), Inline: true},
				{Name: "Failed", Value: fmt.Sprintf("`%d`", v.FailedCount), Inline: true},
				{Name: "Total", Value: fmt.Sprintf("`%d`", v.Count), Inline: true},
			}}, true

	// Cluster settings
	case events.ClusterSettingsChanged:
		return Embed{Title: "Cluster Settings Changed", Description: fmt.Sprintf("**%s** — settings edited.", v.Name),
			Color: colorInfo, Footer: clusterFooter(v.ClusterID), Timestamp: v.At,
			Fields: []EmbedField{{Name: "Non-default keys", Value: fmt.Sprintf("`%d`", v.Count), Inline: true}}}, true
	case events.ClusterSettingsSaved:
		return Embed{Title: "Cluster Settings Saved", Description: fmt.Sprintf("**%s** — settings saved.", v.Name),
			Color: colorSuccess, Footer: clusterFooter(v.ClusterID), Timestamp: v.At,
			Fields: []EmbedField{{Name: "Non-default keys", Value: fmt.Sprintf("`%d`", v.Count), Inline: true}}}, true
	case events.ClusterSettingsApplied:
		return Embed{Title: "Cluster Settings Applied", Description: fmt.Sprintf("**%s** — effective settings written to every server's .ini files.", v.Name),
			Color: colorInfo, Footer: clusterFooter(v.ClusterID), Timestamp: v.At}, true

	// Players
	case events.PlayerJoined:
		return Embed{Title: "Player Joined", Description: fmt.Sprintf("**%s** joined.", v.Name),
			Color: colorSuccess, Footer: serverFooter(v.ServerID), Timestamp: v.At,
			Fields: []EmbedField{{Name: "Steam ID", Value: "`" + v.SteamID + "`", Inline: true}}}, true
	case events.PlayerLeft:
		return Embed{Title: "Player Left", Description: fmt.Sprintf("Player `%s` left.", v.SteamID),
			Color: colorInfo, Footer: serverFooter(v.ServerID), Timestamp: v.At}, true
	case events.PlayerBanned:
		return Embed{Title: "Player Banned", Description: fmt.Sprintf("Banned `%s`.", v.SteamID),
			Color: colorError, Footer: serverFooter(v.ServerID), Timestamp: v.At,
			Fields: []EmbedField{{Name: "Reason", Value: nonEmpty(v.Reason, "(none)")}}}, true

	// Backups
	case events.BackupStarted:
		return Embed{Title: "Backup Started", Description: fmt.Sprintf("Backup started for %s #%d.", v.Scope, v.ScopeID),
			Color: colorProgress, Timestamp: v.At}, true
	case events.BackupCompleted:
		return Embed{Title: "Backup Completed", Description: fmt.Sprintf("Backup written: `%s`", v.Path),
			Color: colorSuccess, Timestamp: v.At,
			Fields: []EmbedField{{Name: "Size", Value: fmt.Sprintf("%d bytes", v.SizeBytes), Inline: true}}}, true
	case events.BackupFailed:
		return Embed{Title: "Backup Failed", Description: fmt.Sprintf("Backup failed for %s #%d.", v.Scope, v.ScopeID),
			Color: colorError, Timestamp: v.At,
			Fields: []EmbedField{{Name: "Error", Value: "`" + v.Err + "`"}}}, true
	case events.ServerBackupRestored:
		return Embed{Title: "Server Backup Restored",
			Description: fmt.Sprintf("**%s** — restored from `%s`.", v.Name, baseName(v.Path)),
			Color:       colorSuccess, Footer: serverFooter(v.ServerID), Timestamp: v.At}, true
	case events.ClusterBackupRestored:
		return Embed{Title: "Cluster Backup Restored",
			Description: fmt.Sprintf("**%s** — restored from `%s`.", v.Name, baseName(v.Path)),
			Color:       colorSuccess, Footer: clusterFooter(v.ClusterID), Timestamp: v.At}, true

	// Schedule & Mods
	case events.ScheduledTaskFired:
		return Embed{Title: "Scheduled Task Fired", Description: fmt.Sprintf("`%s` ran action `%s`.", v.TaskName, v.Action),
			Color: colorInfo, Timestamp: v.At}, true
	case events.ActiveEventApplied:
		return Embed{Title: "ActiveEvent Applied", Description: fmt.Sprintf("`%s` applied to %s #%d.", v.ARKEvent, v.Scope, v.ScopeID),
			Color: colorInfo, Timestamp: v.At}, true
	case events.ActiveEventCleared:
		return Embed{Title: "ActiveEvent Cleared", Description: fmt.Sprintf("ActiveEvent cleared on %s #%d.", v.Scope, v.ScopeID),
			Color: colorInfo, Timestamp: v.At}, true
	case events.RestartChurnWarning:
		return Embed{Title: "Restart Churn Warning",
			Description: fmt.Sprintf("Server #%d will restart %d times within the next %dh.", v.ServerID, v.Count, v.WindowHours),
			Color:       colorWarning, Footer: serverFooter(v.ServerID), Timestamp: v.At}, true
	case events.ModUpdateAvailable:
		return Embed{Title: "Mod Update Available",
			Description: fmt.Sprintf("Mod `%d` has a new version: `%s`", v.ModID, v.NewVersion),
			Color:       colorInfo, Footer: serverFooter(v.ServerID), Timestamp: v.At}, true
	}
	return Embed{}, false
}

func testEmbed(webhookName string) Embed {
	return Embed{
		Title:       "ASA Manager — Webhook Test",
		Description: fmt.Sprintf("Webhook **%s** is wired correctly.", webhookName),
		Color:       colorInfo,
		Timestamp:   time.Now(),
	}
}

func nonEmpty(s, fallback string) string {
	if s == "" {
		return fallback
	}
	return s
}
