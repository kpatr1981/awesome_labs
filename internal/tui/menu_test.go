package tui

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"awesome_labs/internal/exam"
	"awesome_labs/internal/exercise"
	"awesome_labs/internal/lab"
	"awesome_labs/internal/progress"
)

// Two docker exercises and one Kubernetes exercise, in two tracks.
const dockerModule = `track: docker
module: Basics
profile: docker
order: 1
exercises:
- id: d-1
  title: First docker task
  difficulty: 1
  kind: build
  scenario: Do the first thing.
  hints: [one, two]
  solution: the answer
  check: "true"
  solve: "true"
- id: d-2
  title: Second docker task
  difficulty: 3
  kind: fix
  scenario: Do the second thing.
  solution: the answer
  check: "true"
  solve: "true"
`

const k8sModule = `track: kubernetes
module: Pods
profile: k8s
order: 1
exercises:
- id: k-1
  title: First Kubernetes task
  difficulty: 2
  kind: build
  scenario: Run a pod.
  hints: [one]
  solution: the answer
  check: "true"
  solve: "true"
`

// busyOp blocks every lab operation: startOp refuses to run while one is
// going, so no test can reach limactl.
const busyOp = "test operation"

func newTestModel(t *testing.T) *model {
	t.Helper()
	dir := t.TempDir()
	cat, err := exercise.Load(fstest.MapFS{
		"ex/docker.yaml": {Data: []byte(dockerModule)},
		"ex/k8s.yaml":    {Data: []byte(k8sModule)},
	}, "ex")
	if err != nil {
		t.Fatal(err)
	}
	prog, err := progress.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	m := newModel(Deps{Catalog: cat, Manager: &lab.Manager{StateDir: dir}, Progress: prog, Exams: exam.NewStore(dir)}, "dark")
	m.Update(tea.WindowSizeMsg{Width: 150, Height: 50})
	m.endLab = func(context.Context, *exercise.Exercise, io.Writer) error { return nil } // never the real VMs
	m.deleteVMs = func(context.Context, io.Writer) error { return nil }
	m.op = busyOp
	return m
}

func (m *model) ex(t *testing.T, id string) *exercise.Exercise {
	t.Helper()
	e := m.cat.Get(id)
	if e == nil {
		t.Fatalf("no exercise %s", id)
	}
	return e
}

// activate makes id the exercise set up on the VMs, as a finished start does.
func (m *model) activate(t *testing.T, id string) {
	t.Helper()
	e := m.ex(t, id)
	b, _ := json.Marshal(lab.Active{ID: id, Profile: e.Profile, Since: time.Now()})
	if err := os.WriteFile(filepath.Join(m.mgr.StateDir, "active.json"), b, 0o644); err != nil {
		t.Fatal(err)
	}
	m.prog.MarkStarted(id)
	m.refreshActive()
}

func (m *model) runningExam(ids ...string) *exam.Exam {
	e := &exam.Exam{Title: "test exam", Started: time.Now().Add(-time.Minute), Duration: time.Hour}
	for _, id := range ids {
		e.Questions = append(e.Questions, exam.Question{ID: id})
	}
	m.exam = e
	return e
}

// item finds the visible item labelled label in the menu titled title.
func (m *model) item(title, label string) (menuItem, bool) {
	for _, mn := range m.menus() {
		if mn.title != title {
			continue
		}
		if i := itemIndex(mn.items, label); i >= 0 {
			return mn.items[i], true
		}
	}
	return menuItem{}, false
}

// choose runs the visible item labelled label of the menu titled title.
func (m *model) choose(t *testing.T, title, label string) tea.Cmd {
	t.Helper()
	for i, mn := range m.menus() {
		if mn.title == title {
			if j := itemIndex(mn.items, label); j >= 0 {
				m.menuOpen = i
				return m.runMenuItem(j)
			}
		}
	}
	t.Fatalf("no item %q in menu %q", label, title)
	return nil
}

type scenario struct {
	name  string
	setup func(t *testing.T, m *model)
}

