package lab

import (
	"archive/tar"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"awesome_labs/internal/lima"
)

// Working in the user's own VS Code: nothing is installed in the VMs. The
// workspace's terminal runs `awesome_labs shell`, and a task in it runs
// `awesome_labs sync`, which mirrors the VM's /lab into a local folder both
// ways while the window is open.

// CodeTarget is a lab VM whose /lab can be opened in VS Code.
type CodeTarget struct {
	Name   string // docker, k8s, node01, offsec: also the `awesome_labs shell` argument
	Env    Env
	VM     VM
	Worker bool
}

// CodeTargetFor resolves a name as `awesome_labs shell` takes it.
func CodeTargetFor(name string) (CodeTarget, error) {
	switch name {
	case "", "docker":
		return CodeTarget{Name: "docker", Env: DockerEnv, VM: DockerEnv.Primary()}, nil
	case "k8s", "kubernetes", "controlplane", "cp":
		return CodeTarget{Name: "k8s", Env: K8sEnv, VM: K8sEnv.Primary()}, nil
	case "node01", "worker":
		w, _ := K8sEnv.Worker()
		return CodeTarget{Name: "node01", Env: K8sEnv, VM: w, Worker: true}, nil
	case "offsec":
		return CodeTarget{Name: "offsec", Env: OffsecEnv, VM: OffsecEnv.Primary()}, nil
	}
	return CodeTarget{}, fmt.Errorf("unknown lab %q (docker, k8s, node01 or offsec)", name)
}

// CodeTargetOf is the VM an exercise's shell opens on.
func CodeTargetOf(env Env) CodeTarget {
	switch env.Profile {
	case K8sEnv.Profile:
		t, _ := CodeTargetFor("k8s")
		return t
	case OffsecEnv.Profile:
		t, _ := CodeTargetFor("offsec")
		return t
	}
	t, _ := CodeTargetFor("docker")
	return t
}

func (m *Manager) codeBase() string { return filepath.Join(m.StateDir, "vscode") }

// CodeDir is the local folder that mirrors the target's /lab.
func (m *Manager) CodeDir(t CodeTarget) string { return filepath.Join(m.codeBase(), t.Name) }

// CodeWorkspace is the VS Code workspace file for the target.
func (m *Manager) CodeWorkspace(t CodeTarget) string {
	return filepath.Join(m.codeBase(), t.Name+".code-workspace")
}

func (m *Manager) codeBackups(t CodeTarget) string {
	return filepath.Join(m.codeBase(), "backups", t.Name)
}

// WriteCodeWorkspace writes the target's workspace file: its folder, the
// terminal profiles that open a shell in the VM, and the sync task.
func (m *Manager) WriteCodeWorkspace(t CodeTarget, exe string) (string, error) {
	if err := os.MkdirAll(m.CodeDir(t), 0o755); err != nil {
		return "", err
	}
	type profile struct {
		Path string   `json:"path"`
		Args []string `json:"args"`
	}
	label := func(name string) string {
		switch name {
		case "k8s":
			return "lab: controlplane"
		case "node01":
			return "lab: node01"
		}
		return "lab: " + name
	}
	profiles := map[string]profile{label(t.Name): {exe, []string{"shell", t.Name}}}
	if t.Env.Profile == K8sEnv.Profile { // both nodes from either one
		profiles[label("k8s")] = profile{exe, []string{"shell", "k8s"}}
		profiles[label("node01")] = profile{exe, []string{"shell", "node01"}}
	}
	settings := map[string]any{}
	for _, osName := range []string{"osx", "linux"} {
		settings["terminal.integrated.profiles."+osName] = profiles
		settings["terminal.integrated.defaultProfile."+osName] = label(t.Name)
	}
	settings["task.allowAutomaticTasks"] = "on"
	ws := map[string]any{
		"folders":  []map[string]string{{"path": t.Name, "name": t.VM.Hostname + ":/lab"}},
		"settings": settings,
		"tasks": map[string]any{
			"version": "2.0.0",
			"tasks": []map[string]any{{
				"label":          "awesome_labs: sync /lab",
				"type":           "process",
				"command":        exe,
				"args":           []string{"sync", t.Name},
				"isBackground":   true,
				"problemMatcher": []string{},
				"runOptions":     map[string]string{"runOn": "folderOpen"},
				"presentation":   map[string]any{"reveal": "silent", "panel": "dedicated", "showReuseMessage": false},
			}},
		},
	}
	b, err := json.MarshalIndent(ws, "", "  ")
	if err != nil {
		return "", err
	}
	p := m.CodeWorkspace(t)
	return p, os.WriteFile(p, append(b, '\n'), 0o644)
}

