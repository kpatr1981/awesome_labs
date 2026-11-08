package lab

import (
	"os"
	"os/exec"
	"strconv"
	"testing"
)

func TestBusyLock(t *testing.T) {
	m := &Manager{StateDir: t.TempDir()}
	if m.busy(K8sEnv) {
		t.Fatal("no lock file: must not be busy")
	}
	// our own hold never blocks us
	m.hold(K8sEnv)
	if m.busy(K8sEnv) {
		t.Fatal("own lock must not count as busy")
	}
	m.Release()
	if _, err := os.Stat(m.lockPath(K8sEnv)); !os.IsNotExist(err) {
		t.Fatal("Release must remove our lock")
	}
	// another live process holds it
	p := exec.Command("sleep", "30")
	if err := p.Start(); err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile(m.lockPath(K8sEnv), []byte(strconv.Itoa(p.Process.Pid)), 0o644)
	if !m.busy(K8sEnv) {
		t.Fatal("live holder must count as busy")
	}
	m.Release() // not ours: must stay
	if !m.busy(K8sEnv) {
		t.Fatal("Release must not remove another process's lock")
	}
	_ = p.Process.Kill()
	_ = p.Wait()
	if m.busy(K8sEnv) {
		t.Fatal("dead holder must not count as busy")
	}
}
