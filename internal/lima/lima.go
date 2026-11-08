// Package lima wraps the limactl CLI.
package lima

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"
)

// Binary is the limactl executable.
var Binary = "limactl"

// Instance is a subset of `limactl list --json`.
type Instance struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	CPUs   int    `json:"cpus"`
	Memory int64  `json:"memory"`
}

// Available reports whether limactl is on PATH.
func Available() error {
	if _, err := exec.LookPath(Binary); err != nil {
		return fmt.Errorf("limactl not found on PATH (run: awesome_labs install): %w", err)
	}
	return nil
}

// List returns all instances keyed by name.
func List(ctx context.Context) (map[string]Instance, error) {
	out, err := exec.CommandContext(ctx, Binary, "list", "--json").Output()
	if err != nil {
		return nil, fmt.Errorf("limactl list: %w", err)
	}
	res := map[string]Instance{}
	sc := bufio.NewScanner(bytes.NewReader(out))
	sc.Buffer(make([]byte, 1024*1024), 16*1024*1024)
	for sc.Scan() {
		line := bytes.TrimSpace(sc.Bytes())
		if len(line) == 0 {
			continue
		}
		var in Instance
		if err := json.Unmarshal(line, &in); err != nil {
			return nil, fmt.Errorf("parse limactl list: %w", err)
		}
		res[in.Name] = in
	}
	return res, nil
}

// Status returns "" when the instance does not exist.
func Status(ctx context.Context, name string) (string, error) {
	all, err := List(ctx)
	if err != nil {
		return "", err
	}
	return all[name].Status, nil
}

// Create creates (but does not start) an instance from template YAML content.
func Create(ctx context.Context, name, templateYAML string, log io.Writer) error {
	f, err := os.CreateTemp("", "awesome_labs-"+name+"-*.yaml")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err := f.WriteString(templateYAML); err != nil {
		return err
	}
	f.Close()
	return run(ctx, log, "create", "--name="+name, "--tty=false", f.Name())
}

// Start boots an existing instance and waits for its provisioning probes.
func Start(ctx context.Context, name string, log io.Writer) error {
	return run(ctx, log, "start", "--tty=false", "--timeout=30m", name)
}

// Stop shuts an instance down.
func Stop(ctx context.Context, name string, log io.Writer) error {
	return run(ctx, log, "stop", name)
}

// Delete removes an instance (forcefully).
func Delete(ctx context.Context, name string, log io.Writer) error {
	return run(ctx, log, "delete", "--force", name)
}

func run(ctx context.Context, log io.Writer, args ...string) error {
	cmd := exec.CommandContext(ctx, Binary, args...)
	if log == nil {
		log = io.Discard
	}
	cmd.Stdout, cmd.Stderr = log, log
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("limactl %s: %w", strings.Join(args, " "), err)
	}
	return nil
}

// Script describes a bash script to run inside an instance.
type Script struct {
	Body    string
	AsRoot  bool
	Workdir string            // default "/"
	Env     map[string]string // exported before Body
	Stdin   io.Reader         // optional extra data; when set Body is passed via -c
}

// Run executes a script inside the instance and returns its exit code.
// A non-nil error means limactl itself failed, not the script.
func Run(ctx context.Context, name string, s Script, out io.Writer) (int, error) {
	wd := s.Workdir
	if wd == "" {
		wd = "/"
	}
	var pre strings.Builder
	for k, v := range s.Env {
		fmt.Fprintf(&pre, "export %s=%s\n", k, shellQuote(v))
	}
	body := pre.String() + s.Body

	args := []string{"shell", "--workdir", wd, name, "--"}
	sudo := ""
	if s.AsRoot {
		sudo = "sudo -H "
	}
	var stdin io.Reader
	if s.Stdin != nil {
		args = append(args, "bash", "-c", sudo+"bash -c "+shellQuote(body))
		stdin = s.Stdin
	} else {
		// Upload the script to a temp file first, then run it with stdin from
		// /dev/null: commands inside it (ssh, kubectl -f -, read...) must not be
		// able to swallow the rest of the script.
		runner := `f=$(mktemp /tmp/awesome_labs-XXXXXX.sh) && cat >"$f" && chmod 644 "$f" && ` +
			sudo + `bash "$f" </dev/null; rc=$?; rm -f "$f"; exit $rc`
		args = append(args, "bash", "-c", runner)
		stdin = strings.NewReader(body)
	}
	cmd := exec.CommandContext(ctx, Binary, args...)
	cmd.WaitDelay = 10 * time.Second // don't hang on inherited pipes after a cancel
	cmd.Stdin = stdin
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

// ShellCommand returns an interactive login shell command for the instance.
func ShellCommand(name, workdir string) *exec.Cmd {
	if workdir == "" {
		workdir = "/lab"
	}
	cmd := exec.Command(Binary, "shell", "--workdir", workdir, name, "--", "bash", "-l")
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	return cmd
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'"'"'`) + "'"
}
