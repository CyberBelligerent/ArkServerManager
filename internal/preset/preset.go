package preset

import (
	"encoding/json"
	"time"

	"asamanager/internal/settings"
)

const (
	ScopeCluster = "cluster"
	ScopeServer  = "server"
)

// Preset is one saved diff. ScopeType + ScopeID identify the target
// only presets matching the target's scope can be applied to it.
type Preset struct {
	ID          int64
	ScopeType   string
	ScopeID     int64
	Name        string
	Description string
	Payload     Payload
	CreatedAt   time.Time
}

// Payload is the diff the preset will apply
type Payload struct {
	Settings    map[string]settings.Value `json:"settings,omitempty"`
	ActiveEvent *string                   `json:"active_event,omitempty"`
}

type rawPayload struct {
	Settings    json.RawMessage `json:"settings,omitempty"`
	ActiveEvent *string         `json:"active_event,omitempty"`
}

func MarshalPayload(p Payload) (string, error) {
	var raw rawPayload
	if len(p.Settings) > 0 {
		s, err := settings.EncodeValues(p.Settings)
		if err != nil {
			return "", err
		}
		raw.Settings = json.RawMessage(s)
	}
	raw.ActiveEvent = p.ActiveEvent
	body, err := json.Marshal(raw)
	if err != nil {
		return "", err
	}
	return string(body), nil
}

func UnmarshalPayload(s string) (Payload, error) {
	if s == "" || s == "{}" {
		return Payload{}, nil
	}
	var raw rawPayload
	if err := json.Unmarshal([]byte(s), &raw); err != nil {
		return Payload{}, err
	}
	out := Payload{ActiveEvent: raw.ActiveEvent}
	if len(raw.Settings) > 0 {
		decoded, err := settings.DecodeValues(string(raw.Settings))
		if err != nil {
			return Payload{}, err
		}
		out.Settings = decoded
	}
	return out, nil
}
