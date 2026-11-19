package web

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"

	"awesome_labs/internal/exercise"
	"awesome_labs/internal/lab"
	"awesome_labs/internal/update"
)

func trackName(t string) string { return exercise.TrackName(t) }

// decodeJSON reads a request's JSON body (at most 1MiB).
func decodeJSON(r *http.Request, v any) error {
	return json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(v)
}

// progressOf is "solved", "started" or "".
func (s *server) progressOf(id string) string {
	e := s.Progress.Get(id)
	switch {
	case !e.Solved.IsZero():
		return "solved"
	case !e.Started.IsZero():
		return "started"
	}
	return ""
}

// ---- state ----

type activeJSON struct {
	ID      string    `json:"id"`
	Title   string    `json:"title"`
	Profile string    `json:"profile"`
	Since   time.Time `json:"since"`
}

type examJSON struct {
	Title      string   `json:"title"`
	Remaining  int64    `json:"remainingSeconds"`
	AllowHints bool     `json:"allowHints"`
	IDs        []string `json:"ids"`
	Passed     int      `json:"passed"`
}

func (s *server) handleState(w http.ResponseWriter, r *http.Request) {
	out := map[string]any{"version": s.Version, "onExit": s.live.exitAction(), "op": s.ops.current(), "repo": "https://github.com/" + update.Repo}
	if a, ok := s.Manager.Active(); ok {
		title := ""
		if ex := s.Catalog.Get(a.ID); ex != nil {
			title = ex.Title
		}
		out["active"] = activeJSON{ID: a.ID, Title: title, Profile: string(a.Profile), Since: a.Since}
	}
	if e := s.runningExam(); e != nil {
		ej := examJSON{Title: e.Title, Remaining: int64(e.Remaining().Seconds()), AllowHints: e.AllowHints}
		for _, q := range e.Questions {
			ej.IDs = append(ej.IDs, q.ID)
		}
		ej.Passed, _, _ = e.Score()
		out["exam"] = ej
	}
	writeJSON(w, http.StatusOK, out)
}

// ---- catalogue ----

type exSummary struct {
	ID         string   `json:"id"`
	Title      string   `json:"title"`
	Difficulty int      `json:"difficulty"`
	Kind       string   `json:"kind"`
	Tags       []string `json:"tags,omitempty"`
	Track      string   `json:"track"`
	Module     string   `json:"module"`
	Profile    string   `json:"profile"`
}

func (s *server) handleCatalog(w http.ResponseWriter, r *http.Request) {
	type track struct {
		ID    string `json:"id"`
		Name  string `json:"name"`
		Count int    `json:"count"`
	}
	type path struct {
		ID          string   `json:"id"`
		Title       string   `json:"title"`
		Description string   `json:"description"`
		IDs         []string `json:"ids"`
	}
	counts := map[string]int{}
	exs := make([]exSummary, 0, len(s.Catalog.Exercises))
	for _, e := range s.Catalog.Exercises {
		counts[e.Track]++
		exs = append(exs, exSummary{ID: e.ID, Title: e.Title, Difficulty: e.Difficulty, Kind: e.Kind, Tags: e.Tags,
			Track: e.Track, Module: e.Module, Profile: string(e.Profile)})
	}
	// list the exercises in the terminal UI's track order (the catalogue is alphabetical)
	rank := map[string]int{}
	for i, t := range exercise.TrackOrder {
		rank[t] = i + 1
	}
	sort.SliceStable(exs, func(i, j int) bool {
		ri, rj := rank[exs[i].Track], rank[exs[j].Track]
		if ri == 0 {
			ri = len(exercise.TrackOrder) + 1
		}
		if rj == 0 {
			rj = len(exercise.TrackOrder) + 1
		}
		return ri < rj
	})
	var tracks []track
	seen := map[string]bool{}
	for _, t := range exercise.TrackOrder {
		if counts[t] > 0 {
			tracks, seen[t] = append(tracks, track{t, trackName(t), counts[t]}), true
		}
	}
	for _, e := range s.Catalog.Exercises { // tracks beyond the built-in order, as they appear
		if !seen[e.Track] {
			tracks, seen[e.Track] = append(tracks, track{e.Track, trackName(e.Track), counts[e.Track]}), true
		}
	}
	paths := make([]path, 0, len(s.Paths))
	for _, p := range s.Paths {
		ids := make([]string, len(p.Exercises))
		for i, e := range p.Exercises {
			ids[i] = e.ID
		}
		paths = append(paths, path{p.ID, p.Title, p.Description, ids})
	}
	writeJSON(w, http.StatusOK, map[string]any{"tracks": tracks, "exercises": exs, "paths": paths})
}

