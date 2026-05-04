package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"asamanager/internal/events"
	"asamanager/internal/rcon"
)

var _ Supervisor = (*RealSupervisor)(nil)

type SupervisorDeps struct {
	Repo *Repo // optional: when non-nil, status transitions are persisted
	Bus *events.Bus // optional: when non-nil, lifecycle events are published
	Log *slog.Logger
	Spawner Spawner
	RCONFactory func() rcon.Client

	// LaunchOptions resolves cluster-level launch parameters for srv
	LaunchOptions func(ctx context.Context, srv Server) (LaunchOptions, error)

	// LogDir is where per-server log files are written
	LogDir string

	ProbeInterval time.Duration // RCON readiness poll (default 5s)
	ReadyTimeout  time.Duration // overall deadline for "running" (default 5m)
	StopTimeout   time.Duration // graceful-stop wait before kill (default 60s)
}

type RealSupervisor struct {
	deps     SupervisorDeps
	mu       sync.Mutex
	sessions map[int64]*session
}

func NewSupervisor(deps SupervisorDeps) *RealSupervisor {
	if deps.Spawner == nil {
		deps.Spawner = RealSpawner{}
	}
	if deps.ProbeInterval == 0 {
		deps.ProbeInterval = 5 * time.Second
	}
	if deps.ReadyTimeout == 0 {
		deps.ReadyTimeout = 5 * time.Minute
	}
	if deps.StopTimeout == 0 {
		deps.StopTimeout = 60 * time.Second
	}
	if deps.RCONFactory == nil {
		deps.RCONFactory = func() rcon.Client { return rcon.New() }
	}
	if deps.Log == nil {
		deps.Log = slog.Default()
	}
	return &RealSupervisor{
		deps:     deps,
		sessions: make(map[int64]*session),
	}
}

// session is the supervisor's per-server runtime state
type session struct {
	serverID int64
	name     string
	proc     Process
	cancel   context.CancelFunc

	logHub  *lineHub
	logFile *os.File

	rconPort int
	rconPwd  string

	mu     sync.Mutex
	status Status
	rcon   rcon.Client
}

func (s *session) setStatus(st Status) {
	s.mu.Lock()
	s.status = st
	s.mu.Unlock()
}

func (s *session) getStatus() Status {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.status
}

// transition is the only blessed way to change a session's status
func (s *RealSupervisor) transition(sess *session, st Status) {
	s.mu.Lock()
	sess.setStatus(st)
	s.persistStatus(sess.serverID, st)
	s.mu.Unlock()
}

func (s *RealSupervisor) Start(ctx context.Context, serverID int64) error {
	s.mu.Lock()
	if existing, ok := s.sessions[serverID]; ok {
		if !isTerminalStatus(existing.getStatus()) {
			s.mu.Unlock()
			return fmt.Errorf("server %d already running or starting", serverID)
		}
		// Terminal-state tombstone left from a prior run
		delete(s.sessions, serverID)
	}
	s.mu.Unlock()

	if s.deps.Repo == nil {
		return errors.New("supervisor: Repo is required")
	}
	if s.deps.LaunchOptions == nil {
		return errors.New("supervisor: LaunchOptions resolver is required")
	}

	srv, err := s.deps.Repo.Get(ctx, serverID)
	if err != nil {
		return fmt.Errorf("load server %d: %w", serverID, err)
	}
	opts, err := s.deps.LaunchOptions(ctx, srv)
	if err != nil {
		return fmt.Errorf("resolve launch options: %w", err)
	}
	cmd := BuildLaunchCommand(srv, opts)

	if err := os.MkdirAll(s.deps.LogDir, 0o755); err != nil {
		return fmt.Errorf("create log dir: %w", err)
	}
	logPath := filepath.Join(s.deps.LogDir, fmt.Sprintf("server-%d.log", serverID))
	logFile, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open log file: %w", err)
	}

	hub := newLineHub()
	stdout := newLinePump(logFile, hub)
	stderr := newLinePump(logFile, hub)

	procCtx, cancel := context.WithCancel(context.Background())
	proc, err := s.deps.Spawner.Spawn(procCtx, cmd.Exe, cmd.Args, srv.InstallDir, stdout, stderr)
	if err != nil {
		cancel()
		_ = logFile.Close()
		return fmt.Errorf("spawn: %w", err)
	}

	sess := &session{
		serverID: serverID,
		name:     srv.Name,
		proc:     proc,
		cancel:   cancel,
		logHub:   hub,
		logFile:  logFile,
		rconPort: srv.Ports.RCON,
		rconPwd:  srv.RCONPassword,
	}

	s.mu.Lock()
	s.sessions[serverID] = sess
	s.mu.Unlock()

	s.transition(sess, StatusStarting)
	s.publish(events.ServerStarting{ServerID: serverID, Name: srv.Name, At: time.Now()})
	s.deps.Log.Info("server starting", "server_id", serverID, "name", srv.Name, "pid", proc.PID())

	// Tail ARK's own log file into the hub
	go tailARKLog(procCtx, proc.Done(), hub, srv.InstallDir)

	go s.watchReadiness(procCtx, sess)
	go s.watchExit(sess)
	return nil
}

