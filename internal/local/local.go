// Package local runs exercises without a Lima VM, for authors: each exercise
// gets a fresh privileged container that stands in for the kl-docker VM
// (systemd, the pinned Docker engine, the lab user, the real kit in
// /opt/kubelab), with the VM's images copied in from a prepared image store.
// It runs setup, check (must fail), solve, check (must pass) the way the lab
// runner does on a VM, then throws the container away, so no reset is needed.
//
// Everything it creates on the host's Docker is named kl-local-* and labelled
// kl-local, and is removed when a run ends, is interrupted, or (left over from
// a run that was killed) when the next run starts.
package local

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"awesome_labs/internal/exercise"
	"awesome_labs/internal/lab"
)

// Harness runs exercises in throwaway containers on the host's Docker.
type Harness struct {
	Assets   fs.FS     // the embedded vms/ tree (kit and bootstrap scripts)
	CacheDir string    // image archives to load into a new store (the lab's image cache), optional
	Docker   string    // docker CLI, default "docker"
	Log      io.Writer // full output of every docker and script step
	Progress io.Writer // one line per preparation step, optional
	Rebuild  bool      // rebuild the image store even if it is current
	Force    bool      // run exercises that reach the host (see HostRisk)
}

const (
	storeVolume = "kl-local-store"
	labelKey    = "kl-local"
	pidLabel    = "kl-local.pid"
	hashLabel   = "kl-local.hash"
)

func (h *Harness) docker() string {
	if h.Docker != "" {
		return h.Docker
	}
	return "docker"
}

func (h *Harness) progress(format string, a ...any) {
	if h.Progress != nil {
		fmt.Fprintf(h.Progress, "==> "+format+"\n", a...)
	}
	fmt.Fprintf(h.logw(), "\n==> "+format+"\n", a...)
}

func (h *Harness) logw() io.Writer {
	if h.Log == nil {
		return io.Discard
	}
	return h.Log
}

// run runs the docker CLI with output to out (and the log), failing on a
// non-zero exit.
func (h *Harness) run(ctx context.Context, out io.Writer, stdin io.Reader, args ...string) error {
	cmd := exec.CommandContext(ctx, h.docker(), args...)
	cmd.WaitDelay = 10 * time.Second
	var buf bytes.Buffer
	w := io.MultiWriter(&buf, h.logw())
	if out != nil {
		w = io.MultiWriter(&buf, out)
	}
	cmd.Stdout, cmd.Stderr, cmd.Stdin = w, w, stdin
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("docker %s: %w: %s", strings.Join(args[:min(2, len(args))], " "), err, lastLine(buf.String()))
	}
	return nil
}

// output runs the docker CLI and returns its trimmed stdout.
func (h *Harness) output(ctx context.Context, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, h.docker(), args...)
	out, err := cmd.Output()
	return strings.TrimSpace(string(out)), err
}

// Cleanup removes containers and volumes left by runs whose process is gone.
func (h *Harness) Cleanup(ctx context.Context) {
	kinds := []struct{ list, name, rm string }{
		{"ps -a", "{{.Names}}", "rm -f"},
		{"volume ls", "{{.Name}}", "volume rm -f"},
	}
	for _, k := range kinds {
		args := append(strings.Fields(k.list), "--filter", "label="+labelKey+"=run",
			"--format", `{{.Label "`+pidLabel+`"}} `+k.name)
		out, err := h.output(ctx, args...)
		if err != nil {
			continue
		}
		for _, line := range strings.Split(out, "\n") {
			f := strings.Fields(line)
			if len(f) != 2 || alive(f[0]) {
				continue
			}
			_ = h.run(ctx, io.Discard, nil, append(strings.Fields(k.rm), f[1])...)
		}
	}
}

