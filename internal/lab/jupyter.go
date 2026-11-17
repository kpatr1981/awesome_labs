package lab

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"awesome_labs/internal/lima"
)

// JupyterLab for the exercises that suit notebooks (a module's notebook: true).
//
// It runs as a container on the docker host, as the lab user, with /lab as its
// working folder, so what a notebook writes is what the checks read. Its port
// is published on the VM's loopback only; the web interface reaches it over
// the VM's ssh connection (DialJupyter) and adds the token to every request,
// so nothing new listens on the host.
//
// The image is built on first use rather than in the bootstrap: changing the
// bootstrap would make every saved copy stale and force a full rebuild. Once
// built it joins the pristine images, so resets keep it, and the host's image
// cache, so a new VM loads it in seconds instead of building it again.

const (
	// JupyterImage is the data-science image plus JupyterLab.
	JupyterImage     = "kl-jupyter:1"
	jupyterContainer = "kl-jupyter"
	jupyterPort      = "18888" // on the docker host's loopback
	// JupyterBase is the URL path JupyterLab serves under; the web interface
	// proxies it as is.
	JupyterBase = "/jupyter/"
	// jupyterMemory caps the container, so a runaway notebook can't starve the
	// exercise's own containers (the docker host has 4GiB).
	jupyterMemory = "2560m"
)

const jupyterDockerfile = `FROM kl-datascience:1
RUN pip install --no-cache-dir jupyterlab==4.6.4 ipykernel==7.4.0 matplotlib==3.11.2 && \
    jupyter labextension disable "@jupyterlab/apputils-extension:announcements"
`

// jupyterScript reports a running server's token, or starts one with $TOKEN.
// It exits 3 when the image is missing.
const jupyterScript = `set -euo pipefail
source /opt/kubelab/lib.sh
C=` + jupyterContainer + ` I=` + JupyterImage + `
if [ "$(docker inspect -f '{{.State.Running}}' "$C" 2>/dev/null)" = true ]; then
  echo "TOKEN=$(docker inspect -f '{{index .Config.Labels "kl.token"}}' "$C")"
  exit 0
fi
docker rm -f "$C" >/dev/null 2>&1 || true
docker image inspect "$I" >/dev/null 2>&1 || exit 3
# keep the image through resets, also on VMs built before it existed
p=$KL_STATE/pristine
if ! grep -qxF "$I" "$p/image-refs.txt"; then
  docker image inspect -f '{{.Id}}' "$I" >>"$p/images.txt"
  echo "$I" >>"$p/image-refs.txt"
  LC_ALL=C sort -u -o "$p/images.txt" "$p/images.txt"
  LC_ALL=C sort -u -o "$p/image-refs.txt" "$p/image-refs.txt"
fi
U=$(lab_user)
docker run -d --name "$C" --label "kl.token=$TOKEN" --restart no \
  --user "$(id -u "$U"):$(id -g "$U")" -e HOME=/tmp -e PYTHONDONTWRITEBYTECODE=1 \
  --memory ` + jupyterMemory + ` -p 127.0.0.1:` + jupyterPort + `:8888 -v /lab:/lab -w /lab "$I" \
  jupyter lab --ip=0.0.0.0 --port=8888 --no-browser --ServerApp.root_dir=/lab \
    --ServerApp.base_url=` + JupyterBase + ` --IdentityProvider.token="$TOKEN" \
    --ServerApp.allow_origin='*' --ServerApp.allow_remote_access=True \
    --FileCheckpoints.checkpoint_dir=/tmp/checkpoints \
    --LabApp.news_url=None --LabApp.check_for_updates_class=jupyterlab.NeverCheckForUpdate >/dev/null
echo "TOKEN=$TOKEN"
`

