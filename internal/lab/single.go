package lab

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

// ErrRunning is returned by LockApp when another awesome_labs holds the lock.
var ErrRunning = errors.New("another awesome_labs is already running")

// LockApp makes this process the only awesome_labs that may run: it takes an
// exclusive flock on app.lock in the state directory, which the kernel drops
// when the process exits, even on a crash. release gives it up early.
func (m *Manager) LockApp() (release func(), err error) {
	p := filepath.Join(m.StateDir, "app.lock")
	f, err := os.OpenFile(p, os.O_RDWR|os.O_CREATE, 0o644)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		b, _ := os.ReadFile(p)
		f.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) {
			if pid := strings.TrimSpace(string(b)); pid != "" {
				return nil, fmt.Errorf("%w (pid %s) — close it first", ErrRunning, pid)
			}
			return nil, fmt.Errorf("%w — close it first", ErrRunning)
		}
		return nil, err
	}
	_ = f.Truncate(0)
	_, _ = f.WriteAt([]byte(strconv.Itoa(os.Getpid())+"\n"), 0)
	return func() { f.Close() }, nil
}

// AppLockHolder is the pid of the awesome_labs holding app.lock, or 0.
func (m *Manager) AppLockHolder() int {
	p := filepath.Join(m.StateDir, "app.lock")
	f, err := os.OpenFile(p, os.O_RDWR, 0)
	if err != nil {
		return 0
	}
	defer f.Close()
	if syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB) == nil {
		_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		return 0
	}
	b, _ := os.ReadFile(p)
	pid, _ := strconv.Atoi(strings.TrimSpace(string(b)))
	return pid
}

// StaleBusy lists the environments marked busy by a process that is gone
// (a verify run that crashed): auto-stop and the sync skip them for nothing.
func (m *Manager) StaleBusy() []Env {
	var out []Env
	for _, env := range AllEnvs() {
		b, err := os.ReadFile(m.lockPath(env))
		if err != nil {
			continue
		}
		pid, err := strconv.Atoi(strings.TrimSpace(string(b)))
		if err != nil || syscall.Kill(pid, 0) != nil {
			out = append(out, env)
		}
	}
	return out
}

// ClearStaleBusy removes the busy marks StaleBusy finds.
func (m *Manager) ClearStaleBusy() {
	for _, env := range m.StaleBusy() {
		_ = os.Remove(m.lockPath(env))
	}
}
