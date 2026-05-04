package webhook

import (
	"context"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"asamanager/internal/events"
)

// stubSender records every Send call.
type stubSender struct {
	mu    sync.Mutex
	calls []sentCall
}
type sentCall struct {
	URL         string
	Content     string
	Description string
}

func (s *stubSender) Send(_ context.Context, url string, msg Message) error {
	s.mu.Lock()
	desc := ""
	if len(msg.Embeds) > 0 {
		desc = msg.Embeds[0].Description
	}
	s.calls = append(s.calls, sentCall{URL: url, Content: msg.Content, Description: desc})
	s.mu.Unlock()
	return nil
}

func (s *stubSender) snapshot() []sentCall {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]sentCall, len(s.calls))
	copy(out, s.calls)
	return out
}

func silentLog() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func setupDispatcher(t *testing.T) (*Dispatcher, *Repo, *stubSender, *events.Bus) {
	t.Helper()
	d := newTestDB(t)
	repo := NewRepo(d)
	sender := &stubSender{}
	bus := events.NewBus(64)
	bus.Start()
	t.Cleanup(bus.Stop)
	dp := NewDispatcher(DispatcherDeps{
		Repo: repo, Sender: sender, Bus: bus, DB: d, Log: silentLog(),
	})
	stop := dp.Start()
	t.Cleanup(stop)
	return dp, repo, sender, bus
}

func waitFor(t *testing.T, d time.Duration, pred func() bool) bool {
	t.Helper()
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		if pred() {
			return true
		}
		time.Sleep(10 * time.Millisecond)
	}
	return pred()
}

func TestDispatcher_RoutesByEventMask(t *testing.T) {
	_, repo, sender, bus := setupDispatcher(t)
	ctx := context.Background()
	if _, err := repo.Create(ctx, Webhook{
		Name: "starts-only", URL: "https://x/starts",
		Scope:     Scope{Type: "global"},
		EventMask: []string{"server.started"},
		Enabled:   true,
	}); err != nil {
		t.Fatal(err)
	}
	bus.Publish(events.ServerStarted{ServerID: 1, Name: "Island", At: time.Now()})
	bus.Publish(events.ServerCrashed{ServerID: 1, Name: "Island", ExitCode: 1, At: time.Now()})

	if !waitFor(t, time.Second, func() bool { return len(sender.snapshot()) >= 1 }) {
		t.Fatal("expected at least one webhook send")
	}
	calls := sender.snapshot()
	if len(calls) != 1 {
		t.Errorf("got %d sends, want 1 (only server.started should match)", len(calls))
	}
}

func TestDispatcher_WildcardReceivesEverything(t *testing.T) {
	_, repo, sender, bus := setupDispatcher(t)
	if _, err := repo.Create(context.Background(), Webhook{
		Name: "all", URL: "https://x/all",
		Scope:     Scope{Type: "global"},
		EventMask: []string{AllEventsWildcard},
		Enabled:   true,
	}); err != nil {
		t.Fatal(err)
	}
	bus.Publish(events.ServerStarting{ServerID: 1, Name: "A"})
	bus.Publish(events.ServerStarted{ServerID: 1, Name: "A"})
	bus.Publish(events.ServerStopped{ServerID: 1, Name: "A"})

	if !waitFor(t, time.Second, func() bool { return len(sender.snapshot()) >= 3 }) {
		t.Errorf("expected 3 sends, got %d", len(sender.snapshot()))
	}
}

func TestDispatcher_DisabledWebhookSkipped(t *testing.T) {
	_, repo, sender, bus := setupDispatcher(t)
	if _, err := repo.Create(context.Background(), Webhook{
		Name: "off", URL: "https://x/off",
		Scope:     Scope{Type: "global"},
		EventMask: []string{AllEventsWildcard},
		Enabled:   false,
	}); err != nil {
		t.Fatal(err)
	}
	bus.Publish(events.ServerStarted{ServerID: 1, Name: "A"})
	time.Sleep(150 * time.Millisecond)
	if n := len(sender.snapshot()); n != 0 {
		t.Errorf("disabled webhook fired %d times", n)
	}
}

func TestDispatcher_ServerScopeFiltersByID(t *testing.T) {
	_, repo, sender, bus := setupDispatcher(t)
	if _, err := repo.Create(context.Background(), Webhook{
		Name: "srv7", URL: "https://x/srv7",
		Scope:     Scope{Type: "server", ID: 7},
		EventMask: []string{AllEventsWildcard},
		Enabled:   true,
	}); err != nil {
		t.Fatal(err)
	}
	bus.Publish(events.ServerStarted{ServerID: 99, Name: "Other"})
	bus.Publish(events.ServerStarted{ServerID: 7, Name: "Mine"})

	if !waitFor(t, time.Second, func() bool { return len(sender.snapshot()) >= 1 }) {
		t.Fatal("no send")
	}
	calls := sender.snapshot()
	if len(calls) != 1 {
		t.Errorf("got %d sends, want 1 (only server 7 should match)", len(calls))
	}
}

func TestDispatcher_RendersTemplate(t *testing.T) {
	_, repo, sender, bus := setupDispatcher(t)
	if _, err := repo.Create(context.Background(), Webhook{
		Name: "all", URL: "https://x/all",
		Scope:     Scope{Type: "global"},
		EventMask: []string{AllEventsWildcard},
		Enabled:   true,
	}); err != nil {
		t.Fatal(err)
	}
	bus.Publish(events.ServerStarted{ServerID: 1, Name: "Island"})

	if !waitFor(t, time.Second, func() bool { return len(sender.snapshot()) >= 1 }) {
		t.Fatal("no send")
	}
	calls := sender.snapshot()
	if !contains(calls[0].Description, "Island") {
		t.Errorf("embed description missing server name: %q", calls[0].Description)
	}
}

func contains(haystack, needle string) bool {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}