// RemoveOwn removes everything this process created that is still there
// (after an interrupt, a docker run --rm whose CLI was killed keeps running).
func (h *Harness) RemoveOwn() {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	pid := pidLabel + "=" + strconv.Itoa(os.Getpid())
	if out, err := h.output(ctx, "ps", "-aq", "--filter", "label="+pid); err == nil && out != "" {
		_ = h.run(ctx, io.Discard, nil, append([]string{"rm", "-f", "-v"}, strings.Fields(out)...)...)
	}
	if out, err := h.output(ctx, "volume", "ls", "-q", "--filter", "label="+pid); err == nil && out != "" {
		_ = h.run(ctx, io.Discard, nil, append([]string{"volume", "rm", "-f"}, strings.Fields(out)...)...)
	}
}

func alive(pid string) bool {
	n, err := strconv.Atoi(pid)
	if err != nil || n <= 0 {
		return false
	}
	if n == os.Getpid() {
		return true
	}
	return syscall.Kill(n, 0) == nil
}

// Prepare builds the harness image and the image store when they are missing
// or out of date. The store is a Docker volume holding the inner engine's
// /var/lib/docker and /var/lib/containerd after the docker bootstrap ran: the
// pre-pulled images and the kl-* images the tracks use.
func (h *Harness) Prepare(ctx context.Context) error {
	tag := imageTag()
	if _, err := h.output(ctx, "image", "inspect", "--format", "{{.Id}}", tag); err != nil {
		h.progress("building the harness image %s (Ubuntu 24.04, systemd, Docker %s)", tag, shortVersion())
		if err := h.run(ctx, nil, strings.NewReader(dockerfile()), "build", "-t", tag, "-"); err != nil {
			return err
		}
	}
	want, err := storeHash(h.Assets)
	if err != nil {
		return err
	}
	have, _ := h.output(ctx, "volume", "inspect", "--format", `{{index .Labels "`+hashLabel+`"}}`, storeVolume)
	if have == want && !h.Rebuild && h.storeReady(ctx) {
		return nil
	}
	if have != "" {
		h.progress("the image store is out of date: rebuilding it")
		if err := h.run(ctx, nil, nil, "volume", "rm", "-f", storeVolume); err != nil {
			return err
		}
	}
	return h.buildStore(ctx, want)
}

func (h *Harness) storeReady(ctx context.Context) bool {
	return h.run(ctx, io.Discard, nil, "run", "--rm", "--entrypoint", "test", "-v", storeVolume+":/s:ro", imageTag(), "-f", "/s/ready") == nil
}

func (h *Harness) buildStore(ctx context.Context, hash string) error {
	h.progress("building the image store (images the docker VM pre-pulls and builds; this takes a while once)")
	if err := h.run(ctx, nil, nil, "volume", "create", "--label", labelKey+"=store", "--label", hashLabel+"="+hash, storeVolume); err != nil {
		return err
	}
	if err := h.run(ctx, nil, nil, "run", "--rm", "--entrypoint", "mkdir", "-v", storeVolume+":/s", imageTag(), "-p", "/s/docker", "/s/containerd"); err != nil {
		return err
	}
	name := "kl-local-prepare-" + strconv.Itoa(os.Getpid())
	extra := []string{}
	if h.CacheDir != "" {
		if st, err := os.Stat(h.CacheDir); err == nil && st.IsDir() {
			abs, _ := filepath.Abs(h.CacheDir)
			extra = append(extra, "-v", abs+":/kl-cache:ro")
		}
	}
	c, err := h.startContainer(ctx, name, storeVolume, extra)
	if err != nil {
		return err
	}
	defer c.remove()
	if len(extra) > 0 {
		h.progress("loading the cached images from %s", h.CacheDir)
		if _, err := c.script(ctx, true, "/", `for f in /kl-cache/*.tar; do [ -f "$f" ] && docker load -q -i "$f"; done; true`, nil, h.logw()); err != nil {
			return err
		}
	}
	for _, s := range []string{"common.sh", "docker.sh"} {
		h.progress("running the docker bootstrap: %s", s)
		body, err := fs.ReadFile(h.Assets, "vms/bootstrap/"+s)
		if err != nil {
			return err
		}
		code, err := c.script(ctx, true, "/", string(body), map[string]string{"LAB_USER": LabUser}, stepWriter(h.Progress, h.logw()))
		if err != nil {
			return err
		}
		if code != 0 {
			return fmt.Errorf("bootstrap %s exited %d (see the log)", s, code)
		}
	}
	// stop the engine cleanly, so the store holds a consistent image database
	_, _ = c.script(ctx, true, "/", "systemctl stop docker.socket docker containerd", nil, h.logw())
	c.remove()
	if err := h.run(ctx, nil, nil, "run", "--rm", "--entrypoint", "touch", "-v", storeVolume+":/s", imageTag(), "/s/ready"); err != nil {
		return err
	}
	h.progress("image store ready")
	return nil
}

