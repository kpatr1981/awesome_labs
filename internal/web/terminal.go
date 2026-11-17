package web

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"sync"
	"time"

	"github.com/creack/pty"
	"golang.org/x/net/websocket"

	"awesome_labs/internal/exercise"
	"awesome_labs/internal/lab"
)

// The terminal protocol: binary frames carry terminal bytes both ways; text
// frames carry JSON control messages. From the browser: {"type":"resize",
// "cols":..,"rows":..}. From the server: {"type":"ready","host":..},
// {"type":"exit","code":..} and {"type":"error","message":..}.

type frame struct {
	typ  byte
	data []byte
}

// rawCodec keeps the frame type, which websocket.Message drops.
var rawCodec = websocket.Codec{
	Marshal: func(v any) ([]byte, byte, error) {
		f := v.(frame)
		return f.data, f.typ, nil
	},
	Unmarshal: func(data []byte, typ byte, v any) error {
		f := v.(*frame)
		f.typ, f.data = typ, append(f.data[:0], data...)
		return nil
	},
}

// terminals are the shells open in browsers, so that an operation that wipes
// a lab, and the shutdown, can close them.
type terminals struct {
	mu   sync.Mutex
	open map[*term]struct{}
}

type term struct {
	profile exercise.Profile
	vm      string
	close   func()
}

func newTerminals() *terminals { return &terminals{open: map[*term]struct{}{}} }

func (t *terminals) add(x *term) {
	t.mu.Lock()
	t.open[x] = struct{}{}
	t.mu.Unlock()
}

func (t *terminals) remove(x *term) {
	t.mu.Lock()
	delete(t.open, x)
	t.mu.Unlock()
}

func (t *terminals) closeWhere(match func(*term) bool) {
	t.mu.Lock()
	var hit []*term
	for x := range t.open {
		if match(x) {
			hit = append(hit, x)
		}
	}
	t.mu.Unlock()
	for _, x := range hit {
		x.close()
	}
}

func (t *terminals) count() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return len(t.open)
}

func (t *terminals) closeAll() { t.closeWhere(func(*term) bool { return true }) }

func (t *terminals) closeEnv(p exercise.Profile) {
	t.closeWhere(func(x *term) bool { return x.profile == p })
}

// closeVM closes the shells on one VM ("all": on every VM).
func (t *terminals) closeVM(name string) {
	t.closeWhere(func(x *term) bool { return name == "all" || x.vm == name })
}

func (s *server) terminalHandler() http.Handler {
	return websocket.Server{
		// guard has already checked the token, the Host and the Origin
		Handshake: func(*websocket.Config, *http.Request) error { return nil },
		Handler:   s.serveTerminal,
	}
}

func (s *server) serveTerminal(ws *websocket.Conn) {
	defer ws.Close()
	var wmu sync.Mutex
	send := func(f frame) error {
		wmu.Lock()
		defer wmu.Unlock()
		_ = ws.SetWriteDeadline(time.Now().Add(10 * time.Second))
		return rawCodec.Send(ws, f)
	}
	control := func(v map[string]any) { _ = send(frame{websocket.TextFrame, []byte(mustJSON(v))}) }

	q := ws.Request().URL.Query()
	t, err := lab.CodeTargetFor(q.Get("target"))
	if err != nil {
		control(map[string]any{"type": "error", "message": err.Error()})
		return
	}
	if why := s.shellBusy(t.Env); why != "" {
		control(map[string]any{"type": "error", "message": why})
		return
	}
	if !s.Manager.VMRunning(t.Env, t.Worker) {
		control(map[string]any{"type": "error", "message": t.VM.Hostname + " is not running: start the exercise first, or boot its lab again"})
		return
	}
	size := &pty.Winsize{Cols: dim(q.Get("cols"), 80), Rows: dim(q.Get("rows"), 24)}

	cmd := s.Manager.ShellCommand(t.Env, t.Worker)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = nil, nil, nil
	cmd.Env = append(os.Environ(), "TERM=xterm-256color", "COLORTERM=truecolor")
	f, err := pty.StartWithSize(cmd, size)
	if err != nil {
		control(map[string]any{"type": "error", "message": "opening a shell: " + err.Error()})
		return
	}
	var once sync.Once
	hangup := func() { once.Do(func() { _ = f.Close() }) }
	x := &term{profile: t.Env.Profile, vm: t.VM.Name, close: func() { hangup(); _ = ws.Close() }}
	s.terms.add(x)
	s.act.touch()
	defer func() { s.terms.remove(x); s.act.touch() }()
	if why := s.shellBusy(t.Env); why != "" { // an operation started while the shell was opening
		control(map[string]any{"type": "error", "message": why})
		x.close()
	} else {
		control(map[string]any{"type": "ready", "host": t.VM.Hostname})
	}

	exited := make(chan struct{})
	go func() { // shell -> browser
		defer close(exited)
		buf := make([]byte, 32<<10)
		for {
			n, err := f.Read(buf)
			if n > 0 {
				if send(frame{websocket.BinaryFrame, buf[:n]}) != nil {
					break
				}
			}
			if err != nil {
				break
			}
		}
		hangup()
		code := 0
		if err := cmd.Wait(); err != nil {
			var ee *exec.ExitError
			code = -1
			if errors.As(err, &ee) {
				code = ee.ExitCode()
			}
		}
		control(map[string]any{"type": "exit", "code": code})
		_ = ws.Close()
	}()

	for { // browser -> shell
		var in frame
		if err := rawCodec.Receive(ws, &in); err != nil {
			break
		}
		if in.typ == websocket.BinaryFrame {
			if _, err := f.Write(in.data); err != nil {
				break
			}
			continue
		}
		var msg struct {
			Type string `json:"type"`
			Cols int    `json:"cols"`
			Rows int    `json:"rows"`
			Data string `json:"data"`
		}
		if json.Unmarshal(in.data, &msg) != nil {
			continue
		}
		switch msg.Type {
		case "resize":
			_ = pty.Setsize(f, &pty.Winsize{Cols: clamp(msg.Cols), Rows: clamp(msg.Rows)})
		case "input":
			_, _ = f.Write([]byte(msg.Data))
		}
	}
	// the browser went away: hang the shell up, and make sure it goes
	hangup()
	select {
	case <-exited:
	case <-time.After(3 * time.Second):
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		<-exited
	}
}

// shellBusy says why no shell may open on env now: an operation is changing
// it (a check only reads it, and starting JupyterLab only adds a container). Shells already open were closed when it began.
func (s *server) shellBusy(env lab.Env) string {
	o := s.ops.current()
	if o == nil || o.Kind == "check" || o.Kind == "jupyter" || (o.Env != "" && o.Env != string(env.Profile)) {
		return ""
	}
	return "busy: " + o.Label + " — press Enter to reconnect once it's done"
}

func dim(s string, def int) uint16 {
	n, err := strconv.Atoi(s)
	if err != nil {
		return uint16(def)
	}
	return clamp(n)
}

func clamp(n int) uint16 { return uint16(min(max(n, 2), 1000)) }
