package preset

import (
	"context"
	"fmt"
	"log/slog"

	"asamanager/internal/cluster"
	"asamanager/internal/events"
	"asamanager/internal/server"
	"asamanager/internal/settings"
)

type Manager struct {
	deps ManagerDeps
}

type ManagerDeps struct {
	Presets  *Repo
	Servers  *server.Repo
	Clusters *cluster.Repo
	Bus      *events.Bus
	Log      *slog.Logger
}

func NewManager(deps ManagerDeps) *Manager {
	if deps.Log == nil {
		deps.Log = slog.Default()
	}
	return &Manager{deps: deps}
}

// Apply merges the preset's diff into its target and writes affected INI files
func (m *Manager) Apply(ctx context.Context, p Preset) (string, error) {
	switch p.ScopeType {
	case ScopeCluster:
		return m.applyCluster(ctx, p)
	case ScopeServer:
		return m.applyServer(ctx, p)
	}
	return "", fmt.Errorf("unknown preset scope %q", p.ScopeType)
}

func (m *Manager) ApplyByID(ctx context.Context, id int64) (Preset, string, error) {
	p, err := m.deps.Presets.Get(ctx, id)
	if err != nil {
		return Preset{}, "", err
	}
	msg, err := m.Apply(ctx, p)
	return p, msg, err
}

func (m *Manager) applyCluster(ctx context.Context, p Preset) (string, error) {
	c, err := m.deps.Clusters.Get(ctx, p.ScopeID)
	if err != nil {
		return "", fmt.Errorf("load cluster #%d: %w", p.ScopeID, err)
	}
	if c.Settings == nil {
		c.Settings = map[string]settings.Value{}
	}
	for k, v := range p.Payload.Settings {
		c.Settings[k] = v
	}
	if p.Payload.ActiveEvent != nil {
		c.ActiveEvent = *p.Payload.ActiveEvent
	}
	if err := m.deps.Clusters.Update(ctx, c); err != nil {
		return "", fmt.Errorf("save cluster: %w", err)
	}
	if err := m.deps.Clusters.PropagateToServers(ctx, m.deps.Servers, c.ID); err != nil {
		return "", fmt.Errorf("write INIs: %w", err)
	}
	if m.deps.Bus != nil {
		m.deps.Bus.Publish(events.PresetApplied{
			PresetID: p.ID, PresetName: p.Name,
			ScopeType: p.ScopeType, ScopeID: c.ID, ScopeName: c.Name,
		})
	}
	m.deps.Log.Info("preset applied",
		"preset_id", p.ID, "preset", p.Name,
		"scope", "cluster", "scope_id", c.ID,
		"settings_changed", len(p.Payload.Settings),
		"active_event_set", p.Payload.ActiveEvent != nil)
	return fmt.Sprintf("applied %q to cluster %q (%d settings, event=%s)",
		p.Name, c.Name, len(p.Payload.Settings), formatActiveEvent(p.Payload.ActiveEvent)), nil
}

func (m *Manager) applyServer(ctx context.Context, p Preset) (string, error) {
	s, err := m.deps.Servers.Get(ctx, p.ScopeID)
	if err != nil {
		return "", fmt.Errorf("load server #%d: %w", p.ScopeID, err)
	}
	if s.SettingOverrides == nil {
		s.SettingOverrides = map[string]settings.Value{}
	}
	for k, v := range p.Payload.Settings {
		s.SettingOverrides[k] = v
	}
	if p.Payload.ActiveEvent != nil {
		s.ActiveEvent = *p.Payload.ActiveEvent
	}
	if err := m.deps.Servers.Update(ctx, s); err != nil {
		return "", fmt.Errorf("save server: %w", err)
	}
	c, err := m.deps.Clusters.Get(ctx, s.ClusterID)
	if err != nil {
		return "", fmt.Errorf("load owning cluster: %w", err)
	}
	effective := cluster.MergeSettings(c.Settings, s.SettingOverrides)
	if err := cluster.WriteServerINIs(s, effective, c.CustomLines); err != nil {
		return "", fmt.Errorf("write INIs: %w", err)
	}
	if m.deps.Bus != nil {
		m.deps.Bus.Publish(events.PresetApplied{
			PresetID: p.ID, PresetName: p.Name,
			ScopeType: p.ScopeType, ScopeID: s.ID, ScopeName: s.Name,
		})
	}
	m.deps.Log.Info("preset applied",
		"preset_id", p.ID, "preset", p.Name,
		"scope", "server", "scope_id", s.ID,
		"settings_changed", len(p.Payload.Settings),
		"active_event_set", p.Payload.ActiveEvent != nil)
	return fmt.Sprintf("applied %q to server %q (%d settings, event=%s)",
		p.Name, s.Name, len(p.Payload.Settings), formatActiveEvent(p.Payload.ActiveEvent)), nil
}

func formatActiveEvent(p *string) string {
	if p == nil {
		return "unchanged"
	}
	if *p == "" {
		return "(inherit)"
	}
	return *p
}