// StartJupyter makes sure JupyterLab runs on the docker host and returns its
// token. The first time on a VM it loads the image from the host cache, or
// builds it (about a minute) and caches it.
func (m *Manager) StartJupyter(ctx context.Context, log io.Writer) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	vm := DockerEnv.Primary()
	if st, err := lima.Status(ctx, vm.Name); err != nil || st != "Running" {
		return "", fmt.Errorf("%s is not running: start the exercise first", vm.Hostname)
	}
	m.setClean(DockerEnv, false) // a notebook can change anything in /lab
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	env := map[string]string{"TOKEN": hex.EncodeToString(b)}
	tok, code, err := m.jupyterRun(ctx, vm, env)
	if err == nil && code == 3 {
		if err = m.jupyterImage(ctx, vm, log); err == nil {
			tok, code, err = m.jupyterRun(ctx, vm, env)
		}
	}
	switch {
	case err != nil:
		return "", err
	case code != 0 || tok == "":
		return "", fmt.Errorf("JupyterLab didn't start (exit %d): see %s", code, m.LogPath())
	}
	fmt.Fprintf(log, "==> waiting for JupyterLab\n")
	if err := m.waitJupyter(ctx, tok); err != nil {
		return "", err
	}
	fmt.Fprintf(log, "==> JupyterLab is running in /lab\n")
	return tok, nil
}

func (m *Manager) jupyterRun(ctx context.Context, vm VM, env map[string]string) (token string, code int, err error) {
	var out strings.Builder
	code, err = lima.Run(ctx, vm.Name, lima.Script{AsRoot: true, Body: jupyterScript, Env: env},
		io.MultiWriter(&out, prefixWriter(m.logFile, vm.Hostname)))
	for _, line := range strings.Split(out.String(), "\n") {
		if t, ok := strings.CutPrefix(strings.TrimSpace(line), "TOKEN="); ok {
			token = t
		}
	}
	return token, code, err
}

// jupyterImage puts the image on vm: from the host cache when it's there,
// otherwise built and then cached.
func (m *Manager) jupyterImage(ctx context.Context, vm VM, log io.Writer) error {
	cached := filepath.Join(m.ImageCacheDir(rtDocker), cacheFile(JupyterImage))
	if f, err := os.Open(cached); err == nil {
		fmt.Fprintf(log, "==> loading the JupyterLab image from the local cache\n")
		code, err := lima.Run(ctx, vm.Name, lima.Script{AsRoot: true, Body: loadImageCmd[rtDocker], Stdin: f}, m.logFile)
		f.Close()
		if err == nil && code == 0 {
			return nil
		}
		fmt.Fprintf(log, "==> the cached image didn't load — building it instead\n")
	}
	fmt.Fprintf(log, "==> building the JupyterLab image (about a minute, once per VM)\n")
	if _, err := m.root(ctx, vm, "docker build -t "+JupyterImage+" - <<'DOCKERFILE'\n"+jupyterDockerfile+"DOCKERFILE", nil, log); err != nil {
		return fmt.Errorf("building the JupyterLab image: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(cached), 0o755); err == nil {
		fmt.Fprintf(log, "==> caching the image, so the next VM loads it instead of building it\n")
		if err := m.saveImage(ctx, vm, JupyterImage, cached); err != nil {
			fmt.Fprintf(log, "==> could not cache it: %v\n", err)
		}
	}
	return nil
}

// waitJupyter waits until the server answers with the token.
func (m *Manager) waitJupyter(ctx context.Context, token string) error {
	c := &http.Client{Timeout: 5 * time.Second, Transport: &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) { return m.DialJupyter(ctx) },
	}}
	defer c.CloseIdleConnections()
	deadline := time.Now().Add(90 * time.Second)
	for {
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "http://jupyter"+JupyterBase+"api/status", nil)
		req.Header.Set("Authorization", "token "+token)
		if res, err := c.Do(req); err == nil {
			res.Body.Close()
			if res.StatusCode == http.StatusOK {
				return nil
			}
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("JupyterLab didn't answer within 90s: docker logs %s on the docker host says why", jupyterContainer)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Second):
		}
	}
}

