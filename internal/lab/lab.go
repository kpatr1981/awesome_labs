// Package lab builds, resets and drives the Lima VMs an exercise runs on.
package lab

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"awesome_labs/internal/exercise"
	"awesome_labs/internal/lima"
)

// Pinned versions. The cluster runs one minor behind the latest stable
// release so upgrade exercises have somewhere to go.
const (
	K8sVersion        = "1.36.6"
	K8sMinor          = "v1.36"
	CalicoVersion     = "v3.33.0"
	HelmVersion       = "v4.3.1"
	MetricsVersion    = "v0.9.1"
	LocalPathVersion  = "v0.0.37"
	GatewayAPIVersion = "v1.7.0"
	// Exact apt versions, so a VM built later matches the one the labs were
	// verified on. Bump these on purpose and re-run verify.
	DockerVersion     = "5:29.9.3-1~ubuntu.24.04~noble"
	ContainerdVersion = "2.4.2-3~ubuntu.24.04~noble"
	BuildxVersion     = "0.38.1-1~ubuntu.24.04~noble"
	ComposeVersion    = "5.7.0-1~ubuntu.24.04~noble"
	CriToolsVersion   = "1.36.0-1.1"
	TsharkVersion     = "4.2.2-1.1build3"
	// KitVersion is bumped whenever bootstrap output changes incompatibly.
	KitVersion = "1"
)

// VM is one Lima instance in an environment.
type VM struct {
	Name     string
	Role     string // docker | cp | worker
	Hostname string
	CPUs     int
	Memory   string
}

// Env is the set of VMs a profile needs.
type Env struct {
	Profile exercise.Profile
	Title   string
	VMs     []VM
}

// Primary is the VM where setup/check/solve run and the shell opens.
func (e Env) Primary() VM { return e.VMs[0] }

// Worker is node01 for the k8s env.
func (e Env) Worker() (VM, bool) {
	for _, v := range e.VMs {
		if v.Role == "worker" {
			return v, true
		}
	}
	return VM{}, false
}

var (
	DockerEnv = Env{Profile: exercise.ProfileDocker, Title: "Docker host", VMs: []VM{
		{Name: "kl-docker", Role: "docker", Hostname: "docker-host", CPUs: 2, Memory: "3GiB"},
	}}
	K8sEnv = Env{Profile: exercise.ProfileK8s, Title: "Kubernetes cluster", VMs: []VM{
		{Name: "kl-cp", Role: "cp", Hostname: "controlplane", CPUs: 2, Memory: "3GiB"},
		{Name: "kl-worker", Role: "worker", Hostname: "node01", CPUs: 2, Memory: "2560MiB"},
	}}
	OffsecEnv = Env{Profile: exercise.ProfileOffsec, Title: "Offensive security lab", VMs: []VM{
		{Name: "kl-offsec", Role: "offsec", Hostname: "offsec", CPUs: 2, Memory: "3GiB"},
	}}
)

// AllEnvs is every environment, in display order.
func AllEnvs() []Env { return []Env{DockerEnv, K8sEnv, OffsecEnv} }

// EnvFor returns the environment for a profile.
func EnvFor(p exercise.Profile) Env {
	switch p {
	case exercise.ProfileK8s:
		return K8sEnv
	case exercise.ProfileOffsec:
		return OffsecEnv
	default:
		return DockerEnv
	}
}

// Manager runs lab operations. It is safe for one operation at a time.
type Manager struct {
	Assets   fs.FS // contains vms/
	StateDir string
	User     string
	Verbose  bool // copy bootstrap output to the operation log
	// AutoStop stops the other environment before an exercise starts.
	AutoStop bool
	// HoldEnvs marks every environment this process starts as busy, so that
	// another awesome_labs's auto-stop leaves it alone (used by verify).
	HoldEnvs bool
	logFile  io.Writer
	mu       sync.Mutex
}