// CodeBinary finds the VS Code command line: $AWESOME_LABS_CODE (e.g. codium
// or cursor), `code` on PATH, or the one inside the macOS app.
func CodeBinary() (string, error) {
	if c := os.Getenv("AWESOME_LABS_CODE"); c != "" {
		return exec.LookPath(c)
	}
	if p, err := exec.LookPath("code"); err == nil {
		return p, nil
	}
	if runtime.GOOS == "darwin" {
		home, _ := os.UserHomeDir()
		for _, app := range []string{"/Applications", filepath.Join(home, "Applications")} {
			p := filepath.Join(app, "Visual Studio Code.app", "Contents", "Resources", "app", "bin", "code")
			if _, err := os.Stat(p); err == nil {
				return p, nil
			}
		}
	}
	return "", errors.New("VS Code's `code` command was not found: in VS Code run \"Shell Command: Install 'code' command in PATH\", or set AWESOME_LABS_CODE")
}

// OpenInCode writes the target's workspace and opens it in VS Code. It
// returns the workspace file.
func (m *Manager) OpenInCode(t CodeTarget) (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	if r, err := filepath.EvalSymlinks(exe); err == nil {
		exe = r
	}
	code, err := CodeBinary()
	if err != nil {
		return "", err
	}
	ws, err := m.WriteCodeWorkspace(t, exe)
	if err != nil {
		return "", err
	}
	cmd := exec.Command(code, ws)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = nil, nil, nil
	if err := cmd.Start(); err != nil {
		return "", err
	}
	go func() { _ = cmd.Wait() }()
	return ws, nil
}

// SyncLab mirrors the target's /lab and its local folder until ctx ends.
// Only one sync per target runs at a time.
func (m *Manager) SyncLab(ctx context.Context, t CodeTarget, out io.Writer) error {
	if err := os.MkdirAll(m.CodeDir(t), 0o755); err != nil {
		return err
	}
	lp := m.syncLock(t)
	f, err := os.OpenFile(lp, os.O_RDWR|os.O_CREATE, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		b, _ := os.ReadFile(lp)
		return fmt.Errorf("%s:/lab is already being synced (pid %s)", t.VM.Hostname, strings.TrimSpace(string(b)))
	}
	_ = f.Truncate(0)
	_, _ = f.WriteAt([]byte(strconv.Itoa(os.Getpid())+"\n"), 0)

	s := newLabSync(m.CodeDir(t), m.codeBackups(t), &vmRemote{vm: t.VM.Name}, out)
	s.onPush = func() {
		m.setClean(t.Env, false) // the lab isn't pristine any more
		_ = os.WriteFile(m.pushedFile(t), nil, 0o644)
	}
	fmt.Fprintf(out, "syncing %s:/lab ↔ %s (ctrl+c stops)\n", t.VM.Hostname, m.CodeDir(t))
	waiting := ""
	for {
		var err error
		if m.busy(t.Env) {
			err = fmt.Errorf("%w: an awesome_labs verify run is using it", errUnavailable)
		} else {
			err = s.round(ctx)
		}
		if ctx.Err() != nil {
			return stopped(out)
		}
		delay := time.Second
		switch {
		case errors.Is(err, errUnavailable):
			s.first = true // the VM wins when it's back
			if msg := err.Error(); msg != waiting {
				fmt.Fprintf(out, "waiting for %s:/lab (%s)\n", t.VM.Hostname, strings.TrimPrefix(msg, errUnavailable.Error()+": "))
				waiting = msg
			}
			delay = 3 * time.Second
		case err != nil:
			fmt.Fprintf(out, "sync: %v\n", err)
			waiting = ""
		default:
			waiting = ""
		}
		select {
		case <-ctx.Done():
			return stopped(out)
		case <-time.After(delay):
		}
	}
}