// handleProgress returns the status of every exercise with progress.
func (s *server) handleProgress(w http.ResponseWriter, r *http.Request) {
	out := map[string]string{}
	for _, e := range s.Catalog.Exercises {
		if st := s.progressOf(e.ID); st != "" {
			out[e.ID] = st
		}
	}
	writeJSON(w, http.StatusOK, out)
}

// ---- one exercise ----

type terminalJSON struct {
	Target string `json:"target"`
	Label  string `json:"label"`
}

func terminalsFor(p exercise.Profile) []terminalJSON {
	switch p {
	case exercise.ProfileK8s:
		return []terminalJSON{{"k8s", "controlplane"}, {"node01", "node01"}}
	case exercise.ProfileOffsec:
		return []terminalJSON{{"offsec", "offsec"}}
	}
	return []terminalJSON{{"docker", "docker-host"}}
}

func (s *server) exerciseJSON(ex *exercise.Exercise) map[string]any {
	e := s.Progress.Get(ex.ID)
	locked := s.locked(ex)
	hints := []string{}
	if !locked {
		for i := 0; i < e.HintsUsed && i < len(ex.Hints); i++ {
			hints = append(hints, s.md.render(ex.Hints[i]))
		}
	}
	a, hasActive := s.Manager.Active()
	env := lab.EnvFor(ex.Profile)
	out := map[string]any{
		"id": ex.ID, "title": ex.Title, "difficulty": ex.Difficulty, "difficultyLabel": exercise.DifficultyLabel(ex.Difficulty),
		"kind": ex.Kind, "tags": ex.Tags, "track": ex.Track, "trackName": trackName(ex.Track), "module": ex.Module,
		"profile": string(ex.Profile), "envTitle": env.Title, "destructive": ex.Destructive,
		"scenario": s.md.render(ex.Scenario), "tutorial": s.md.render(ex.Tutorial),
		"hintCount": len(ex.Hints), "hints": hints, "locked": locked,
		"notes": s.Notes.Get(ex.ID), "status": s.progressOf(ex.ID),
		"progress": map[string]any{"checks": e.Checks, "hintsUsed": e.HintsUsed, "sawSolution": e.SawSolution,
			"started": e.Started, "solved": e.Solved},
		"active":       hasActive && a.ID == ex.ID,
		"terminals":    terminalsFor(ex.Profile),
		"notebook":     ex.Notebook,
		"deliverables": ex.Deliverables,
		"inExam":       false,
	}
	if ex := s.runningExam(); ex != nil {
		out["inExam"] = ex.Has(out["id"].(string))
	}
	if out["active"] == true {
		out["running"] = s.Manager.VMRunning(env, false)
	}
	if ex.Notebook {
		out["guide"], out["starter"] = exercise.NotebookGuide(ex), ex.NotebookName()
	}
	if e.SawSolution && !locked {
		out["solution"] = s.md.render(ex.Solution)
		out["history"] = s.Manager.History(ex.ID)
	}
	return out
}

func (s *server) handleExercise(w http.ResponseWriter, r *http.Request) {
	if ex, ok := s.exercise(w, r); ok {
		writeJSON(w, http.StatusOK, s.exerciseJSON(ex))
	}
}

func (s *server) handleHint(w http.ResponseWriter, r *http.Request) {
	ex, ok := s.exercise(w, r)
	if !ok {
		return
	}
	if s.locked(ex) {
		fail(w, http.StatusForbidden, "hints are locked during this exam")
		return
	}
	n := s.Progress.Get(ex.ID).HintsUsed
	if n >= len(ex.Hints) {
		fail(w, http.StatusConflict, "no more hints")
		return
	}
	s.Progress.UseHint(ex.ID, n+1)
	writeJSON(w, http.StatusOK, s.exerciseJSON(ex))
}

func (s *server) handleSolution(w http.ResponseWriter, r *http.Request) {
	ex, ok := s.exercise(w, r)
	if !ok {
		return
	}
	if s.locked(ex) {
		fail(w, http.StatusForbidden, "the solution is locked during this exam")
		return
	}
	s.Progress.SawSolution(ex.ID)
	writeJSON(w, http.StatusOK, s.exerciseJSON(ex))
}

func (s *server) handleNotes(w http.ResponseWriter, r *http.Request) {
	ex, ok := s.exercise(w, r)
	if !ok {
		return
	}
	var body struct {
		Text string `json:"text"`
	}
	if err := decodeJSON(r, &body); err != nil {
		fail(w, http.StatusBadRequest, "bad request: %v", err)
		return
	}
	if err := s.Notes.Set(ex.ID, body.Text); err != nil {
		fail(w, http.StatusInternalServerError, "saving notes: %v", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"notes": s.Notes.Get(ex.ID)})
}