// LogPath is where verbose output of every operation is appended.
func (m *Manager) LogPath() string { return filepath.Join(m.StateDir, "logs", "awesome_labs.log") }

// NewManager prepares ~/.awesome_labs.
func NewManager(assets fs.FS) (*Manager, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	dir := filepath.Join(home, ".awesome_labs")
	// The project used to be called kubelab: keep existing progress and settings.
	old := filepath.Join(home, ".kubelab")
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		if _, err := os.Stat(old); err == nil {
			if err := os.Rename(old, dir); err != nil {
				return nil, err
			}
		}
	}
	if err := os.MkdirAll(filepath.Join(dir, "logs"), 0o755); err != nil {
		return nil, err
	}
	u, err := user.Current()
	if err != nil {
		return nil, err
	}
	m := &Manager{Assets: assets, StateDir: dir, User: u.Username}
	f, err := os.OpenFile(m.LogPath(), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, err
	}
	m.logFile = &syncWriter{w: f}
	if err := lima.UseQEMUStreamNetdev(dir); err != nil {
		fmt.Fprintln(m.logFile, "warning: QEMU user-v2 workaround not applied:", err)
	}
	return m, nil
}

type syncWriter struct {
	mu sync.Mutex
	w  io.Writer
}

func (s *syncWriter) Write(b []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.w.Write(b)
}

// Status values.
const (
	StatusMissing  = "not created"
	StatusStopped  = "stopped"
	StatusRunning  = "running (not bootstrapped)"
	StatusReady    = "ready"
	StatusDegraded = "partially running"
)

// EnvStatus summarises the environment without changing it.
func (m *Manager) EnvStatus(ctx context.Context, env Env) string {
	all, err := lima.List(ctx)
	if err != nil {
		return "unknown: " + err.Error()
	}
	missing, running := 0, 0
	for _, v := range env.VMs {
		switch all[v.Name].Status {
		case "":
			missing++
		case "Running":
			running++
		}
	}
	switch {
	case missing == len(env.VMs):
		return StatusMissing
	case running == 0:
		return StatusStopped
	case running < len(env.VMs) || missing > 0:
		return StatusDegraded
	}
	if m.ready(ctx, env.Primary()) {
		return StatusReady
	}
	return StatusRunning
}

func (m *Manager) ready(ctx context.Context, vm VM) bool {
	var b bytes.Buffer
	code, err := lima.Run(ctx, vm.Name, lima.Script{Body: "cat /var/lib/kubelab/ready 2>/dev/null"}, &b)
	return err == nil && code == 0 && strings.TrimSpace(b.String()) == KitVersion
}

// Ensure creates, starts and bootstraps the environment as needed.
func (m *Manager) Ensure(ctx context.Context, env Env, log io.Writer) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.ensure(ctx, env, log)
}

