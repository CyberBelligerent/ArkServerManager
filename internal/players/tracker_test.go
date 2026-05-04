package players

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"asamanager/internal/events"
	"asamanager/internal/rcon"
)

// fixedClock returns the same time on every Now() call. Sleep is a no-op.
type fixedClock struct{ t time.Time }

func (f *fixedClock) Now() time.Time     { return f.t }
func (f *fixedClock) Sleep(time.Duration) {}

func (f *fixedClock) advance(d time.Duration) { f.t = f.t.Add(d) }

func silentLog() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func setupTracker(t *testing.T) (*Tracker, *Repo, *stubRCON, *fixedClock, *events.Bus, chan events.Event, int64) {
	t.Helper()
	d := newTestDB(t)
	repo := NewRepo(d)
	sid := seedServer(t, d)
	rc := newStubRCON()
	sup := &stubSupervisor{rc: rc}
	bus := events.NewBus(64)
	bus.Start()
	t.Cleanup(bus.Stop)
	got := make(chan events.Event, 16)
	bus.SubscribeAll(func(e events.Event) {
		select {
		case got <- e:
		default:
		}
	})
	clk := &fixedClock{t: time.Date(2026, 5, 3, 12, 0, 0, 0, time.UTC)}
	tracker := NewTracker(TrackerDeps{
		Repo:         repo,
		Sup:          sup,
		Bus:          bus,
		Log:          silentLog(),
		Clock:        clk,
		PollInterval: time.Hour, // suppress the background loop; tests drive PollOnce
	})
	return tracker, repo, rc, clk, bus, got, sid
}

func waitForKind[T events.Event](t *testing.T, ch <-chan events.Event, d time.Duration) (T, bool) {
	t.Helper()
	deadline := time.After(d)
	for {
		select {
		case e := <-ch:
			if v, ok := e.(T); ok {
				return v, true
			}
		case <-deadline:
			var zero T
			return zero, false
		}
	}
}

func TestTracker_FirstPollEmitsJoined(t *testing.T) {
	tr, repo, rc, _, _, got, sid := setupTracker(t)
	rc.respByCmd[rcon.CmdListPlayers] = "0. Alice, 76561198000000001\n"

	if err := tr.PollOnce(context.Background(), sid); err != nil {
		t.Fatalf("PollOnce: %v", err)
	}

	ev, ok := waitForKind[events.PlayerJoined](t, got, time.Second)
	if !ok {
		t.Fatal("expected PlayerJoined event")
	}
	if ev.SteamID != "76561198000000001" || ev.Name != "Alice" {
		t.Errorf("event = %+v", ev)
	}
	open, _ := repo.ListOpenSessions(context.Background(), sid)
	if len(open) != 1 {
		t.Errorf("expected 1 open session, got %d", len(open))
	}
}

func TestTracker_DiffEmitsLeftWhenPlayerDrops(t *testing.T) {
	tr, repo, rc, clk, _, got, sid := setupTracker(t)
	rc.respByCmd[rcon.CmdListPlayers] = "0. Alice, 76561198000000001\n"

	if err := tr.PollOnce(context.Background(), sid); err != nil {
		t.Fatalf("first poll: %v", err)
	}
	// Drain the joined event we don't care about for this test.
	waitForKind[events.PlayerJoined](t, got, time.Second)

	clk.advance(20 * time.Minute)
	rc.respByCmd[rcon.CmdListPlayers] = "No Players Connected\n"
	if err := tr.PollOnce(context.Background(), sid); err != nil {
		t.Fatalf("second poll: %v", err)
	}

	ev, ok := waitForKind[events.PlayerLeft](t, got, time.Second)
	if !ok {
		t.Fatal("expected PlayerLeft event")
	}
	if ev.SteamID != "76561198000000001" {
		t.Errorf("event = %+v", ev)
	}
	open, _ := repo.ListOpenSessions(context.Background(), sid)
	if len(open) != 0 {
		t.Errorf("expected open sessions to be closed, got %d", len(open))
	}
	p, _ := repo.GetPlayer(context.Background(), "76561198000000001")
	if p.TotalMinutes != 20 {
		t.Errorf("TotalMinutes = %d, want 20", p.TotalMinutes)
	}
}

func TestTracker_PollOnce_NoRCON_ReturnsSilentSentinel(t *testing.T) {
	d := newTestDB(t)
	repo := NewRepo(d)
	seedServer(t, d)
	tr := NewTracker(TrackerDeps{
		Repo: repo,
		Sup:  &stubSupervisor{rc: nil}, // no RCON yet
		Bus:  events.NewBus(8),
		Log:  silentLog(),
	})
	err := tr.PollOnce(context.Background(), 1)
	if err != errRCONUnavailable {
		t.Errorf("got %v, want errRCONUnavailable", err)
	}
}

func TestTracker_NoChurnWhenSnapshotStable(t *testing.T) {
	tr, _, rc, _, _, got, sid := setupTracker(t)
	rc.respByCmd[rcon.CmdListPlayers] = "0. Alice, 76561198000000001\n"

	for i := 0; i < 3; i++ {
		if err := tr.PollOnce(context.Background(), sid); err != nil {
			t.Fatalf("poll %d: %v", i, err)
		}
	}
	// Drain — only one Joined; no Left over three identical polls.
	var joined, left int
	deadline := time.After(200 * time.Millisecond)
loop:
	for {
		select {
		case e := <-got:
			switch e.(type) {
			case events.PlayerJoined:
				joined++
			case events.PlayerLeft:
				left++
			}
		case <-deadline:
			break loop
		}
	}
	if joined != 1 {
		t.Errorf("joined = %d, want 1", joined)
	}
	if left != 0 {
		t.Errorf("left = %d, want 0", left)
	}
}
