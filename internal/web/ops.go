package web

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"
)

// An opFunc is one lab operation; passed is only meaningful for a check.
type opFunc func(ctx context.Context, w io.Writer) (passed bool, err error)

// op is a lab operation and its output. Like the terminal UI, only one runs
// at a time: the manager is safe for one operation at a time, and a second
// one would act on VMs the first is still changing.
type op struct {
	ID       int       `json:"id"`
	Kind     string    `json:"kind"`   // start, check, boot, end, env
	Label    string    `json:"label"`  // what is happening, for the status line
	Target   string    `json:"target"` // exercise id, or "env"
	Env      string    `json:"env"`    // the environment it changes, "" when it can't tell (all)
	Started  time.Time `json:"started"`
	Finished time.Time `json:"finished,omitzero"`
	Done     bool      `json:"done"`
	Passed   bool      `json:"passed"`
	Err      string    `json:"error,omitempty"`
	// Dropped is how many lines from the start are no longer kept (maxLines);
	// line numbers are absolute, so lines[i] is line Dropped+i.
	Dropped int `json:"dropped"`
	lines   []string
	partial string // the line being written; may end in "\r" (to be overwritten)
}

type opSnapshot struct {
	op
	Lines   []string `json:"lines,omitempty"`
	Partial string   `json:"partial,omitempty"`
}

// maxLines bounds an operation's kept output; a full build is a few thousand.
const maxLines = 20000

type opRunner struct {
	mu      sync.Mutex
	seq     int
	cur     *op
	last    map[string]*op // the latest op per target, for the Output tab
	changed chan struct{}  // closed and replaced on every change
	wg      sync.WaitGroup
}

func newOpRunner() *opRunner {
	return &opRunner{last: map[string]*op{}, changed: make(chan struct{})}
}

// notify wakes every stream; the caller holds r.mu.
func (r *opRunner) notify() {
	close(r.changed)
	r.changed = make(chan struct{})
}

// snapshot copies o; the caller holds r.mu.
func (o *op) snapshot(lines bool) opSnapshot {
	s := opSnapshot{op: *o}
	s.lines, s.partial = nil, ""
	if lines {
		s.Lines = append([]string(nil), o.lines...)
		s.Partial = strings.TrimRight(o.partial, "\r")
	}
	return s
}

// current is the running operation, or nil.
func (r *opRunner) current() *opSnapshot {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.cur == nil || r.cur.Done {
		return nil
	}
	s := r.cur.snapshot(false)
	return &s
}

// start runs f in the background unless another operation is running. env
// names the environment f changes ("" for all of them).
func (r *opRunner) start(ctx context.Context, kind, label, target, env string, f opFunc, done func(bool, error)) (opSnapshot, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.cur != nil && !r.cur.Done {
		return opSnapshot{}, fmt.Errorf("busy: %s", r.cur.Label)
	}
	if ctx.Err() != nil {
		return opSnapshot{}, fmt.Errorf("the web interface is stopping")
	}
	r.seq++
	o := &op{ID: r.seq, Kind: kind, Label: label, Target: target, Env: env, Started: time.Now()}
	r.cur, r.last[target] = o, o
	r.notify()
	snap := o.snapshot(false) // under r.mu: the goroutine below writes o
	r.wg.Add(1)
	go func() {
		defer r.wg.Done()
		w := &opWriter{r: r, o: o}
		passed, err := f(ctx, w)
		if done != nil {
			done(passed, err)
		}
		r.mu.Lock()
		defer r.mu.Unlock()
		if p := strings.TrimRight(o.partial, "\r"); p != "" {
			o.lines = append(o.lines, p)
		}
		o.partial = ""
		o.trim()
		o.Done, o.Passed, o.Finished = true, passed && err == nil, time.Now()
		if err != nil {
			o.Err = err.Error()
		}
		r.notify()
	}()
	return snap, nil
}

// wait gives running operations up to d to finish; it reports whether they did.
func (r *opRunner) wait(d time.Duration) bool {
	c := make(chan struct{})
	go func() { r.wg.Wait(); close(c) }()
	select {
	case <-c:
		return true
	case <-time.After(d):
		return false
	}
}

