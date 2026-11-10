// Package tui is the interactive exercise browser.
package tui

import (
	"context"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"awesome_labs/internal/exam"
	"awesome_labs/internal/exercise"
	"awesome_labs/internal/lab"
	"awesome_labs/internal/progress"
)

type screen int

const (
	scrBrowse screen = iota
	scrDetail
	scrEnv
	scrPaths
	scrExamSetup
	scrExam
	scrExamResult
	scrStarted
)

type tab int

const (
	tabScenario tab = iota
	tabLearn
	tabHints
	tabSolution
	tabOutput
)

var tabNames = []string{"Scenario", "Learn", "Hints", "Solution", "Output"}

// ---- messages ----

type opLineMsg string
type opDoneMsg struct {
	kind   string
	id     string
	err    error
	passed bool
}
type envStatusMsg struct {
	status map[exercise.Profile]string
	vms    map[string]lab.VMInfo
}
type shellDoneMsg struct{ err error }
type tickMsg time.Time
type stoppedMsg struct{ err error }

// ---- model ----

type model struct {
	cat  *exercise.Catalog
	mgr  *lab.Manager
	prog *progress.Store

	w, h   int
	screen screen

	// browse
	tracks     []string // "" = all
	trackIdx   int
	progFilter progFilter
	filter     textinput.Model
	filterOn   bool
	visible    []*exercise.Exercise
	cursor     int
	offset     int

	// detail
	cur       *exercise.Exercise
	tab       tab
	vp        viewport.Model
	hintsShow int
	solShow   bool
	back      screen // where the exercise screen returns to
	confirm   string // pending confirmation question
	onYes     func() tea.Cmd
	onNo      func() tea.Cmd

	// operations
	op     string // running operation label, "" = idle
	opID   string
	opCh   chan tea.Msg
	output map[string]*strings.Builder // per exercise id / "env"
	result map[string]string           // last check result per id
	spin   spinner.Model
	flash  string

	// env screen
	envStatus map[exercise.Profile]string
	vmInfo    map[string]lab.VMInfo
	envCursor int
	envBack   screen // where the environments screen returns to
	// endLab ends the active exercise and deletes its VMs; tests replace it
	endLab func(ctx context.Context, ex *exercise.Exercise, w io.Writer) error
	// deleteVMs deletes every lab VM, on start and on exit; tests replace it
	deleteVMs func(ctx context.Context, w io.Writer) error

	// the one exercise set up on the VMs right now
	active    lab.Active
	hasActive bool

	// started-labs screen
	startedCursor int
	started       []*exercise.Exercise

	style    string
	trackIDs map[string][]string // exercise ids per track, "" = all of them
	mdMu     sync.Mutex
	mdCache  map[string]string

	// learning paths
	paths      []*exercise.Path
	path       *exercise.Path // active path filter, nil = none
	pathCursor int

	// exams
	exams       *exam.Store
	exam        *exam.Exam // running exam, nil = none
	finished    *exam.Exam // last finished exam (result screen)
	examCursor  int
	setupField  int
	setupSource int
	setupCount  int
	setupMins   int
	setupHints  bool
	quitting    bool

	// menus, dialogs and the mouse
	menuOpen    int    // index of the open menu, -1 = none
	menuSel     string // label of the highlighted item of the open menu
	menuX       []int  // x of each menu title
	modal       string
	confirmYes  bool // the Yes button of the confirmation has the focus
	zones       []zone
	hover       string
	lastClick   string
	lastClickAt time.Time
}

// inExam reports whether ex is a question of the running exam.
func (m *model) inExam(ex *exercise.Exercise) bool {
	return ex != nil && m.exam != nil && !m.exam.Over() && m.exam.Has(ex.ID)
}

// examLocked reports whether hints/solution of ex are hidden by a running exam.
func (m *model) examLocked(ex *exercise.Exercise) bool {
	return m.inExam(ex) && !m.exam.AllowHints
}

// Deps are the services the TUI works with.
type Deps struct {
	Catalog  *exercise.Catalog
	Manager  *lab.Manager
	Progress *progress.Store
	Paths    []*exercise.Path
	Exams    *exam.Store
}

// Run starts the TUI.
func Run(d Deps) error {
	style := "dark"
	if !lipgloss.HasDarkBackground() {
		style = "light"
	}
	m := newModel(d, style)
	_, err := tea.NewProgram(m, tea.WithAltScreen(), tea.WithMouseAllMotion()).Run()
	return err
}

