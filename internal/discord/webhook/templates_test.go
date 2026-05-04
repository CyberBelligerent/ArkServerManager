package webhook

import (
	"strings"
	"testing"
	"time"

	"asamanager/internal/events"
)

func TestSubscribesTo(t *testing.T) {
	cases := []struct {
		mask     []string
		event    string
		expected bool
	}{
		{[]string{"*"}, "server.started", true},
		{[]string{"server.started"}, "server.started", true},
		{[]string{"server.crashed"}, "server.started", false},
		{[]string{"server.started", "*"}, "anything", true},
		{nil, "server.started", false},
		{[]string{}, "server.started", false},
	}
	for _, c := range cases {
		if got := SubscribesTo(c.mask, c.event); got != c.expected {
			t.Errorf("SubscribesTo(%v, %q) = %v, want %v", c.mask, c.event, got, c.expected)
		}
	}
}

func TestRender_DefaultEmbedHasServerName(t *testing.T) {
	now := time.Now()
	msg, err := Render("server.started", events.ServerStarted{ServerID: 1, Name: "Island", At: now}, nil)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if len(msg.Embeds) != 1 {
		t.Fatalf("expected 1 embed, got %d", len(msg.Embeds))
	}
	if !strings.Contains(msg.Embeds[0].Description, "Island") {
		t.Errorf("description missing server name: %q", msg.Embeds[0].Description)
	}
	if msg.Embeds[0].Color == 0 {
		t.Errorf("expected non-zero color")
	}
}

func TestRender_OverrideReplacesDescription(t *testing.T) {
	overrides := map[string]string{"server.started": "custom-{{.Name}}-message"}
	msg, err := Render("server.started", events.ServerStarted{Name: "Foo"}, overrides)
	if err != nil {
		t.Fatal(err)
	}
	if msg.Embeds[0].Description != "custom-Foo-message" {
		t.Errorf("override not applied: %q", msg.Embeds[0].Description)
	}
}

type fakeEvent struct{}

func (fakeEvent) EventName() string { return "fake.event" }

func TestRender_NoEmbedForUnknown(t *testing.T) {
	if _, err := Render("fake.event", fakeEvent{}, nil); err == nil {
		t.Error("expected error for unknown event type")
	}
}

func TestSubscribableEvents_IncludesStarting(t *testing.T) {
	for _, name := range SubscribableEvents {
		if name == "server.starting" {
			return
		}
	}
	t.Error("server.starting missing from SubscribableEvents catalog")
}
