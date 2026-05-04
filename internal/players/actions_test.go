package players

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"asamanager/internal/events"
)

func setupActions(t *testing.T) (*Actions, *Repo, *stubRCON, chan events.Event, int64) {
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
	a := NewActions(ActionsDeps{
		Repo:  repo,
		Sup:   sup,
		Bus:   bus,
		Log:   silentLog(),
		Clock: clk,
	})
	return a, repo, rc, got, sid
}

func TestActions_KickSendsRCONCommand(t *testing.T) {
	a, _, rc, _, sid := setupActions(t)
	if err := a.Kick(context.Background(), sid, "76561198000000001", "afk"); err != nil {
		t.Fatalf("Kick: %v", err)
	}
	calls := rc.callsCopy()
	if len(calls) != 1 || !strings.HasPrefix(calls[0], "kickplayer ") || !strings.Contains(calls[0], "76561198000000001") {
		t.Errorf("unexpected RCON calls: %v", calls)
	}
}

func TestActions_BanRecordsRowAndEmitsEvent(t *testing.T) {
	a, repo, rc, got, sid := setupActions(t)
	if err := a.Ban(context.Background(), sid, "76561198000000001", "griefing", "admin"); err != nil {
		t.Fatalf("Ban: %v", err)
	}
	calls := rc.callsCopy()
	if len(calls) != 1 || !strings.HasPrefix(calls[0], "banplayer ") {
		t.Errorf("expected banplayer RCON, got %v", calls)
	}
	bans, _ := repo.ListBans(context.Background(), Scope{Type: "server", ID: sid})
	if len(bans) != 1 || bans[0].Reason != "griefing" || bans[0].BannedBy != "admin" {
		t.Errorf("ban row = %+v", bans)
	}
	deadline := time.After(time.Second)
	for {
		select {
		case e := <-got:
			if pb, ok := e.(events.PlayerBanned); ok {
				if pb.SteamID != "76561198000000001" || pb.Reason != "griefing" {
					t.Errorf("event = %+v", pb)
				}
				return
			}
		case <-deadline:
			t.Fatal("expected PlayerBanned event")
		}
	}
}

func TestActions_UnbanRemovesRow(t *testing.T) {
	a, repo, rc, _, sid := setupActions(t)
	scope := Scope{Type: "server", ID: sid}
	if _, err := repo.AddBan(context.Background(), Ban{SteamID: "S1", Scope: scope, Reason: "x"}); err != nil {
		t.Fatal(err)
	}
	if err := a.Unban(context.Background(), sid, "S1"); err != nil {
		t.Fatalf("Unban: %v", err)
	}
	calls := rc.callsCopy()
	if len(calls) != 1 || !strings.HasPrefix(calls[0], "unbanplayer ") {
		t.Errorf("expected unbanplayer RCON, got %v", calls)
	}
	bans, _ := repo.ListBans(context.Background(), scope)
	if len(bans) != 0 {
		t.Errorf("expected ban row removed, got %d", len(bans))
	}
}

func TestActions_NoRCON_ReturnsNotReady(t *testing.T) {
	d := newTestDB(t)
	repo := NewRepo(d)
	seedServer(t, d)
	a := NewActions(ActionsDeps{
		Repo: repo,
		Sup:  &stubSupervisor{rc: nil},
		Log:  silentLog(),
	})
	if err := a.Kick(context.Background(), 1, "S1", ""); !errors.Is(err, ErrServerNotReady) {
		t.Errorf("Kick: got %v, want ErrServerNotReady", err)
	}
	if err := a.Ban(context.Background(), 1, "S1", "", ""); !errors.Is(err, ErrServerNotReady) {
		t.Errorf("Ban: got %v, want ErrServerNotReady", err)
	}
	if err := a.Unban(context.Background(), 1, "S1"); !errors.Is(err, ErrServerNotReady) {
		t.Errorf("Unban: got %v, want ErrServerNotReady", err)
	}
}

func TestActions_BroadcastSendsCommand(t *testing.T) {
	a, _, rc, _, sid := setupActions(t)
	if err := a.Broadcast(context.Background(), sid, "rebooting in 5"); err != nil {
		t.Fatalf("Broadcast: %v", err)
	}
	calls := rc.callsCopy()
	if len(calls) != 1 || !strings.HasPrefix(calls[0], "broadcast ") || !strings.Contains(calls[0], "rebooting in 5") {
		t.Errorf("unexpected RCON calls: %v", calls)
	}
}
