// Package web serves awesome_labs in the browser: the exercise catalogue, the
// exercise screen (scenario, tutorial, hints, solution, notes, output) and lab
// operations, plus terminals into the lab VMs over a WebSocket.
//
// It runs in place of the terminal UI and drives the same lab.Manager,
// progress and notes stores. It listens on loopback by default and every
// request needs the random token printed at start (kept in a cookie after the
// first visit): a terminal in the browser is a shell on the lab VMs.
package web

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"awesome_labs/internal/config"
	"awesome_labs/internal/exam"
	"awesome_labs/internal/exercise"
	"awesome_labs/internal/lab"
	"awesome_labs/internal/progress"
)

//go:embed static
var static embed.FS

// Deps are the stores and the lab manager the server works on.
type Deps struct {
	Catalog  *exercise.Catalog
	Manager  *lab.Manager
	Progress *progress.Store
	Notes    *progress.Notes
	Paths    []*exercise.Path
	Exams    *exam.Store
	Version  string
	OnExit   string // delete, stop or keep: shown in the UI
	AutoStop bool   // auto_stop at start: stop lab VMs that nothing needs (see idle.go); Settings can change it
}

// Options configure the listener.
type Options struct {
	Addr string    // host:port; port 0 picks a free one
	Open bool      // open the URL in the default browser
	Out  io.Writer // where the URL and notices go
}

const cookieName = "awesome_labs_token"

type server struct {
	Deps
	token    string
	loopback bool
	ops      *opRunner
	terms    *terminals
	md       *markdown
	ctx      context.Context // cancelled at shutdown: ops and terminals stop with it
	jupyter  jupyter
	act      activity
	live     *live // settings the Settings screen changes while running
	// dialJupyter reaches JupyterLab; nil uses the docker host (tests set it)
	dialJupyter func(context.Context) (net.Conn, error)
}

// Server is a web interface listening, not yet serving.
type Server struct {
	s    *server
	ln   net.Listener
	url  string
	opts Options
}

// Listen opens the listener and makes this run's access token. Nothing has
// touched the lab yet when it fails.
func Listen(d Deps, o Options) (*Server, error) {
	ln, err := net.Listen("tcp", o.Addr)
	if err != nil {
		return nil, err
	}
	tok := make([]byte, 24)
	if _, err := rand.Read(tok); err != nil {
		ln.Close()
		return nil, err
	}
	host, port, _ := net.SplitHostPort(ln.Addr().String())
	ip := net.ParseIP(host)
	s := &server{
		Deps:     d,
		token:    hex.EncodeToString(tok),
		loopback: ip != nil && ip.IsLoopback(),
		ops:      newOpRunner(),
		terms:    newTerminals(),
		md:       newMarkdown(),
	}
	days := 0
	if d.Manager != nil {
		if c, err := config.Load(d.Manager.StateDir); err == nil {
			days = c.DeleteStoppedAfterDays
		}
	}
	s.live = newLive(d.AutoStop, days)
	s.jupyter.proxy = s.newJupyterProxy()
	shown := host
	if ip != nil && ip.IsUnspecified() {
		shown = "127.0.0.1"
	} else if ip != nil && ip.To4() == nil {
		shown = "[" + host + "]"
	}
	return &Server{s: s, ln: ln, opts: o, url: fmt.Sprintf("http://%s:%s/?token=%s", shown, port, s.token)}, nil
}

// Serve serves until ctx is cancelled, then closes every terminal and stops.
// An operation still running is cancelled: busy reports that one was, so the
// caller can leave the VMs alone as the terminal UI does when you quit while
// it is busy.
func (srv *Server) Serve(ctx context.Context) (busy bool, err error) {
	s, o := srv.s, srv.opts
	s.ctx = ctx
	s.act.touch()
	go s.reapIdle(ctx)
	go s.reapStale(ctx)
	fmt.Fprintf(o.Out, "awesome_labs web interface: %s\n", srv.url)
	if !s.loopback {
		fmt.Fprintf(o.Out, "  warning: listening on %s, beyond this machine. Anyone with the link gets a shell on the lab VMs.\n", srv.ln.Addr())
	}
	fmt.Fprintf(o.Out, "  ctrl+c stops it (on_exit: %s)\n", s.OnExit)
	if o.Open {
		openBrowser(srv.url)
	}
	hs := &http.Server{Handler: s.routes(), ReadHeaderTimeout: 10 * time.Second}
	errc := make(chan error, 1)
	go func() { errc <- hs.Serve(srv.ln) }()
	select {
	case err = <-errc:
	case <-ctx.Done():
		fmt.Fprintln(o.Out, "\nstopping the web interface…")
	}
	if c := s.ops.current(); c != nil {
		busy = true
		fmt.Fprintf(o.Out, "  cancelling: %s\n", c.Label)
	}
	s.terms.closeAll()
	sctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_ = hs.Shutdown(sctx) // SSE streams and terminals don't end on their own
	if busy && !s.ops.wait(30*time.Second) {
		fmt.Fprintln(o.Out, "  the operation hasn't stopped yet")
	}
	return busy, err
}

func openBrowser(url string) {
	var c *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		c = exec.Command("open", url)
	default:
		c = exec.Command("xdg-open", url)
	}
	_ = c.Start()
	go func() { _ = c.Wait() }()
}