func stopped(out io.Writer) error {
	fmt.Fprintln(out, "sync stopped: edits in this window no longer reach the VM — close it when you're done")
	return nil
}

func (m *Manager) pushedFile(t CodeTarget) string {
	return filepath.Join(m.codeBase(), t.Name+".pushed")
}

// CodePushed is when the VS Code sync last wrote into the target's /lab (the
// zero time if never): the app's auto-check watches it.
func (m *Manager) CodePushed(t CodeTarget) time.Time {
	fi, err := os.Stat(m.pushedFile(t))
	if err != nil {
		return time.Time{}
	}
	return fi.ModTime()
}

func (m *Manager) syncLock(t CodeTarget) string {
	return filepath.Join(m.codeBase(), t.Name+".sync.lock")
}

// CodeSyncPID is the process syncing the target's /lab, or 0 when none is.
func (m *Manager) CodeSyncPID(t CodeTarget) int {
	f, err := os.OpenFile(m.syncLock(t), os.O_RDWR, 0)
	if err != nil {
		return 0
	}
	defer f.Close()
	if syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB) == nil {
		_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		return 0 // nobody holds it
	}
	b, _ := os.ReadFile(m.syncLock(t))
	pid, _ := strconv.Atoi(strings.TrimSpace(string(b)))
	return pid
}

