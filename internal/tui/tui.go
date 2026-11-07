// Package tui is the interactive exercise browser.
package tui

import (
	"context"
	"fmt"
	"io"
	"strings"
	"sync"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"kubelab/internal/exercise"
	"kubelab/internal/lab"
	"kubelab/internal/progress"
)

type screen int

const (
	scrBrowse screen = iota
	scrDetail
	scrEnv
)

type tab int

const (
	tabScenario tab = iota
	tabHints
	tabSolution
	tabOutput
)

var tabNames = []string{"Scenario", "Hints", "Solution", "Output"}

// ---- messages ----

type opLineMsg string
type opDoneMsg struct {
	kind   string
	id     string
	err    error
	passed bool
}
type envStatusMsg map[exercise.Profile]string
type shellDoneMsg struct{ err error }

// ---- model ----

type model struct {
	cat  *exercise.Catalog
	mgr  *lab.Manager
	prog *progress.Store

	w, h   int
	screen screen

	// browse
	tracks   []string // "" = all
	trackIdx int
	unsolved bool
	filter   textinput.Model
	filterOn bool
	visible  []*exercise.Exercise
	cursor   int
	offset   int

	// detail
	cur       *exercise.Exercise
	tab       tab
	vp        viewport.Model
	hintsShow int
	solShow   bool
	confirm   string // pending confirmation question
	onYes     func() tea.Cmd

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
	envCursor int

	style   string
	mdMu    sync.Mutex
	mdCache map[string]string
}

// Run starts the TUI.
func Run(cat *exercise.Catalog, mgr *lab.Manager, prog *progress.Store) error {
	style := "dark"
	if !lipgloss.HasDarkBackground() {
		style = "light"
	}
	ti := textinput.New()
	ti.Placeholder = "type to filter (id, title, tag, module)…"
	ti.Prompt = "/ "
	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = lipgloss.NewStyle().Foreground(colAccent)
	m := &model{
		cat: cat, mgr: mgr, prog: prog,
		tracks: []string{"", "docker", "kubernetes"},
		filter: ti, spin: sp, style: style,
		output: map[string]*strings.Builder{}, result: map[string]string{},
		mdCache: map[string]string{}, envStatus: map[exercise.Profile]string{},
	}
	m.refilter()
	_, err := tea.NewProgram(m, tea.WithAltScreen(), tea.WithMouseCellMotion()).Run()
	return err
}

func (m *model) Init() tea.Cmd { return m.spin.Tick }

// ---- filtering ----

func (m *model) refilter() {
	q := strings.ToLower(strings.TrimSpace(m.filter.Value()))
	track := m.tracks[m.trackIdx]
	m.visible = m.visible[:0]
	for _, e := range m.cat.Exercises {
		if track != "" && e.Track != track {
			continue
		}
		if m.unsolved && !m.prog.Get(e.ID).Solved.IsZero() {
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

func (m *model) listHeight() int { return max(3, m.h-7) }

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
	return m.startOp("check", "Checking "+ex.ID, ex.ID, func(ctx context.Context, w io.Writer) (bool, error) {
		fmt.Fprintf(w, "Checking %s — %s\n\n", ex.ID, ex.Title)
		return m.mgr.Check(ctx, ex, w)
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
		return envStatusMsg{
			exercise.ProfileDocker: m.mgr.EnvStatus(ctx, lab.DockerEnv),
			exercise.ProfileK8s:    m.mgr.EnvStatus(ctx, lab.K8sEnv),
		}
	}
}

func (m *model) envAction(action string) tea.Cmd {
	env := lab.DockerEnv
	if m.envCursor == 1 {
		env = lab.K8sEnv
	}
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
	}
	return nil
}

// ---- update ----

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.w, m.h = msg.Width, msg.Height
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
			if msg.passed {
				m.result[msg.id] = "passed"
				m.flash = "✔ solved! great work"
			} else {
				m.result[msg.id] = "failed"
				m.flash = "not yet — read the failing checks in Output"
			}
		case msg.kind == "env":
			m.flash = "done"
			return m, m.cmdEnvStatus()
		}
		m.refreshViewport(true)
		return m, nil

	case envStatusMsg:
		m.envStatus = msg
		return m, nil

	case shellDoneMsg:
		if msg.err != nil {
			m.flash = "shell: " + msg.err.Error()
		}
		return m, nil

	case tea.MouseMsg:
		if m.screen == scrDetail {
			var c tea.Cmd
			m.vp, c = m.vp.Update(msg)
			return m, c
		}
		if msg.Button == tea.MouseButtonWheelUp {
			m.move(-3)
		} else if msg.Button == tea.MouseButtonWheelDown {
			m.move(3)
		}
		return m, nil

	case tea.KeyMsg:
		if m.confirm != "" {
			k := msg.String()
			m.confirm = ""
			if k == "y" || k == "Y" {
				f := m.onYes
				m.onYes = nil
				return m, f()
			}
			return m, nil
		}
		if msg.String() == "ctrl+c" {
			return m, tea.Quit
		}
		switch m.screen {
		case scrBrowse:
			return m.updateBrowse(msg)
		case scrDetail:
			return m.updateDetail(msg)
		case scrEnv:
			return m.updateEnv(msg)
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
		return m, tea.Quit
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
		m.unsolved = !m.unsolved
		m.refilter()
	case "e":
		m.screen = scrEnv
		return m, m.cmdEnvStatus()
	case "enter", "right", "l":
		if len(m.visible) > 0 {
			m.openDetail(m.visible[m.cursor])
		}
	}
	return m, nil
}

