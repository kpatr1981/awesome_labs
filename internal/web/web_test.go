package web

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"awesome_labs/internal/exercise"
)

func testServer() *server {
	return &server{token: "secret", loopback: true, ops: newOpRunner(), terms: newTerminals(), md: newMarkdown(), ctx: context.Background(), live: newLive(true, 0, "stop")}
}

func TestGuard(t *testing.T) {
	s := testServer()
	ok := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusTeapot) })
	h := s.guard(ok)
	cookie := &http.Cookie{Name: cookieName, Value: "secret"}
	cases := []struct {
		name, method, url, host, origin string
		cookie                          *http.Cookie
		want                            int
	}{
		{"no cookie", "GET", "/api/state", "127.0.0.1:7681", "", nil, http.StatusUnauthorized},
		{"wrong cookie", "GET", "/api/state", "127.0.0.1:7681", "", &http.Cookie{Name: cookieName, Value: "nope"}, http.StatusUnauthorized},
		{"cookie", "GET", "/api/state", "127.0.0.1:7681", "", cookie, http.StatusTeapot},
		{"localhost", "GET", "/api/state", "localhost:7681", "", cookie, http.StatusTeapot},
		{"rebinding host", "GET", "/api/state", "evil.example:7681", "", cookie, http.StatusForbidden},
		{"token sets cookie", "GET", "/?token=secret", "127.0.0.1:7681", "", nil, http.StatusSeeOther},
		{"wrong token", "GET", "/?token=bad", "127.0.0.1:7681", "", nil, http.StatusUnauthorized},
		{"same-origin post", "POST", "/api/x", "127.0.0.1:7681", "http://127.0.0.1:7681", cookie, http.StatusTeapot},
		{"cross-origin post", "POST", "/api/x", "127.0.0.1:7681", "http://evil.example", cookie, http.StatusForbidden},
	}
	for _, c := range cases {
		r := httptest.NewRequest(c.method, c.url, nil)
		r.Host = c.host
		if c.origin != "" {
			r.Header.Set("Origin", c.origin)
		}
		if c.cookie != nil {
			r.AddCookie(c.cookie)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != c.want {
			t.Errorf("%s: got %d, want %d", c.name, w.Code, c.want)
		}
		if c.name == "token sets cookie" && !strings.Contains(w.Header().Get("Set-Cookie"), "HttpOnly") {
			t.Errorf("token login: cookie %q is not HttpOnly", w.Header().Get("Set-Cookie"))
		}
	}
}

func TestOpWriterLines(t *testing.T) {
	r := newOpRunner()
	o := &op{}
	w := &opWriter{r: r, o: o}
	io.WriteString(w, "\x1b[32mone\x1b[0m\ntw")
	io.WriteString(w, "o\r\n10%\r50%\r100%\nthree")
	want := []string{"one", "two", "100%"}
	if strings.Join(o.lines, "|") != strings.Join(want, "|") || o.partial != "three" {
		t.Fatalf("lines %q partial %q", o.lines, o.partial)
	}
}

func TestOpWriterProgressMeter(t *testing.T) {
	o := &op{}
	w := &opWriter{r: newOpRunner(), o: o}
	for i := range 1000 {
		fmt.Fprintf(w, "%d%%\r", i) // a meter that never writes a newline
	}
	if o.partial != "999%\r" {
		t.Fatalf("partial grew to %d bytes: %.40q", len(o.partial), o.partial)
	}
	io.WriteString(w, "done\n")
	if got := o.lines[len(o.lines)-1]; got != "done" {
		t.Fatalf("last line %q", got)
	}
}

func TestOpLinesCapKeepsAbsoluteNumbers(t *testing.T) {
	o := &op{}
	w := &opWriter{r: newOpRunner(), o: o}
	for i := range maxLines + 10 {
		fmt.Fprintf(w, "line %d\n", i)
	}
	if len(o.lines) != maxLines || o.Dropped != 10 || o.lines[0] != "line 10" {
		t.Fatalf("kept %d, dropped %d, first %q", len(o.lines), o.Dropped, o.lines[0])
	}
}

