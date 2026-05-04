package players

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"asamanager/internal/clock"
	"asamanager/internal/events"
	"asamanager/internal/rcon"
	"asamanager/internal/server"
)

type ActionsDeps struct {
	Repo  *Repo
	Sup   server.Supervisor
	Bus   *events.Bus
	Log   *slog.Logger
	Clock clock.Clock
}

// Runs the RCON command
type Actions struct {
	deps ActionsDeps
}

func NewActions(deps ActionsDeps) *Actions {
	if deps.Clock == nil {
		deps.Clock = clock.System()
	}
	if deps.Log == nil {
		deps.Log = slog.Default()
	}
	return &Actions{deps: deps}
}

var ErrServerNotReady = errors.New("server has no active RCON connection")

// Kick disconnects steamID from the running server, optionally with a reason logged for the audit trail.
func (a *Actions) Kick(ctx context.Context, serverID int64, steamID, reason string) error {
	c := a.rconFor(serverID)
	if c == nil {
		return ErrServerNotReady
	}
	cmd := rcon.CmdKickPlayer + " " + steamID
	if _, err := c.Exec(ctx, cmd); err != nil {
		return fmt.Errorf("kickplayer: %w", err)
	}
	a.deps.Log.Info("player kicked", "server_id", serverID, "steam_id", steamID, "reason", reason)
	return nil
}

// Ban kicks steamID via RCON and records a server-scoped ban row
func (a *Actions) Ban(ctx context.Context, serverID int64, steamID, reason, bannedBy string) error {
	c := a.rconFor(serverID)
	if c == nil {
		return ErrServerNotReady
	}
	cmd := rcon.CmdBanPlayer + " " + steamID
	if _, err := c.Exec(ctx, cmd); err != nil {
		return fmt.Errorf("banplayer: %w", err)
	}
	now := a.deps.Clock.Now()
	if _, err := a.deps.Repo.AddBan(ctx, Ban{
		SteamID:  steamID,
		Scope:    Scope{Type: "server", ID: serverID},
		Reason:   reason,
		BannedAt: now,
		BannedBy: bannedBy,
	}); err != nil {
		return fmt.Errorf("record ban: %w", err)
	}
	a.publish(events.PlayerBanned{ServerID: serverID, SteamID: steamID, Reason: reason, At: now})
	return nil
}

// Unban removes the server-scoped ban from RCON and the DB
func (a *Actions) Unban(ctx context.Context, serverID int64, steamID string) error {
	c := a.rconFor(serverID)
	if c == nil {
		return ErrServerNotReady
	}
	cmd := rcon.CmdUnbanPlayer + " " + steamID
	if _, err := c.Exec(ctx, cmd); err != nil {
		return fmt.Errorf("unbanplayer: %w", err)
	}
	if _, err := a.deps.Repo.RemoveBan(ctx, steamID, Scope{Type: "server", ID: serverID}); err != nil {
		return fmt.Errorf("remove ban row: %w", err)
	}
	return nil
}

// Broadcast sends a chat message visible to every player on the server
func (a *Actions) Broadcast(ctx context.Context, serverID int64, message string) error {
	c := a.rconFor(serverID)
	if c == nil {
		return ErrServerNotReady
	}
	cmd := rcon.CmdBroadcast + " " + message
	if _, err := c.Exec(ctx, cmd); err != nil {
		return fmt.Errorf("broadcast: %w", err)
	}
	return nil
}

func (a *Actions) rconFor(serverID int64) rcon.Client {
	return a.deps.Sup.RCONFor(serverID)
}

func (a *Actions) publish(e events.Event) {
	if a.deps.Bus == nil {
		return
	}
	a.deps.Bus.Publish(e)
}