var scenarios = []scenario{
	{"list", func(t *testing.T, m *model) {}},
	{"list with a filter and an active exercise", func(t *testing.T, m *model) {
		m.filter.SetValue("docker")
		m.refilter()
		m.activate(t, "d-1")
	}},
	{"list with nothing matching", func(t *testing.T, m *model) {
		m.filter.SetValue("nothing-matches-this")
		m.refilter()
	}},
	{"exercise never started", func(t *testing.T, m *model) { m.openDetail(m.ex(t, "d-1"), scrBrowse) }},
	{"exercise set up on the VMs", func(t *testing.T, m *model) {
		m.activate(t, "d-1")
		m.openDetail(m.ex(t, "d-1"), scrBrowse)
	}},
	{"exercise while another is set up", func(t *testing.T, m *model) {
		m.activate(t, "k-1")
		m.openDetail(m.ex(t, "d-2"), scrBrowse)
	}},
	{"kubernetes exercise set up", func(t *testing.T, m *model) {
		m.activate(t, "k-1")
		m.openDetail(m.ex(t, "k-1"), scrBrowse)
	}},
	{"started labs", func(t *testing.T, m *model) {
		m.prog.MarkStarted("d-2")
		m.activate(t, "d-1")
		m.openStarted()
	}},
	{"exercise opened from started labs", func(t *testing.T, m *model) {
		m.prog.MarkStarted("d-2")
		m.activate(t, "d-1")
		m.openStarted()
		m.openDetail(m.ex(t, "d-2"), scrStarted)
	}},
	{"environments", func(t *testing.T, m *model) { m.openEnv() }},
	{"environments opened from an exercise", func(t *testing.T, m *model) {
		m.activate(t, "d-1")
		m.openDetail(m.ex(t, "d-1"), scrBrowse)
		m.openEnv()
	}},
	{"learning paths", func(t *testing.T, m *model) { m.screen = scrPaths }},
	{"exam setup", func(t *testing.T, m *model) { m.screen = scrExamSetup }},
	{"running exam", func(t *testing.T, m *model) {
		m.runningExam("d-1", "d-2")
		m.activate(t, "d-1")
		m.screen = scrExam
	}},
	{"exam question", func(t *testing.T, m *model) {
		m.runningExam("d-1", "d-2")
		m.activate(t, "d-1")
		m.openDetail(m.ex(t, "d-1"), scrExam)
	}},
	{"list during an exam", func(t *testing.T, m *model) { m.runningExam("d-1", "d-2") }},
	{"exam result", func(t *testing.T, m *model) {
		e := m.runningExam("d-1", "d-2")
		e.Finished = time.Now()
		m.finished, m.exam = e, nil
		m.screen = scrExamResult
	}},
}

func named(name string) scenario {
	for _, sc := range scenarios {
		if sc.name == name {
			return sc
		}
	}
	panic("no scenario " + name)
}

func build(t *testing.T, sc scenario, busy bool) *model {
	t.Helper()
	m := newTestModel(t)
	m.op = ""
	sc.setup(t, m)
	m.flash = ""
	if busy {
		m.op = busyOp
	}
	return m
}

// state is what a key press or a menu item can change.
type state struct {
	screen, back, envBack           screen
	tab                             tab
	cur, confirm, flash, modal      string
	filterOn                        bool
	filter                          string
	progFilter                      progFilter
	trackIdx, cursor                int
	startedCursor, examCursor       int
	hintsShow                       int
	solShow, quitting, path, inExam bool
	menuOpen                        int
}

func snapshot(m *model) state {
	s := state{
		screen: m.screen, back: m.back, envBack: m.envBack, tab: m.tab,
		confirm: m.confirm, flash: m.flash, modal: m.modal,
		filterOn: m.filterOn, filter: m.filter.Value(), progFilter: m.progFilter,
		trackIdx: m.trackIdx, cursor: m.cursor, startedCursor: m.startedCursor, examCursor: m.examCursor,
		hintsShow: m.hintsShow, solShow: m.solShow, quitting: m.quitting, path: m.path != nil,
		inExam: m.exam != nil, menuOpen: m.menuOpen,
	}
	if m.cur != nil {
		s.cur = m.cur.ID
	}
	return s
}

// startsLabWork reports whether key k could start a lab operation or a shell
// when nothing else is running.
func startsLabWork(s screen, k string) bool {
	switch k {
	case "s", "t", "w", "c":
		return true
	case "u", "x", "R", "X":
		return s == scrEnv
	}
	return false
}

