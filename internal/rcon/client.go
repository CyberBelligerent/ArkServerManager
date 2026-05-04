package rcon

import "context"

type Client interface {
	Dial(ctx context.Context, host string, port int, password string) error
	Exec(ctx context.Context, command string) (string, error)
	Close() error
}

const (
	CmdSaveWorld   = "saveworld"
	CmdDoExit      = "doexit"
	CmdListPlayers = "listplayers"
	CmdBroadcast   = "broadcast"
	CmdServerChat  = "serverchat"
	CmdKickPlayer  = "kickplayer"
	CmdBanPlayer   = "banplayer"
	CmdUnbanPlayer = "unbanplayer"
)
