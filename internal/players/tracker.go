package players

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"asamanager/internal/clock"
	"asamanager/internal/events"
	"asamanager/internal/rcon"
	"asamanager/internal/server"
)

type TrackerDeps struct {
	Repo         *Repo
	Sup          server.Supervisor
	Bus          *events.Bus
	Log          *slog.Logger
	Clock        clock.Clock
	PollInterval time.Duration
}

type Tracker struct {
	deps TrackerDeps

	mu      sync.Mutex
	pollers map[int64]context.CancelFunc
}

func NewTracker(deps TrackerDeps) *Tracker {
	if deps.Clock == nil {
		deps.Clock = clock.System()
	}
	if deps.PollInterval == 0 {
		deps.PollInterval = 30 * time.Second
	}
	if deps.Log == nil {
		deps.Log = slog.Default()
	}
	return &Tracker{deps: deps, pollers: map[int64]context.CancelFunc{}}
}

func (t *Tracker) Start() func() {
	unsubs := []func(){
		t.deps.Bus.Subscribe(events.ServerStarted{}.EventName(), func(e events.Event) {
			if ev, ok := e.(events.ServerStarted); ok {
				t.startPolling(ev.ServerID, ev.Name)
			}
		}),
		t.deps.Bus.Subscribe(events.ServerStopped{}.EventName(), func(e events.Event) {
			if ev, ok := e.(events.ServerStopped); ok {
				t.stopPolling(ev.ServerID)
			}
		}),
		t.deps.Bus.Subscribe(events.ServerCrashed{}.EventName(), func(e events.Event) {
			if ev, ok := e.(events.ServerCrashed); ok {
				t.stopPolling(ev.ServerID)
			}
		}),
	}
	return func() {
		for _, u := range unsubs {
			u()
		}
		t.StopAll()
	}
}

func (t *Tracker) startPolling(serverID int64, name string) {
	t.mu.Lock()
	if _, ok := t.pollers[serverID]; ok {
		t.mu.Unlock()
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.pollers[serverID] = cancel
	t.mu.Unlock()
	go t.pollLoop(ctx, serverID, name)
}

func (t *Tracker) stopPolling(serverID int64) {
	t.mu.Lock()
	cancel, ok := t.pollers[serverID]
	if ok {
		delete(t.pollers, serverID)
	}
	t.mu.Unlock()
	if ok {
		cancel()
	}
	t.endOpenSessions(serverID)
}

func (t *Tracker) StopAll() {
	t.mu.Lock()
	ids := make([]int64, 0, len(t.pollers))
	for id := range t.pollers {
		ids = append(ids, id)
	}
	t.mu.Unlock()
	for _, id := range ids {
		t.stopPolling(id)
	}
}

func (t *Tracker) PollOnce(ctx context.Context, serverID int64) error {
	c := t.deps.Sup.RCONFor(serverID)
	if c == nil {
		return errRCONUnavailable
	}
	raw, err := c.Exec(ctx, rcon.CmdListPlayers)
	if err != nil {
		return err
	}
	snapshot := ParseListPlayers(raw)
	return t.applySnapshot(ctx, serverID, snapshot)
}

func (t *Tracker) applySnapshot(ctx context.Context, serverID int64, current []OnlinePlayer) error {
	now := t.deps.Clock.Now()

	nowSet := make(map[string]OnlinePlayer, len(current))
	for _, p := range current {
		nowSet[p.SteamID] = p
		if err := t.deps.Repo.UpsertSeen(ctx, p.SteamID, p.Name, now); err != nil {
			t.deps.Log.Error("upsert seen", "steam_id", p.SteamID, "err", err)
		}
	}

	open, err := t.deps.Repo.ListOpenSessions(ctx, serverID)
	if err != nil {
		return err
	}
	openSet := make(map[string]Session, len(open))
	for _, s := range open {
		openSet[s.SteamID] = s
	}

	// Joined
	for id, p := range nowSet {
		if _, ok := openSet[id]; ok {
			continue
		}
		if _, err := t.deps.Repo.StartSession(ctx, p.SteamID, serverID, now); err != nil {
			t.deps.Log.Error("start session", "steam_id", p.SteamID, "err", err)
			continue
		}
		t.publish(events.PlayerJoined{ServerID: serverID, SteamID: p.SteamID, Name: p.Name, At: now})
	}

	// Left
	for id, s := range openSet {
		if _, ok := nowSet[id]; ok {
			continue
		}
		if err := t.deps.Repo.EndSession(ctx, s.ID, now); err != nil {
			t.deps.Log.Error("end session", "session_id", s.ID, "err", err)
			continue
		}
		t.publish(events.PlayerLeft{ServerID: serverID, SteamID: s.SteamID, At: now})
	}
	return nil
}

func (t *Tracker) pollLoop(ctx context.Context, serverID int64, name string) {
	ticker := time.NewTicker(t.deps.PollInterval)
	defer ticker.Stop()

	t.tick(ctx, serverID, name)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
		t.tick(ctx, serverID, name)
	}
}

func (t *Tracker) tick(ctx context.Context, serverID int64, name string) {
	if err := t.PollOnce(ctx, serverID); err != nil && !errors.Is(err, errRCONUnavailable) {
		t.deps.Log.Warn("listplayers poll", "server_id", serverID, "name", name, "err", err)
	}
}

func (t *Tracker) endOpenSessions(serverID int64) {
	sessions, err := t.deps.Repo.ListOpenSessions(context.Background(), serverID)
	if err != nil {
		t.deps.Log.Error("list open sessions for cleanup", "server_id", serverID, "err", err)
		return
	}
	now := t.deps.Clock.Now()
	for _, s := range sessions {
		if err := t.deps.Repo.EndSession(context.Background(), s.ID, now); err != nil {
			t.deps.Log.Error("end session on shutdown", "session_id", s.ID, "err", err)
		}
	}
}

func (t *Tracker) publish(e events.Event) {
	if t.deps.Bus == nil {
		return
	}
	t.deps.Bus.Publish(e)
}

var errRCONUnavailable = errors.New("rcon not available")
