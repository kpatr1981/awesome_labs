package lab

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"awesome_labs/internal/lima"
)

// A base is a stopped copy of a VM taken right after its first bootstrap, so
// that the next time the environment is needed (each app launch deletes the
// lab VMs) it is cloned and booted in seconds instead of re-installing
// packages and re-pulling every image. Bases are named klbase-*, outside the
// kl-* prefix that the lab-VM deletes sweep, and are dropped by a rebuild or
// when anything that shapes a bootstrapped VM changes (see baseKey).

// baseInfo is what was recorded when a base was saved.
type baseInfo struct {
	Key string `json:"key"`
	IP  string `json:"ip"` // the k8s nodes' certificates and kubelets are bound to it
}

// baseName is the instance holding vm's base: kl-docker → klbase-docker.
func baseName(vm VM) string { return "klbase-" + strings.TrimPrefix(vm.Name, "kl-") }

func (m *Manager) baseInfoPath(vm VM) string {
	return filepath.Join(m.StateDir, "base", vm.Name+".json")
}

func (m *Manager) readBase(vm VM) (baseInfo, bool) {
	var bi baseInfo
	b, err := os.ReadFile(m.baseInfoPath(vm))
	if err != nil || json.Unmarshal(b, &bi) != nil || bi.Key == "" {
		return baseInfo{}, false
	}
	return bi, true
}