// StopCode stops the target's sync, if one runs. The VS Code window stays
// open (VS Code can't be told to close one window), but its files no longer
// reach the VM.
func (m *Manager) StopCode(t CodeTarget) bool {
	pid := m.CodeSyncPID(t)
	if pid <= 0 || pid == os.Getpid() {
		return false
	}
	if syscall.Kill(pid, syscall.SIGTERM) != nil {
		return false
	}
	for range 30 { // it lets go of the lock as it exits
		if m.CodeSyncPID(t) == 0 {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	return true
}

// StopAllCode stops every VS Code sync.
func (m *Manager) StopAllCode() {
	for _, n := range []string{"docker", "k8s", "node01", "offsec"} {
		t, _ := CodeTargetFor(n)
		m.StopCode(t)
	}
}

// vmRemote is the /lab of a Lima VM, read and written as root over limactl.
type vmRemote struct{ vm string }

const syncHeader = "KLSYNC\x00"

func (v *vmRemote) run(ctx context.Context, body string, stdin io.Reader) ([]byte, int, error) {
	var out bytes.Buffer
	code, err := lima.Run(ctx, v.vm, lima.Script{AsRoot: true, Body: body, Stdin: stdin, Stderr: io.Discard}, &out)
	return out.Bytes(), code, err
}

func (v *vmRemote) list(ctx context.Context) (map[string]entry, error) {
	out, code, err := v.run(ctx, `cd /lab 2>/dev/null || exit 3
printf 'KLSYNC\0'
find . -mindepth 1 -name .git -prune -o \( -type f -o -type d \) -printf '%y %T@ %s %m %P\0'`, nil)
	switch {
	case err != nil:
		return nil, fmt.Errorf("%w: %v", errUnavailable, err)
	case code == 3:
		return nil, fmt.Errorf("%w: no /lab yet", errUnavailable)
	case !bytes.HasPrefix(out, []byte(syncHeader)):
		return nil, fmt.Errorf("%w: the VM isn't running", errUnavailable)
	}
	return parseList(out[len(syncHeader):]), nil
}

// parseList reads `find -printf '%y %T@ %s %m %P\0'` records.
func parseList(b []byte) map[string]entry {
	res := map[string]entry{}
	for _, rec := range strings.Split(string(b), "\x00") {
		f := strings.SplitN(rec, " ", 5)
		if len(f) != 5 || f[4] == "" {
			continue
		}
		mode, _ := strconv.ParseUint(f[3], 8, 32)
		switch f[0] {
		case "d":
			res[f[4]] = entry{dir: true, sig: "d", mode: fs.FileMode(mode)}
		case "f":
			if size, _ := strconv.ParseInt(f[2], 10, 64); size > maxSyncSize {
				continue
			}
			res[f[4]] = entry{sig: f[1] + " " + f[2] + " " + f[3], mode: fs.FileMode(mode)}
		}
	}
	return res
}

func nulList(paths []string) io.Reader {
	return strings.NewReader(strings.Join(paths, "\x00") + "\x00")
}

func (v *vmRemote) pull(ctx context.Context, paths []string) (map[string]pulled, error) {
	out, _, err := v.run(ctx, `cd /lab && tar --null --no-recursion -T - -cf -; exit 0`, nulList(paths))
	if err != nil {
		return nil, err
	}
	res := map[string]pulled{}
	tr := tar.NewReader(bytes.NewReader(out))
	for {
		h, err := tr.Next()
		if err != nil {
			break // io.EOF, or a file that vanished mid-way cut the stream short
		}
		if h.Typeflag != tar.TypeReg || !filepath.IsLocal(h.Name) {
			continue
		}
		b, err := io.ReadAll(tr)
		if err != nil {
			break
		}
		res[filepath.ToSlash(filepath.Clean(h.Name))] = pulled{data: b, mode: fs.FileMode(h.Mode).Perm()}
	}
	return res, nil
}

// pushScript unpacks the files into a temp folder, then writes each one into
// /lab in place (keeping its owner and inode, which bind mounts depend on), or
// creates it owned by the lab user. Nothing is written outside /lab.
const pushScript = `set -u
source /opt/kubelab/lib.sh
U=$(lab_user); G=$(id -gn "$U")
T=$(mktemp -d /tmp/kl-sync-XXXXXX); trap 'rm -rf "$T"' EXIT
tar -xf - -C "$T" --no-same-owner || exit 1
cd "$T"
inside() { case "$(realpath -m "/lab/$1")" in /lab/?*) return 0;; esac; return 1; }
find . -mindepth 1 -type d -printf '%P\0' | while IFS= read -r -d '' d; do
  inside "$d" || continue
  [ -d "/lab/$d" ] || { rm -f -- "/lab/$d"; install -d -o "$U" -g "$G" -- "/lab/$d"; }
done
find . -type f -printf '%P\0' | while IFS= read -r -d '' f; do
  inside "$f" || continue
  if [ -f "/lab/$f" ] && [ ! -L "/lab/$f" ]; then
    cat -- "$f" >"/lab/$f"
  else
    rm -rf -- "/lab/$f"
    p=$(dirname -- "$f"); [ -d "/lab/$p" ] || install -d -o "$U" -g "$G" -- "/lab/$p"
    install -o "$U" -g "$G" -m "$(stat -c %a -- "$f")" -- "$f" "/lab/$f"
  fi
done`

func (v *vmRemote) push(ctx context.Context, dirs []string, files []pushItem) error {
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	now := time.Now()
	for _, d := range dirs {
		if err := tw.WriteHeader(&tar.Header{Typeflag: tar.TypeDir, Name: d + "/", Mode: 0o755, ModTime: now}); err != nil {
			return err
		}
	}
	for _, f := range files {
		h := &tar.Header{Typeflag: tar.TypeReg, Name: f.path, Mode: int64(f.mode.Perm()), Size: int64(len(f.data)), ModTime: now}
		if err := tw.WriteHeader(h); err != nil {
			return err
		}
		if _, err := tw.Write(f.data); err != nil {
			return err
		}
	}
	if err := tw.Close(); err != nil {
		return err
	}
	_, code, err := v.run(ctx, pushScript, &buf)
	if err == nil && code != 0 {
		err = fmt.Errorf("writing to the VM's /lab exited %d", code)
	}
	return err
}

func (v *vmRemote) remove(ctx context.Context, paths []string) error {
	_, code, err := v.run(ctx, `while IFS= read -r -d '' p; do
  case "$(realpath -m "/lab/$p")" in /lab/?*) rm -rf -- "/lab/$p";; esac
done`, nulList(paths))
	if err == nil && code != 0 {
		err = fmt.Errorf("deleting in the VM's /lab exited %d", code)
	}
	return err
}
