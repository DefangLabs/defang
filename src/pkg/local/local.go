//go:build !windows
// +build !windows

package local

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"strconv"
	"syscall"

	"github.com/DefangLabs/defang/src/pkg/types"
)

type PID = types.TaskID

type Local struct {
	entrypoint []string
	cmd        *exec.Cmd
	outReader  io.ReadCloser
	errReader  io.ReadCloser
}

var _ types.Driver = (*Local)(nil)

func New() *Local {
	return &Local{}
}

func (l *Local) SetUp(ctx context.Context, containers []types.Container) error {
	if len(containers) != 1 {
		return errors.New("expected exactly one container")
	}
	if len(containers[0].EntryPoint) == 0 {
		return errors.New("entrypoint not set")
	}
	l.entrypoint = containers[0].EntryPoint
	return nil
}

func (l *Local) TearDown(ctx context.Context) error {
	if l.cmd == nil {
		return nil
	}
	// l.cmd.Process.Kill()
	return l.cmd.Wait()
}

func (l *Local) Run(ctx context.Context, env map[string]string, args ...string) (PID, error) {
	if l.cmd != nil {
		return nil, errors.New("already running")
	}
	args = append(l.entrypoint[1:], args...)
	cmd := exec.CommandContext(ctx, l.entrypoint[0], args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	or, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	er, err := cmd.StderrPipe()
	if err != nil {
		return nil, err
	}
	for k, v := range env {
		cmd.Env = append(cmd.Env, k+"="+v)
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	l.outReader = or
	l.errReader = er
	l.cmd = cmd
	pid := strconv.Itoa(cmd.Process.Pid)
	return &pid, nil
}

func (l *Local) Tail(ctx context.Context, taskID PID) error {
	if l.cmd == nil {
		return errors.New("not running")
	}
	if strconv.Itoa(l.cmd.Process.Pid) != *taskID {
		return errors.New("task ID does not match")
	}
	go io.Copy(os.Stderr, l.errReader)
	_, err := io.Copy(os.Stdout, l.outReader)
	return err
}

func (l *Local) Stop(ctx context.Context, taskID PID) error {
	pid, err := strconv.Atoi(*taskID)
	if err != nil {
		return err
	}
	return syscall.Kill(-pid, syscall.SIGTERM) // negative pid kills the process group
}

func (l *Local) GetInfo(ctx context.Context, taskID PID) (*types.TaskInfo, error) {
	return nil, errors.New("not implemented for local driver")
}

func (l *Local) PutSecret(ctx context.Context, name, value string) error {
	return errors.New("not implemented for local driver")
}

func (l *Local) ListSecrets(ctx context.Context) ([]string, error) {
	return nil, errors.New("not implemented for local driver")
}

func (l *Local) CreateUploadURL(ctx context.Context, name string) (string, error) {
	return "", errors.New("not implemented for local driver")
}
