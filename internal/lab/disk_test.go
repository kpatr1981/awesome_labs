package lab

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"awesome_labs/internal/lima"
)

func TestDiskUsageCountsBlocksNotSize(t *testing.T) {
	dir := t.TempDir()
	f, err := os.Create(filepath.Join(dir, "diffdisk"))
	if err != nil {
		t.Fatal(err)
	}
	// a sparse 1 GiB disk holding 64 KiB of data
	if err := f.Truncate(1 << 30); err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteAt(make([]byte, 64<<10), 0); err != nil {
		t.Fatal(err)
	}
	f.Close()
	got := VMInfo{Dir: dir}.DiskUsage()
	if got < 64<<10 || got > 8<<20 {
		t.Fatalf("disk usage %d bytes: want the data written, not the 1 GiB allocated", got)
	}
	if (VMInfo{}).DiskUsage() != 0 {
		t.Fatal("a VM that isn't created takes no disk")
	}
}

func TestStaleStopped(t *testing.T) {
	now := time.Now()
	mk := func(name, status string, age time.Duration) VMInfo {
		dir := filepath.Join(t.TempDir(), name)
		os.MkdirAll(dir, 0o755)
		log := filepath.Join(dir, "ha.stderr.log")
		os.WriteFile(log, []byte("bye\n"), 0o644)
		at := now.Add(-age)
		os.Chtimes(log, at, at)
		os.Chtimes(dir, at, at)
		return VMInfo{Name: name, Status: status, Dir: dir}
	}
	vms := []VMInfo{
		mk("kl-old", "Stopped", 10*24*time.Hour),
		mk("kl-new", "Stopped", 2*24*time.Hour),
		mk("kl-running", "Running", 10*24*time.Hour),
		mk("kl-broken", "Broken", 10*24*time.Hour),
		{Name: "kl-missing", Status: ""},
	}
	got := StaleStopped(vms, 7*24*time.Hour, now)
	if len(got) != 1 || got[0].Name != "kl-old" {
		t.Fatalf("stale: %+v", got)
	}
	// the host agent's log written after the directory changed: the later time counts
	late := mk("kl-late", "Stopped", 10*24*time.Hour)
	recent := now.Add(-time.Hour)
	os.Chtimes(filepath.Join(late.Dir, "ha.stderr.log"), recent, recent)
	if s := late.StoppedSince(); now.Sub(s) > 2*time.Hour {
		t.Fatalf("stopped since %v: the log's later time should count", s)
	}
}

// DeleteStaleStopped deletes only the long-stopped lab VMs, never a saved
// copy, and forgets the exercise set up on a deleted VM's environment.
func TestDeleteStaleStopped(t *testing.T) {
	dir := t.TempDir()
	old := time.Now().Add(-30 * 24 * time.Hour)
	inst := func(name string, at time.Time) string {
		d := filepath.Join(dir, "lima", name)
		os.MkdirAll(d, 0o755)
		os.Chtimes(d, at, at)
		return d
	}
	docker, offsec, base := inst("kl-docker", old), inst("kl-offsec", time.Now()), inst("klbase-docker", old)
	calls := filepath.Join(dir, "calls")
	fake := filepath.Join(dir, "limactl")
	os.WriteFile(fake, []byte(`#!/bin/sh
echo "$@" >> `+calls+`
if [ "$1" = list ]; then
  echo '{"name":"kl-docker","status":"Stopped","dir":"`+docker+`"}'
  echo '{"name":"kl-offsec","status":"Stopped","dir":"`+offsec+`"}'
  echo '{"name":"klbase-docker","status":"Stopped","dir":"`+base+`"}'
fi
`), 0o755)
	defer func(b string) { lima.Binary = b }(lima.Binary)
	lima.Binary = fake
	m := &Manager{StateDir: dir}
	os.WriteFile(m.activePath(), []byte(`{"id":"d-1","profile":"docker"}`), 0o644)
	deleted, err := m.DeleteStaleStopped(context.Background(), 7*24*time.Hour, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if len(deleted) != 1 || deleted[0] != "kl-docker" {
		t.Fatalf("deleted %v", deleted)
	}
	b, _ := os.ReadFile(calls)
	if !strings.Contains(string(b), "delete --force kl-docker") || strings.Contains(string(b), "klbase") || strings.Contains(string(b), "kl-offsec") {
		t.Fatalf("limactl calls:\n%s", b)
	}
	if _, ok := m.Active(); ok {
		t.Fatal("the exercise set up on the deleted VM is still active")
	}
}
