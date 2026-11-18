package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"awesome_labs/internal/config"
	"awesome_labs/internal/exam"
	"awesome_labs/internal/exercise"
	"awesome_labs/internal/lab"
	"awesome_labs/internal/progress"
)

const testModule = `track: docker
module: Basics
profile: docker
order: 1
exercises:
- {id: d-1, title: One, difficulty: 1, kind: build, scenario: s, solution: s, check: "true", solve: "true"}
- {id: d-2, title: Two, difficulty: 2, kind: fix, scenario: s, solution: s, check: "true", solve: "true"}
- {id: d-3, title: Three, difficulty: 3, kind: build, scenario: s, solution: s, check: "true", solve: "true"}
- {id: d-4, title: Four, difficulty: 4, kind: build, scenario: s, solution: s, check: "true", solve: "true"}
- {id: d-5, title: Five, difficulty: 5, kind: build, tags: [capstone], scenario: s, solution: s, check: "true", solve: "true"}
`

// featureServer is a server over a small catalogue and stores in a temp dir.
func featureServer(t *testing.T) (*server, http.Handler, string) {
	t.Helper()
	dir := t.TempDir()
	cat, err := exercise.Load(fstest.MapFS{"ex/docker/01.yaml": {Data: []byte(testModule)}}, "ex")
	if err != nil {
		t.Fatal(err)
	}
	prog, err := progress.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	s := testServer()
	s.Deps = Deps{Catalog: cat, Progress: prog, Exams: exam.NewStore(dir), Manager: &lab.Manager{StateDir: dir}, OnExit: "stop", AutoStop: true}
	s.jupyter.proxy = s.newJupyterProxy()
	return s, s.routes(), dir
}

func call(t *testing.T, h http.Handler, method, path, body string) (int, map[string]any) {
	t.Helper()
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	r.Host = "127.0.0.1:7681"
	r.AddCookie(&http.Cookie{Name: cookieName, Value: "secret"})
	if body != "" {
		r.Header.Set("Content-Type", "application/json")
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	var out map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &out)
	return w.Code, out
}

func TestExamLifecycle(t *testing.T) {
	s, h, _ := featureServer(t)
	code, setup := call(t, h, "GET", "/api/exam/setup", "")
	if code != 200 {
		t.Fatalf("setup: %d", code)
	}
	srcs := setup["sources"].([]any)
	docker := -1
	for _, x := range srcs {
		m := x.(map[string]any)
		if m["label"] == "Docker — everything" {
			docker = int(m["index"].(float64))
			if m["count"].(float64) != 5 {
				t.Errorf("docker pool has %v exercises", m["count"])
			}
		}
	}
	if docker < 0 {
		t.Fatalf("no Docker pool in %v", srcs)
	}
	if code, out := call(t, h, "POST", "/api/exam", `{"source": 0, "count": 7, "minutes": 60}`); code != 400 {
		t.Errorf("a question count not on offer: %d %v", code, out)
	}
	code, out := call(t, h, "POST", "/api/exam", `{"source": `+itoaT(docker)+`, "count": 5, "minutes": 30, "hints": false}`)
	if code != 201 {
		t.Fatalf("start: %d %v", code, out)
	}
	e := out["exam"].(map[string]any)
	if len(e["questions"].([]any)) != 5 || e["allowHints"] != false || e["remainingSeconds"].(float64) < 1700 {
		t.Fatalf("exam %v", e)
	}
	if code, _ := call(t, h, "POST", "/api/exam", `{"source": 0, "count": 5, "minutes": 30}`); code != 409 {
		t.Errorf("a second exam while one runs: %d", code)
	}
	// hints are locked on its questions
	if code, _ := call(t, h, "POST", "/api/exercises/d-1/hint", ""); code != 403 {
		t.Errorf("hint during a locked exam: %d", code)
	}
	cur := s.Exams.Current()
	cur.Record("d-1", true)
	cur.Record("d-2", false)
	s.Exams.Save(cur)
	code, out = call(t, h, "POST", "/api/exam/finish", "")
	if code != 200 {
		t.Fatalf("finish: %d %v", code, out)
	}
	last := out["last"].(map[string]any)
	if last["passed"].(float64) != 1 || last["total"].(float64) != 5 || last["pass"] != false || last["finished"] != true {
		t.Fatalf("result %v", last)
	}
	if code, out := call(t, h, "GET", "/api/exam", ""); code != 200 || out["exam"] != nil || out["last"] == nil {
		t.Fatalf("after finishing: %d %v", code, out)
	}
	if code, _ := call(t, h, "POST", "/api/exam/abandon", ""); code != 409 {
		t.Errorf("abandon with no exam: %d", code)
	}
}