// TestMenuKeysDoWhatTheItemDoes checks the rule behind the key column: a key
// is shown next to an item only where pressing it, with the menu closed or
// open, does the same as choosing the item.
func TestMenuKeysDoWhatTheItemDoes(t *testing.T) {
	for _, sc := range scenarios {
		for _, busy := range []bool{true, false} {
			probe := build(t, sc, busy)
			for mi, mn := range probe.menus() {
				for _, it := range mn.items {
					if it.sep || it.disabled || it.key == "" || !busy && startsLabWork(probe.screen, it.key) {
						continue
					}
					name := sc.name + " / " + mn.title + " / " + it.label + " (" + it.key + ")"
					if busy {
						name += " while busy"
					}

					viaItem := build(t, sc, busy)
					viaItem.menuOpen = mi
					viaItem.runMenuItem(itemIndex(viaItem.menus()[mi].items, it.label))
					want := snapshot(viaItem)

					closed := build(t, sc, busy)
					closed.Update(keyMsg(it.key))
					if got := snapshot(closed); got != want {
						t.Errorf("%s: key with the menu closed\n got  %+v\n want %+v", name, got, want)
					}

					// an open menu takes single keys and F-keys as its items' keys
					if len([]rune(it.key)) == 1 || it.key[0] == 'F' {
						open := build(t, sc, busy)
						open.openMenu(mi)
						open.Update(keyMsg(it.key))
						if got := snapshot(open); got != want {
							t.Errorf("%s: key with the menu open\n got  %+v\n want %+v", name, got, want)
						}
					}
				}
			}
		}
	}
}

func TestEveryScreenRenders(t *testing.T) {
	for _, sc := range scenarios {
		m := build(t, sc, true)
		if out := m.View(); !strings.Contains(out, "awesome_labs") {
			t.Errorf("%s: no menu bar in\n%s", sc.name, out)
		}
		for i := range m.menus() {
			m.openMenu(i)
			m.View()
		}
	}
}

// buildEnding is build for the exercise set up on the VMs, with ending the lab
// recorded instead of reaching the VMs.
func buildEnding(t *testing.T) (*model, chan string) {
	m := build(t, named("exercise set up on the VMs"), false)
	ended := make(chan string, 1)
	m.endLab = func(_ context.Context, ex *exercise.Exercise, _ io.Writer) error { ended <- ex.ID; return nil }
	return m, ended
}

func TestLeavingTheActiveExerciseStopsIt(t *testing.T) {
	ways := map[string]func(t *testing.T, m *model){
		"esc":                      func(t *testing.T, m *model) { m.Update(keyMsg("esc")) },
		"q":                        func(t *testing.T, m *model) { m.Update(keyMsg("q")) },
		"S":                        func(t *testing.T, m *model) { m.Update(keyMsg("S")) },
		"n":                        func(t *testing.T, m *model) { m.Update(keyMsg("n")) },
		"mouse on the Back button": func(t *testing.T, m *model) { m.press("esc") },
	}
	for name, do := range ways {
		m, ended := buildEnding(t)
		do(t, m)
		if m.confirm != "" || m.screen == scrDetail && m.cur.ID == "d-1" {
			t.Errorf("%s: did not leave straight away: screen %v, confirm %q", name, m.screen, m.confirm)
			continue
		}
		select {
		case id := <-ended:
			if id != "d-1" {
				t.Errorf("%s: ended %s, want d-1", name, id)
			}
		case <-time.After(2 * time.Second):
			t.Errorf("%s: left without stopping the lab", name)
		}
	}

	// busy: it goes back and says the lab was left running
	m := build(t, named("exercise set up on the VMs"), true)
	m.Update(keyMsg("esc"))
	if m.screen != scrBrowse || !strings.Contains(m.flash, "busy") {
		t.Errorf("busy: screen %v, flash %q", m.screen, m.flash)
	}
}

