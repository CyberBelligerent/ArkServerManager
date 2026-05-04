package players

import (
	"context"
	"sync"

	"asamanager/internal/rcon"
	"asamanager/internal/server"
)

type stubRCON struct {
	mu       sync.Mutex
	respByCmd map[string]string
	calls    []string
	dialErr  error
	execErr  error
}

func newStubRCON() *stubRCON {
	return &stubRCON{respByCmd: map[string]string{}}
}

func (s *stubRCON) Dial(_ context.Context, _ string, _ int, _ string) error { return s.dialErr }
func (s *stubRCON) Close() error                                            { return nil }

func (s *stubRCON) Exec(_ context.Context, cmd string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls = append(s.calls, cmd)
	if s.execErr != nil {
		return "", s.execErr
	}
	if resp, ok := s.respByCmd[cmd]; ok {
		return resp, nil
	}
	// Allow prefix matches so "kickplayer 76561..." picks up the
	// "kickplayer" entry without needing exact-match seeding.
	for prefix, resp := range s.respByCmd {
		if len(cmd) >= len(prefix) && cmd[:len(prefix)] == prefix {
			return resp, nil
		}
	}
	return "", nil
}

func (s *stubRCON) callsCopy() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]string, len(s.calls))
	copy(out, s.calls)
	return out
}

type stubSupervisor struct {
	rc rcon.Client
}

func (s *stubSupervisor) Start(context.Context, int64) error             { return nil }
func (s *stubSupervisor) Stop(context.Context, int64, bool) error        { return nil }
func (s *stubSupervisor) Restart(context.Context, int64) error           { return nil }
func (s *stubSupervisor) Status(int64) server.Status                     { return server.StatusRunning }
func (s *stubSupervisor) Logs(int64) (<-chan string, error)              { return nil, nil }
func (s *stubSupervisor) RCONFor(int64) rcon.Client                      { return s.rc }
