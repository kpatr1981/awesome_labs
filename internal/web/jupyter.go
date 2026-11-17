package web

import (
	"context"
	"fmt"
	"html"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"sync"
	"time"

	"awesome_labs/internal/lab"
)

// JupyterLab is proxied at lab.JupyterBase on this server's own origin, so it
// sits behind the same token check as everything else and can be framed by
// the page. The proxy adds JupyterLab's token to each request; the browser
// never sees it, and this server's cookie never reaches the notebook.

type jupyter struct {
	mu    sync.Mutex
	token string // of the server last started or found; "" = ask the VM
	proxy *httputil.ReverseProxy
}

type tokenKey struct{}

func (s *server) newJupyterProxy() *httputil.ReverseProxy {
	dial := s.dialJupyter
	if dial == nil {
		dial = s.Manager.DialJupyter
	}
	return &httputil.ReverseProxy{
		Rewrite: func(pr *httputil.ProxyRequest) {
			pr.SetURL(&url.URL{Scheme: "http", Host: "jupyter"})
			pr.Out.Host = "127.0.0.1:8888"
			pr.Out.Header.Set("Authorization", "token "+pr.In.Context().Value(tokenKey{}).(string))
			dropCookie(pr.Out, cookieName)
		},
		Transport: &http.Transport{
			DialContext:         func(ctx context.Context, _, _ string) (net.Conn, error) { return dial(ctx) },
			MaxIdleConnsPerHost: 8,
			IdleConnTimeout:     90 * time.Second,
		},
		FlushInterval: -1, // event streams and kernel messages go out as they come
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			s.jupyter.forget()
			notebookGone(w, "JupyterLab isn't answering ("+err.Error()+"). A reset or a new exercise removes it: open it again from the exercise.")
		},
	}
}

func dropCookie(r *http.Request, name string) {
	var keep []string
	for _, c := range r.Cookies() {
		if c.Name != name {
			keep = append(keep, c.Name+"="+c.Value)
		}
	}
	r.Header.Del("Cookie")
	if len(keep) > 0 {
		r.Header.Set("Cookie", strings.Join(keep, "; "))
	}
}

func (j *jupyter) set(tok string) {
	j.mu.Lock()
	j.token = tok
	j.mu.Unlock()
}

func (j *jupyter) forget() { j.set("") }

// currentToken is the running server's token, asking the VM when this
// process didn't start it (or it may have gone).
func (s *server) jupyterToken(ctx context.Context) string {
	s.jupyter.mu.Lock()
	tok := s.jupyter.token
	s.jupyter.mu.Unlock()
	if tok != "" {
		return tok
	}
	if !s.Manager.VMRunning(lab.DockerEnv, false) {
		return ""
	}
	if t, ok := s.Manager.JupyterToken(ctx); ok {
		s.jupyter.set(t)
		return t
	}
	return ""
}

func (s *server) handleJupyterProxy(w http.ResponseWriter, r *http.Request) {
	if s.shellBusy(lab.DockerEnv) != "" {
		notebookGone(w, "The docker host is busy: "+s.ops.current().Label+". Open the notebook again when it's done.")
		return
	}
	s.act.touch() // an open notebook polls, so this keeps the VMs needed
	tok := s.jupyterToken(r.Context())
	if tok == "" {
		notebookGone(w, "JupyterLab isn't running. Open it from a notebook exercise once its lab is started.")
		return
	}
	s.jupyter.proxy.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), tokenKey{}, tok)))
}

func notebookGone(w http.ResponseWriter, msg string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusServiceUnavailable)
	fmt.Fprintf(w, `<!doctype html><meta charset="utf-8"><title>JupyterLab</title>`+
		`<body style="font:14px system-ui;padding:2.5rem;background:#0d0f12;color:#e4e7ee">`+
		`<h3 style="margin:0 0 .5rem">No notebook here</h3><p style="color:#8a93a3;max-width:36rem">%s</p>`, html.EscapeString(msg))
}

// handleJupyterStart starts JupyterLab for the active exercise, as an
// operation (the first time on a VM it builds or loads the image).
func (s *server) handleJupyterStart(w http.ResponseWriter, r *http.Request) {
	a, ok := s.Manager.Active()
	ex := s.Catalog.Get(a.ID)
	switch {
	case !ok || ex == nil:
		fail(w, http.StatusConflict, "no exercise is set up on the VMs: start a notebook exercise first")
		return
	case !ex.Notebook:
		fail(w, http.StatusConflict, "%s doesn't use notebooks", ex.ID)
		return
	}
	s.startOp(w, "jupyter", "Starting JupyterLab", ex.ID, string(lab.DockerEnv.Profile),
		func(ctx context.Context, w io.Writer) (bool, error) {
			s.jupyter.forget()
			tok, err := s.Manager.StartJupyter(ctx, w)
			if err != nil {
				return false, err
			}
			s.jupyter.set(tok)
			if err := s.ensureStarter(ctx, tok, ex, w); err != nil { // the notebook still works without it
				fmt.Fprintf(w, "==> could not make /lab/%s: %v\n", ex.NotebookName(), err)
			}
			return false, nil
		}, nil)
}