func (m *Manager) ensure(ctx context.Context, env Env, log io.Writer) error {
	if err := lima.Available(); err != nil {
		return err
	}
	all, err := lima.List(ctx)
	if err != nil {
		return err
	}
	// Create + start every VM in parallel (image download / apt install dominate).
	var wg sync.WaitGroup
	errs := make([]error, len(env.VMs))
	for i, vm := range env.VMs {
		st := all[vm.Name].Status
		if st == "Running" {
			continue
		}
		wg.Add(1)
		go func(i int, vm VM, st string) {
			defer wg.Done()
			pw := prefixWriter(m.detail(log), vm.Hostname)
			if st == "" {
				fmt.Fprintf(log, "==> creating VM %s (%s)\n", vm.Name, vm.Hostname)
				tmpl, err := m.template(env, vm)
				if err == nil {
					err = lima.Create(ctx, vm.Name, tmpl, pw)
				}
				if err != nil {
					errs[i] = err
					return
				}
			}
			fmt.Fprintf(log, "==> starting VM %s — first boot installs packages and can take several minutes\n", vm.Name)
			errs[i] = lima.Start(ctx, vm.Name, pw)
		}(i, vm, st)
	}
	wg.Wait()
	for _, e := range errs {
		if e != nil {
			return e
		}
	}
	if err := m.pushKit(ctx, env, log); err != nil {
		return err
	}
	if env.Profile == exercise.ProfileK8s {
		if err := m.writeHosts(ctx, env, log); err != nil {
			return err
		}
	}
	if m.ready(ctx, env.Primary()) {
		return nil
	}
	fmt.Fprintf(log, "==> bootstrapping %s\n", env.Title)
	switch env.Profile {
	case exercise.ProfileDocker:
		err = m.bootstrapDocker(ctx, env, log)
	case exercise.ProfileOffsec:
		err = m.bootstrapOffsec(ctx, env, log)
	default:
		err = m.bootstrapK8s(ctx, env, log)
	}
	if err != nil {
		return err
	}
	for _, vm := range env.VMs {
		resetSSHMux(vm.Name) // pick up new group memberships (docker group)
		if _, err := m.root(ctx, vm, "echo "+KitVersion+" >/var/lib/kubelab/ready", nil, log); err != nil {
			return err
		}
	}
	m.SetTainted(env, false)
	fmt.Fprintf(log, "==> %s is ready\n", env.Title)
	return nil
}

func (m *Manager) template(env Env, vm VM) (string, error) {
	name := "vms/docker.yaml"
	switch env.Profile {
	case exercise.ProfileK8s:
		name = "vms/k8s-node.yaml"
	case exercise.ProfileOffsec:
		name = "vms/offsec.yaml"
	}
	b, err := fs.ReadFile(m.Assets, name)
	if err != nil {
		return "", err
	}
	r := strings.NewReplacer(
		"__HOSTNAME__", vm.Hostname,
		"__K8S_MINOR__", K8sMinor,
		"__K8S_VERSION__", K8sVersion,
		"__DOCKER_VERSION__", DockerVersion,
		"__CONTAINERD_VERSION__", ContainerdVersion,
		"__BUILDX_VERSION__", BuildxVersion,
		"__COMPOSE_VERSION__", ComposeVersion,
		"__CRI_TOOLS_VERSION__", CriToolsVersion,
		"__TSHARK_VERSION__", TsharkVersion,
		"__CPUS__", fmt.Sprint(vm.CPUs),
		"__MEMORY__", vm.Memory,
	)
	return r.Replace(string(b)), nil
}

// pushKit copies vms/kit/* to /opt/kubelab on every VM.
func (m *Manager) pushKit(ctx context.Context, env Env, log io.Writer) error {
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	err := fs.WalkDir(m.Assets, "vms/kit", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := fs.ReadFile(m.Assets, p)
		if err != nil {
			return err
		}
		hdr := &tar.Header{Name: strings.TrimPrefix(p, "vms/kit/"), Mode: 0o755, Size: int64(len(data)), ModTime: time.Now()}
		if err := tw.WriteHeader(hdr); err != nil {
			return err
		}
		_, err = tw.Write(data)
		return err
	})
	if err != nil {
		return err
	}
	tw.Close()
	gz.Close()
	for _, vm := range env.VMs {
		code, err := lima.Run(ctx, vm.Name, lima.Script{
			AsRoot: true,
			Body:   "mkdir -p /opt/kubelab /var/lib/kubelab && tar -m --warning=no-timestamp -xzf - -C /opt/kubelab",
			Stdin:  bytes.NewReader(buf.Bytes()),
		}, m.detail(log))
		if err != nil || code != 0 {
			return fmt.Errorf("copy kit to %s failed (exit %d): %v", vm.Name, code, err)
		}
	}
	return nil
}

func (m *Manager) bootstrapScript(name string) (string, error) {
	b, err := fs.ReadFile(m.Assets, "vms/bootstrap/"+name)
	return string(b), err
}

