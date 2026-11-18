package lab

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"awesome_labs/internal/lima"
)

// DiskUsage is the space a VM's instance directory really takes on the host:
// the blocks its files use, so a sparse or thin disk counts what it holds,
// not its allocated size. 0 when the VM isn't created.
func (v VMInfo) DiskUsage() int64 {
	if v.Dir == "" {
		return 0
	}
	return diskUsage(v.Dir)
}

func diskUsage(dir string) int64 {
	var total int64
	_ = filepath.WalkDir(dir, func(_ string, d fs.DirEntry, err error) error {
		if err != nil || d.Type()&fs.ModeSymlink != 0 {
			return nil
		}
		fi, err := d.Info()
		if err != nil {
			return nil
		}
		if st, ok := fi.Sys().(*syscall.Stat_t); ok {
			total += int64(st.Blocks) * 512 // st_blocks counts 512-byte units on Linux and macOS
		} else {
			total += fi.Size()
		}
		return nil
	})
	return total
}

// StoppedSince estimates when a stopped VM was stopped.
//
// The signal is Lima's instance directory itself: starting a VM creates its
// pid and socket files there (ha.pid, ha.sock, qemu.pid…) and stopping it
// removes them, which sets the directory's modification time; nothing else
// adds or removes files there while the VM is down. The host agent's logs
// (ha.stdout.log, ha.stderr.log) are written up to the moment it exits, so
// the latest of those times is when the VM went down. This holds however it
// was stopped (awesome_labs, limactl, a host reboot), which a time recorded by
// awesome_labs alone would miss. A clone that was never started dates from its
// creation. Zero when the directory can't be read.
func (v VMInfo) StoppedSince() time.Time {
	if v.Dir == "" || v.Running() {
		return time.Time{}
	}
	return stoppedSince(v.Dir)
}

func stoppedSince(dir string) time.Time {
	var latest time.Time
	for _, p := range []string{dir, filepath.Join(dir, "ha.stdout.log"), filepath.Join(dir, "ha.stderr.log")} {
		if fi, err := os.Stat(p); err == nil && fi.ModTime().After(latest) {
			latest = fi.ModTime()
		}
	}
	return latest
}

// StaleStopped picks the lab VMs that have been stopped for longer than
// maxAge at now. Only VMs Lima reports as Stopped count: a broken or running
// one is left for the user to look at.
func StaleStopped(vms []VMInfo, maxAge time.Duration, now time.Time) []VMInfo {
	var out []VMInfo
	for _, v := range vms {
		if v.Status != "Stopped" || v.Dir == "" {
			continue
		}
		if since := v.StoppedSince(); !since.IsZero() && now.Sub(since) > maxAge {
			out = append(out, v)
		}
	}
	return out
}

// DeleteStaleStopped deletes the lab VMs stopped for longer than maxAge (the
// saved klbase-* copies are not lab VMs and are never touched). Deleting a VM
// forgets the exercise set up on its environment. It returns the names it
// deleted.
func (m *Manager) DeleteStaleStopped(ctx context.Context, maxAge time.Duration, log io.Writer) ([]string, error) {
	vms, err := m.VMs(ctx)
	if err != nil {
		return nil, err
	}
	var deleted []string
	for _, v := range StaleStopped(vms, maxAge, time.Now()) {
		days := int(time.Since(v.StoppedSince()).Hours() / 24)
		fmt.Fprintf(log, "==> %s has been stopped for %d days\n", v.Name, days)
		if err := m.DeleteVM(ctx, v.Name, log); err != nil {
			return deleted, err
		}
		deleted = append(deleted, v.Name)
	}
	return deleted, nil
}

// SavedCopyUsage is the disk each environment's saved copies (klbase-*) take
// on the host, by profile; environments without one are left out.
func SavedCopyUsage(ctx context.Context) (map[string]int64, error) {
	all, err := lima.List(ctx)
	if err != nil {
		return nil, err
	}
	out := map[string]int64{}
	for _, env := range AllEnvs() {
		for _, vm := range env.VMs {
			if in, ok := all[baseName(vm)]; ok && in.Dir != "" {
				out[string(env.Profile)] += diskUsage(in.Dir)
			}
		}
	}
	return out, nil
}