// baseKey fingerprints everything that goes into a bootstrapped VM: the Lima
// template (with its pinned versions and size), the bootstrap scripts, the
// kit, the other pinned versions and the lab user.
func (m *Manager) baseKey(env Env, vm VM) (string, error) {
	h := sha256.New()
	tmpl, err := m.template(env, vm)
	if err != nil {
		return "", err
	}
	fmt.Fprintf(h, "template\x00%s\x00", tmpl)
	fmt.Fprintf(h, "versions\x00%s\x00", strings.Join([]string{KitVersion, K8sVersion, CalicoVersion, HelmVersion,
		MetricsVersion, LocalPathVersion, GatewayAPIVersion, CriToolsVersion, TsharkVersion}, " "))
	fmt.Fprintf(h, "user\x00%s\x00role\x00%s\x00host\x00%s\x00", m.User, vm.Role, vm.Hostname)
	for _, dir := range []string{"vms/bootstrap", "vms/kit"} {
		err := fs.WalkDir(m.Assets, dir, func(p string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return err
			}
			data, err := fs.ReadFile(m.Assets, p)
			if err != nil {
				return err
			}
			fmt.Fprintf(h, "%s\x00%d\x00", p, len(data))
			h.Write(data)
			return nil
		})
		if err != nil {
			return "", err
		}
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// usableBases reports whether every VM of env has a saved, up-to-date base.
// Stale bases are deleted, since a fresh build replaces them anyway.
func (m *Manager) usableBases(ctx context.Context, env Env, all map[string]lima.Instance, log io.Writer) bool {
	ok := true
	for _, vm := range env.VMs {
		bi, has := m.readBase(vm)
		key, err := m.baseKey(env, vm)
		if !has || err != nil || all[baseName(vm)].Status != "Stopped" {
			ok = false
			continue
		}
		if bi.Key != key {
			fmt.Fprintf(log, "==> the saved copy of %s is out of date — building it from scratch\n", vm.Name)
			m.dropBase(ctx, vm, all, m.detail(log))
			ok = false
		}
	}
	return ok
}

// dropBase deletes vm's base, if any.
func (m *Manager) dropBase(ctx context.Context, vm VM, all map[string]lima.Instance, log io.Writer) {
	_ = os.Remove(m.baseInfoPath(vm))
	if all == nil || all[baseName(vm)].Status != "" {
		_ = lima.Delete(ctx, baseName(vm), log)
	}
}

// DropBases deletes the saved copies of env's VMs, so the next start builds
// them from scratch.
func (m *Manager) DropBases(ctx context.Context, env Env, log io.Writer) {
	all, _ := lima.List(ctx)
	for _, vm := range env.VMs {
		m.dropBase(ctx, vm, all, log)
	}
}

// basesMatch reports whether VMs cloned from their bases came up on the
// addresses the bases were saved with. The k8s nodes need it; the other
// environments always get the same address.
func (m *Manager) basesMatch(ctx context.Context, env Env) bool {
	for _, vm := range env.VMs {
		bi, _ := m.readBase(vm)
		ip, err := m.ip(ctx, vm)
		if err != nil || ip != bi.IP {
			return false
		}
	}
	return true
}

// saveBases stops env's freshly bootstrapped VMs, clones each into its base
// and starts them again. Failing to save is not fatal: the next launch just
// builds the environment again.
func (m *Manager) saveBases(ctx context.Context, env Env, log io.Writer) error {
	ips := map[string]string{}
	for _, vm := range env.VMs {
		// The image pulls are best effort (a registry rate limit only costs a
		// pull at exercise time), but such a VM must not become the copy every
		// later launch starts from.
		if missing := m.missingImages(ctx, vm); missing != "" {
			fmt.Fprintf(log, "==> not saving a copy of %s: %s could not pull %s — the next launch builds it again\n", env.Title, vm.Hostname, missing)
			return nil
		}
	}
	fmt.Fprintf(log, "==> saving a copy of %s so the next launch starts in seconds\n", env.Title)
	for _, vm := range env.VMs {
		ip, err := m.ip(ctx, vm)
		if err != nil {
			fmt.Fprintf(log, "==> not saving a copy: %v\n", err)
			return nil
		}
		ips[vm.Name] = ip
	}
	all, _ := lima.List(ctx)
	d := m.detail(log)
	saveErr := func() error {
		for _, vm := range env.VMs {
			if err := lima.Stop(ctx, vm.Name, d); err != nil {
				return err
			}
		}
		for _, vm := range env.VMs {
			key, err := m.baseKey(env, vm)
			if err != nil {
				return err
			}
			m.dropBase(ctx, vm, all, d)
			if err := lima.Clone(ctx, vm.Name, baseName(vm), d); err != nil {
				return err
			}
			b, _ := json.Marshal(baseInfo{Key: key, IP: ips[vm.Name]})
			if err := os.MkdirAll(filepath.Dir(m.baseInfoPath(vm)), 0o755); err != nil {
				return err
			}
			if err := os.WriteFile(m.baseInfoPath(vm), b, 0o644); err != nil {
				return err
			}
		}
		return nil
	}()
	if saveErr != nil {
		fmt.Fprintf(log, "==> could not save a copy (the next launch builds again): %v\n", saveErr)
		for _, vm := range env.VMs {
			m.dropBase(ctx, vm, nil, d)
		}
	}
	return m.startAll(ctx, env, log)
}

// missingImages lists the pre-pulled images (vms/kit/images.txt) that vm
// doesn't have, or "" when none are missing.
func (m *Manager) missingImages(ctx context.Context, vm VM) string {
	var tag, has string
	switch vm.Role {
	case "docker":
		tag, has = "d", "docker image inspect"
	case "cp", "worker":
		tag, has = "k", "crictl inspecti"
	default:
		return "" // offsec's bootstrap fails outright when a pull fails
	}
	var out strings.Builder
	_, err := lima.Run(ctx, vm.Name, lima.Script{AsRoot: true, Body: fmt.Sprintf(
		`grep -v '^#' /opt/kubelab/images.txt | awk '$2 ~ /%s/ {print $1}' | while read -r i; do %s "$i" >/dev/null 2>&1 || echo "$i"; done`,
		tag, has)}, &out)
	if err != nil {
		return "its images (cannot check: " + err.Error() + ")"
	}
	return strings.Join(strings.Fields(out.String()), ", ")
}

// startAll starts every VM of env that isn't running, in parallel.
func (m *Manager) startAll(ctx context.Context, env Env, log io.Writer) error {
	all, err := lima.List(ctx)
	if err != nil {
		return err
	}
	errs := make(chan error, len(env.VMs))
	for _, vm := range env.VMs {
		go func(vm VM) {
			if all[vm.Name].Status == "Running" {
				errs <- nil
				return
			}
			errs <- lima.Start(ctx, vm.Name, prefixWriter(m.detail(log), vm.Hostname))
		}(vm)
	}
	var first error
	for range env.VMs {
		if err := <-errs; err != nil && first == nil {
			first = err
		}
	}
	return first
}

// HasBase reports whether every VM of env has an up-to-date saved copy, so the
// environment starts in seconds. It only looks; it never deletes anything.
func (m *Manager) HasBase(ctx context.Context, env Env) bool {
	if m.Assets == nil {
		return false
	}
	all, err := lima.List(ctx)
	if err != nil {
		return false
	}
	for _, vm := range env.VMs {
		bi, has := m.readBase(vm)
		key, err := m.baseKey(env, vm)
		if !has || err != nil || bi.Key != key || all[baseName(vm)].Status != "Stopped" {
			return false
		}
	}
	return true
}

// Prepare builds env ahead of time and keeps only its saved copy: the next
// exercise that needs it then starts in seconds. An environment that already
// has an up-to-date copy is left alone. Its running VMs are deleted
// afterwards, so preparing several environments one after another never needs
// more memory than the largest of them.
func (m *Manager) Prepare(ctx context.Context, env Env, log io.Writer) error {
	if m.HasBase(ctx, env) {
		fmt.Fprintf(log, "==> %s already has a saved copy — nothing to build\n", env.Title)
		return nil
	}
	// A copy is only saved by a build from scratch, so VMs left from an
	// earlier build without one go first.
	if err := m.Destroy(ctx, env, log); err != nil {
		return err
	}
	fmt.Fprintf(log, "==> building %s (the first build takes several minutes)\n", env.Title)
	if err := m.Ensure(ctx, env, log); err != nil {
		return err
	}
	if !m.HasBase(ctx, env) {
		fmt.Fprintf(log, "==> %s is built, but its copy could not be saved: its first exercise builds it again\n", env.Title)
	}
	if err := m.Destroy(ctx, env, log); err != nil {
		return err
	}
	fmt.Fprintf(log, "==> %s is prepared: its exercises now start in seconds\n", env.Title)
	return nil
}