func newModel(d Deps, style string) *model {
	cat, mgr, prog := d.Catalog, d.Manager, d.Progress
	ti := textinput.New()
	ti.Placeholder = "type to filter (id, title, tag, module)…"
	ti.Prompt = "/ "
	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = lipgloss.NewStyle().Foreground(colAccent)
	m := &model{
		cat: cat, mgr: mgr, prog: prog,
		tracks: []string{"", "docker", "kubernetes", "offsec", "linux", "networking", "cloud", "data", "defensive", "programming", "datascience"},
		filter: ti, spin: sp, style: style,
		output: map[string]*strings.Builder{}, result: map[string]string{},
		mdCache: map[string]string{}, envStatus: map[exercise.Profile]string{},
		paths: d.Paths, exams: d.Exams, setupCount: 1, setupMins: 1, menuOpen: -1,
	}
	m.endLab = mgr.EndExercise
	m.deleteVMs = mgr.DeleteAll
	m.exam = d.Exams.Current()
	if m.exam != nil && m.exam.Over() {
		m.finishExam()
	}
	m.trackIDs = map[string][]string{}
	for _, e := range cat.Exercises {
		m.trackIDs[e.Track] = append(m.trackIDs[e.Track], e.ID)
		m.trackIDs[""] = append(m.trackIDs[""], e.ID)
	}
	m.refreshActive()
	m.refilter()
	return m
}

func (m *model) refreshActive() { m.active, m.hasActive = m.mgr.Active() }

// isActive reports whether ex is the exercise currently set up on the VMs.
func (m *model) isActive(ex *exercise.Exercise) bool {
	return ex != nil && m.hasActive && m.active.ID == ex.ID
}

// Init starts by deleting every lab VM left over from an earlier run,
// running or stopped: exiting deletes them, so any still there were left by a
// crash or a ctrl+c while busy. No exercise is set up on the VMs after it.
func (m *model) Init() tea.Cmd {
	cleanup := m.startOp("cleanup", "Deleting leftover lab VMs", "env", func(ctx context.Context, w io.Writer) (bool, error) {
		if err := m.deleteVMs(ctx, w); err != nil {
			return false, err
		}
		m.mgr.ClearActive("")
		return false, nil
	})
	return tea.Batch(m.spin.Tick, tick(), cleanup)
}

func tick() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg { return tickMsg(t) })
}

// ---- filtering ----

// progFilter narrows the exercise list by recorded progress.
type progFilter int

const (
	progAll        progFilter = iota
	progSolved                // solved (✔ in the list)
	progInProgress            // started, not solved (● in the list)
	progNone                  // not started, not solved
	progFilterCount
)

var progFilterNames = []string{"All exercises", "Solved", "In progress", "No progress"}

func (f progFilter) keep(p progress.Entry) bool {
	solved, started := !p.Solved.IsZero(), !p.Started.IsZero()
	switch f {
	case progSolved:
		return solved
	case progInProgress:
		return started && !solved
	case progNone:
		return !started && !solved
	}
	return true
}

func (m *model) setProgFilter(f progFilter) {
	m.progFilter = f
	m.cursor, m.offset = 0, 0
	m.refilter()
}

func (m *model) refilter() {
	q := strings.ToLower(strings.TrimSpace(m.filter.Value()))
	track := m.tracks[m.trackIdx]
	m.visible = m.visible[:0]
	pool := m.cat.Exercises
	if m.path != nil {
		pool = m.path.Exercises
	}
	for _, e := range pool {
		if track != "" && e.Track != track {
			continue
		}
		if !m.progFilter.keep(m.prog.Get(e.ID)) {
			continue
		}
		if q != "" {
			hay := strings.ToLower(e.ID + " " + e.Title + " " + e.Module + " " + e.Kind + " " + strings.Join(e.Tags, " ") + " " + exercise.DifficultyLabel(e.Difficulty))
			ok := true
			for _, w := range strings.Fields(q) {
				if !strings.Contains(hay, w) {
					ok = false
					break
				}
			}
			if !ok {
				continue
			}
		}
		m.visible = append(m.visible, e)
	}
	if m.cursor >= len(m.visible) {
		m.cursor = max(0, len(m.visible)-1)
	}
	m.clampOffset()
}