// The exercise screen has no menus: its buttons do everything there.
func TestExerciseScreenHasNoMenus(t *testing.T) {
	m := build(t, named("exercise set up on the VMs"), false)
	if mns := m.menus(); len(mns) != 0 {
		t.Fatalf("the exercise screen has %d menus", len(mns))
	}
	for _, k := range []tea.KeyMsg{{Type: tea.KeyF10}, {Type: tea.KeyRunes, Runes: []rune("f"), Alt: true}} {
		m.Update(k)
		if m.menuOpen >= 0 || m.screen != scrDetail {
			t.Errorf("%s: menu %d, screen %v", k, m.menuOpen, m.screen)
		}
	}
	if out := m.View(); !strings.Contains(out, "awesome_labs") {
		t.Error("no title bar on the exercise screen")
	}
}

func TestLeavingWithoutStopping(t *testing.T) {
	// not the active exercise: nothing to stop
	m := build(t, named("exercise while another is set up"), false)
	m.Update(keyMsg("esc"))
	if m.confirm != "" || m.screen != scrBrowse {
		t.Errorf("inactive exercise: screen %v, confirm %q", m.screen, m.confirm)
	}
	// an exam question: the VMs carry on to the next question
	m = build(t, named("exam question"), false)
	m.Update(keyMsg("esc"))
	if m.confirm != "" || m.screen != scrExam {
		t.Errorf("exam question: screen %v, confirm %q", m.screen, m.confirm)
	}
	// the environments screen is not leaving: it goes back to the exercise
	m = build(t, named("exercise set up on the VMs"), false)
	m.Update(keyMsg("e"))
	if m.confirm != "" || m.screen != scrEnv {
		t.Fatalf("e: screen %v, confirm %q", m.screen, m.confirm)
	}
	m.Update(keyMsg("esc"))
	if m.screen != scrDetail || m.cur.ID != "d-1" {
		t.Errorf("esc from environments: screen %v, want the exercise", m.screen)
	}
}

func TestLeftArrowSwitchesTabs(t *testing.T) {
	m := build(t, named("exercise set up on the VMs"), false)
	m.Update(keyMsg("left"))
	if m.screen != scrDetail || m.tab != tabOutput || m.confirm != "" {
		t.Errorf("left: screen %v, tab %v, confirm %q; want the previous tab", m.screen, m.tab, m.confirm)
	}
	m.Update(keyMsg("right"))
	if m.tab != tabScenario {
		t.Errorf("right: tab %v, want the next tab", m.tab)
	}
}

func TestBackGoesWhereTheExerciseWasOpened(t *testing.T) {
	m := build(t, named("started labs"), true) // started labs
	m.startedCursor = 1
	m.Update(keyMsg("enter"))
	m.Update(keyMsg("esc"))
	if m.screen != scrStarted {
		t.Errorf("from started labs: back to %v", m.screen)
	}

	m = build(t, named("exam result"), true) // exam result
	m.Update(keyMsg("o"))
	if m.screen != scrDetail {
		t.Fatalf("o did not open the question")
	}
	m.Update(keyMsg("esc"))
	if m.screen != scrExamResult {
		t.Errorf("from the exam result: back to %v", m.screen)
	}

	// a from the exam screen opens the active question as one of the exam's
	m = build(t, named("running exam"), true)
	m.examCursor = 1
	m.Update(keyMsg("a"))
	if m.screen != scrDetail || m.back != scrExam || m.examCursor != 0 {
		t.Errorf("a from the exam: screen %v, back %v, cursor %d", m.screen, m.back, m.examCursor)
	}
}

func TestNextWalksTheListItCameFrom(t *testing.T) {
	m := newTestModel(t)
	m.trackIdx = 1 // docker
	m.refilter()
	m.cursor = 1
	m.openDetail(m.ex(t, "d-2"), scrBrowse)
	m.Update(keyMsg("n"))
	if m.cur.ID != "d-2" || !strings.Contains(m.flash, "no next exercise") {
		t.Errorf("past the end of the docker track: on %s, flash %q", m.cur.ID, m.flash)
	}
	m.Update(keyMsg("N"))
	if m.cur.ID != "d-1" || m.cursor != 0 {
		t.Errorf("N: on %s, list cursor %d", m.cur.ID, m.cursor)
	}

	m = newTestModel(t) // all tracks
	m.openDetail(m.ex(t, "d-2"), scrBrowse)
	m.Update(keyMsg("n"))
	if m.cur.ID != "k-1" || m.cursor != 2 {
		t.Errorf("all tracks: on %s, list cursor %d", m.cur.ID, m.cursor)
	}

	m = newTestModel(t)
	m.runningExam("d-2", "k-1")
	m.openDetail(m.ex(t, "d-2"), scrExam)
	m.Update(keyMsg("n"))
	if m.cur.ID != "k-1" || m.examCursor != 1 || m.back != scrExam {
		t.Errorf("exam: on %s, exam cursor %d, back %v", m.cur.ID, m.examCursor, m.back)
	}
}