func TestOneOpAtATime(t *testing.T) {
	r := newOpRunner()
	release := make(chan struct{})
	_, err := r.start(context.Background(), "env", "first", "env", "docker", func(ctx context.Context, w io.Writer) (bool, error) {
		<-release
		return true, nil
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.start(context.Background(), "env", "second", "env", "", nil, nil); err == nil || !strings.Contains(err.Error(), "busy: first") {
		t.Fatalf("second start: %v", err)
	}
	close(release)
	r.wait(time.Second)
	if c := r.current(); c != nil {
		t.Fatalf("still running: %+v", c)
	}
	if !r.last["env"].Passed {
		t.Fatal("result not recorded")
	}
}

func TestLoopbackHost(t *testing.T) {
	for h, want := range map[string]bool{"127.0.0.1:1": true, "localhost:1": true, "[::1]:1": true, "localhost": true,
		"10.0.0.1:1": false, "evil.example": false, "127.0.0.1.evil.example:1": false} {
		if loopbackHost(h) != want {
			t.Errorf("loopbackHost(%q) != %v", h, want)
		}
	}
}

func TestMarkdownDropsRawHTML(t *testing.T) {
	out := newMarkdown().render("hi <script>alert(1)</script>\n\n| a | b |\n|---|---|\n| 1 | 2 |")
	if strings.Contains(out, "<script>") {
		t.Fatalf("raw HTML passed through: %s", out)
	}
	if !strings.Contains(out, "<table>") {
		t.Fatalf("GFM table not rendered: %s", out)
	}
}

func TestJupyterProxy(t *testing.T) {
	var got *http.Request
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r
		io.WriteString(w, "jupyter "+r.URL.Path)
	}))
	defer backend.Close()
	s := testServer()
	s.dialJupyter = func(ctx context.Context) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "tcp", backend.Listener.Addr().String())
	}
	s.jupyter.proxy = s.newJupyterProxy()
	s.jupyter.set("nbtoken")
	h := s.guard(http.HandlerFunc(s.handleJupyterProxy))

	r := httptest.NewRequest("GET", "/jupyter/api/status", nil)
	r.Host = "127.0.0.1:7681"
	r.AddCookie(&http.Cookie{Name: cookieName, Value: "secret"})
	r.AddCookie(&http.Cookie{Name: "_xsrf", Value: "x"})
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 200 || w.Body.String() != "jupyter /jupyter/api/status" {
		t.Fatalf("got %d %q", w.Code, w.Body.String())
	}
	if a := got.Header.Get("Authorization"); a != "token nbtoken" {
		t.Errorf("Authorization %q", a)
	}
	if c := got.Header.Get("Cookie"); strings.Contains(c, cookieName) || !strings.Contains(c, "_xsrf=x") {
		t.Errorf("cookies forwarded: %q", c)
	}
	if f := w.Header().Get("X-Frame-Options"); f != "SAMEORIGIN" {
		t.Errorf("X-Frame-Options %q", f)
	}

	// no session cookie: never reaches the notebook
	got = nil
	r = httptest.NewRequest("GET", "/jupyter/api/status", nil)
	r.Host = "127.0.0.1:7681"
	w = httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != http.StatusUnauthorized || got != nil {
		t.Fatalf("anonymous request: %d, reached backend: %v", w.Code, got != nil)
	}
}

func TestRoutes(t *testing.T) {
	s := testServer()
	s.jupyter.proxy = s.newJupyterProxy()
	h := s.routes() // panics on conflicting patterns
	for path, want := range map[string]int{"/": 200, "/app.js": 200, "/vendor/xterm.js": 200, "/api/nope": 404} {
		r := httptest.NewRequest("GET", path, nil)
		r.Host = "127.0.0.1:7681"
		r.AddCookie(&http.Cookie{Name: cookieName, Value: "secret"})
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != want {
			t.Errorf("GET %s: %d, want %d", path, w.Code, want)
		}
	}
}

func TestEnsureStarter(t *testing.T) {
	exists := false
	var put map[string]any
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "token nb" || r.URL.Path != "/jupyter/api/contents/ds-x.ipynb" {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		switch {
		case r.Method == "GET" && exists:
			io.WriteString(w, "{}")
		case r.Method == "GET":
			w.WriteHeader(http.StatusNotFound)
		case r.Method == "PUT":
			json.NewDecoder(r.Body).Decode(&put)
			exists = true
			w.WriteHeader(http.StatusCreated)
		}
	}))
	defer backend.Close()
	s := testServer()
	s.dialJupyter = func(ctx context.Context) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "tcp", backend.Listener.Addr().String())
	}
	s.jupyter.proxy = s.newJupyterProxy()
	ex := &exercise.Exercise{ID: "ds-x", Title: "T", Deliverables: []string{"answer.txt"}}
	if err := s.ensureStarter(context.Background(), "nb", ex, io.Discard); err != nil {
		t.Fatal(err)
	}
	nb, _ := put["content"].(map[string]any)
	if put["type"] != "notebook" || nb["nbformat"] != float64(4) {
		t.Fatalf("saved %v", put)
	}
	cells, _ := nb["cells"].([]any)
	first, _ := cells[0].(map[string]any)
	if src, _ := first["source"].(string); !strings.Contains(src, "`/lab/answer.txt`") || !strings.Contains(src, "How it's graded") {
		t.Fatalf("first cell %q", src)
	}
	put = nil
	if err := s.ensureStarter(context.Background(), "nb", ex, io.Discard); err != nil || put != nil {
		t.Fatalf("an existing notebook was overwritten (err %v)", err)
	}
}

func TestIdleReason(t *testing.T) {
	now := time.Now()
	a := &activity{last: now.Add(-11 * time.Minute)}
	if a.idleReason(now, false, false) == "" {
		t.Error("free and idle for 11 minutes: should stop")
	}
	if a.idleReason(now, true, false) != "" {
		t.Error("an operation or a terminal keeps them")
	}
	if a.idleReason(now, false, true) != "" {
		t.Error("an exercise with a page that only just went away keeps them")
	}
	a = &activity{last: now.Add(-5 * time.Minute)}
	if a.idleReason(now, false, false) != "" {
		t.Error("5 minutes isn't idle yet")
	}
	a = &activity{last: now.Add(-31 * time.Minute), goneAt: now.Add(-31 * time.Minute)}
	if a.idleReason(now, false, true) == "" {
		t.Error("exercise set up but no page for 31 minutes: should stop")
	}
	a.page(true)
	if a.idleReason(now, false, true) != "" {
		t.Error("an open page keeps an exercise's VMs")
	}
}