func (m *model) listHeight() int {
	if m.hasBrowseBar() {
		return max(3, m.h-6)
	}
	return max(3, m.h-5)
}

// hasBrowseBar reports whether the browse screen shows its info line above the list.
func (m *model) hasBrowseBar() bool {
	return m.path != nil || m.progFilter != progAll || m.filterOn || m.filter.Value() != ""
}

func (m *model) clampOffset() {
	lh := m.listHeight()
	// rows include module headers; approximate by keeping cursor within window of items
	if m.cursor < m.offset {
		m.offset = m.cursor
	}
	for m.rowsBetween(m.offset, m.cursor) >= lh {
		m.offset++
	}
}

// rowsBetween counts rendered rows (items + module headers) from item a to b inclusive.
func (m *model) rowsBetween(a, b int) int {
	rows := 0
	for i := a; i <= b && i < len(m.visible); i++ {
		if i == a || m.visible[i].Module != m.visible[i-1].Module || m.visible[i].Track != m.visible[i-1].Track {
			rows++
		}
		rows++
	}
	return rows
}

// ---- operations ----

type chanWriter struct {
	ch  chan tea.Msg
	buf []byte
	mu  sync.Mutex
}

func (c *chanWriter) Write(b []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.buf = append(c.buf, b...)
	for {
		i := strings.IndexByte(string(c.buf), '\n')
		if i < 0 {
			break
		}
		c.ch <- opLineMsg(string(c.buf[:i]))
		c.buf = c.buf[i+1:]
	}
	return len(b), nil
}

func (c *chanWriter) flush() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.buf) > 0 {
		c.ch <- opLineMsg(string(c.buf))
		c.buf = nil
	}
}

func waitOp(ch chan tea.Msg) tea.Cmd {
	return func() tea.Msg {
		msg, ok := <-ch
		if !ok {
			return nil
		}
		return msg
	}
}

func (m *model) out(id string) *strings.Builder {
	b := m.output[id]
	if b == nil {
		b = &strings.Builder{}
		m.output[id] = b
	}
	return b
}

// startOp runs f in the background, streaming its output to the Output tab.
func (m *model) startOp(kind, label, id string, f func(ctx context.Context, w io.Writer) (bool, error)) tea.Cmd {
	if m.op != "" {
		m.flash = "busy: " + m.op
		return nil
	}
	m.op, m.opID = label, id
	ch := make(chan tea.Msg, 512)
	m.opCh = ch
	b := m.out(id)
	b.Reset()
	if m.screen == scrDetail {
		m.tab = tabOutput
	}
	m.refreshViewport(true)
	go func() {
		w := &chanWriter{ch: ch}
		passed, err := f(context.Background(), w)
		w.flush()
		ch <- opDoneMsg{kind: kind, id: id, err: err, passed: passed}
		close(ch)
	}()
	return tea.Batch(waitOp(ch), m.spin.Tick)
}

func (m *model) cmdStart() tea.Cmd {
	ex := m.cur
	return m.startOp("start", "Preparing lab for "+ex.ID, ex.ID, func(ctx context.Context, w io.Writer) (bool, error) {
		return false, m.mgr.Start(ctx, ex, w)
	})
}

func (m *model) cmdCheck() tea.Cmd {
	ex := m.cur
	inExam := m.exam != nil
	return m.startOp("check", "Checking "+ex.ID, ex.ID, func(ctx context.Context, w io.Writer) (bool, error) {
		fmt.Fprintf(w, "Checking %s — %s\n\n", ex.ID, ex.Title)
		passed, err := m.mgr.Check(ctx, ex, w)
		// an exam moves on to its next task on the same VM, so only stop outside one
		if err == nil && passed && !inExam {
			if serr := m.mgr.StopAfterPass(ctx, ex, w); serr != nil {
				fmt.Fprintf(w, "(could not stop the lab VM: %v)\n", serr)
			}
		}
		return passed, err
	})
}