// The highlight stays on its item when items appear above it while the menu
// is open.
func TestMenuHighlightFollowsItsItem(t *testing.T) {
	m := newTestModel(t)
	for i, mn := range m.menus() {
		if mn.title == "View" {
			m.openMenu(i)
		}
	}
	m.menuSel = "Choose a learning path…"
	m.filter.SetValue("zzz") // brings "Clear the filter" in above it
	if _, ok := m.item("View", "Clear the filter"); !ok {
		t.Fatal("no clear-filter item with a filter set")
	}
	m.Update(keyMsg("enter"))
	if m.screen != scrPaths {
		t.Errorf("enter ran the wrong item: screen %v", m.screen)
	}
}

// The Program menu holds only Exit, and exiting deletes the VMs.
func TestProgramMenuIsOnlyExit(t *testing.T) {
	m := build(t, named("list"), false)
	mn := m.menus()[0]
	if mn.title != "Program" || len(mn.items) != 1 || mn.items[0].label != "Exit" {
		t.Fatalf("first menu %q has %d items", mn.title, len(mn.items))
	}
	mn.items[0].run()
	if !m.quitting || !strings.Contains(m.flash, "deleting") {
		t.Errorf("exit: quitting %v, flash %q", m.quitting, m.flash)
	}
}

// btn finds the button labelled label among bs.
func btn(t *testing.T, bs []button, label string) button {
	t.Helper()
	for _, b := range bs {
		if b.label == label {
			return b
		}
	}
	t.Fatalf("no %q button", label)
	return button{}
}

// Every start resets the environment, so only the active exercise has its lab,
// in an exam too.
func TestLabActionsNeedTheActiveExercise(t *testing.T) {
	m := newTestModel(t)
	m.runningExam("d-1", "d-2")
	m.prog.MarkStarted("d-2") // started earlier in the exam…
	m.activate(t, "d-1")      // …then replaced by d-1
	m.op = ""
	if btn(t, m.detailButtons(m.ex(t, "d-1")), "✔ Check").off {
		t.Error("Check is dimmed for the active question")
	}
	m.openDetail(m.ex(t, "d-2"), scrExam)
	if !btn(t, m.detailButtons(m.cur), "✔ Check").off {
		t.Error("Check is lit for a question whose lab was reset")
	}
	m.Update(keyMsg("c"))
	if m.op != "" || !strings.Contains(m.flash, "isn't set up") {
		t.Errorf("c on an exercise not set up: op %q, flash %q", m.op, m.flash)
	}
}

func TestFinishExamKeyOnlyOnTheExamScreen(t *testing.T) {
	m := newTestModel(t)
	m.runningExam("d-1")
	if it, _ := m.item("Exam", "Finish the exam…"); it.key != "" {
		t.Errorf("Finish shows key %q on the list, where F does nothing", it.key)
	}
	m.screen = scrExam
	if it, _ := m.item("Exam", "Finish the exam…"); it.key != "F" {
		t.Errorf("Finish shows key %q on the exam screen, want F", it.key)
	}
}

func TestStartedLabsResetChecks(t *testing.T) {
	m := build(t, named("started labs"), true)
	m.Update(keyMsg("r"))
	if m.confirm != "" || !strings.Contains(m.flash, "busy") {
		t.Errorf("r while busy: confirm %q, flash %q", m.confirm, m.flash)
	}
	m.flash = ""
	m.Update(keyMsg("A"))
	if m.confirm != "" || !strings.Contains(m.flash, "busy") {
		t.Errorf("A while busy: confirm %q, flash %q", m.confirm, m.flash)
	}

	m = newTestModel(t)
	m.op = ""
	m.runningExam("d-1")
	m.prog.MarkStarted("d-1")
	m.openStarted()
	if !btn(t, m.startedButtons(), "Reset selected").off {
		t.Error("Reset selected is lit for a question of the running exam")
	}
}