// root runs a script as root on vm, failing on non-zero exit.
func (m *Manager) root(ctx context.Context, vm VM, body string, env map[string]string, log io.Writer) (string, error) {
	var out bytes.Buffer
	ws := []io.Writer{&out, prefixWriter(m.logFile, vm.Hostname)}
	if log != nil && log != m.logFile && log != io.Discard {
		ws = append(ws, prefixWriter(log, vm.Hostname))
	}
	w := io.MultiWriter(ws...)
	code, err := lima.Run(ctx, vm.Name, lima.Script{AsRoot: true, Body: body, Env: env}, w)
	if err != nil {
		return out.String(), err
	}
	if code != 0 {
		return out.String(), fmt.Errorf("script on %s exited %d", vm.Hostname, code)
	}
	return out.String(), nil
}

func (m *Manager) runBootstrap(ctx context.Context, vm VM, script string, env map[string]string, log io.Writer) error {
	body, err := m.bootstrapScript(script)
	if err != nil {
		return err
	}
	if env == nil {
		env = map[string]string{}
	}
	env["LAB_USER"] = m.User
	fmt.Fprintf(log, "==> [%s] %s\n", vm.Hostname, script)
	_, err = m.root(ctx, vm, body, env, io.Discard)
	return err
}

func (m *Manager) bootstrapDocker(ctx context.Context, env Env, log io.Writer) error {
	vm := env.Primary()
	for _, s := range []string{"common.sh", "docker.sh"} {
		if err := m.runBootstrap(ctx, vm, s, nil, log); err != nil {
			return err
		}
	}
	return nil
}

func (m *Manager) bootstrapOffsec(ctx context.Context, env Env, log io.Writer) error {
	vm := env.Primary()
	for _, s := range []string{"common.sh", "offsec.sh"} {
		if err := m.runBootstrap(ctx, vm, s, nil, log); err != nil {
			return err
		}
	}
	fmt.Fprintf(log, "==> taking pristine snapshot\n")
	_, err := m.root(ctx, vm, "/opt/kubelab/state.sh snapshot offsec", nil, m.detail(log))
	return err
}

func (m *Manager) ip(ctx context.Context, vm VM) (string, error) {
	var b bytes.Buffer
	code, err := lima.Run(ctx, vm.Name, lima.Script{Body: "hostname -I | awk '{print $1}'"}, &b)
	if err != nil || code != 0 {
		return "", fmt.Errorf("cannot read IP of %s: %v", vm.Name, err)
	}
	return strings.TrimSpace(b.String()), nil
}

func (m *Manager) writeHosts(ctx context.Context, env Env, log io.Writer) error {
	cp := env.Primary()
	w, _ := env.Worker()
	cpIP, err := m.ip(ctx, cp)
	if err != nil {
		return err
	}
	wIP, err := m.ip(ctx, w)
	if err != nil {
		return err
	}
	for _, vm := range env.VMs {
		if err := m.runBootstrap(ctx, vm, "k8s-hosts.sh", map[string]string{"CP_IP": cpIP, "WORKER_IP": wIP}, io.Discard); err != nil {
			return err
		}
	}
	return nil
}