func (m *model) cmdShell(worker bool) tea.Cmd {
	env := lab.EnvFor(m.cur.Profile)
	if !m.mgr.VMRunning(env, worker) {
		m.flash = "the " + env.Title + " is not running — press s to prepare the lab first"
		return nil
	}
	c := m.mgr.ShellCommand(env, worker)
	c.Stdin, c.Stdout, c.Stderr = nil, nil, nil
	where := env.Primary().Hostname
	if worker {
		where = "node01"
	}
	m.flash = "returned from " + where + " shell — press c to check your work"
	return tea.ExecProcess(c, func(err error) tea.Msg { return shellDoneMsg{err} })
}

func (m *model) cmdEnvStatus() tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		st := envStatusMsg{status: map[exercise.Profile]string{}, vms: map[string]lab.VMInfo{}}
		for _, env := range lab.AllEnvs() {
			st.status[env.Profile] = m.mgr.EnvStatus(ctx, env)
		}
		if vms, err := m.mgr.VMs(ctx); err == nil {
			for _, v := range vms {
				st.vms[v.Name] = v
			}
		}
		return st
	}
}

func (m *model) envAction(action string) tea.Cmd {
	envs := lab.AllEnvs()
	if m.envCursor < 0 || m.envCursor >= len(envs) {
		m.envCursor = 0
	}
	env := envs[m.envCursor]
	run := func(f func(ctx context.Context, w io.Writer) error) tea.Cmd {
		return m.startOp("env", fmt.Sprintf("%s: %s", env.Title, action), "env", func(ctx context.Context, w io.Writer) (bool, error) {
			return false, f(ctx, w)
		})
	}
	switch action {
	case "up":
		return run(func(ctx context.Context, w io.Writer) error { return m.mgr.Ensure(ctx, env, w) })
	case "stop":
		return run(func(ctx context.Context, w io.Writer) error { return m.mgr.Stop(ctx, env, w) })
	case "reset":
		return run(func(ctx context.Context, w io.Writer) error { return m.mgr.Reset(ctx, env, w) })
	case "destroy":
		return run(func(ctx context.Context, w io.Writer) error { return m.mgr.Destroy(ctx, env, w) })
	case "rebuild":
		return run(func(ctx context.Context, w io.Writer) error { return m.mgr.Rebuild(ctx, env, w) })
	case "stop-all":
		return m.startOp("env", "Stopping all lab VMs", "env", func(ctx context.Context, w io.Writer) (bool, error) {
			return false, m.mgr.StopAll(ctx, w)
		})
	case "delete-all":
		return m.startOp("env", "Deleting all lab VMs", "env", func(ctx context.Context, w io.Writer) (bool, error) {
			return false, m.mgr.DeleteAll(ctx, w)
		})
	}
	return nil
}

// ---- update ----

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		// m.h is the height below the menu bar
		m.w, m.h = msg.Width, max(1, msg.Height-1)
		m.vp.Width, m.vp.Height = m.w-2, max(3, m.h-7)
		m.clampOffset()
		m.refreshViewport(false)
		return m, nil

	case spinner.TickMsg:
		if m.op == "" {
			return m, nil
		}
		var c tea.Cmd
		m.spin, c = m.spin.Update(msg)
		return m, c

	case opLineMsg:
		b := m.out(m.opID)
		b.WriteString(string(msg))
		b.WriteByte('\n')
		if m.showingOutputOf(m.opID) {
			m.refreshViewport(true)
		}
		return m, waitOp(m.opCh)

	case opDoneMsg:
		m.op = ""
		m.refreshActive()
		b := m.out(msg.id)
		switch {
		case msg.err != nil:
			fmt.Fprintf(b, "\n✘ %v\n  (details: %s)\n", msg.err, m.mgr.LogPath())
			m.flash = "failed: " + msg.err.Error()
		case msg.kind == "start":
			m.prog.MarkStarted(msg.id)
			m.flash = "lab ready — press t to open a terminal"
		case msg.kind == "check":
			m.prog.RecordCheck(msg.id, msg.passed)
			if m.exam != nil && m.exam.Record(msg.id, msg.passed) {
				_ = m.exams.Save(m.exam)
			}
			if msg.passed {
				m.result[msg.id] = "passed"
				m.flash = "✔ solved! great work"
			} else {
				m.result[msg.id] = "failed"
				m.flash = "not yet — read the failing checks in Output"
			}
		case msg.kind == "stop":
			m.flash = msg.id + " ended — its VMs are deleted"
		case msg.kind == "cleanup":
			m.flash = "no lab VMs left over"
		case msg.kind == "env":
			if msg.err == nil {
				m.flash = "done"
			}
			return m, m.cmdEnvStatus()
		}
		m.refreshViewport(true)
		return m, nil

	case envStatusMsg:
		m.envStatus, m.vmInfo = msg.status, msg.vms
		return m, nil

	case tickMsg:
		m.refreshActive()
		if m.exam != nil && m.exam.Over() && m.op == "" {
			m.finishExam()
			m.screen = scrExamResult
		}
		return m, tick()

	case stoppedMsg:
		return m, tea.Quit

	case shellDoneMsg:
		if msg.err != nil {
			m.flash = "shell: " + msg.err.Error()
		}
		// bubbletea turns the mouse off while the shell has the terminal and
		// doesn't turn it back on: without this the menus and buttons stop
		// answering clicks
		return m, tea.EnableMouseAllMotion

	case tea.MouseMsg:
		return m.mouse(msg)

	case tea.KeyMsg:
		if msg.String() == "ctrl+c" {
			if m.op != "" || m.quitting { // nothing else gets out while busy
				return m, tea.Quit
			}
			return m, m.quitAndStop()
		}
		if c, ok := m.globalKey(msg.String()); ok {
			return m, c
		}
		switch m.screen {
		case scrBrowse:
			return m.updateBrowse(msg)
		case scrDetail:
			return m.updateDetail(msg)
		case scrEnv:
			return m.updateEnv(msg)
		case scrPaths:
			return m.updatePaths(msg)
		case scrExamSetup:
			return m.updateExamSetup(msg)
		case scrExam:
			return m.updateExam(msg)
		case scrExamResult:
			return m.updateExamResult(msg)
		case scrStarted:
			return m.updateStarted(msg)
		}
	}
	return m, nil
}

