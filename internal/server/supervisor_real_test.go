package server

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"asamanager/internal/events"
	"asamanager/internal/rcon"
)

type stubProcess struct {
	pid     int
	done    chan struct{}
	mu      sync.Mutex
	waitErr error
	killed  bool
	closed  bool
}

func newStubProcess(pid int) *stubProcess {
	return &stubProcess{pid: pid, done: make(chan struct{})}
}

func (p *stubProcess) PID() int              { return p.pid }
func (p *stubProcess) Done() <-chan struct{} { return p.done }
func (p *stubProcess) Wait() error           { <-p.done; return p.waitErr }

func (p *stubProcess) Kill() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return nil
	}
	p.killed = true
	p.closed = true
	close(p.done)
	return nil
}

func (p *stubProcess) simulateExit(code int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return
	}
	if code != 0 {
		p.waitErr = &stubExitErr{code: code}
	}
	p.closed = true
	close(p.done)
}

type stubExitErr struct{ code int }

func (e *stubExitErr) Error() string { return fmt.Sprintf("exit %d", e.code) }
func (e *stubExitErr) ExitCode() int { return e.code }

type stubSpawner struct {
	mu     sync.Mutex
	proc   *stubProcess
	stdout io.Writer
	stderr io.Writer
}

func (s *stubSpawner) Spawn(_ context.Context, _ string, _ []string, _ string, stdout, stderr io.Writer) (Process, error) {
	s.mu.Lock()
	s.stdout = stdout
	s.stderr = stderr
	s.mu.Unlock()
	return s.proc, nil
}

func (s *stubSpawner) writeStdout(b []byte) (int, error) {
	s.mu.Lock()
	w := s.stdout
	s.mu.Unlock()
	return w.Write(b)
}

type stubRCON struct {
	mu       sync.Mutex
	dialErrs []error
	execLog  []string
	execResp string
}

func (r *stubRCON) Dial(_ context.Context, _ string, _ int, _ string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.dialErrs) > 0 {
		e := r.dialErrs[0]
		r.dialErrs = r.dialErrs[1:]
		return e
	}
	return nil
}

func (r *stubRCON) Exec(_ context.Context, cmd string) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.execLog = append(r.execLog, cmd)
	return r.execResp, nil
}

func (r *stubRCON) Close() error { return nil }

func (r *stubRCON) execCalls() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]string, len(r.execLog))
	copy(out, r.execLog)
	return out
}

func silentLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func waitFor(t *testing.T, d time.Duration, pred func() bool) bool {
	t.Helper()
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		if pred() {
			return true
		}
		time.Sleep(5 * time.Millisecond)
	}
	return pred()
}

func setupSupervisor(t *testing.T, sp Spawner, rc *stubRCON) (*RealSupervisor, *Repo, int64, *events.Bus, chan events.Event) {
	t.Helper()
	d := newTestDB(t)
	sr := NewRepo(d)
	cid := seedCluster(t, d, "c1")
	srv, err := sr.Create(context.Background(), Server{
		ClusterID:    cid,
		Name:         "Island",
		Map:          "TheIsland_WP",
		InstallDir:   t.TempDir(),
		Ports:        DefaultBase,
		RCONEnabled:  true,
		RCONPassword: "rcon-pw",
	})
	if err != nil {
		t.Fatalf("create server: %v", err)
	}
	bus := events.NewBus(64)
	bus.Start()
	t.Cleanup(bus.Stop)
	received := make(chan events.Event, 8)
	bus.SubscribeAll(func(e events.Event) {
		select {
		case received <- e:
		default:
		}
	})
	sup := NewSupervisor(SupervisorDeps{
		Repo:        sr,
		Bus:         bus,
		Log:         silentLogger(),
		Spawner:     sp,
		RCONFactory: func() rcon.Client { return rc },
		LaunchOptions: func(_ context.Context, _ Server) (LaunchOptions, error) {
			return LaunchOptions{ClusterID: "c1"}, nil
		},
		LogDir:        t.TempDir(),
		ProbeInterval: 10 * time.Millisecond,
		ReadyTimeout:  2 * time.Second,
		StopTimeout:   500 * time.Millisecond,
	})
	return sup, sr, srv.ID, bus, received
}