func (m *Manager) bootstrapK8s(ctx context.Context, env Env, log io.Writer) error {
	cp := env.Primary()
	w, _ := env.Worker()
	vers := map[string]string{
		"K8S_VERSION": K8sVersion, "CALICO_VERSION": CalicoVersion, "HELM_VERSION": HelmVersion,
		"METRICS_VERSION": MetricsVersion, "LOCALPATH_VERSION": LocalPathVersion, "GATEWAY_API_VERSION": GatewayAPIVersion,
	}
	for _, vm := range env.VMs {
		if err := m.runBootstrap(ctx, vm, "common.sh", nil, log); err != nil {
			return err
		}
	}
	// pre-pull workload images on the worker while the control plane initialises
	prepull := make(chan error, 1)
	go func() { prepull <- m.runBootstrap(ctx, w, "k8s-prepull.sh", nil, log) }()

	if err := m.runBootstrap(ctx, cp, "k8s-init.sh", vers, log); err != nil {
		return err
	}
	// ssh from controlplane to node01 as the lab user
	pub, err := m.root(ctx, cp, fmt.Sprintf(`H=$(getent passwd %[1]s | cut -d: -f6)
sudo -u %[1]s bash -c 'test -f ~/.ssh/id_ed25519 || ssh-keygen -q -t ed25519 -N "" -f ~/.ssh/id_ed25519'
printf 'Host node01 controlplane\n  StrictHostKeyChecking no\n  UserKnownHostsFile /dev/null\n  LogLevel ERROR\n' > "$H/.ssh/config"; chown %[1]s: "$H/.ssh/config"
cat "$H/.ssh/id_ed25519.pub"`, m.User), nil, m.detail(log))
	if err != nil {
		return err
	}
	pub = strings.TrimSpace(lastLine(pub))
	if _, err := m.root(ctx, w, fmt.Sprintf(`H=$(getent passwd %s | cut -d: -f6)
grep -qF %q "$H/.ssh/authorized_keys" || echo %q >> "$H/.ssh/authorized_keys"`, m.User, pub, pub), nil, m.detail(log)); err != nil {
		return err
	}
	if err := <-prepull; err != nil {
		return err
	}
	// join the worker
	join, err := m.root(ctx, cp, "kubeadm token create --print-join-command 2>/dev/null | tail -1", nil, m.detail(log))
	if err != nil {
		return err
	}
	join = strings.TrimSpace(lastLine(join))
	if !strings.HasPrefix(join, "kubeadm join") {
		return fmt.Errorf("unexpected join command: %q", join)
	}
	fmt.Fprintf(log, "==> [node01] joining the cluster\n")
	if _, err := m.root(ctx, w, "test -f /etc/kubernetes/kubelet.conf || "+join+" --node-name node01", nil, m.detail(log)); err != nil {
		return err
	}
	if err := m.runBootstrap(ctx, cp, "k8s-addons.sh", nil, log); err != nil {
		return err
	}
	if err := m.runBootstrap(ctx, cp, "k8s-prepull.sh", nil, log); err != nil {
		return err
	}
	fmt.Fprintf(log, "==> taking pristine snapshots\n")
	if _, err := m.root(ctx, cp, "/opt/kubelab/state.sh snapshot cp", nil, m.detail(log)); err != nil {
		return err
	}
	if _, err := m.root(ctx, w, "/opt/kubelab/state.sh snapshot worker", nil, m.detail(log)); err != nil {
		return err
	}
	return m.runBootstrap(ctx, cp, "k8s-finalize.sh", nil, log)
}

// Reset restores every VM of env to its pristine state.
func (m *Manager) Reset(ctx context.Context, env Env, log io.Writer) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.pushKit(ctx, env, log); err != nil {
		return err
	}
	m.clearActiveIn(env)
	return m.reset(ctx, env, log)
}

func (m *Manager) reset(ctx context.Context, env Env, log io.Writer) error {
	fmt.Fprintf(log, "==> resetting %s\n", env.Title)
	switch env.Profile {
	case exercise.ProfileDocker:
		_, err := m.root(ctx, env.Primary(), "/opt/kubelab/reset-docker.sh", nil, log)
		return err
	case exercise.ProfileOffsec:
		_, err := m.root(ctx, env.Primary(), "/opt/kubelab/reset-offsec.sh", nil, log)
		return err
	}
	var wg sync.WaitGroup
	errs := make([]error, len(env.VMs))
	for i, vm := range env.VMs {
		wg.Add(1)
		go func(i int, vm VM) {
			defer wg.Done()
			_, errs[i] = m.root(ctx, vm, "/opt/kubelab/reset-k8s-node.sh", nil, log)
		}(i, vm)
	}
	wg.Wait()
	for _, e := range errs {
		if e != nil {
			return e
		}
	}
	_, err := m.root(ctx, env.Primary(), "/opt/kubelab/k8s-api.sh reset", nil, log)
	return err
}

