package lab

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"awesome_labs/internal/exercise"
	"awesome_labs/internal/lima"
)

// Active is the one exercise currently set up on the lab VMs. Starting an
// exercise resets the environment, so only the latest start is really in
// progress; earlier unsolved starts no longer have their lab state.
type Active struct {
	ID      string           `json:"id"`
	Profile exercise.Profile `json:"profile"`
	Since   time.Time        `json:"since"`
}

func (m *Manager) activePath() string { return filepath.Join(m.StateDir, "active.json") }

// Active returns the exercise being worked on, if any.
func (m *Manager) Active() (Active, bool) {
	var a Active
	b, err := os.ReadFile(m.activePath())
	if err != nil || json.Unmarshal(b, &a) != nil || a.ID == "" {
		return Active{}, false
	}
	return a, true
}

func (m *Manager) setActive(ex *exercise.Exercise) {
	b, _ := json.Marshal(Active{ID: ex.ID, Profile: ex.Profile, Since: time.Now()})
	tmp := m.activePath() + ".tmp"
	if os.WriteFile(tmp, b, 0o644) == nil {
		_ = os.Rename(tmp, m.activePath())
	}
}

// ClearActive forgets the active exercise when it is id ("" clears any).
func (m *Manager) ClearActive(id string) {
	if a, ok := m.Active(); ok && (id == "" || a.ID == id) {
		_ = os.Remove(m.activePath())
	}
}

// EndExercise abandons ex when it is the active exercise: it is forgotten and
// its environment's VMs are stopped. An environment that a verify run is using
// is left running.
func (m *Manager) EndExercise(ctx context.Context, ex *exercise.Exercise, log io.Writer) error {
	m.ClearActive(ex.ID)
	env := EnvFor(ex.Profile)
	if m.busy(env) {
		fmt.Fprintf(log, "==> leaving %s running: a awesome_labs verify run is using it\n", env.Title)
		return nil
	}
	return m.Stop(ctx, env, log)
}

// clearActiveIn forgets the active exercise when it lives in env, whose lab
// state is about to be wiped.
func (m *Manager) clearActiveIn(env Env) {
	if a, ok := m.Active(); ok && a.Profile == env.Profile {
		_ = os.Remove(m.activePath())
	}
}

// VMInfo is the live state of one lab VM.
type VMInfo struct {
	Name   string
	Env    string // environment title, "" for a stray kl-* instance
	Role   string
	Status string // Running, Stopped, … or "" when not created
	CPUs   int
	Memory int64 // bytes, as Lima reports it
}

// Running reports whether the VM is up.
func (v VMInfo) Running() bool { return v.Status == "Running" }

// MemoryLabel is the memory size in GiB, or "-".
func (v VMInfo) MemoryLabel() string {
	if v.Memory <= 0 {
		return "-"
	}
	return fmt.Sprintf("%.0fGiB", float64(v.Memory)/(1<<30))
}

// VMs lists every lab VM: the ones the environments define (created or not),
// then any other kl-* instance left behind.
func (m *Manager) VMs(ctx context.Context) ([]VMInfo, error) {
	all, err := lima.List(ctx)
	if err != nil {
		return nil, err
	}
	known := map[string]bool{}
	var out []VMInfo
	for _, env := range AllEnvs() {
		for _, vm := range env.VMs {
			known[vm.Name] = true
			in := all[vm.Name]
			out = append(out, VMInfo{Name: vm.Name, Env: env.Title, Role: vm.Role, Status: in.Status, CPUs: in.CPUs, Memory: in.Memory})
		}
	}
	var stray []string
	for name := range all {
		if !known[name] && strings.HasPrefix(name, "kl-") {
			stray = append(stray, name)
		}
	}
	sort.Strings(stray)
	for _, name := range stray {
		in := all[name]
		out = append(out, VMInfo{Name: name, Status: in.Status, CPUs: in.CPUs, Memory: in.Memory})
	}
	return out, nil
}

// envOf returns the environment a VM belongs to.
func envOf(name string) (Env, bool) {
	for _, env := range AllEnvs() {
		for _, vm := range env.VMs {
			if vm.Name == name {
				return env, true
			}
		}
	}
	return Env{}, false
}

// StopVM stops one lab VM.
func (m *Manager) StopVM(ctx context.Context, name string, log io.Writer) error {
	if !strings.HasPrefix(name, "kl-") {
		return fmt.Errorf("%s is not a lab VM", name)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if env, ok := envOf(name); ok && m.busy(env) {
		return fmt.Errorf("%s is in use by a running awesome_labs verify", name)
	}
	fmt.Fprintf(log, "==> stopping %s\n", name)
	return lima.Stop(ctx, name, log)
}

// DeleteVM deletes one lab VM. The next exercise on its environment
// re-creates it.
func (m *Manager) DeleteVM(ctx context.Context, name string, log io.Writer) error {
	if !strings.HasPrefix(name, "kl-") {
		return fmt.Errorf("%s is not a lab VM", name)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	env, ok := envOf(name)
	if ok && m.busy(env) {
		return fmt.Errorf("%s is in use by a running awesome_labs verify", name)
	}
	fmt.Fprintf(log, "==> deleting %s\n", name)
	if err := lima.Delete(ctx, name, log); err != nil {
		return err
	}
	if ok {
		m.clearActiveIn(env)
	}
	return nil
}

// DeleteAll deletes every lab VM, including stray kl-* instances.
func (m *Manager) DeleteAll(ctx context.Context, log io.Writer) error {
	vms, err := m.VMs(ctx)
	if err != nil {
		return err
	}
	for _, v := range vms {
		if v.Status == "" {
			continue
		}
		if err := m.DeleteVM(ctx, v.Name, log); err != nil {
			return err
		}
	}
	for _, env := range AllEnvs() {
		m.SetTainted(env, false)
	}
	return nil
}