func (m *model) openDetail(ex *exercise.Exercise) {
	m.cur = ex
	m.screen = scrDetail
	m.tab = tabScenario
	m.solShow = m.prog.Get(ex.ID).SawSolution
	m.hintsShow = m.prog.Get(ex.ID).HintsUsed
	m.flash = ""
	m.refreshViewport(false)
	m.vp.GotoTop()
}

func (m *model) updateDetail(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	ex := m.cur
	k := msg.String()
	switch k {
	case "esc", "q", "backspace", "left":
		m.screen = scrBrowse
		m.flash = ""
		return m, nil
	case "tab", "right", "l":
		m.tab = (m.tab + 1) % 4
		m.refreshViewport(false)
		m.vp.GotoTop()
	case "shift+tab":
		m.tab = (m.tab + 3) % 4
		m.refreshViewport(false)
		m.vp.GotoTop()
	case "1", "2", "3", "4":
		m.tab = tab(k[0] - '1')
		if m.tab == tabSolution && !m.solShow {
			return m, m.askSolution()
		}
		m.refreshViewport(false)
		m.vp.GotoTop()
	case "s":
		if m.op != "" {
			m.flash = "busy: " + m.op
			return m, nil
		}
		if !m.prog.Get(ex.ID).Started.IsZero() || m.result[ex.ID] != "" {
			m.confirm = "Reset the lab and set this exercise up from scratch? (y/n)"
			m.onYes = m.cmdStart
			return m, nil
		}
		return m, m.cmdStart()
	case "t":
		if m.op != "" {
			m.flash = "wait for: " + m.op
			return m, nil
		}
		return m, m.cmdShell(false)
	case "w":
		if ex.Profile != exercise.ProfileK8s {
			m.flash = "node01 exists only in the Kubernetes lab"
			return m, nil
		}
		return m, m.cmdShell(true)
	case "c":
		return m, m.cmdCheck()
	case "h", "H":
		return m, m.revealHint()
	case "v":
		return m, m.askSolution()
	case "n", "N":
		// next / previous exercise
		for i, e := range m.cat.Exercises {
			if e == ex {
				j := i + 1
				if k == "N" {
					j = i - 1
				}
				if j >= 0 && j < len(m.cat.Exercises) {
					m.openDetail(m.cat.Exercises[j])
				}
				break
			}
		}
	default:
		var c tea.Cmd
		m.vp, c = m.vp.Update(msg)
		return m, c
	}
	return m, nil
}

func (m *model) revealHint() tea.Cmd {
	ex := m.cur
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
	if m.solShow {
		m.tab = tabSolution
		m.refreshViewport(false)
		m.vp.GotoTop()
		return nil
	}
	m.confirm = "Reveal the solution? Try the hints first! (y/n)"
	m.onYes = func() tea.Cmd {
		m.solShow = true
		m.prog.SawSolution(m.cur.ID)
		m.tab = tabSolution
		m.refreshViewport(false)
		m.vp.GotoTop()
		return nil
	}
	return nil
}

func (m *model) updateEnv(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "q", "e":
		m.screen = scrBrowse
	case "up", "k":
		m.envCursor = 0
	case "down", "j":
		m.envCursor = 1
	case "r":
		return m, m.cmdEnvStatus()
	case "u":
		return m, m.envAction("up")
	case "x":
		return m, m.envAction("stop")
	case "R":
		return m, m.envAction("reset")
	case "D":
		m.confirm = "Delete the VMs of this environment? (y/n)"
		m.onYes = func() tea.Cmd { return m.envAction("destroy") }
	case "B":
		m.confirm = "Destroy and rebuild this environment from scratch (takes minutes)? (y/n)"
		m.onYes = func() tea.Cmd { return m.envAction("rebuild") }
	}
	return m, nil
}

func (m *model) showingOutputOf(id string) bool {
	if m.screen == scrDetail && m.tab == tabOutput && m.cur != nil && m.cur.ID == id {
		return true
	}
	return m.screen == scrEnv && id == "env"
}
