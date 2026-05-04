package cluster

import "asamanager/internal/settings"

// Use this to compute a server's effective settings as
// (cluster.Settings, server.SettingOverrides).
func MergeSettings(base, overrides map[string]settings.Value) map[string]settings.Value {
	out := make(map[string]settings.Value, len(base)+len(overrides))
	for k, v := range base {
		out[k] = v
	}
	for k, v := range overrides {
		out[k] = v
	}
	return out
}
