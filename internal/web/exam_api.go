package web

import (
	"net/http"
	"slices"
	"strconv"
	"time"

	"awesome_labs/internal/exam"
	"awesome_labs/internal/exercise"
	"awesome_labs/internal/insight"
	"awesome_labs/internal/lab"
)

// Exams work as in the terminal UI: pick a pool, a number of questions, a time
// limit and whether hints are allowed; questions are graded by their checks
// (the last check counts) until the time runs out or the exam is finished.

func (s *server) examSources() []exam.Source {
	return exam.Sources(s.Catalog, s.Paths, func(p exercise.Profile) string { return lab.EnvFor(p).Title })
}

func (s *server) handleExamSetup(w http.ResponseWriter, r *http.Request) {
	type srcJSON struct {
		Index int    `json:"index"`
		Label string `json:"label"`
		Count int    `json:"count"`
	}
	srcs := []srcJSON{}
	for i, src := range s.examSources() {
		srcs = append(srcs, srcJSON{i, src.Label, len(src.Pool)})
	}
	writeJSON(w, http.StatusOK, map[string]any{"sources": srcs, "counts": exam.Counts, "minutes": exam.Minutes,
		"passMark": exam.PassMark, "running": s.runningExam() != nil})
}

type questionJSON struct {
	ID         string `json:"id"`
	Title      string `json:"title"`
	Difficulty int    `json:"difficulty"`
	Passed     bool   `json:"passed"`
	Checks     int    `json:"checks"`
}

func (s *server) examJSON(e *exam.Exam) map[string]any {
	qs := []questionJSON{}
	for _, q := range e.Questions {
		qj := questionJSON{ID: q.ID, Title: q.ID, Passed: q.Passed, Checks: q.Checks}
		if ex := s.Catalog.Get(q.ID); ex != nil {
			qj.Title, qj.Difficulty = ex.Title, ex.Difficulty
		}
		qs = append(qs, qj)
	}
	p, n, sc := e.Score()
	out := map[string]any{
		"title": e.Title, "source": e.Source, "allowHints": e.AllowHints, "questions": qs,
		"passed": p, "total": n, "percent": sc * 100, "pass": e.Pass(), "passMark": exam.PassMark * 100,
		"durationSeconds": int64(e.Duration.Seconds()), "remainingSeconds": int64(e.Remaining().Seconds()),
		"timeUsedSeconds": int64(e.TimeUsed().Seconds()), "started": e.Started,
		"finished": !e.Finished.IsZero(), "abandoned": e.Abandoned,
	}
	if !e.Finished.IsZero() {
		best := -1.0
		for _, h := range s.Exams.History() {
			if h.Source == e.Source && !h.Started.Equal(e.Started) {
				if _, _, hs := h.Score(); hs > best {
					best = hs
				}
			}
		}
		if best > 0 {
			out["previousBest"] = best * 100
		}
	}
	return out
}

// handleExam returns the running exam, or the last finished one as the
// result. An exam whose time ran out is finished here, as the terminal UI
// does when it notices.
func (s *server) handleExam(w http.ResponseWriter, r *http.Request) {
	if e := s.Exams.Current(); e != nil {
		if e.Over() {
			_ = s.Exams.Finish(e)
		} else {
			writeJSON(w, http.StatusOK, map[string]any{"exam": s.examJSON(e)})
			return
		}
	}
	out := map[string]any{"exam": nil}
	if h := s.Exams.History(); len(h) > 0 {
		out["last"] = s.examJSON(&h[len(h)-1])
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *server) handleExamStart(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Source  int  `json:"source"`
		Count   int  `json:"count"`
		Minutes int  `json:"minutes"`
		Hints   bool `json:"hints"`
	}
	if err := decodeJSON(r, &body); err != nil {
		fail(w, http.StatusBadRequest, "bad request: %v", err)
		return
	}
	if s.runningExam() != nil {
		fail(w, http.StatusConflict, "an exam is already running: finish or abandon it first")
		return
	}
	srcs := s.examSources()
	if body.Source < 0 || body.Source >= len(srcs) {
		fail(w, http.StatusBadRequest, "no exam pool %d", body.Source)
		return
	}
	if !slices.Contains(exam.Counts, body.Count) || !slices.Contains(exam.Minutes, body.Minutes) {
		fail(w, http.StatusBadRequest, "questions must be one of %v and minutes one of %v", exam.Counts, exam.Minutes)
		return
	}
	src := srcs[body.Source]
	e, err := exam.New(exam.Options{
		Title:      src.Label + " — " + strconv.Itoa(body.Count) + " questions",
		Source:     src.Label,
		Pool:       src.Pool,
		Count:      body.Count,
		Duration:   time.Duration(body.Minutes) * time.Minute,
		AllowHints: body.Hints,
	})
	if err != nil {
		fail(w, http.StatusBadRequest, "%v", err)
		return
	}
	if err := s.Exams.Save(e); err != nil {
		fail(w, http.StatusInternalServerError, "%v", err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"exam": s.examJSON(e)})
}

// handleExamEnd finishes the running exam ("finish": see the result) or gives
// it up ("abandon": recorded as abandoned). Either way it goes to the history.
func (s *server) handleExamEnd(w http.ResponseWriter, r *http.Request) {
	e := s.Exams.Current()
	if e == nil {
		fail(w, http.StatusConflict, "no exam is running")
		return
	}
	e.Abandoned = r.PathValue("action") == "abandon"
	if err := s.Exams.Finish(e); err != nil {
		fail(w, http.StatusInternalServerError, "%v", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"exam": nil, "last": s.examJSON(e)})
}

// ---- weak spots ----

func (s *server) handleWeakSpots(w http.ResponseWriter, r *http.Request) {
	rep := insight.WeakSpots(s.Catalog, func(id string) insight.Effort {
		p := s.Progress.Get(id)
		return insight.Effort{Hints: p.HintsUsed, Checks: p.Checks, SawSolution: p.SawSolution, Solved: !p.Solved.IsZero(), Started: !p.Started.IsZero()}
	})
	type retryJSON struct {
		ID    string `json:"id"`
		Title string `json:"title"`
	}
	retry := []retryJSON{}
	for _, id := range rep.Retry {
		t := id
		if ex := s.Catalog.Get(id); ex != nil {
			t = ex.Title
		}
		retry = append(retry, retryJSON{id, t})
	}
	mods := rep.Modules
	if mods == nil {
		mods = []insight.Module{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"any": rep.Any, "modules": mods, "retry": retry})
}