func (s *RealSupervisor) Stop(ctx context.Context, serverID int64, graceful bool) error {
	s.mu.Lock()
	sess, ok := s.sessions[serverID]
	s.mu.Unlock()
	if !ok {
		return fmt.Errorf("server %d not running", serverID)
	}
	if isTerminalStatus(sess.getStatus()) {
		return nil
	}

	s.transition(sess, StatusStopping)

	if graceful {
		if err := s.gracefulStop(ctx, sess); err == nil {
			return nil
		} else {
			s.deps.Log.Warn("graceful stop failed; killing", "server_id", serverID, "err", err)
		}
	}
	return sess.proc.Kill()
}

func (s *RealSupervisor) gracefulStop(ctx context.Context, sess *session) error {
	sess.mu.Lock()
	c := sess.rcon
	sess.mu.Unlock()
	if c == nil {
		c = s.deps.RCONFactory()
		dialCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		err := c.Dial(dialCtx, "127.0.0.1", sess.rconPort, sess.rconPwd)
		cancel()
		if err != nil {
			return fmt.Errorf("rcon dial: %w", err)
		}
		sess.mu.Lock()
		sess.rcon = c
		sess.mu.Unlock()
	}

	saveCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	if _, err := c.Exec(saveCtx, rcon.CmdSaveWorld); err != nil {
		cancel()
		return fmt.Errorf("saveworld: %w", err)
	}
	cancel()
	s.publish(events.ServerSaved{ServerID: sess.serverID, Name: sess.name, At: time.Now()})

	exitCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if _, err := c.Exec(exitCtx, rcon.CmdDoExit); err != nil {
		return fmt.Errorf("doexit: %w", err)
	}

	timer := time.NewTimer(s.deps.StopTimeout)
	defer timer.Stop()
	select {
	case <-sess.proc.Done():
		return nil
	case <-timer.C:
		return errors.New("graceful stop timed out")
	}
}