func (m *model) move(d int) {
	m.cursor = min(max(0, m.cursor+d), max(0, len(m.visible)-1))
	m.clampOffset()
}

func (m *model) updateBrowse(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.filterOn {
		switch msg.String() {
		case "esc":
			m.filterOn = false
			m.filter.Blur()
			m.filter.SetValue("")
			m.refilter()
			return m, nil
		case "enter", "down", "up":
			m.filterOn = false
			m.filter.Blur()
			return m, nil
		}
		var c tea.Cmd
		m.filter, c = m.filter.Update(msg)
		m.cursor, m.offset = 0, 0
		m.refilter()
		return m, c
	}
	m.flash = ""
	switch msg.String() {
	case "q":
		return m, m.quitAndStop()
	case "/":
		m.filterOn = true
		m.filter.Focus()
		return m, textinput.Blink
	case "esc":
		if m.filter.Value() != "" {
			m.filter.SetValue("")
			m.refilter()
		}
	case "up", "k":
		m.move(-1)
	case "down", "j":
		m.move(1)
	case "pgup", "ctrl+u":
		m.move(-m.listHeight() / 2)
	case "pgdown", "ctrl+d", " ":
		m.move(m.listHeight() / 2)
	case "home", "g":
		m.cursor, m.offset = 0, 0
	case "end", "G":
		m.move(len(m.visible))
	case "n": // next module
		if len(m.visible) > 0 {
			cur := m.visible[m.cursor].Module
			for i := m.cursor; i < len(m.visible); i++ {
				if m.visible[i].Module != cur {
					m.move(i - m.cursor)
					break
				}
			}
		}
	case "p": // previous module
		if len(m.visible) > 0 {
			i := m.cursor
			for i > 0 && m.visible[i-1].Module == m.visible[m.cursor].Module {
				i--
			}
			if i > 0 {
				prev := m.visible[i-1].Module
				for i > 0 && m.visible[i-1].Module == prev {
					i--
				}
			}
			m.move(i - m.cursor)
		}
	case "tab":
		m.trackIdx = (m.trackIdx + 1) % len(m.tracks)
		m.cursor, m.offset = 0, 0
		m.refilter()
	case "u":
		m.setProgFilter((m.progFilter + 1) % progFilterCount)
	case "L":
		m.screen = scrPaths
	case "X":
		if m.exam != nil {
			m.screen = scrExam
		} else {
			m.screen = scrExamSetup
		}
	case "enter", "right", "l":
		if len(m.visible) > 0 {
			m.openDetail(m.visible[m.cursor], scrBrowse)
		}
	}
	return m, nil
}

