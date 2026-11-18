package web

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"awesome_labs/internal/lab"
)

// Lab VMs run only while something needs them. With auto_stop on (the
// default), the server stops running lab VMs once
//   - no exercise is set up on them and nothing has used them (an operation,
//     a terminal, the notebook) for idleFree, or
//   - an exercise is set up but no page has been open for idleAway: its lab is
//     kept on the stopped disks, and its page boots the VMs again.
// Stopping frees their memory and keeps their disks; nothing is deleted.

const (
	idleFree = 10 * time.Minute
	idleAway = 30 * time.Minute
)

type activity struct {
	mu      sync.Mutex
	last    time.Time // the last use: an operation, a terminal, a notebook request
	pages   int       // open event streams: pages watching this server
	goneAt  time.Time // when the last page went away
	stopped time.Time // the last idle stop, so it isn't repeated every minute
}

func (a *activity) touch() {
	a.mu.Lock()
	a.last = time.Now()
	a.mu.Unlock()
}

func (a *activity) page(open bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if open {
		a.pages++
	} else if a.pages--; a.pages == 0 {
		a.goneAt = time.Now()
	}
}

// idleReason says why running lab VMs aren't needed now, or "" when they may
// be. busy: an operation runs or a terminal is open; active: an exercise is set up.
func (a *activity) idleReason(now time.Time, busy, active bool) string {
	a.mu.Lock()
	defer a.mu.Unlock()
	idle := now.Sub(a.last)
	switch {
	case busy:
		return ""
	case !active && idle >= idleFree:
		return fmt.Sprintf("no exercise is set up and nothing has used them for %d minutes", int(idleFree.Minutes()))
	case active && a.pages == 0 && idle >= idleAway && now.Sub(a.goneAt) >= idleAway:
		return fmt.Sprintf("no page has been open for %d minutes (the lab is kept: its page boots it again)", int(idleAway.Minutes()))
	}
	return ""
}

// reapIdle checks every minute whether the running lab VMs are still needed.
func (s *server) reapIdle(ctx context.Context) {
	t := time.NewTicker(time.Minute)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
		if on, _ := s.live.get(); !on {
			continue
		}
		_, active := s.Manager.Active()
		why := s.act.idleReason(time.Now(), s.ops.current() != nil || s.terms.count() > 0, active)
		if why == "" || !s.anyVMRunning(ctx) {
			continue
		}
		s.act.touch() // whatever happens, not again for another while
		_, _ = s.ops.start(s.ctx, "idle", "Stopping the lab VMs: "+why, "env", "",
			func(ctx context.Context, w io.Writer) (bool, error) {
				s.terms.closeAll()
				s.jupyter.forget()
				return false, s.Manager.StopAll(ctx, w)
			}, nil)
	}
}

func (s *server) anyVMRunning(ctx context.Context) bool {
	vms, err := s.Manager.VMs(ctx)
	if err != nil {
		return false
	}
	for _, v := range vms {
		if v.Running() {
			return true
		}
	}
	return false
}

// ---- the first-launch offer to build the environments ahead of time ----

func (s *server) handlePrepareOffer(w http.ResponseWriter, r *http.Request) {
	if s.Manager.PrepareOffered() {
		writeJSON(w, http.StatusOK, map[string]any{"offer": false})
		return
	}
	type envJSON struct {
		Profile string `json:"profile"`
		Title   string `json:"title"`
		Minutes string `json:"minutes"`
		Tracks  string `json:"tracks"`
	}
	tracks := map[string][]string{}
	seen := map[string]bool{}
	for _, e := range s.Catalog.Exercises {
		p := string(lab.EnvFor(e.Profile).Profile)
		if !seen[p+e.Track] {
			seen[p+e.Track] = true
			tracks[p] = append(tracks[p], trackName(e.Track))
		}
	}
	envs := []envJSON{}
	for _, env := range s.Manager.Unprepared(r.Context()) {
		mins := "5–15 minutes"
		if env.Profile == lab.K8sEnv.Profile {
			mins = "10–20 minutes"
		}
		envs = append(envs, envJSON{string(env.Profile), env.Title, mins, joinNames(tracks[string(env.Profile)])})
	}
	if len(envs) == 0 { // everything is built already: nothing to offer, ever
		s.Manager.MarkPrepareOffered()
	}
	writeJSON(w, http.StatusOK, map[string]any{"offer": len(envs) > 0, "envs": envs})
}

// handlePrepare answers the offer: it is made once, yes or no. The chosen
// environments are built one after another, and each keeps only its saved
// copy (stopped): no VM runs until an exercise needs it.
func (s *server) handlePrepare(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Envs []string `json:"envs"`
	}
	if err := decodeJSON(r, &body); err != nil {
		fail(w, http.StatusBadRequest, "bad request: %v", err)
		return
	}
	s.Manager.MarkPrepareOffered()
	var envs []lab.Env
	for _, p := range body.Envs {
		env, ok := envByProfile(p)
		if !ok {
			fail(w, http.StatusBadRequest, "unknown environment %q", p)
			return
		}
		envs = append(envs, env)
	}
	if len(envs) == 0 {
		writeJSON(w, http.StatusOK, map[string]any{"op": nil})
		return
	}
	names := make([]string, len(envs))
	for i, e := range envs {
		names[i] = e.Title
	}
	s.startOp(w, "env", "Pre-building "+joinNames(names), "env", "", func(ctx context.Context, w io.Writer) (bool, error) {
		for _, env := range envs {
			if a, ok := s.Manager.Active(); ok && lab.EnvFor(a.Profile).Profile == env.Profile {
				fmt.Fprintf(w, "==> skipping the %s: %s is set up on it\n", env.Title, a.ID)
				continue
			}
			s.terms.closeEnv(env.Profile)
			if err := s.Manager.Prepare(ctx, env, w); err != nil {
				return false, err
			}
		}
		fmt.Fprintf(w, "==> done: the saved copies are stopped until an exercise needs them\n")
		return false, nil
	}, nil)
}

func joinNames(n []string) string {
	switch len(n) {
	case 0:
		return ""
	case 1:
		return n[0]
	}
	out := n[0]
	for _, x := range n[1 : len(n)-1] {
		out += ", " + x
	}
	return out + " and " + n[len(n)-1]
}

func (s *server) handleJupyterStop(w http.ResponseWriter, r *http.Request) {
	s.jupyter.forget()
	if err := s.Manager.StopJupyter(r.Context()); err != nil {
		fail(w, http.StatusInternalServerError, "%v", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"stopped": true})
}
