package server

import (
	"context"
	"errors"
	"io"
	"os/exec"
)

type Process interface {
	PID() int
	Done() <-chan struct{}
	Wait() error
	Kill() error
}

// Spawner starts processes
type Spawner interface {
	Spawn(ctx context.Context, exe string, args []string, workDir string, stdout, stderr io.Writer) (Process, error)
}

type RealSpawner struct{}

// Spawn starts exe in workDir, wiring stdout and stderr to the supplied writers
func (RealSpawner) Spawn(ctx context.Context, exe string, args []string, workDir string, stdout, stderr io.Writer) (Process, error) {
	if exe == "" {
		return nil, errors.New("spawn: exe path is empty")
	}
	cmd := exec.CommandContext(ctx, exe, args...)
	cmd.Dir = workDir
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	if attr := windowsHideAttr(); attr != nil {
		cmd.SysProcAttr = attr
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	p := &realProcess{cmd: cmd, done: make(chan struct{})}
	go func() {
		p.waitErr = cmd.Wait()
		close(p.done)
	}()
	return p, nil
}

type realProcess struct {
	cmd     *exec.Cmd
	done    chan struct{}
	waitErr error
}

func (p *realProcess) PID() int              { return p.cmd.Process.Pid }
func (p *realProcess) Done() <-chan struct{} { return p.done }
func (p *realProcess) Wait() error           { <-p.done; return p.waitErr }

func (p *realProcess) Kill() error {
	if p.cmd.Process == nil {
		return errors.New("process not started")
	}
	return p.cmd.Process.Kill()
}

var (
	_ Process = (*realProcess)(nil)
	_ Spawner = RealSpawner{}
)