// openDetail opens ex on the exercise screen; back is the screen it returns
// to, and the list there is the one n/N walk.
func (m *model) openDetail(ex *exercise.Exercise, back screen) {
	m.cur = ex
	m.back = back
	m.screen = scrDetail
	m.tab = tabScenario
	m.solShow = m.prog.Get(ex.ID).SawSolution
	m.hintsShow = m.prog.Get(ex.ID).HintsUsed
	if m.examLocked(ex) {
		m.solShow, m.hintsShow = false, 0
	}
	m.flash = ""
	m.refreshViewport(false)
	m.vp.GotoTop()
}

func (m *model) setTab(t tab) {
	m.tab = t
	m.refreshViewport(false)
	m.vp.GotoTop()
}

func (m *model) updateDetail(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	ex := m.cur
	k := msg.String()
	switch k {
	case "esc", "q":
		m.flash = ""
		return m, m.leave(func() tea.Cmd { m.goTo(m.back); return nil })
	case "tab", "right", "l":
		m.setTab((m.tab + 1) % 5)
	case "shift+tab", "left":
		m.setTab((m.tab + 4) % 5)
	case "1", "2", "3", "4", "5":
		t := tab(k[0] - '1')
		if t == tabSolution && !m.solShow {
			return m, m.askSolution()
		}
		m.setTab(t)
	case "s":
		if m.op != "" {
			m.flash = "busy: " + m.op
			return m, nil
		}
		if m.hasActive && m.active.ID != ex.ID && m.exam == nil {
			m.ask("You're working on "+m.active.ID+" — starting "+ex.ID+" replaces it and resets the lab. Continue?", m.cmdStart, nil)
			return m, nil
		}
		if m.wasStarted(ex) {
			m.ask("Reset the lab and set this exercise up from scratch?", m.cmdStart, nil)
			return m, nil
		}
		return m, m.cmdStart()
	case "t", "w", "c":
		switch {
		case m.op != "":
			m.flash = "wait for: " + m.op
		case k == "w" && ex.Profile != exercise.ProfileK8s:
			m.flash = "node01 exists only in the Kubernetes lab"
		case !m.isActive(ex):
			// every start resets the environment, so the VMs hold another
			// exercise's lab, or none: a check would grade the wrong thing
			m.flash = "the lab isn't set up for " + ex.ID + " — press s to start it"
		case k == "c":
			return m, m.cmdCheck()
		default:
			return m, m.cmdShell(k == "w")
		}
		return m, nil
	case "h", "H":
		return m, m.revealHint()
	case "v":
		return m, m.askSolution()
	case "R":
		m.askForget(ex, "Forget all progress on this exercise, including solved status?", func() {
			m.solShow, m.hintsShow = false, 0
			m.flash = "progress reset"
			m.setTab(tabScenario)
		})
		return m, nil
	case "n", "N":
		d := 1
		if k == "N" {
			d = -1
		}
		return m, m.step(d)
	default:
		var c tea.Cmd
		m.vp, c = m.vp.Update(msg)
		return m, c
	}
	return m, nil
}

// wasStarted reports whether a lab was ever set up for ex.
func (m *model) wasStarted(ex *exercise.Exercise) bool {
	return ex != nil && (!m.prog.Get(ex.ID).Started.IsZero() || m.result[ex.ID] != "")
}

// hasProgress reports whether anything is recorded for ex.
func (m *model) hasProgress(ex *exercise.Exercise) bool {
	return ex != nil && (!m.prog.Get(ex.ID).IsZero() || m.result[ex.ID] != "")
}

// ask shows a Yes/No dialog; yes or no (either may be nil) runs on the answer.
func (m *model) ask(q string, yes, no func() tea.Cmd) {
	m.confirm, m.confirmYes = q, false
	m.onYes, m.onNo = yes, no
}

// goTo shows screen s, or the exercise list when s has nothing left to show.
func (m *model) goTo(s screen) {
	switch {
	case s == scrStarted:
		m.openStarted()
		return
	case s == scrDetail && m.cur == nil, s == scrExam && m.exam == nil, s == scrExamResult && m.finished == nil:
		s = scrBrowse
	}
	m.screen = s
	if s == scrDetail { // its output may have grown while it was away
		m.refreshViewport(false)
	}
}