func waitForEvent[T events.Event](t *testing.T, ch <-chan events.Event, d time.Duration) (T, bool) {
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

func TestSupervisor_StartTransitionsToRunning(t *testing.T) {
	proc := newStubProcess(1234)
	sp := &stubSpawner{proc: proc}
	rc := &stubRCON{}
	sup, sr, id, _, recv := setupSupervisor(t, sp, rc)

	if err := sup.Start(context.Background(), id); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if !waitFor(t, 1*time.Second, func() bool { return sup.Status(id) == StatusRunning }) {
		t.Fatalf("status never reached Running, got %q", sup.Status(id))
	}
	if _, ok := waitForEvent[events.ServerStarted](t, recv, time.Second); !ok {
		t.Error("expected ServerStarted event")
	}
	dbServer, _ := sr.Get(context.Background(), id)
	if dbServer.Status != StatusRunning {
		t.Errorf("DB status = %q, want running", dbServer.Status)
	}

	// Tear down so the test goroutine doesn't leak.
	proc.simulateExit(0)
	waitFor(t, time.Second, func() bool { return sup.Status(id) == StatusStopped })
}

func TestSupervisor_GracefulStopSendsRCONCommands(t *testing.T) {
	proc := newStubProcess(1234)
	sp := &stubSpawner{proc: proc}
	rc := &stubRCON{}
	sup, _, id, _, recv := setupSupervisor(t, sp, rc)

	_ = sup.Start(context.Background(), id)
	waitFor(t, time.Second, func() bool { return sup.Status(id) == StatusRunning })

	go func() {
		time.Sleep(20 * time.Millisecond)
		proc.simulateExit(0)
	}()
	if err := sup.Stop(context.Background(), id, true); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if !waitFor(t, time.Second, func() bool { return sup.Status(id) == StatusStopped }) {
		t.Fatalf("status never reached Stopped")
	}

	calls := rc.execCalls()
	if len(calls) < 2 || calls[0] != rcon.CmdSaveWorld || calls[1] != rcon.CmdDoExit {
		t.Errorf("expected saveworld then doexit, got %v", calls)
	}
	if proc.killed {
		t.Error("graceful stop should not have killed the process")
	}
	if _, ok := waitForEvent[events.ServerSaved](t, recv, time.Second); !ok {
		t.Error("expected ServerSaved event")
	}
}

func TestSupervisor_GracefulStopFallsBackToKillOnTimeout(t *testing.T) {
	proc := newStubProcess(1234)
	sp := &stubSpawner{proc: proc}
	rc := &stubRCON{}
	sup, _, id, _, _ := setupSupervisor(t, sp, rc)

	_ = sup.Start(context.Background(), id)
	waitFor(t, time.Second, func() bool { return sup.Status(id) == StatusRunning })

	// Don't trigger simulateExit — graceful path should time out and Kill.
	if err := sup.Stop(context.Background(), id, true); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if !waitFor(t, time.Second, func() bool { return sup.Status(id) == StatusStopped }) {
		t.Fatalf("status never reached Stopped after kill fallback")
	}
	if !proc.killed {
		t.Error("expected Kill to be called after graceful stop timed out")
	}
}

func TestSupervisor_UnexpectedExitBecomesCrashed(t *testing.T) {
	proc := newStubProcess(1234)
	sp := &stubSpawner{proc: proc}
	rc := &stubRCON{}
	sup, sr, id, _, recv := setupSupervisor(t, sp, rc)

	_ = sup.Start(context.Background(), id)
	waitFor(t, time.Second, func() bool { return sup.Status(id) == StatusRunning })

	proc.simulateExit(1)
	if !waitFor(t, time.Second, func() bool { return sup.Status(id) == StatusCrashed }) {
		t.Fatalf("status never reached Crashed")
	}
	dbServer, _ := sr.Get(context.Background(), id)
	if dbServer.Status != StatusCrashed {
		t.Errorf("DB status = %q, want crashed", dbServer.Status)
	}
	ev, ok := waitForEvent[events.ServerCrashed](t, recv, time.Second)
	if !ok {
		t.Fatal("expected ServerCrashed event")
	}
	if ev.ExitCode != 1 {
		t.Errorf("ExitCode = %d, want 1", ev.ExitCode)
	}
}

func TestSupervisor_LogsSubscriberReceivesLines(t *testing.T) {
	proc := newStubProcess(1234)
	sp := &stubSpawner{proc: proc}
	rc := &stubRCON{}
	sup, _, id, _, _ := setupSupervisor(t, sp, rc)

	_ = sup.Start(context.Background(), id)
	waitFor(t, time.Second, func() bool { return sup.Status(id) == StatusRunning })

	sub, err := sup.Logs(id)
	if err != nil {
		t.Fatalf("Logs: %v", err)
	}
	if _, err := sp.writeStdout([]byte("hello\nworld\n")); err != nil {
		t.Fatalf("writeStdout: %v", err)
	}
	for _, want := range []string{"hello", "world"} {
		select {
		case got := <-sub:
			if got != want {
				t.Errorf("got %q, want %q", got, want)
			}
		case <-time.After(500 * time.Millisecond):
			t.Fatalf("no line received (wanted %q)", want)
		}
	}

	proc.simulateExit(0)
	waitFor(t, time.Second, func() bool { return sup.Status(id) == StatusStopped })
}

func TestSupervisor_StartTwiceRejected(t *testing.T) {
	proc := newStubProcess(1234)
	sp := &stubSpawner{proc: proc}
	rc := &stubRCON{}
	sup, _, id, _, _ := setupSupervisor(t, sp, rc)

	if err := sup.Start(context.Background(), id); err != nil {
		t.Fatalf("first Start: %v", err)
	}
	if err := sup.Start(context.Background(), id); err == nil {
		t.Error("second Start should fail while server is running")
	}
	proc.simulateExit(0)
	waitFor(t, time.Second, func() bool { return sup.Status(id) == StatusStopped })
}