func (s *server) routes() http.Handler {
	mux := http.NewServeMux()
	sub, _ := fs.Sub(static, "static")
	files := http.FileServer(http.FS(sub))
	// method-less, like the notebook proxy's pattern, which the mux would
	// otherwise refuse as conflicting
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Cache-Control", "no-cache")
		files.ServeHTTP(w, r)
	})

	mux.HandleFunc("GET /api/state", s.handleState)
	mux.HandleFunc("GET /api/catalog", s.handleCatalog)
	mux.HandleFunc("GET /api/progress", s.handleProgress)
	mux.HandleFunc("GET /api/exercises/{id}", s.handleExercise)
	mux.HandleFunc("POST /api/exercises/{id}/hint", s.handleHint)
	mux.HandleFunc("POST /api/exercises/{id}/solution", s.handleSolution)
	mux.HandleFunc("PUT /api/exercises/{id}/notes", s.handleNotes)
	mux.HandleFunc("POST /api/exercises/{id}/forget", s.handleForget)
	mux.HandleFunc("POST /api/exercises/{id}/{action}", s.handleExerciseOp) // start, check, boot, end
	mux.HandleFunc("GET /api/envs", s.handleEnvs)
	mux.HandleFunc("POST /api/envs/{profile}/{action}", s.handleEnvOp)
	mux.HandleFunc("POST /api/vms/{name}/{action}", s.handleVMOp)
	mux.HandleFunc("GET /api/ops/stream", s.handleOpStream)
	mux.HandleFunc("GET /api/ops/last", s.handleOpLast)
	mux.Handle("GET /ws/term", s.terminalHandler())
	mux.HandleFunc("POST /api/jupyter/start", s.handleJupyterStart)
	mux.HandleFunc("POST /api/jupyter/stop", s.handleJupyterStop)
	mux.HandleFunc("GET /api/prepare", s.handlePrepareOffer)
	mux.HandleFunc("POST /api/prepare", s.handlePrepare)
	mux.HandleFunc("GET /api/exam", s.handleExam)
	mux.HandleFunc("GET /api/exam/setup", s.handleExamSetup)
	mux.HandleFunc("POST /api/exam", s.handleExamStart)
	mux.HandleFunc("POST /api/exam/{action}", s.handleExamEnd) // finish, abandon
	mux.HandleFunc("GET /api/weakspots", s.handleWeakSpots)
	mux.HandleFunc("GET /api/settings", s.handleSettings)
	mux.HandleFunc("PUT /api/settings", s.handleSetSettings)
	mux.HandleFunc(lab.JupyterBase, s.handleJupyterProxy)
	return s.guard(mux)
}

// guard enforces the token, the Host header (against DNS rebinding while on
// loopback) and the Origin of anything that isn't a plain GET.
func (s *server) guard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.loopback && !loopbackHost(r.Host) {
			http.Error(w, "unexpected Host header", http.StatusForbidden)
			return
		}
		w.Header().Set("X-Content-Type-Options", "nosniff")
		if strings.HasPrefix(r.URL.Path, lab.JupyterBase) {
			w.Header().Set("X-Frame-Options", "SAMEORIGIN") // shown in the page's notebook panel
		} else {
			w.Header().Set("X-Frame-Options", "DENY")
		}
		w.Header().Set("Referrer-Policy", "no-referrer")
		if t := r.URL.Query().Get("token"); t != "" && r.URL.Path == "/" {
			if !s.validToken(t) {
				http.Error(w, "wrong token: open the link awesome_labs web printed", http.StatusUnauthorized)
				return
			}
			http.SetCookie(w, &http.Cookie{Name: cookieName, Value: s.token, Path: "/", HttpOnly: true, SameSite: http.SameSiteStrictMode})
			http.Redirect(w, r, "/", http.StatusSeeOther) // keep the token out of the address bar and history
			return
		}
		c, err := r.Cookie(cookieName)
		if err != nil || !s.validToken(c.Value) {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(http.StatusUnauthorized)
			fmt.Fprint(w, `<!doctype html><meta charset="utf-8"><title>awesome_labs</title><body style="font:15px system-ui;padding:3rem;background:#0f1115;color:#e6e8ee">`+
				`<h2>Not signed in</h2><p>Open the link that <code>awesome_labs web</code> printed in your terminal; it carries this session's access token.</p>`)
			return
		}
		if r.Method != http.MethodGet || r.Header.Get("Upgrade") != "" {
			if o := r.Header.Get("Origin"); o != "" && o != "http://"+r.Host && o != "https://"+r.Host {
				http.Error(w, "cross-origin request refused", http.StatusForbidden)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func (s *server) validToken(t string) bool {
	return subtle.ConstantTimeCompare([]byte(t), []byte(s.token)) == 1
}

func loopbackHost(hostport string) bool {
	h := hostport
	if host, _, err := net.SplitHostPort(hostport); err == nil {
		h = host
	}
	h = strings.Trim(h, "[]")
	if h == "localhost" {
		return true
	}
	ip := net.ParseIP(h)
	return ip != nil && ip.IsLoopback()
}

// ---- helpers ----

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func fail(w http.ResponseWriter, code int, format string, a ...any) {
	writeJSON(w, code, map[string]string{"error": fmt.Sprintf(format, a...)})
}

func (s *server) exercise(w http.ResponseWriter, r *http.Request) (*exercise.Exercise, bool) {
	ex := s.Catalog.Get(r.PathValue("id"))
	if ex == nil {
		fail(w, http.StatusNotFound, "no exercise %q", r.PathValue("id"))
		return nil, false
	}
	return ex, true
}

// runningExam is the exam in progress, or nil.
func (s *server) runningExam() *exam.Exam {
	e := s.Exams.Current()
	if e == nil || e.Over() {
		return nil
	}
	return e
}

// locked reports whether a running exam hides the hints and solution of ex.
func (s *server) locked(ex *exercise.Exercise) bool {
	e := s.runningExam()
	return e != nil && e.Has(ex.ID) && !e.AllowHints
}
