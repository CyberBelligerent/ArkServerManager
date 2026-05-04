package webhook

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Message is a single Discord webhook payload.
type Message struct {
	Content   string  // optional plain text above the embed(s)
	Username  string
	AvatarURL string
	Embeds    []Embed
}

type Embed struct {
	Title       string
	Description string
	Color       int				// hex for a color like 0x57F287
	Fields      []EmbedField
	Footer      string
	Timestamp   time.Time
}

type EmbedField struct {
	Name   string
	Value  string
	Inline bool
}

type Sender interface {
	Send(ctx context.Context, webhookURL string, msg Message) error
}

type HTTPSender struct {
	client *http.Client
}

func NewHTTPSender() *HTTPSender {
	return &HTTPSender{client: &http.Client{Timeout: 30 * time.Second}}
}

func (s *HTTPSender) Send(ctx context.Context, webhookURL string, msg Message) error {
	payload := map[string]any{}
	if msg.Content != "" {
		payload["content"] = msg.Content
	}
	if msg.Username != "" {
		payload["username"] = msg.Username
	}
	if msg.AvatarURL != "" {
		payload["avatar_url"] = msg.AvatarURL
	}
	if len(msg.Embeds) > 0 {
		embeds := make([]map[string]any, 0, len(msg.Embeds))
		for _, e := range msg.Embeds {
			em := map[string]any{}
			if e.Title != "" {
				em["title"] = e.Title
			}
			if e.Description != "" {
				em["description"] = e.Description
			}
			if e.Color != 0 {
				em["color"] = e.Color
			}
			if !e.Timestamp.IsZero() {
				em["timestamp"] = e.Timestamp.UTC().Format(time.RFC3339)
			}
			if e.Footer != "" {
				em["footer"] = map[string]any{"text": e.Footer}
			}
			if len(e.Fields) > 0 {
				fields := make([]map[string]any, len(e.Fields))
				for i, f := range e.Fields {
					fields[i] = map[string]any{
						"name":   f.Name,
						"value":  f.Value,
						"inline": f.Inline,
					}
				}
				em["fields"] = fields
			}
			embeds = append(embeds, em)
		}
		payload["embeds"] = embeds
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, webhookURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("discord post: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusTooManyRequests {
		return fmt.Errorf("discord rate limited (retry-after %s)", resp.Header.Get("Retry-After"))
	}
	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("discord %s: %s", resp.Status, string(b))
	}
	return nil
}

var _ Sender = (*HTTPSender)(nil)