func (s *server) handleForget(w http.ResponseWriter, r *http.Request) {
	ex, ok := s.exercise(w, r)
	if !ok {
		return
	}
	if e := s.runningExam(); e != nil && e.Has(ex.ID) {
		fail(w, http.StatusConflict, "progress can't be reset during an exam on this question")
		return
	}
	s.Progress.Reset(ex.ID)
	s.Manager.ForgetHistory(ex.ID)
	writeJSON(w, http.StatusOK, s.exerciseJSON(ex))
}

// handleExerciseOp starts, checks, boots or ends an exercise's lab.
func (s *server) handleExerciseOp(w http.ResponseWriter, r *http.Request) {
	ex, ok := s.exercise(w, r)
	if !ok {
		return
	}
	mgr := s.Manager
	env := lab.EnvFor(ex.Profile)
	a, hasActive := mgr.Active()
	isActive := hasActive && a.ID == ex.ID
	var (
		label string
		f     opFunc
		done  func(passed bool, err error)
	)
	switch action := r.PathValue("action"); action {
	case "start":
		label = "Preparing the lab for " + ex.ID
		f = func(ctx context.Context, w io.Writer) (bool, error) { return false, mgr.Start(ctx, ex, w) }
		done = func(_ bool, err error) {
			if err == nil {
				s.Progress.MarkStarted(ex.ID)
			}
		}
	case "check":
		if !isActive {
			fail(w, http.StatusConflict, "%s isn't set up on the VMs: start it first", ex.ID)
			return
		}
		label = "Checking " + ex.ID
		f = func(ctx context.Context, w io.Writer) (bool, error) {
			fmt.Fprintf(w, "Checking %s — %s\n\n", ex.ID, ex.Title)
			return mgr.Check(ctx, ex, w)
		}
		done = func(passed bool, err error) {
			if err != nil {
				return
			}
			s.Progress.RecordCheck(ex.ID, passed)
			if e := s.runningExam(); e != nil && e.Record(ex.ID, passed) {
				_ = s.Exams.Save(e)
			}
		}
	case "boot":
		if !isActive {
			fail(w, http.StatusConflict, "only the exercise set up on the VMs can boot them again: start %s instead", ex.ID)
			return
		}
		label = "Starting the " + env.Title + " again (your lab is kept)"
		f = func(ctx context.Context, w io.Writer) (bool, error) { return false, mgr.Ensure(ctx, env, w) }
	case "end":
		if !isActive {
			fail(w, http.StatusConflict, "%s isn't set up on the VMs", ex.ID)
			return
		}
		if e := s.runningExam(); e != nil {
			fail(w, http.StatusConflict, "during an exam the lab carries on to the next question — it can't be ended here")
			return
		}
		label = "Ending " + ex.ID
		f = func(ctx context.Context, w io.Writer) (bool, error) { return false, mgr.EndExercise(ctx, ex, w) }
	default:
		fail(w, http.StatusNotFound, "unknown action %q", action)
		return
	}
	if a := r.PathValue("action"); a == "start" || a == "end" {
		f = s.closingFirst(func() { s.terms.closeEnv(env.Profile) }, f) // their lab is about to be wiped
	}
	s.startOp(w, r.PathValue("action"), label, ex.ID, string(env.Profile), f, done)
}

// closingFirst runs closeTerms once the operation is under way, so a request
// refused as busy leaves the open terminals alone.
func (s *server) closingFirst(closeTerms func(), f opFunc) opFunc {
	return func(ctx context.Context, w io.Writer) (bool, error) {
		closeTerms()
		return f(ctx, w)
	}
}

// startOp runs f as the one lab operation; env is the environment it changes
// ("" for all of them), so terminals on it can wait until it is done.
func (s *server) startOp(w http.ResponseWriter, kind, label, target, env string, f opFunc, done func(bool, error)) {
	if kind != "check" && kind != "jupyter" && (env == "" || env == string(lab.DockerEnv.Profile)) {
		// whatever wipes the docker host removes its JupyterLab: a new one has a new token
		f = s.closingFirst(s.jupyter.forget, f)
	}
	s.act.touch()
	finished := func(passed bool, err error) {
		s.act.touch()
		if done != nil {
			done(passed, err)
		}
	}
	snap, err := s.ops.start(s.ctx, kind, label, target, env, f, finished)
	if err != nil {
		fail(w, http.StatusConflict, "%v", err)
		return
	}
	writeJSON(w, http.StatusAccepted, snap)
}

// ---- environments ----