func shortVersion() string {
	v := strings.TrimPrefix(lab.DockerVersion, "5:")
	if i := strings.IndexByte(v, '-'); i > 0 {
		v = v[:i]
	}
	return v
}

// container is a running harness container.
type container struct {
	h    *Harness
	name string
	vol  string // per-run volume removed with it ("" for the store's own)
}

// startContainer starts the harness image with the inner engine's state on vol
// (subpaths docker/ and containerd/), waits for Docker, and pushes the kit.
func (h *Harness) startContainer(ctx context.Context, name, vol string, extra []string) (*container, error) {
	label := labelKey + "=run"
	if vol == storeVolume {
		label = labelKey + "=prepare"
	}
	args := []string{"run", "-d", "--name", name, "--hostname", Hostname,
		"--privileged", "--cgroupns=private", "--tmpfs", "/run", "--tmpfs", "/run/lock",
		"--label", label, "--label", pidLabel + "=" + strconv.Itoa(os.Getpid()),
		"--mount", "type=volume,src=" + vol + ",dst=/var/lib/docker,volume-subpath=docker",
		"--mount", "type=volume,src=" + vol + ",dst=/var/lib/containerd,volume-subpath=containerd"}
	args = append(args, extra...)
	args = append(args, imageTag())
	c := &container{h: h, name: name}
	if err := h.run(ctx, nil, nil, args...); err != nil {
		c.remove()
		return nil, err
	}
	deadline := time.Now().Add(120 * time.Second)
	for {
		if code, err := c.script(ctx, true, "/", "docker info >/dev/null 2>&1", nil, io.Discard); err == nil && code == 0 {
			break
		}
		if ctx.Err() != nil {
			c.remove()
			return nil, ctx.Err()
		}
		if time.Now().After(deadline) {
			logs, _ := h.output(context.Background(), "logs", "--tail", "20", name)
			c.remove()
			return nil, fmt.Errorf("docker did not start in the harness container: %s", lastLine(logs))
		}
		time.Sleep(time.Second)
	}
	kit, err := kitTar(h.Assets)
	if err != nil {
		c.remove()
		return nil, err
	}
	if err := h.run(ctx, io.Discard, bytes.NewReader(kit), "exec", "-i", "-u", "root", name, "bash", "-c",
		"mkdir -p /opt/kubelab /var/lib/kubelab && tar -m --warning=no-timestamp -xf - -C /opt/kubelab"); err != nil {
		c.remove()
		return nil, fmt.Errorf("copy kit: %w", err)
	}
	return c, nil
}