func (s *RealSupervisor) Restart(ctx context.Context, serverID int64) error {
	if err := s.Stop(ctx, serverID, true); err != nil {
		return fmt.Errorf("restart stop: %w", err)
	}
	deadline := time.Now().Add(s.deps.StopTimeout + 10*time.Second)
	for time.Now().Before(deadline) {
		s.mu.Lock()
		_, ok := s.sessions[serverID]
		s.mu.Unlock()
		if !ok {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	return s.Start(ctx, serverID)
}

func (s *RealSupervisor) Status(serverID int64) Status {
	s.mu.Lock()
	sess, ok := s.sessions[serverID]
	s.mu.Unlock()
	if !ok {
		return StatusStopped
	}
	return sess.getStatus()
}

// RCONFor returns the long-lived RCON client the supervisor opened for
// serverID during the readiness probe, or nil if the server is not
// running or readiness has not yet completed. The returned client is
// owned by the supervisor. Callers should not close it
func (s *RealSupervisor) RCONFor(serverID int64) rcon.Client {
	s.mu.Lock()
	sess, ok := s.sessions[serverID]
	s.mu.Unlock()
	if !ok {
		return nil
	}
	sess.mu.Lock()
	defer sess.mu.Unlock()
	return sess.rcon
}

// Logs returns a channel that delivers log lines from the time of
// subscription onward. The channel closes when the server exits
func (s *RealSupervisor) Logs(serverID int64) (<-chan string, error) {
	s.mu.Lock()
	sess, ok := s.sessions[serverID]
	s.mu.Unlock()
	if !ok {
		return nil, fmt.Errorf("server %d not running", serverID)
	}
	return sess.logHub.subscribe(), nil
}

// Shutdown attempts to stop every running server
func (s *RealSupervisor) Shutdown(ctx context.Context, graceful bool) {
	s.mu.Lock()
	ids := make([]int64, 0, len(s.sessions))
	for id := range s.sessions {
		ids = append(ids, id)
	}
	s.mu.Unlock()
	for _, id := range ids {
		_ = s.Stop(ctx, id, graceful)
	}
}

func (s *RealSupervisor) watchReadiness(ctx context.Context, sess *session) {
	deadline := time.Now().Add(s.deps.ReadyTimeout)

	// Subscribe to log lines so we can detect ARK's "Server Ready"
	// signal even when the user hasn't set an RCON password
	logCh := sess.logHub.subscribe()

	ticker := time.NewTicker(s.deps.ProbeInterval)
	defer ticker.Stop()

	for {
		select {
		case <-sess.proc.Done():
			return
		case <-ctx.Done():
			return
		case line, ok := <-logCh:
			if !ok {
				logCh = nil
				continue
			}
			if isReadySignal(line) {
				s.markReady(sess, "log signal")
				return
			}
		case <-ticker.C:
			if time.Now().After(deadline) {
				s.deps.Log.Warn("server readiness timeout", "server_id", sess.serverID)
				return
			}
			if sess.getStatus() == StatusStopping {
				return
			}
			c := s.deps.RCONFactory()
			probeCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
			err := c.Dial(probeCtx, "127.0.0.1", sess.rconPort, sess.rconPwd)
			cancel()
			if err != nil {
				_ = c.Close()
				s.deps.Log.Debug("rcon probe failed", "server_id", sess.serverID, "err", err)
				continue
			}
			sess.mu.Lock()
			sess.rcon = c
			sess.mu.Unlock()
			s.markReady(sess, "rcon probe")
			return
		}
	}
}

func (s *RealSupervisor) markReady(sess *session, via string) {
	s.transition(sess, StatusRunning)
	s.publish(events.ServerStarted{ServerID: sess.serverID, Name: sess.name, At: time.Now()})
	s.deps.Log.Info("server ready", "server_id", sess.serverID, "name", sess.name, "via", via)
}

func isReadySignal(line string) bool {
	l := strings.ToLower(line)
	switch {
	case strings.Contains(l, "completed startup"):
		return true
	case strings.Contains(l, "advertising for join"):
		return true
	case strings.Contains(l, "server status: ready"),
		strings.Contains(l, "server status to ready"),
		strings.Contains(l, "server ready"):
		return true
	}
	return false
}

func (s *RealSupervisor) watchExit(sess *session) {
	waitErr := sess.proc.Wait()

	prev := sess.getStatus()
	var newStatus Status
	var ev events.Event
	if waitErr != nil && prev != StatusStopping {
		newStatus = StatusCrashed
		ev = events.ServerCrashed{
			ServerID: sess.serverID,
			Name:     sess.name,
			ExitCode: exitCodeOf(waitErr),
			At:       time.Now(),
		}
		s.deps.Log.Warn("server crashed", "server_id", sess.serverID, "err", waitErr)
	} else {
		newStatus = StatusStopped
		ev = events.ServerStopped{
			ServerID: sess.serverID,
			Name:     sess.name,
			At:       time.Now(),
		}
	}
	s.transition(sess, newStatus)
	s.publish(ev)

	sess.logHub.close()
	if sess.logFile != nil {
		_ = sess.logFile.Close()
	}
	sess.mu.Lock()
	if sess.rcon != nil {
		_ = sess.rcon.Close()
		sess.rcon = nil
	}
	sess.mu.Unlock()
	sess.cancel()
}

func (s *RealSupervisor) persistStatus(id int64, st Status) {
	if s.deps.Repo == nil {
		return
	}
	if err := s.deps.Repo.UpdateStatus(context.Background(), id, st); err != nil {
		s.deps.Log.Error("persist status", "server_id", id, "status", st, "err", err)
	}
}

func (s *RealSupervisor) publish(e events.Event) {
	if s.deps.Bus == nil {
		return
	}
	s.deps.Bus.Publish(e)
}

type exitCoder interface {
	ExitCode() int
}

func exitCodeOf(err error) int {
	var ec exitCoder
	if errors.As(err, &ec) {
		return ec.ExitCode()
	}
	return -1
}

func isTerminalStatus(st Status) bool {
	return st == StatusStopped || st == StatusCrashed
}