// JupyterToken is the token of the JupyterLab running on the docker host, if
// one is (a reset removes it with every other container).
func (m *Manager) JupyterToken(ctx context.Context) (string, bool) {
	var out strings.Builder
	code, err := lima.Run(ctx, DockerEnv.Primary().Name, lima.Script{
		Body: `[ "$(docker inspect -f '{{.State.Running}}' ` + jupyterContainer + ` 2>/dev/null)" = true ] && docker inspect -f '{{index .Config.Labels "kl.token"}}' ` + jupyterContainer,
	}, &out)
	tok := strings.TrimSpace(out.String())
	return tok, err == nil && code == 0 && tok != ""
}

// DialJupyter connects to JupyterLab's port on the docker host through the
// VM's ssh connection (Lima keeps a shared one open, so this is quick).
func (m *Manager) DialJupyter(ctx context.Context) (net.Conn, error) {
	vm := DockerEnv.Primary()
	home, _ := os.UserHomeDir()
	cfg := filepath.Join(home, ".lima", vm.Name, "ssh.config")
	if _, err := os.Stat(cfg); err != nil {
		return nil, fmt.Errorf("%s has no ssh config: is it created?", vm.Name)
	}
	cmd := exec.Command("ssh", "-F", cfg, "-o", "BatchMode=yes", "-o", "LogLevel=ERROR", "-W", "127.0.0.1:"+jupyterPort, "lima-"+vm.Name)
	cmd.Stderr = m.logFile
	c, err := commandConn(cmd)
	if err == nil && ctx.Err() != nil {
		c.Close()
		return nil, ctx.Err()
	}
	return c, err
}

// commandConn starts cmd and is a connection to its stdin and stdout.
func commandConn(cmd *exec.Cmd) (net.Conn, error) {
	in, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return &pipeConn{cmd: cmd, r: out, w: in}, nil
}

// pipeConn is a net.Conn over a process's stdin and stdout. Deadlines are
// ignored: the HTTP transport and the proxy don't rely on them.
type pipeConn struct {
	cmd *exec.Cmd
	r   io.ReadCloser
	w   io.WriteCloser
}

func (c *pipeConn) Read(b []byte) (int, error)  { return c.r.Read(b) }
func (c *pipeConn) Write(b []byte) (int, error) { return c.w.Write(b) }
func (c *pipeConn) Close() error {
	c.w.Close()
	if c.cmd.Process != nil {
		_ = c.cmd.Process.Kill()
	}
	_ = c.cmd.Wait()
	return nil
}
func (c *pipeConn) LocalAddr() net.Addr              { return pipeAddr{} }
func (c *pipeConn) RemoteAddr() net.Addr             { return pipeAddr{} }
func (c *pipeConn) SetDeadline(time.Time) error      { return nil }
func (c *pipeConn) SetReadDeadline(time.Time) error  { return nil }
func (c *pipeConn) SetWriteDeadline(time.Time) error { return nil }

type pipeAddr struct{}

func (pipeAddr) Network() string { return "pipe" }
func (pipeAddr) String() string  { return "jupyter" }

// StopJupyter removes the JupyterLab container, if there is one. Notebooks
// are files in /lab and stay. It doesn't wait for other operations: removing
// one container doesn't get in their way.
func (m *Manager) StopJupyter(ctx context.Context) error {
	vm := DockerEnv.Primary()
	if st, err := lima.Status(ctx, vm.Name); err != nil || st != "Running" {
		return nil // nothing runs on a stopped VM
	}
	code, err := lima.Run(ctx, vm.Name, lima.Script{AsRoot: true, Body: "docker rm -f " + jupyterContainer + " >/dev/null 2>&1; true"}, m.logFile)
	if err == nil && code != 0 {
		err = fmt.Errorf("removing JupyterLab: exit %d", code)
	}
	return err
}