// Stop shuts the environment's VMs down.
func (m *Manager) Stop(ctx context.Context, env Env, log io.Writer) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	all, err := lima.List(ctx)
	if err != nil {
		return err
	}
	for _, vm := range env.VMs {
		if all[vm.Name].Status == "Running" {
			fmt.Fprintf(log, "==> stopping %s\n", vm.Name)
			if err := lima.Stop(ctx, vm.Name, log); err != nil {
				return err
			}
		}
	}
	return nil
}

// StopAfterPass stops an exercise's environment once the exercise is solved, so a
// finished lab doesn't keep using memory. It honours auto_stop and leaves alone an
// environment that a verify run is using.
func (m *Manager) StopAfterPass(ctx context.Context, ex *exercise.Exercise, log io.Writer) error {
	if !m.AutoStop {
		return nil
	}
	env := EnvFor(ex.Profile)
	if m.busy(env) {
		fmt.Fprintf(log, "==> leaving %s running: a awesome_labs verify run is using it\n", env.Title)
		return nil
	}
	return m.Stop(ctx, env, log)
}

// stopOthers stops every running VM that belongs to another environment.
func (m *Manager) stopOthers(ctx context.Context, keep Env, log io.Writer) {
	all, err := lima.List(ctx)
	if err != nil {
		return
	}
	for _, env := range AllEnvs() {
		if env.Profile == keep.Profile {
			continue
		}
		if m.busy(env) {
			fmt.Fprintf(log, "==> leaving %s running: a awesome_labs verify run is using it\n", env.Title)
			continue
		}
		for _, vm := range env.VMs {
			if all[vm.Name].Status == "Running" {
				fmt.Fprintf(log, "==> stopping %s (%s) to free memory — auto_stop in %s\n", vm.Name, env.Title, filepath.Join(m.StateDir, "config.yaml"))
				_ = lima.Stop(ctx, vm.Name, m.detail(log))
			}
		}
	}
}

func (m *Manager) lockPath(env Env) string {
	return filepath.Join(m.StateDir, "busy-"+string(env.Profile)+".pid")
}

// hold marks env as in use by this process.
func (m *Manager) hold(env Env) {
	_ = os.WriteFile(m.lockPath(env), []byte(strconv.Itoa(os.Getpid())), 0o644)
}

// Release drops the busy marks this process holds.
func (m *Manager) Release() {
	for _, env := range AllEnvs() {
		if b, err := os.ReadFile(m.lockPath(env)); err == nil && strings.TrimSpace(string(b)) == strconv.Itoa(os.Getpid()) {
			_ = os.Remove(m.lockPath(env))
		}
	}
}

// busy reports whether another live awesome_labs process holds env.
func (m *Manager) busy(env Env) bool {
	b, err := os.ReadFile(m.lockPath(env))
	if err != nil {
		return false
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(b)))
	if err != nil || pid == os.Getpid() {
		return false
	}
	return syscall.Kill(pid, 0) == nil
}

// StopAll stops every running lab VM.
func (m *Manager) StopAll(ctx context.Context, log io.Writer) error {
	for _, env := range AllEnvs() {
		if m.busy(env) {
			fmt.Fprintf(log, "==> leaving %s running: a awesome_labs verify run is using it\n", env.Title)
			continue
		}
		if err := m.Stop(ctx, env, log); err != nil {
			return err
		}
	}
	return nil
}

// ApplySizes overrides VM sizes (from the config file).
func ApplySizes(docker, cp, worker, offsec [2]string) {
	set := func(vm *VM, s [2]string) {
		if s[0] != "" {
			fmt.Sscan(s[0], &vm.CPUs)
		}
		if s[1] != "" {
			vm.Memory = s[1]
		}
	}
	set(&DockerEnv.VMs[0], docker)
	set(&K8sEnv.VMs[0], cp)
	set(&K8sEnv.VMs[1], worker)
	set(&OffsecEnv.VMs[0], offsec)
}