func TestExamAbandonAndTimeout(t *testing.T) {
	s, h, _ := featureServer(t)
	e, err := exam.New(exam.Options{Title: "T", Source: "Docker — everything", Pool: s.Catalog.Exercises, Count: 5, Duration: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	s.Exams.Save(e)
	code, out := call(t, h, "POST", "/api/exam/abandon", "")
	if code != 200 || out["last"].(map[string]any)["abandoned"] != true {
		t.Fatalf("abandon: %d %v", code, out)
	}
	if h := s.Exams.History(); len(h) != 1 || !h[0].Abandoned {
		t.Fatalf("history %+v", h)
	}
	// out of time: reading it finishes it
	e.Started, e.Finished, e.Abandoned = time.Now().Add(-2*time.Hour), time.Time{}, false
	s.Exams.Save(e)
	if _, out := call(t, h, "GET", "/api/exam", ""); out["exam"] != nil || out["last"] == nil {
		t.Fatalf("an exam out of time still runs: %v", out)
	}
	if s.Exams.Current() != nil || len(s.Exams.History()) != 2 {
		t.Fatal("the timed-out exam wasn't moved to the history")
	}
}

func TestWeakSpotsEndpoint(t *testing.T) {
	s, h, _ := featureServer(t)
	if _, out := call(t, h, "GET", "/api/weakspots", ""); out["any"] != false {
		t.Fatalf("no progress: %v", out)
	}
	s.Progress.MarkStarted("d-2")
	s.Progress.UseHint("d-2", 2)
	s.Progress.SawSolution("d-2")
	s.Progress.RecordCheck("d-2", false)
	s.Progress.RecordCheck("d-1", true)
	_, out := call(t, h, "GET", "/api/weakspots", "")
	mods := out["modules"].([]any)
	if len(mods) != 1 || mods[0].(map[string]any)["name"] != "Docker · Basics" || mods[0].(map[string]any)["solutions"].(float64) != 1 {
		t.Fatalf("modules %v", mods)
	}
	retry := out["retry"].([]any)
	if len(retry) != 1 || retry[0].(map[string]any)["id"] != "d-2" || retry[0].(map[string]any)["title"] != "Two" {
		t.Fatalf("retry %v", retry)
	}
}

func TestSettingsEndpoint(t *testing.T) {
	s, h, dir := featureServer(t)
	code, out := call(t, h, "GET", "/api/settings", "")
	if code != 200 || len(out["settings"].([]any)) != 6 {
		t.Fatalf("get: %d %v", code, out)
	}
	if code, out := call(t, h, "PUT", "/api/settings", `{"theme": "neon"}`); code != 400 {
		t.Errorf("invalid theme: %d %v", code, out)
	}
	if code, _ := call(t, h, "PUT", "/api/settings", `{"nope": 1}`); code != 400 {
		t.Errorf("unknown key: %d", code)
	}
	if code, _ := call(t, h, "PUT", "/api/settings", `{"delete_stopped_after_days": 1.5}`); code != 400 {
		t.Errorf("fractional days: %d", code)
	}
	changed := s.live.wait()
	code, out = call(t, h, "PUT", "/api/settings", `{"auto_stop": false, "delete_stopped_after_days": 14, "on_exit": "delete"}`)
	if code != 200 {
		t.Fatalf("put: %d %v", code, out)
	}
	if on, days := s.live.get(); on || days != 14 {
		t.Errorf("live settings not applied: auto_stop %v, days %d", on, days)
	}
	select {
	case <-changed:
	default:
		t.Error("a new day limit didn't wake the clean-up")
	}
	c, err := config.Load(dir)
	if err != nil || c.ExitAction() != "delete" || c.AutoStopEnabled() || c.DeleteStoppedAfterDays != 14 {
		t.Fatalf("saved config %+v, %v", c, err)
	}
	b, _ := os.ReadFile(config.Path(dir))
	if !strings.Contains(string(b), "# What leaving the app does") {
		t.Error("the template's comments were lost")
	}
	if out["runningOnExit"] != "stop" {
		t.Errorf("this run's on_exit is what it started with: %v", out["runningOnExit"])
	}
}

func TestCatalogCarriesCapstoneTags(t *testing.T) {
	_, h, _ := featureServer(t)
	_, out := call(t, h, "GET", "/api/catalog", "")
	for _, x := range out["exercises"].([]any) {
		e := x.(map[string]any)
		if e["id"] == "d-5" && !strings.Contains(strings.Join(anyStrings(e["tags"]), ","), "capstone") {
			t.Fatalf("d-5 tags %v: the list needs them for the Capstone tier", e["tags"])
		}
	}
}

func anyStrings(v any) []string {
	var out []string
	for _, x := range v.([]any) {
		out = append(out, x.(string))
	}
	return out
}

func itoaT(n int) string { b, _ := json.Marshal(n); return string(b) }