// remove deletes the container and its per-run volume; safe to call twice.
func (c *container) remove() {
	if c == nil || c.name == "" {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	_ = c.h.run(ctx, io.Discard, nil, "rm", "-f", "-v", c.name)
	if c.vol != "" {
		_ = c.h.run(ctx, io.Discard, nil, "volume", "rm", "-f", c.vol)
	}
	c.name, c.vol = "", ""
}

// script runs body in the container the way lima.Run does on a VM: uploaded
// to a temp file, run with stdin from /dev/null, as the lab user (root
// through sudo -H, like the lab runner), in workdir. It returns the exit code.
func (c *container) script(ctx context.Context, asRoot bool, workdir, body string, env map[string]string, out io.Writer) (int, error) {
	var pre strings.Builder
	for k, v := range env {
		fmt.Fprintf(&pre, "export %s=%s\n", k, shellQuote(v))
	}
	sudo := ""
	if asRoot {
		sudo = "sudo -H "
	}
	runner := `f=$(mktemp /tmp/awesome_labs-XXXXXX.sh) && cat >"$f" && chmod 644 "$f" && ` +
		sudo + `bash "$f" </dev/null; rc=$?; rm -f "$f"; exit $rc`
	home := "/home/" + LabUser
	cmd := exec.CommandContext(ctx, c.h.docker(), "exec", "-i", "-u", LabUser, "-w", workdir,
		"-e", "HOME="+home, "-e", "USER="+LabUser, "-e", "LOGNAME="+LabUser, "-e", "SHELL=/bin/bash",
		c.name, "bash", "-c", runner)
	cmd.WaitDelay = 10 * time.Second
	cmd.Stdin = strings.NewReader(pre.String() + body)
	if out == nil {
		out = io.Discard
	}
	cmd.Stdout, cmd.Stderr = out, out
	err := cmd.Run()
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		return ee.ExitCode(), nil
	}
	if err != nil {
		return -1, err
	}
	return 0, nil
}

// Result strings, the same as awesome_labs verify's.
const (
	ResultOK         = "ok"
	ResultSetupError = "setup error"
	ResultCheckError = "check error"
	ResultPassEarly  = "passes before solving"
	ResultSolveError = "solve error"
	ResultFailsAfter = "fails after solving"
)

// Verify runs one exercise end to end in a fresh container: setup, check must
// fail, solve, check must pass. It returns verify's result string, or
// "skipped: ..." when the exercise can't run here.
func (h *Harness) Verify(ctx context.Context, ex *exercise.Exercise, log io.Writer) string {
	if why := Unsupported(ex); why != "" {
		return "skipped: " + why
	}
	if why := HostRisk(ex); why != "" && !h.Force {
		return "skipped: " + why + " (VM only)"
	}
	suffix := randHex(3)
	name := "kl-local-" + sanitize(ex.ID) + "-" + suffix
	vol := "kl-local-run-" + sanitize(ex.ID) + "-" + suffix
	fmt.Fprintf(log, "==> copying the image store into %s\n", vol)
	if err := h.run(ctx, log, nil, "volume", "create", "--label", labelKey+"=run", "--label", pidLabel+"="+strconv.Itoa(os.Getpid()), vol); err != nil {
		fmt.Fprintln(log, "HARNESS ERROR:", err)
		return "harness error"
	}
	if err := h.run(ctx, log, nil, "run", "--rm", "--privileged", "--entrypoint", "cp",
		"--label", labelKey+"=run", "--label", pidLabel+"="+strconv.Itoa(os.Getpid()),
		"-v", storeVolume+":/s:ro", "-v", vol+":/d", imageTag(),
		"-a", "--reflink=auto", "/s/docker", "/s/containerd", "/d/"); err != nil {
		_ = h.run(context.Background(), io.Discard, nil, "volume", "rm", "-f", vol)
		fmt.Fprintln(log, "HARNESS ERROR:", err)
		return "harness error"
	}
	c, err := h.startContainer(ctx, name, vol, nil)
	if err != nil {
		_ = h.run(context.Background(), io.Discard, nil, "volume", "rm", "-f", vol)
		fmt.Fprintln(log, "HARNESS ERROR:", err)
		return "harness error"
	}
	c.vol = vol
	defer c.remove()
	if code, err := c.script(ctx, true, "/", mustRead(h.Assets, "vms/bootstrap/common.sh"), map[string]string{"LAB_USER": LabUser}, log); err != nil || code != 0 {
		fmt.Fprintln(log, "HARNESS ERROR: common.sh failed", code, err)
		return "harness error"
	}
	return verifySteps(ctx, ex, c, log)
}

// stepper runs exercise scripts; the container, or a fake in tests.
type stepper interface {
	script(ctx context.Context, asRoot bool, workdir, body string, env map[string]string, out io.Writer) (int, error)
}

