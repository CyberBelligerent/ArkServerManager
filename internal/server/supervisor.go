package server

import (
	"context"

	"asamanager/internal/rcon"
)

type Status string

const (
	StatusStopped  Status = "stopped"
	StatusStarting Status = "starting"
	StatusRunning  Status = "running"
	StatusStopping Status = "stopping"
	StatusCrashed  Status = "crashed"
)

type Supervisor interface {
	Start(ctx context.Context, serverID int64) error
	Stop(ctx context.Context, serverID int64, graceful bool) error
	Restart(ctx context.Context, serverID int64) error
	Status(serverID int64) Status
	Logs(serverID int64) (<-chan string, error)
	RCONFor(serverID int64) rcon.Client
}