var ansi = regexp.MustCompile(`\x1b\[[0-9;?]*[ -/]*[@-~]|\x1b\][^\a]*\a`)

// opWriter appends output line by line; a carriage return rewrites the line,
// as a progress meter does on a terminal.
type opWriter struct {
	r *opRunner
	o *op
}

// lastSegment is what a terminal would show of a line written with carriage
// returns: the text after the last "\r" that has text after it.
func lastSegment(p string) string {
	p = strings.TrimRight(p, "\r")
	if i := strings.LastIndex(p, "\r"); i >= 0 {
		p = p[i+1:]
	}
	return p
}

func (w *opWriter) Write(b []byte) (int, error) {
	w.r.mu.Lock()
	defer w.r.mu.Unlock()
	o := w.o
	text := o.partial + ansi.ReplaceAllString(string(b), "")
	parts := strings.Split(text, "\n")
	for _, p := range parts[:len(parts)-1] {
		o.lines = append(o.lines, lastSegment(p))
	}
	// keep only what the terminal would still show, and a trailing "\r": the
	// next write starts the line over
	p := parts[len(parts)-1]
	o.partial = lastSegment(p)
	if strings.HasSuffix(p, "\r") {
		o.partial += "\r"
	}
	o.trim()
	w.r.notify()
	return len(b), nil
}

// trim drops the oldest lines beyond maxLines; the caller holds r.mu.
func (o *op) trim() {
	if n := len(o.lines) - maxLines; n > 0 {
		o.lines = append([]string(nil), o.lines[n:]...)
		o.Dropped += n
	}
}

// handleOpStream is a server-sent event stream of the operations: "op" when
// one starts, "lines" as output arrives and "done". Line numbers in "lines"
// are absolute (see op.Dropped).
func (s *server) handleOpStream(w http.ResponseWriter, r *http.Request) {
	fl, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	s.act.page(true)
	defer s.act.page(false)
	send := func(event string, v any) {
		fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, mustJSON(v))
	}
	var (
		id, next int // the op streamed, and the absolute number of its next line to send
		partial  string
		doneSent bool
	)
	keep := time.NewTicker(20 * time.Second)
	defer keep.Stop()
	ro := s.ops
	ro.mu.Lock()
	if o := ro.cur; o != nil && o.Done {
		// finished before this stream connected: a page that saw it run
		// catches up through /api/ops/last when it reconnects
		id, next, partial, doneSent = o.ID, o.Dropped+len(o.lines), o.partial, true
	}
	ro.mu.Unlock()
	for {
		ro.mu.Lock()
		o := ro.cur
		ch := ro.changed
		if o != nil {
			if o.ID != id {
				send("op", o.snapshot(false))
				id, next, partial, doneSent = o.ID, 0, "", false
			}
			from := max(next, o.Dropped)
			if end := o.Dropped + len(o.lines); from < end || o.partial != partial {
				send("lines", map[string]any{"id": o.ID, "from": from, "lines": o.lines[from-o.Dropped:],
					"partial": strings.TrimRight(o.partial, "\r")})
				next, partial = end, o.partial
			}
			if o.Done && !doneSent {
				send("done", o.snapshot(false))
				doneSent = true
			}
		}
		ro.mu.Unlock()
		fl.Flush()
		select {
		case <-ch:
		case <-keep.C:
			fmt.Fprint(w, ": keep-alive\n\n")
		case <-r.Context().Done():
			return
		case <-s.ctx.Done():
			return
		}
	}
}

// handleOpLast returns the latest operation on a target with its output.
func (s *server) handleOpLast(w http.ResponseWriter, r *http.Request) {
	s.ops.mu.Lock()
	o := s.ops.last[r.URL.Query().Get("target")]
	var snap *opSnapshot
	if o != nil {
		v := o.snapshot(true)
		snap = &v
	}
	s.ops.mu.Unlock()
	writeJSON(w, http.StatusOK, map[string]any{"op": snap})
}