func (s *server) handleEnvs(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	type envJSON struct {
		Profile string `json:"profile"`
		Title   string `json:"title"`
		Status  string `json:"status"`
		Saved   bool   `json:"saved"`
		Tainted bool   `json:"tainted"`
		// SavedDisk is the host disk its saved copies take, in bytes
		SavedDisk int64 `json:"savedDisk"`
	}
	type vmJSON struct {
		Name   string `json:"name"`
		Env    string `json:"env"`
		Role   string `json:"role"`
		Status string `json:"status"`
		CPUs   int    `json:"cpus"`
		Memory string `json:"memory"`
		Disk   int64  `json:"disk"` // bytes its instance directory takes on the host
		// StoppedSince is when a stopped VM went down (lab.VMInfo.StoppedSince)
		StoppedSince time.Time `json:"stoppedSince,omitzero"`
	}
	out := map[string]any{"cache": s.Manager.CacheSummary(), "logPath": s.Manager.LogPath()}
	envs, list, err := s.Manager.Overview(ctx)
	if err != nil {
		out["error"] = err.Error()
	}
	saved, _ := lab.SavedCopyUsage(ctx)
	ej := []envJSON{}
	for _, e := range envs {
		ej = append(ej, envJSON{string(e.Env.Profile), e.Env.Title, e.Status, e.Saved, e.Tainted, saved[string(e.Env.Profile)]})
	}
	vms := []vmJSON{}
	for _, v := range list {
		vms = append(vms, vmJSON{v.Name, v.Env, v.Role, v.Status, v.CPUs, v.MemoryLabel(), v.DiskUsage(), v.StoppedSince()})
	}
	_, days := s.live.get()
	out["envs"], out["vms"], out["deleteStoppedAfterDays"] = ej, vms, days
	writeJSON(w, http.StatusOK, out)
}

func envByProfile(p string) (lab.Env, bool) {
	for _, env := range lab.AllEnvs() {
		if string(env.Profile) == p {
			return env, true
		}
	}
	return lab.Env{}, false
}

func (s *server) handleEnvOp(w http.ResponseWriter, r *http.Request) {
	env, ok := envByProfile(r.PathValue("profile"))
	if !ok {
		fail(w, http.StatusNotFound, "unknown environment %q", r.PathValue("profile"))
		return
	}
	mgr := s.Manager
	acts := map[string]struct {
		label string
		f     func(context.Context, lab.Env, io.Writer) error
	}{
		"up":      {"Starting", mgr.Ensure},
		"stop":    {"Stopping", mgr.Stop},
		"reset":   {"Resetting", mgr.Reset},
		"rebuild": {"Rebuilding", mgr.Rebuild},
		"prepare": {"Preparing a saved copy of", mgr.Prepare},
		"destroy": {"Deleting", mgr.Destroy},
		"cache":   {"Caching the images of", mgr.CacheEnv},
	}
	action := r.PathValue("action")
	a, ok := acts[action]
	if !ok {
		fail(w, http.StatusNotFound, "unknown action %q", action)
		return
	}
	f := opFunc(func(ctx context.Context, w io.Writer) (bool, error) { return false, a.f(ctx, env, w) })
	if action != "up" && action != "cache" {
		f = s.closingFirst(func() { s.terms.closeEnv(env.Profile) }, f)
	}
	s.startOp(w, "env", a.label+" the "+env.Title, "env", string(env.Profile), f, nil)
}

func (s *server) handleVMOp(w http.ResponseWriter, r *http.Request) {
	name, action := r.PathValue("name"), r.PathValue("action")
	mgr := s.Manager
	var f func(context.Context, io.Writer) error
	switch {
	case name == "all" && action == "stop":
		f = mgr.StopAll
	case name == "all" && action == "delete":
		f = mgr.DeleteAll
	case action == "stop":
		f = func(ctx context.Context, w io.Writer) error { return mgr.StopVM(ctx, name, w) }
	case action == "delete":
		f = func(ctx context.Context, w io.Writer) error { return mgr.DeleteVM(ctx, name, w) }
	default:
		fail(w, http.StatusNotFound, "unknown action %q", action)
		return
	}
	if !strings.HasPrefix(name, "kl-") && name != "all" {
		fail(w, http.StatusBadRequest, "%s is not a lab VM", name)
		return
	}
	verb := map[string]string{"stop": "Stopping", "delete": "Deleting"}[action]
	what := name
	if name == "all" {
		what = "every lab VM"
	}
	envName := "" // all of them
	if e, ok := lab.EnvOfVM(name); ok {
		envName = string(e.Profile)
	}
	s.startOp(w, "env", verb+" "+what, "env", envName, s.closingFirst(func() { s.terms.closeVM(name) },
		func(ctx context.Context, w io.Writer) (bool, error) { return false, f(ctx, w) }), nil)
}