// verifySteps is lab.Manager's Start, Check, Solve, Check on one container.
func verifySteps(ctx context.Context, ex *exercise.Exercise, c stepper, log io.Writer) string {
	fmt.Fprintf(log, "==> setting up %s: %s\n", ex.ID, ex.Title)
	if ex.Setup != "" {
		if code, err := c.script(ctx, true, "/", exScript(ex.Setup, false), nil, log); err != nil || code != 0 {
			fmt.Fprintf(log, "START ERROR: setup exited %d %v\n", code, err)
			return ResultSetupError
		}
	}
	if ex.Notebook {
		nb := exercise.StarterNotebook(ex)
		body := fmt.Sprintf("source /opt/kubelab/lib.sh\nlab_file /lab/%s <<'AWESOME_LABS_NB_EOF'\n%s\nAWESOME_LABS_NB_EOF", ex.NotebookName(), nb)
		if code, err := c.script(ctx, true, "/", body, nil, io.Discard); err != nil || code != 0 {
			fmt.Fprintf(log, "START ERROR: writing the notebook exited %d %v\n", code, err)
			return ResultSetupError
		}
	}
	task := fmt.Sprintf("# %s — %s\n\n%s\n", ex.ID, ex.Title, strings.TrimSpace(ex.Scenario))
	_, _ = c.script(ctx, true, "/", fmt.Sprintf("echo %s > /var/lib/kubelab/current\nsource /opt/kubelab/lib.sh\nlab_file /lab/TASK.md <<'AWESOME_LABS_TASK_EOF'\n%s\nAWESOME_LABS_TASK_EOF", ex.ID, task), nil, io.Discard)
	check := func() (bool, error) {
		code, err := c.script(ctx, true, "/", exScript(ex.Check, true), nil, log)
		return code == 0, err
	}
	ok, err := check()
	if err != nil {
		return ResultCheckError
	}
	if ok {
		return ResultPassEarly
	}
	if code, err := c.script(ctx, false, "/lab", exScript(ex.Solve, false), nil, log); err != nil || code != 0 {
		fmt.Fprintf(log, "SOLVE ERROR: solve script exited %d %v\n", code, err)
		return ResultSolveError
	}
	ok, err = check()
	if err != nil {
		return ResultCheckError
	}
	if !ok {
		return ResultFailsAfter
	}
	return ResultOK
}

func exScript(body string, finish bool) string {
	s := "source /opt/kubelab/lib.sh\n" + body
	if finish {
		s += "\nkl_finish\n"
	}
	return s
}

// kitTar packs vms/kit the way the lab runner pushes it to /opt/kubelab.
func kitTar(assets fs.FS) ([]byte, error) {
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	err := fs.WalkDir(assets, "vms/kit", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := fs.ReadFile(assets, p)
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
		return nil, err
	}
	if err := tw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func mustRead(assets fs.FS, p string) string {
	b, err := fs.ReadFile(assets, path.Clean(p))
	if err != nil {
		return "echo missing " + p + " >&2; exit 1"
	}
	return string(b)
}

func shellQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }

func randHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func lastLine(s string) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	return strings.TrimSpace(lines[len(lines)-1])
}

// stepWriter copies everything to log and the bootstrap's "==> " progress
// lines to progress too.
func stepWriter(progress, log io.Writer) io.Writer {
	if progress == nil {
		return log
	}
	return io.MultiWriter(log, &lineFilter{out: progress, prefix: "==> "})
}

type lineFilter struct {
	out    io.Writer
	prefix string
	buf    []byte
}

func (l *lineFilter) Write(p []byte) (int, error) {
	l.buf = append(l.buf, p...)
	for {
		i := bytes.IndexByte(l.buf, '\n')
		if i < 0 {
			break
		}
		line := string(l.buf[:i])
		l.buf = l.buf[i+1:]
		if strings.HasPrefix(line, l.prefix) {
			fmt.Fprintln(l.out, "   "+line)
		}
	}
	return len(p), nil
}
