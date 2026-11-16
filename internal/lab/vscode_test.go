package lab

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"syscall"
	"testing"
	"time"
)

// TestHelperSync stands in for a running `awesome_labs sync`: it holds the
// target's sync lock until it is signalled.
func TestHelperSync(t *testing.T) {
	lock := os.Getenv("KL_TEST_SYNC_LOCK")
	if lock == "" {
		t.Skip("helper process")
	}
	f, err := os.OpenFile(lock, os.O_RDWR|os.O_CREATE, 0o644)
	if err != nil {
		os.Exit(2)
	}
	if syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB) != nil {
		os.Exit(3)
	}
	_, _ = f.WriteAt([]byte(strconv.Itoa(os.Getpid())+"\n"), 0)
	time.Sleep(time.Minute)
	os.Exit(0)
}

func TestStopCodeStopsTheRunningSync(t *testing.T) {
	m := &Manager{StateDir: t.TempDir()}
	target, _ := CodeTargetFor("docker")
	if m.CodeSyncPID(target) != 0 || m.StopCode(target) {
		t.Fatal("a sync reported with none running")
	}
	if err := os.MkdirAll(filepath.Dir(m.syncLock(target)), 0o755); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestHelperSync$")
	cmd.Env = append(os.Environ(), "KL_TEST_SYNC_LOCK="+m.syncLock(target))
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() { _ = cmd.Wait(); close(done) }()
	deadline := time.Now().Add(10 * time.Second)
	for m.CodeSyncPID(target) != cmd.Process.Pid {
		if time.Now().After(deadline) {
			t.Fatal("helper never took the lock")
		}
		time.Sleep(20 * time.Millisecond)
	}
	if !m.StopCode(target) {
		t.Fatal("StopCode didn't stop the sync")
	}
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("the sync process is still running")
	}
	if m.CodeSyncPID(target) != 0 {
		t.Error("the lock is still held after the stop")
	}
}