// A button is lit only when pressing it can act now.
func TestButtonsLitOnlyWhenTheyApply(t *testing.T) {
	lit := func(bs []button) string {
		var out []string
		for _, b := range bs {
			if !b.off {
				out = append(out, b.label)
			}
		}
		return strings.Join(out, ",")
	}
	cases := []struct {
		scenario string
		busy     bool
		want     string
	}{
		{"exercise never started", false, "▶ Start,Hint,Solution,Back"},
		{"exercise set up on the VMs", false, "↻ Restart,Terminal,✔ Check,Hint,Solution,Reset,Back"},
		{"exercise set up on the VMs", true, "Hint,Solution,Back"},
		{"exercise while another is set up", false, "▶ Start,Solution,Back"}, // d-2 has no hints
		{"kubernetes exercise set up", false, "↻ Restart,Terminal,node01,✔ Check,Hint,Solution,Reset,Back"},
		{"exam question", false, "↻ Restart,Terminal,✔ Check,Back"},
	}
	for _, c := range cases {
		m := build(t, named(c.scenario), c.busy)
		if got := lit(m.detailButtons(m.cur)); got != c.want {
			t.Errorf("%s (busy %v): lit %q, want %q", c.scenario, c.busy, got, c.want)
		}
	}
	m := build(t, named("started labs"), false)
	if got := lit(m.startedButtons()); got != "Open,Reset selected,Reset all,Back" {
		t.Errorf("started labs: lit %q", got)
	}
	m.started = nil
	if got := lit(m.startedButtons()); got != "Back" {
		t.Errorf("no started labs: lit %q", got)
	}
}

func TestShellReturnTurnsTheMouseBackOn(t *testing.T) {
	m := newTestModel(t)
	_, c := m.Update(shellDoneMsg{})
	if c == nil {
		t.Fatal("no command after the shell")
	}
	if msg := c(); msg != tea.EnableMouseAllMotion() {
		t.Errorf("after the shell: %T, want the mouse turned back on", msg)
	}
}

func TestVisibleItems(t *testing.T) {
	a := menuItem{label: "a"}
	b := menuItem{label: "b"}
	hid := menuItem{label: "x", hidden: true}
	got := visibleItems([]menuItem{sep(), a, sep(), hid, sep(), head("H", ""), b, sep(), hid})
	var labels []string
	for _, it := range got {
		if it.sep {
			labels = append(labels, "─"+it.label)
		} else {
			labels = append(labels, it.label)
		}
	}
	if s := strings.Join(labels, " "); s != "a ─H b" {
		t.Errorf("got %q, want %q", s, "a ─H b")
	}
}

func TestQuitStopsTheVMs(t *testing.T) {
	for _, k := range []string{"q", "Q", "ctrl+c"} {
		m := build(t, named("list"), false)
		m.Update(keyMsg(k)) // the stop itself is the returned command, not run here
		if !m.quitting {
			t.Errorf("%s: quit without stopping the VMs", k)
		}
	}
	// busy: q refuses, ctrl+c still gets out
	m := build(t, named("list"), true)
	m.Update(keyMsg("q"))
	if m.quitting || !strings.Contains(m.flash, "busy") {
		t.Errorf("q while busy: quitting %v, flash %q", m.quitting, m.flash)
	}
	if _, c := m.Update(keyMsg("ctrl+c")); c == nil {
		t.Error("ctrl+c while busy did not quit")
	}
}

// Starting the app deletes the lab VMs left over from an earlier run and
// forgets the exercise that was set up on them.
func TestStartDeletesLeftoverVMs(t *testing.T) {
	m := newTestModel(t)
	m.op = ""
	m.activate(t, "d-1")
	deleted := make(chan struct{}, 1)
	m.deleteVMs = func(context.Context, io.Writer) error { deleted <- struct{}{}; return nil }
	m.Init()
	if m.op == "" {
		t.Fatal("start did not begin deleting the VMs")
	}
	select {
	case <-deleted:
	case <-time.After(2 * time.Second):
		t.Fatal("start never deleted the VMs")
	}
	for msg := range m.opCh {
		m.Update(msg)
	}
	if m.op != "" || m.hasActive {
		t.Errorf("after the cleanup: op %q, active %v", m.op, m.hasActive)
	}
}