// Destroy deletes the environment's VMs.
func (m *Manager) Destroy(ctx context.Context, env Env, log io.Writer) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	all, err := lima.List(ctx)
	if err != nil {
		return err
	}
	for _, vm := range env.VMs {
		if all[vm.Name].Status != "" {
			fmt.Fprintf(log, "==> deleting %s\n", vm.Name)
			if err := lima.Delete(ctx, vm.Name, log); err != nil {
				return err
			}
		}
	}
	m.SetTainted(env, false)
	m.clearActiveIn(env)
	return nil
}

// Rebuild destroys then re-creates the environment.
func (m *Manager) Rebuild(ctx context.Context, env Env, log io.Writer) error {
	if err := m.Destroy(ctx, env, log); err != nil {
		return err
	}
	return m.Ensure(ctx, env, log)
}

// ---- exercise operations ----

func (m *Manager) exScript(body string, finish bool) string {
	s := "source /opt/kubelab/lib.sh\n" + body
	if finish {
		s += "\nkl_finish\n"
	}
	return s
}

// Start prepares an exercise: ensure env, reset, run setup.
func (m *Manager) Start(ctx context.Context, ex *exercise.Exercise, log io.Writer) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	env := EnvFor(ex.Profile)
	if m.HoldEnvs {
		m.hold(env)
	}
	if m.AutoStop {
		m.stopOthers(ctx, env, log)
	}
	if !m.HoldEnvs {
		m.ClearActive("") // one exercise at a time: this start replaces it
	}
	if m.Tainted(env) {
		fmt.Fprintf(log, "==> the previous exercise changed the cluster beyond repair — rebuilding %s\n", env.Title)
		for _, vm := range env.VMs {
			_ = lima.Delete(ctx, vm.Name, log)
		}
	}
	if err := m.ensure(ctx, env, log); err != nil {
		return err
	}
	if err := m.reset(ctx, env, log); err != nil {
		return err
	}
	fmt.Fprintf(log, "==> setting up %s: %s\n", ex.ID, ex.Title)
	// Worker first: control-plane setups often wait for its effects
	// (e.g. node01 turning NotReady).
	if ex.SetupWorker != "" {
		w, _ := env.Worker()
		if _, err := m.root(ctx, w, m.exScript(ex.SetupWorker, false), nil, log); err != nil {
			return fmt.Errorf("worker setup failed: %w", err)
		}
	}
	if ex.Setup != "" {
		if _, err := m.root(ctx, env.Primary(), m.exScript(ex.Setup, false), nil, log); err != nil {
			return fmt.Errorf("setup failed: %w", err)
		}
	}
	if ex.Destructive {
		m.SetTainted(env, true)
	}
	task := fmt.Sprintf("# %s — %s\n\n%s\n", ex.ID, ex.Title, strings.TrimSpace(ex.Scenario))
	if !m.HoldEnvs { // verify runs through every exercise; it isn't the user's work
		m.setActive(ex)
	}
	_, _ = m.root(ctx, env.Primary(), fmt.Sprintf("echo %s > /var/lib/kubelab/current\nsource /opt/kubelab/lib.sh\nlab_file /lab/TASK.md <<'AWESOME_LABS_TASK_EOF'\n%s\nAWESOME_LABS_TASK_EOF", ex.ID, task), nil, io.Discard)
	fmt.Fprintf(log, "==> ready: open a terminal and solve the task\n")
	return nil
}