// leave runs then, which moves off the exercise screen. Every way off it goes
// through here. Leaving the exercise set up on the VMs always ends it: it is
// cleared and its VMs are deleted. Inside an exam the VMs carry on to the next
// question, so they are left alone.
func (m *model) leave(then func() tea.Cmd) tea.Cmd {
	ex := m.cur
	if m.screen != scrDetail || m.exam != nil || !m.isActive(ex) {
		return then()
	}
	if m.op != "" {
		c := then()
		m.flash = "lab left running — busy: " + m.op
		return c
	}
	// start the delete first: then may open another exercise, whose Output tab
	// the delete must not take over
	end := m.startOp("stop", "Ending "+ex.ID+" and deleting its VMs", ex.ID, func(ctx context.Context, w io.Writer) (bool, error) {
		return false, m.endLab(ctx, ex, w)
	})
	return tea.Batch(end, then())
}

// siblings is the list the open exercise was picked from, which n/N walk, and
// a pointer to that list's cursor (nil for the whole catalog). Exam lists keep
// the questions' positions, with nil for one missing from the catalog.
func (m *model) siblings() ([]*exercise.Exercise, *int) {
	questions := func(e *exam.Exam) []*exercise.Exercise {
		out := make([]*exercise.Exercise, len(e.Questions))
		for i, q := range e.Questions {
			out[i] = m.cat.Get(q.ID)
		}
		return out
	}
	var list []*exercise.Exercise
	var cur *int
	switch m.back {
	case scrBrowse:
		list, cur = m.visible, &m.cursor
	case scrStarted:
		list, cur = m.started, &m.startedCursor
	case scrExam:
		if m.exam != nil {
			list, cur = questions(m.exam), &m.examCursor
		}
	case scrExamResult:
		if m.finished != nil {
			list, cur = questions(m.finished), &m.examCursor
		}
	}
	for _, e := range list {
		if e == m.cur {
			return list, cur
		}
	}
	return m.cat.Exercises, nil
}

// siblingNoun names what n/N move between.
func (m *model) siblingNoun() string {
	if m.back == scrExam || m.back == scrExamResult {
		return "question"
	}
	return "exercise"
}

// step opens the next (d=1) or previous (d=-1) exercise of the list the open
// one came from, and moves that list's cursor along.
func (m *model) step(d int) tea.Cmd {
	list, cur := m.siblings()
	i := -1
	for j, e := range list {
		if e == m.cur {
			i = j
		}
	}
	for j := i + d; i >= 0 && j >= 0 && j < len(list); j += d {
		if list[j] == nil {
			continue
		}
		next, at := list[j], j
		return m.leave(func() tea.Cmd {
			if cur != nil {
				*cur = at
			}
			if cur == &m.cursor {
				m.clampOffset()
			}
			m.openDetail(next, m.back)
			return nil
		})
	}
	where := "next"
	if d < 0 {
		where = "previous"
	}
	m.flash = "no " + where + " " + m.siblingNoun() + " in this list"
	return nil
}

func (m *model) revealHint() tea.Cmd {
	ex := m.cur
	if m.examLocked(ex) {
		m.flash = "hints are locked during this exam"
		return nil
	}
	if len(ex.Hints) == 0 {
		m.flash = "this exercise has no hints — try the solution"
		return nil
	}
	if m.hintsShow < len(ex.Hints) {
		m.hintsShow++
		m.prog.UseHint(ex.ID, m.hintsShow)
	}
	m.tab = tabHints
	m.refreshViewport(false)
	m.vp.GotoBottom()
	return nil
}

func (m *model) askSolution() tea.Cmd {
	if m.examLocked(m.cur) {
		m.flash = "the solution is locked during this exam"
		return nil
	}
	if m.solShow {
		m.setTab(tabSolution)
		return nil
	}
	m.ask("Reveal the solution? Try the hints first!", func() tea.Cmd {
		m.solShow = true
		m.prog.SawSolution(m.cur.ID)
		m.setTab(tabSolution)
		return nil
	}, nil)
	return nil
}

// forgetBlocked says why the progress of ex can't be reset now, or "".
func (m *model) forgetBlocked(ex *exercise.Exercise) string {
	switch {
	case m.op != "":
		return "busy: " + m.op
	case m.inExam(ex):
		return "progress can't be reset during an exam on this question"
	case !m.hasProgress(ex):
		return "no progress recorded for this exercise"
	}
	return ""
}

