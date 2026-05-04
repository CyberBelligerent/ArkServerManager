package gui

import (
	asaevents "asamanager/pkg/asa/events"
)

// The ActiveEvent dropdowns map between two parallel slices:
//   - labels:  what the user picks ("(none)", "Summer Bash", ...)
//   - values:  what gets stored in the DB ("", "Summer", ...)
func buildClusterEventOptions(a *App) (labels, values []string) {
	labels = append(labels, a.T("active_event.cluster_none"))
	values = append(values, "")
	for _, ev := range asaevents.Known {
		labels = append(labels, ev.DisplayName)
		values = append(values, ev.Name)
	}
	return labels, values
}

// buildServerEventOptions includes the cluster's current event
func buildServerEventOptions(a *App, clusterEvent string) (labels, values []string) {
	inheritLabel := a.T("active_event.server_inherit")
	if resolved := clusterEvent; resolved != "" {
		display := resolved
		for _, ev := range asaevents.Known {
			if ev.Name == resolved {
				display = ev.DisplayName
				break
			}
		}
		inheritLabel = a.T("active_event.server_inherit_current", display)
	}
	labels = append(labels, inheritLabel)
	values = append(values, "")
	labels = append(labels, a.T("active_event.server_clear"))
	values = append(values, "None")
	for _, ev := range asaevents.Known {
		labels = append(labels, ev.DisplayName)
		values = append(values, ev.Name)
	}
	return labels, values
}

// displayLabelForEvent finds the label corresponding to a stored value.
// Falls back to labels[0] when the value isn't in the list
func displayLabelForEvent(value string, labels, values []string) string {
	for i, v := range values {
		if v == value {
			return labels[i]
		}
	}
	return labels[0]
}

func eventValueForLabel(label string, labels, values []string) string {
	for i, l := range labels {
		if l == label {
			return values[i]
		}
	}
	return ""
}