// Check runs the exercise's checks. passed is false when any check fails.
func (m *Manager) Check(ctx context.Context, ex *exercise.Exercise, log io.Writer) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	env := EnvFor(ex.Profile)
	ok := true
	run := func(vm VM, body string) error {
		code, err := lima.Run(ctx, vm.Name, lima.Script{AsRoot: true, Body: m.exScript(body, true)}, log)
		if err != nil {
			return err
		}
		if code != 0 {
			ok = false
		}
		return nil
	}
	if err := run(env.Primary(), ex.Check); err != nil {
		return false, err
	}
	if ex.CheckWorker != "" {
		w, _ := env.Worker()
		fmt.Fprintln(log, "  — on node01 —")
		if err := run(w, ex.CheckWorker); err != nil {
			return false, err
		}
	}
	if ok && !m.HoldEnvs {
		m.ClearActive(ex.ID) // solved: no longer being worked on
	}
	return ok, nil
}

// Solve applies the reference solution (used by `awesome_labs verify`).
func (m *Manager) Solve(ctx context.Context, ex *exercise.Exercise, log io.Writer) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	env := EnvFor(ex.Profile)
	if strings.TrimSpace(ex.Solve) == "" {
		return fmt.Errorf("%s has no solve script", ex.ID)
	}
	code, err := lima.Run(ctx, env.Primary().Name, lima.Script{Workdir: "/lab", Body: m.exScript(ex.Solve, false)}, log)
	if err != nil {
		return err
	}
	if code != 0 {
		return fmt.Errorf("solve script exited %d", code)
	}
	return nil
}

// VMRunning reports whether the VM a shell would open on is running.
func (m *Manager) VMRunning(env Env, worker bool) bool {
	vm := env.Primary()
	if worker {
		if w, ok := env.Worker(); ok {
			vm = w
		}
	}
	st, err := lima.Status(context.Background(), vm.Name)
	return err == nil && st == "Running"
}

// ShellCommand returns an interactive shell into a VM of the environment.
func (m *Manager) ShellCommand(env Env, worker bool) *exec.Cmd {
	vm := env.Primary()
	if worker {
		if w, ok := env.Worker(); ok {
			vm = w
		}
	}
	return lima.ShellCommand(vm.Name, "/lab")
}

// ---- tainted environments (destructive exercises) ----

func (m *Manager) taintFile(env Env) string {
	return filepath.Join(m.StateDir, "tainted-"+string(env.Profile))
}

// Tainted reports whether the env must be rebuilt before the next exercise.
func (m *Manager) Tainted(env Env) bool {
	_, err := os.Stat(m.taintFile(env))
	return err == nil
}

// SetTainted marks or clears the env as needing a rebuild.
func (m *Manager) SetTainted(env Env, v bool) {
	if v {
		_ = os.WriteFile(m.taintFile(env), []byte(time.Now().Format(time.RFC3339)), 0o644)
	} else {
		_ = os.Remove(m.taintFile(env))
	}
}

// ---- helpers ----

func resetSSHMux(name string) {
	home, _ := os.UserHomeDir()
	cfg := filepath.Join(home, ".lima", name, "ssh.config")
	_ = exec.Command("ssh", "-F", cfg, "-O", "exit", "lima-"+name).Run()
}

func lastLine(s string) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	return lines[len(lines)-1]
}

type prefixed struct {
	w      io.Writer
	prefix string
	buf    []byte
	mu     sync.Mutex
}

func (p *prefixed) Write(b []byte) (int, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.buf = append(p.buf, b...)
	for {
		i := bytes.IndexByte(p.buf, '\n')
		if i < 0 {
			break
		}
		line := p.buf[:i]
		p.buf = p.buf[i+1:]
		fmt.Fprintf(p.w, "%s%s\n", p.prefix, line)
	}
	return len(b), nil
}

func prefixWriter(w io.Writer, name string) io.Writer {
	if w == nil {
		return io.Discard
	}
	return &prefixed{w: w, prefix: name + " | "}
}

// detail returns a writer for verbose output: the log file, plus the
// operation log when Verbose is set.
func (m *Manager) detail(log io.Writer) io.Writer {
	if m.Verbose {
		return io.MultiWriter(m.logFile, log)
	}
	return m.logFile
}