// askForget offers to erase the recorded progress of ex; done runs after.
func (m *model) askForget(ex *exercise.Exercise, q string, done func()) {
	if why := m.forgetBlocked(ex); why != "" {
		m.flash = why
		return
	}
	m.ask(q, func() tea.Cmd {
		m.prog.Reset(ex.ID)
		delete(m.result, ex.ID)
		done()
		return nil
	}, nil)
}

// startedLabs returns the active exercise first, then the others started but
// not yet solved, in catalog order.
func (m *model) startedLabs() []*exercise.Exercise {
	var out []*exercise.Exercise
	if ex := m.cat.Get(m.active.ID); m.hasActive && ex != nil {
		out = append(out, ex)
	}
	for _, e := range m.cat.Exercises {
		p := m.prog.Get(e.ID)
		if !p.Started.IsZero() && p.Solved.IsZero() && !m.isActive(e) {
			out = append(out, e)
		}
	}
	return out
}

func (m *model) openStarted() {
	m.refreshActive()
	m.started = m.startedLabs()
	if m.startedCursor >= len(m.started) {
		m.startedCursor = max(0, len(m.started)-1)
	}
	m.screen = scrStarted
	m.flash = ""
}

func (m *model) resetStarted() {
	m.started = m.startedLabs()
	if m.startedCursor >= len(m.started) {
		m.startedCursor = max(0, len(m.started)-1)
	}
}

func (m *model) updateStarted(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "q", "S":
		m.screen = scrBrowse
	case "up", "k":
		if m.startedCursor > 0 {
			m.startedCursor--
		}
	case "down", "j":
		if m.startedCursor < len(m.started)-1 {
			m.startedCursor++
		}
	case "enter":
		if m.startedCursor < len(m.started) {
			m.openDetail(m.started[m.startedCursor], scrStarted)
		}
	case "r":
		if m.startedCursor >= len(m.started) {
			return m, nil
		}
		ex := m.started[m.startedCursor]
		m.askForget(ex, "Reset progress on "+ex.ID+"?", func() {
			m.resetStarted()
			m.flash = "reset " + ex.ID
		})
	case "A":
		if len(m.started) == 0 {
			return m, nil
		}
		if m.op != "" {
			m.flash = "busy: " + m.op
			return m, nil
		}
		m.ask(fmt.Sprintf("Reset progress on all %d started labs?", len(m.started)), func() tea.Cmd {
			for _, e := range m.started {
				if m.inExam(e) {
					continue
				}
				m.prog.Reset(e.ID)
				delete(m.result, e.ID)
			}
			m.resetStarted()
			m.flash = "reset started labs"
			return nil
		}, nil)
	}
	return m, nil
}

func (m *model) updateEnv(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	k := msg.String()
	// the VM actions wait for the running one: refuse before asking
	if m.op != "" && strings.Contains("uxRBDXK", k) && len(k) == 1 {
		m.flash = "busy: " + m.op
		return m, nil
	}
	switch k {
	case "esc", "q", "e":
		m.goTo(m.envBack)
	case "up", "k":
		if m.envCursor > 0 {
			m.envCursor--
		}
	case "down", "j":
		if m.envCursor < len(lab.AllEnvs())-1 {
			m.envCursor++
		}
	case "r":
		return m, m.cmdEnvStatus()
	case "u":
		return m, m.envAction("up")
	case "x":
		return m, m.envAction("stop")
	case "R":
		return m, m.envAction("reset")
	case "D":
		m.ask("Delete the VMs of this environment?", func() tea.Cmd { return m.envAction("destroy") }, nil)
	case "B":
		m.ask("Destroy and rebuild this environment from scratch (takes minutes)?", func() tea.Cmd { return m.envAction("rebuild") }, nil)
	case "X":
		return m, m.envAction("stop-all")
	case "K":
		m.ask("Delete ALL lab VMs (every environment)? They are re-created on the next start.", func() tea.Cmd { return m.envAction("delete-all") }, nil)
	}
	return m, nil
}

func (m *model) showingOutputOf(id string) bool {
	if m.screen == scrDetail && m.tab == tabOutput && m.cur != nil && m.cur.ID == id {
		return true
	}
	return m.screen == scrEnv && id == "env"
}
