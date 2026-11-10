package tui

// Menu bar, drop-down menus, dialogs and mouse hit-testing.
//
// The top row of the screen is a menu bar (F10 or alt+letter opens a menu, or
// click it). Views register clickable zones while they render; a mouse click
// is matched against the zones of the last frame. Zone ids:
//
//	key:<k>    a button: acts as if key k was pressed on the current screen
//	row:<n>    a list row: click selects it, double-click opens it
//	tab:<n>    a tab of the exercise screen
//	menu:<n>   a menu title in the menu bar
//	mi:<n>     an item of the open menu
//	dlg:yes / dlg:no / dlg:close   dialog buttons

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"awesome_labs/internal/exercise"
)

var (
	colBarBg  = lipgloss.AdaptiveColor{Light: "#d0d7de", Dark: "#30363d"}
	colMenuBg = lipgloss.AdaptiveColor{Light: "#f6f8fa", Dark: "#1c2128"}
	colText   = lipgloss.AdaptiveColor{Light: "#1f2328", Dark: "#e6edf3"}
	colHiFg   = lipgloss.AdaptiveColor{Light: "#ffffff", Dark: "#0d1117"}

	sBar     = lipgloss.NewStyle().Background(colBarBg).Foreground(colText)
	sMenuBox = lipgloss.NewStyle().Border(lipgloss.NormalBorder()).BorderForeground(colMuted).BorderBackground(colMenuBg)
	sButton  = lipgloss.NewStyle().Background(colBarBg).Foreground(colText).Padding(0, 1)
	sBtnHot  = lipgloss.NewStyle().Background(colAccent).Foreground(colHiFg).Bold(true).Padding(0, 1)
)

type zone struct {
	x, y, w, h int
	id         string
}

type menuItem struct {
	label, key string
	note       string // shown in the key column when there is no key
	why        string // why the item is disabled, shown when it is clicked
	sep        bool   // a separator; with a label it heads a group of items
	disabled   bool
	hidden     bool // not offered on this screen
	checked    bool
	run        func() tea.Cmd
}

type menu struct {
	title string
	acc   rune // alt+acc opens it
	items []menuItem
}

func sep() menuItem { return menuItem{sep: true} }

// head is a separator that names the group below it; key is the shortcut that
// cycles through the group.
func head(label, key string) menuItem { return menuItem{sep: true, label: label, key: key} }

// off disables an item with a reason when cond holds.
func (it menuItem) off(cond bool, why string) menuItem {
	if cond && !it.disabled {
		it.disabled, it.why = true, why
	}
	return it
}

// keyMsg builds the key message a real key press would produce.
func keyMsg(k string) tea.KeyMsg {
	switch k {
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEsc}
	case "F1", "f1":
		return tea.KeyMsg{Type: tea.KeyF1}
	case "tab":
		return tea.KeyMsg{Type: tea.KeyTab}
	case "up":
		return tea.KeyMsg{Type: tea.KeyUp}
	case "down":
		return tea.KeyMsg{Type: tea.KeyDown}
	case "left":
		return tea.KeyMsg{Type: tea.KeyLeft}
	case "right":
		return tea.KeyMsg{Type: tea.KeyRight}
	}
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(k)}
}

// press sends key k to the current screen.
func (m *model) press(k string) tea.Cmd {
	_, c := m.Update(keyMsg(k))
	return c
}

// target is the exercise the Exercise menu acts on.
func (m *model) target() *exercise.Exercise {
	switch m.screen {
	case scrDetail:
		return m.cur
	case scrBrowse:
		if len(m.visible) > 0 {
			return m.visible[m.cursor]
		}
	case scrStarted:
		if m.startedCursor < len(m.started) {
			return m.started[m.startedCursor]
		}
	case scrExam:
		if m.exam != nil && m.examCursor < len(m.exam.Questions) {
			return m.cat.Get(m.exam.Questions[m.examCursor].ID)
		}
	case scrExamResult:
		if m.finished != nil && m.examCursor < len(m.finished.Questions) {
			return m.cat.Get(m.finished.Questions[m.examCursor].ID)
		}
	}
	return nil
}

// onExercise runs key k on the exercise screen of the target exercise,
// opening it first when another screen is showing.
func (m *model) onExercise(k string) func() tea.Cmd {
	return func() tea.Cmd {
		if m.screen != scrDetail {
			ex := m.target()
			if ex == nil {
				return nil
			}
			m.openDetail(ex, m.screen)
		}
		_, c := m.updateDetail(keyMsg(k))
		return c
	}
}

// leaveTo moves to screen s, through leave when on the exercise screen.
func (m *model) leaveTo(s screen) func() tea.Cmd {
	return func() tea.Cmd { return m.leave(func() tea.Cmd { m.goTo(s); return nil }) }
}

// openEnv shows the environments screen, which goes back where it was opened
// from: checking on the VMs is not leaving the exercise.
func (m *model) openEnv() tea.Cmd {
	if m.screen != scrEnv {
		m.envBack = m.screen
	}
	m.screen = scrEnv
	return m.cmdEnvStatus()
}

// onEnv presses k on the environments screen.
func (m *model) onEnv(k string) func() tea.Cmd {
	return func() tea.Cmd {
		c := m.openEnv()
		return tea.Batch(m.press(k), c)
	}
}

// menus builds the menu bar for the current screen. Items that only make
// sense on another screen are hidden rather than greyed out, and a menu left
// with nothing to offer is dropped from the bar. Greyed-out items say why
// when clicked.
func (m *model) menus() []menu {
	ex := m.target()
	noEx := ex == nil
	locked := m.examLocked(ex)
	onBrowse, onDetail, onEnv := m.screen == scrBrowse, m.screen == scrDetail, m.screen == scrEnv
	onStarted, onExam, onResult := m.screen == scrStarted, m.screen == scrExam, m.screen == scrExamResult
	const lockedWhy = "locked during this exam"
	const inExamWhy = "progress can't be reset during an exam on this question"

	// a key is shown next to an item only where pressing it does the same thing
	on := func(cond bool, k string) string {
		if cond {
			return k
		}
		return ""
	}
	exKey := func(k string) string { return on(onDetail, k) } // lab keys work on the exercise screen
	press := func(k string) func() tea.Cmd { return func() tea.Cmd { return m.press(k) } }
	// esc leads to the exercise list, except from the exercise and environments
	// screens, which go back where they were opened from
	backKey := on(!onDetail && !onEnv || onDetail && m.back == scrBrowse || onEnv && m.envBack == scrBrowse, "esc")
	openKey := "enter"
	if onResult {
		openKey = "o"
	}
	file := menu{"File", 'f', []menuItem{
		{label: "Exercise list", key: backKey, hidden: onBrowse, run: func() tea.Cmd {
			if backKey != "" { // esc gets there, with whatever it says on the way
				return m.press("esc")
			}
			return m.leaveTo(scrBrowse)()
		}},
		// only when there is one, and it is not the one already open
		{label: "Go to the active exercise", key: "a", hidden: !m.hasActive || onDetail && m.isActive(m.cur),
			run: func() tea.Cmd { m.goActive(); return nil }},
		{label: "Started labs", key: "S", hidden: onStarted, run: m.leaveTo(scrStarted)},
		sep(),
		{label: "Quit", key: on(onBrowse, "q"), run: func() tea.Cmd { return tea.Quit }},
		{label: "Quit and stop all VMs", key: "Q", run: m.quitAndStop},
	}}

	// the Exercise menu acts on the selected or open exercise
	exm := menu{"Exercise", 'e', nil}
	// on the list it stays, greyed out, when the filter leaves nothing selected
	if onBrowse || !noEx && (onDetail || onStarted || onExam || onResult) {
		var hasHints bool
		if ex != nil {
			hasHints = len(ex.Hints) > 0
		}
		active := m.isActive(ex)
		startLabel := "Start the lab"
		switch {
		case active || m.wasStarted(ex):
			startLabel = "Set the lab up again…"
		case m.hasActive && m.exam == nil:
			startLabel = "Start the lab…" // asks first: it replaces the active exercise
		}
		// Terminals and checks need the lab set up, and only the active exercise
		// has it (every start resets the environment, in an exam too). Elsewhere
		// they are offered only for it; on the exercise screen they say why not.
		const notReady = "the lab isn't set up for it — start it first (s)"
		labOnly := !active && !onDetail
		noun := "exercise"
		if onDetail {
			noun = m.siblingNoun()
		}
		exm.items = []menuItem{
			menuItem{label: "Open", key: openKey, hidden: onDetail, run: func() tea.Cmd { m.openDetail(ex, m.screen); return nil }}.
				off(noEx, "nothing is selected"),
			head("Lab", ""),
			menuItem{label: startLabel, key: exKey("s"), run: m.onExercise("s")}.off(noEx, "nothing is selected"),
			menuItem{label: "Open a terminal", key: exKey("t"), hidden: noEx || labOnly, run: m.onExercise("t")}.off(!active, notReady),
			menuItem{label: "Terminal on node01", key: exKey("w"), hidden: noEx || labOnly || ex.Profile != exercise.ProfileK8s, run: m.onExercise("w")}.
				off(!active, notReady),
			menuItem{label: "Check my work", key: exKey("c"), hidden: noEx || labOnly, run: m.onExercise("c")}.off(!active, notReady),
			// hints and the solution belong to the open exercise, not to a list
			{label: "Help with it", sep: true, hidden: !onDetail},
			menuItem{label: "Show the next hint", key: "h", hidden: !onDetail, run: m.onExercise("h")}.
				off(noEx, "nothing is selected").off(locked, "hints are "+lockedWhy).off(!hasHints, "this exercise has no hints"),
			menuItem{label: "Reveal the solution…", key: "v", hidden: !onDetail, run: m.onExercise("v")}.
				off(noEx, "nothing is selected").off(locked, "the solution is "+lockedWhy),
			sep(),
			// nothing to reset on an exercise never tried
			menuItem{label: "Reset my progress…", key: exKey("R"), hidden: onStarted || !m.hasProgress(ex), run: m.onExercise("R")}.
				off(m.inExam(ex), inExamWhy),
			// the started-labs list resets in place, with its own key
			menuItem{label: "Reset my progress…", key: "r", hidden: !onStarted, run: press("r")}.off(m.inExam(ex), inExamWhy),
			menuItem{label: "Reset all started labs…", key: "A", hidden: !onStarted, run: press("A")}.
				off(len(m.started) == 0, "no labs are started"),
			sep(),
			{label: "Next " + noun, key: "n", hidden: !onDetail, run: m.onExercise("n")},
			{label: "Previous " + noun, key: "N", hidden: !onDetail, run: m.onExercise("N")},
		}
	}

	// View shapes the exercise list, or picks the tab of the exercise screen
	view := menu{"View", 'v', nil}
	tracks := menu{title: "Tracks", acc: 't'}
	switch {
	case onDetail:
		view.items = []menuItem{head("Tab", "tab")}
		for i, name := range tabNames {
			it := menuItem{label: name, key: fmt.Sprint(i + 1), checked: m.tab == tab(i), run: press(fmt.Sprint(i + 1))}
			if tab(i) == tabSolution {
				it = it.off(locked, "the solution is "+lockedWhy)
			}
			view.items = append(view.items, it)
		}
	case onBrowse:
		view.items = []menuItem{
			{label: "Filter…", key: "/", run: func() tea.Cmd {
				m.filterOn = true
				m.filter.Focus()
				return textinput.Blink
			}},
			{label: "Clear the filter", key: "esc", hidden: m.filter.Value() == "", run: func() tea.Cmd {
				m.filter.SetValue("")
				m.refilter()
				return nil
			}},
			head("Show", "u"),
		}
		for i, name := range progFilterNames {
			f := progFilter(i)
			view.items = append(view.items, menuItem{label: name, checked: m.progFilter == f,
				run: func() tea.Cmd { m.setProgFilter(f); return nil }})
		}
		view.items = append(view.items,
			head("Jump", ""),
			menuItem{label: "Next module", key: "n", run: press("n")},
			menuItem{label: "Previous module", key: "p", run: press("p")},
			head("Learning path", ""),
			menuItem{label: "Choose a learning path…", key: "L", run: func() tea.Cmd { m.screen = scrPaths; return nil }},
			menuItem{label: "Clear the learning path", hidden: m.path == nil, run: func() tea.Cmd {
				m.path = nil
				m.cursor, m.offset = 0, 0
				m.refilter()
				return nil
			}},
		)

		tracks.items = []menuItem{head("Track", "tab")}
		var counts map[string]string
		if m.menuOpen >= 0 { // shown only in the open menu
			counts = m.trackCounts()
		}
		for i, t := range m.tracks {
			tracks.items = append(tracks.items, menuItem{label: trackName(t), note: counts[t], checked: i == m.trackIdx, run: func() tea.Cmd {
				m.trackIdx, m.cursor, m.offset = i, 0, 0
				m.refilter()
				return nil
			}})
		}
	}

	envs := menu{"VMs", 'm', []menuItem{
		{label: "Lab environments & VMs", key: "e", hidden: onEnv, run: m.openEnv},
		{label: "Refresh status", key: "r", hidden: !onEnv, run: press("r")},
		head("Selected environment", ""),
		{label: "Start / create", key: "u", hidden: !onEnv, run: press("u")},
		{label: "Stop", key: "x", hidden: !onEnv, run: press("x")},
		{label: "Reset VMs", key: "R", hidden: !onEnv, run: press("R")},
		{label: "Rebuild…", key: "B", hidden: !onEnv, run: press("B")},
		{label: "Delete…", key: "D", hidden: !onEnv, run: press("D")},
		head("All environments", ""),
		{label: "Stop all lab VMs", key: on(onEnv, "X"), run: m.onEnv("X")},
		{label: "Delete all lab VMs…", key: on(onEnv, "K"), run: m.onEnv("K")},
	}}

	examKey := on(onBrowse, "X") // X opens the exam from the list
	exams := menu{"Exam", 'x', []menuItem{
		{label: "New exam…", key: examKey, hidden: m.exam != nil || m.screen == scrExamSetup, run: m.leaveTo(scrExamSetup)},
		{label: "Return to the exam", key: examKey, hidden: m.exam == nil || onExam, run: m.leaveTo(scrExam)},
		{label: "Finish the exam…", key: on(onExam, "F"), hidden: m.exam == nil, run: func() tea.Cmd {
			return m.leave(func() tea.Cmd { m.screen = scrExam; return m.press("F") })
		}},
		sep(),
		{label: "Last result", hidden: onResult || m.finished == nil, run: m.leaveTo(scrExamResult)},
	}}

	help := menu{"Help", 'h', []menuItem{
		{label: "Keyboard shortcuts", key: "F1", run: func() tea.Cmd { m.modal = shortcutsText; return nil }},
		{label: "Using the mouse", run: func() tea.Cmd { m.modal = mouseText; return nil }},
		sep(),
		{label: "About awesome_labs", run: func() tea.Cmd { m.modal = aboutText; return nil }},
	}}

	var out []menu
	for _, mn := range []menu{file, exm, view, tracks, envs, exams, help} {
		if mn.items = visibleItems(mn.items); len(mn.items) > 0 {
			out = append(out, mn)
		}
	}
	return out
}

// trackCounts returns "solved/total" for every track ("" is all of them).
func (m *model) trackCounts() map[string]string {
	out := map[string]string{}
	for t, l := range m.trackIDs {
		s, _ := m.prog.Counts(l)
		out[t] = fmt.Sprintf("%d/%d", s, len(l))
	}
	return out
}

// trackName is the full name of a track, for places with room for it.
func trackName(t string) string {
	switch t {
	case "":
		return "All tracks"
	case "offsec":
		return "Offensive Security"
	case "defensive":
		return "Defensive Security"
	}
	return trackLabel(t)
}

// visibleItems drops hidden items and the separators left at the ends or
// doubled up by them. A group heading may open the menu; a separator or heading
// with nothing under it before the next one is dropped.
func visibleItems(items []menuItem) []menuItem {
	var out []menuItem
	for _, it := range items {
		if it.hidden {
			continue
		}
		if it.sep {
			if len(out) > 0 && out[len(out)-1].sep {
				out = out[:len(out)-1]
			}
			if len(out) == 0 && it.label == "" {
				continue
			}
		}
		out = append(out, it)
	}
	for len(out) > 0 && out[len(out)-1].sep {
		out = out[:len(out)-1]
	}
	return out
}

func (m *model) quitAndStop() tea.Cmd {
	if m.op != "" {
		m.flash = "busy: " + m.op
		return nil
	}
	m.quitting = true
	m.flash = "stopping all lab VMs…"
	return func() tea.Msg { return stoppedMsg{m.mgr.StopAll(context.Background(), io.Discard)} }
}

// goActive opens the exercise set up on the VMs. It goes back to the exam
// when it is a question of it, to the started labs when opened from there,
// and to the exercise list otherwise.
func (m *model) goActive() {
	ex := m.cat.Get(m.active.ID)
	switch {
	case !m.hasActive || ex == nil:
		m.flash = "no exercise is set up on the VMs — open one and press s to start it (S lists labs started earlier)"
		return
	case m.screen == scrDetail && m.cur == ex:
		return // already open
	}
	back := scrBrowse
	switch {
	case m.inExam(ex):
		back = scrExam
		for i, q := range m.exam.Questions {
			if q.ID == ex.ID {
				m.examCursor = i
			}
		}
	case m.screen == scrStarted:
		back = scrStarted
	}
	m.openDetail(ex, back)
}

// ---- menu state ----

// The highlighted item is kept by its label, not its position: menus are
// rebuilt on every frame and items come and go while one is open (a start
// finishing, the active exercise changing), which would move a position onto
// another item.

func (m *model) openMenu(i int) {
	m.menuOpen, m.menuSel = i, ""
	m.menuStep(1)
}

func (m *model) closeMenu() { m.menuOpen = -1 }

// selected is the index of the highlighted item in items, or -1.
func (m *model) selected(items []menuItem) int {
	return itemIndex(items, m.menuSel)
}

// itemIndex is the index of the item labelled label, or -1.
func itemIndex(items []menuItem, label string) int {
	if label == "" {
		return -1
	}
	for i, it := range items {
		if !it.sep && it.label == label {
			return i
		}
	}
	return -1
}

// menuStep moves the highlight to the next enabled item in direction d, from
// the top (or bottom) when nothing is highlighted.
func (m *model) menuStep(d int) {
	items := m.menus()[m.menuOpen].items
	n := len(items)
	i := m.selected(items)
	if i < 0 && d < 0 {
		i = n
	}
	for range items {
		i = ((i+d)%n + n) % n
		if !items[i].sep && !items[i].disabled {
			m.menuSel = items[i].label
			return
		}
	}
}

func (m *model) runMenuItem(i int) tea.Cmd {
	items := m.menus()[m.menuOpen].items
	m.closeMenu()
	if i < 0 || i >= len(items) || items[i].sep {
		return nil
	}
	if items[i].disabled {
		m.flash = items[i].label + ": " + items[i].why
		return nil
	}
	m.flash = ""
	return items[i].run()
}

// menuShortcut is the item of the open menu whose key is k, or -1. Keys that
// move around the menu don't count.
func (m *model) menuShortcut(k string) int {
	fkey := len(k) > 1 && k[0] == 'f' // F1 and the like
	if len([]rune(k)) != 1 && !fkey {
		return -1
	}
	for i, it := range m.menus()[m.menuOpen].items {
		if !it.sep && it.key != "" && (it.key == k || fkey && strings.EqualFold(it.key, k)) {
			return i
		}
	}
	return -1
}

func (m *model) menuKey(k string) tea.Cmd {
	n := len(m.menus())
	switch k {
	case "esc", "f10", "alt":
		m.closeMenu()
	case "left":
		m.openMenu((m.menuOpen + n - 1) % n)
	case "right", "tab":
		m.openMenu((m.menuOpen + 1) % n)
	case "up":
		m.menuStep(-1)
	case "down":
		m.menuStep(1)
	case "home", "pgup":
		m.menuSel = ""
		m.menuStep(1)
	case "end", "pgdown":
		m.menuSel = ""
		m.menuStep(-1)
	case "enter", " ":
		if i := m.selected(m.menus()[m.menuOpen].items); i >= 0 {
			return m.runMenuItem(i)
		}
	default:
		if i := m.menuForAlt(k); i >= 0 {
			m.openMenu(i)
		} else if i := m.menuShortcut(k); i >= 0 {
			return m.runMenuItem(i) // an item's own key picks it
		}
	}
	return nil
}

func (m *model) menuForAlt(k string) int {
	if !strings.HasPrefix(k, "alt+") || len(k) != 5 {
		return -1
	}
	for i, mn := range m.menus() {
		if rune(k[4]) == mn.acc {
			return i
		}
	}
	return -1
}

// globalKey handles keys that work on every screen. ok is false when the key
// should go to the screen.
func (m *model) globalKey(k string) (tea.Cmd, bool) {
	if m.modal != "" {
		if k == "esc" || k == "enter" || k == "q" || k == "f1" || k == " " {
			m.modal = ""
		}
		return nil, true
	}
	if m.confirm != "" {
		switch k {
		case "y", "Y":
			return m.answer(true), true
		case "n", "N", "esc", "q":
			return m.answer(false), true
		case "left", "right", "tab", "shift+tab", "h", "l":
			m.confirmYes = !m.confirmYes
		case "enter", " ":
			return m.answer(m.confirmYes), true
		}
		return nil, true
	}
	if m.menuOpen >= 0 {
		return m.menuKey(k), true
	}
	switch k {
	case "f10":
		m.openMenu(0)
		return nil, true
	case "f1":
		m.modal = shortcutsText
		return nil, true
	}
	if i := m.menuForAlt(k); i >= 0 {
		m.openMenu(i)
		return nil, true
	}
	return m.navKey(k)
}

// navKey handles the File and VMs menu keys that work on every screen: a
// (active exercise), S (started labs), e (environments) and Q (quit and stop
// the VMs). Not while typing a filter, and not on a screen that gives the key
// its own meaning (S and e lead back from their own screens). These are the
// screen handlers' only copies of these keys.
func (m *model) navKey(k string) (tea.Cmd, bool) {
	if m.filterOn && m.screen == scrBrowse {
		return nil, false
	}
	switch {
	case k == "a":
		m.goActive()
	case k == "S" && m.screen != scrStarted:
		return m.leaveTo(scrStarted)(), true
	case k == "e" && m.screen != scrEnv:
		return m.openEnv(), true
	case k == "Q":
		return m.quitAndStop(), true
	default:
		return nil, false
	}
	return nil, true
}

func (m *model) answer(yes bool) tea.Cmd {
	f := m.onNo
	if yes {
		f = m.onYes
	}
	m.confirm, m.confirmYes, m.onYes, m.onNo = "", false, nil, nil
	if f != nil {
		return f()
	}
	return nil
}

// ---- mouse ----

func (m *model) zoneAt(x, y int) string {
	for i := len(m.zones) - 1; i >= 0; i-- {
		z := m.zones[i]
		if x >= z.x && x < z.x+z.w && y >= z.y && y < z.y+z.h {
			return z.id
		}
	}
	return ""
}

// addZone registers a clickable area in body coordinates (below the menu bar).
func (m *model) addZone(id string, x, y, w, h int) {
	m.zones = append(m.zones, zone{x, y + 1, w, h, id})
}

func (m *model) mouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	id := m.zoneAt(msg.X, msg.Y)
	press := msg.Action == tea.MouseActionPress && msg.Button == tea.MouseButtonLeft
	wheel := msg.Button == tea.MouseButtonWheelUp || msg.Button == tea.MouseButtonWheelDown

	// hovering moves the highlight in an open menu, or switches menus
	if msg.Action == tea.MouseActionMotion {
		if m.menuOpen >= 0 {
			var n int
			if label, ok := strings.CutPrefix(id, "mi:"); ok {
				m.menuSel = label
			} else if sc(id, "menu:", &n) && n != m.menuOpen {
				m.openMenu(n)
			}
		}
		m.hover = id
		return m, nil
	}

	if m.modal != "" {
		if press {
			m.modal = ""
		}
		return m, nil
	}
	if m.confirm != "" {
		if press {
			switch id {
			case "dlg:yes":
				return m, m.answer(true)
			case "dlg:no":
				return m, m.answer(false)
			}
		}
		return m, nil
	}
	if m.menuOpen >= 0 {
		if !press {
			return m, nil
		}
		var n int
		switch {
		case strings.HasPrefix(id, "mi:"):
			i := itemIndex(m.menus()[m.menuOpen].items, strings.TrimPrefix(id, "mi:"))
			if i < 0 { // the item went away since the frame was drawn
				return m, nil
			}
			return m, m.runMenuItem(i)
		case sc(id, "menu:", &n) && n != m.menuOpen:
			m.openMenu(n)
		default:
			m.closeMenu()
		}
		return m, nil
	}

	if wheel {
		up := msg.Button == tea.MouseButtonWheelUp
		switch m.screen {
		case scrDetail:
			var c tea.Cmd
			m.vp, c = m.vp.Update(msg)
			return m, c
		case scrBrowse:
			if up {
				m.move(-3)
			} else {
				m.move(3)
			}
			return m, nil
		}
		if up {
			return m, m.press("up")
		}
		return m, m.press("down")
	}
	if !press {
		return m, nil
	}

	double := id != "" && id == m.lastClick && time.Since(m.lastClickAt) < 450*time.Millisecond
	m.lastClick, m.lastClickAt = id, time.Now()
	if double {
		m.lastClick = "" // a third click starts over
	}

	var n int
	switch {
	case sc(id, "menu:", &n):
		m.openMenu(n)
	case strings.HasPrefix(id, "key:"):
		m.flash = ""
		return m, m.press(strings.TrimPrefix(id, "key:"))
	case sc(id, "tab:", &n):
		return m, m.press(fmt.Sprint(n + 1))
	case id == "preview":
		if len(m.visible) > 0 && double {
			m.openDetail(m.visible[m.cursor], scrBrowse)
		}
	case sc(id, "row:", &n):
		return m, m.clickRow(n, double)
	case id == "filter":
		return m, m.press("/")
	}
	return m, nil
}

// clickRow selects row n of the current list; a double click opens it.
func (m *model) clickRow(n int, double bool) tea.Cmd {
	switch m.screen {
	case scrBrowse:
		m.cursor = n
		m.clampOffset()
		if double {
			m.openDetail(m.visible[n], scrBrowse)
		}
	case scrStarted:
		m.startedCursor = n
		if double {
			return m.press("enter")
		}
	case scrEnv:
		m.envCursor = n
	case scrPaths:
		m.pathCursor = n
		if double {
			return m.press("enter")
		}
	case scrExamSetup:
		if m.setupField == n {
			return m.press("right")
		}
		m.setupField = n
	case scrExam:
		m.examCursor = n
		if double {
			return m.press("enter")
		}
	case scrExamResult:
		m.examCursor = n
		if double {
			return m.press("o")
		}
	}
	return nil
}

// sc parses "<prefix><int>" ids.
func sc(id, prefix string, n *int) bool {
	if !strings.HasPrefix(id, prefix) {
		return false
	}
	_, err := fmt.Sscan(strings.TrimPrefix(id, prefix), n)
	return err == nil
}

// ---- rendering ----

type button struct {
	label, key string
	off        bool // dimmed: pressing it only says why it can't run yet
}

// buttons renders a row of clickable buttons starting at body position (x, y),
// without their keys when bare.
func (m *model) buttons(x, y int, bare bool, bs ...button) string {
	var parts []string
	for _, b := range bs {
		id := "key:" + b.key
		st := sButton
		if m.hover == id {
			st = sBtnHot
		}
		r := b.render(st, m.hover == id, bare)
		if b.off && m.hover != id {
			r = b.render(st.Foreground(colMuted), false, bare)
		}
		m.addZone(id, x, y, lipgloss.Width(r), 1)
		x += lipgloss.Width(r) + 1
		parts = append(parts, r)
	}
	return strings.Join(parts, " ")
}

// render draws a button as its label followed by its key in a dimmer colour.
func (b button) render(st lipgloss.Style, hot, bare bool) string {
	if bare {
		return st.Render(b.label)
	}
	inner := st.UnsetPadding()
	key := inner.Foreground(colMuted)
	if hot {
		key = inner
	}
	return inner.Render(" "+b.label+" ") + key.Render(b.key+" ")
}

// width is the rendered width of the button.
func (b button) width(bare bool) int {
	if bare {
		return lipgloss.Width(b.label) + 2
	}
	return lipgloss.Width(b.label) + lipgloss.Width(b.key) + 3
}

// fitButtons reports how a row of buttons fits in avail columns: with their
// keys if that fits, else as bare labels, else ok is false. w is the row width.
func fitButtons(avail int, bs []button) (bare bool, w int, ok bool) {
	for _, bare := range []bool{false, true} {
		w := 0
		for _, b := range bs {
			w += b.width(bare) + 1
		}
		if w <= avail {
			return bare, w, true
		}
	}
	return true, 0, false
}

// titleButtons appends buttons to title when they fit on the line.
func (m *model) titleButtons(title string, y int, bs ...button) string {
	tw := lipgloss.Width(title)
	bare, _, ok := fitButtons(m.w-tw, bs)
	if !ok {
		return strings.TrimRight(title, " ")
	}
	return title + m.buttons(tw, y, bare, bs...)
}

func (m *model) menuBar(mns []menu) string {
	var b strings.Builder
	b.WriteString(sBar.Bold(true).Foreground(colAccent).Render(" awesome_labs "))
	x := lipgloss.Width(b.String())
	m.menuX = m.menuX[:0]
	for i, mn := range mns {
		st := sBar
		if i == m.menuOpen {
			st = lipgloss.NewStyle().Background(colAccent).Foreground(colHiFg)
		} else if m.hover == fmt.Sprintf("menu:%d", i) {
			st = sBar.Foreground(colAccent)
		}
		// underline the alt+ accelerator letter
		t := mn.title
		j := strings.IndexRune(strings.ToLower(t), mn.acc)
		seg := st.Render(" ") + st.Render(t[:j]) + st.Underline(true).Render(t[j:j+1]) + st.Render(t[j+1:]) + st.Render(" ")
		w := lipgloss.Width(seg)
		m.zones = append(m.zones, zone{x, 0, w, 1, fmt.Sprintf("menu:%d", i)})
		m.menuX = append(m.menuX, x)
		x += w
		b.WriteString(seg)
	}

	ids := m.trackIDs[""]
	solved, started := m.prog.Counts(ids)
	var right string
	if m.exam != nil {
		left := m.exam.Remaining()
		st := sBar.Foreground(colOK)
		if left < 10*time.Minute {
			st = sBar.Foreground(colBad)
		} else if left < 30*time.Minute {
			st = sBar.Foreground(colWarn)
		}
		p, n, _ := m.exam.Score()
		right += st.Bold(true).Render(fmt.Sprintf("EXAM %s %d/%d", formatLeft(left), p, n)) + sBar.Render("   ")
	} else if m.hasActive {
		right += sBar.Foreground(colAccent).Bold(true).Render("▶ "+m.active.ID) + sBar.Render("   ")
	}
	right += sBar.Foreground(colOK).Render(fmt.Sprintf("✔ %d", solved)) + sBar.Render(fmt.Sprintf("/%d solved  ", len(ids))) +
		sBar.Foreground(colWarn).Render(fmt.Sprintf("● %d", started)) + sBar.Render(" started, unsolved ")
	gap := m.w - x - lipgloss.Width(right)
	if gap < 1 { // short form
		right = sBar.Foreground(colOK).Render(fmt.Sprintf("✔%d", solved)) + sBar.Render(" ") +
			sBar.Foreground(colWarn).Render(fmt.Sprintf("●%d", started)) + sBar.Render(" ")
		gap = m.w - x - lipgloss.Width(right)
	}
	if gap < 1 {
		right = ""
		gap = max(0, m.w-x)
	}
	return b.String() + sBar.Render(strings.Repeat(" ", gap)) + right
}

// dropdown renders the open menu mn and registers its item zones.
func (m *model) dropdown(mn menu) (string, int) {
	lw, kw := 0, 0
	for _, it := range mn.items {
		if it.sep {
			lw = max(lw, lipgloss.Width(it.label)-1)
		} else {
			lw = max(lw, lipgloss.Width(it.label))
		}
		kw = max(kw, lipgloss.Width(it.key), lipgloss.Width(it.note))
	}
	inner := 2 + lw + 3 + kw + 1
	for _, it := range mn.items {
		if it.sep && it.key != "" { // room for "─ label ─ key cycles ─"
			inner = max(inner, lipgloss.Width(it.label)+lipgloss.Width(it.key)+14)
		}
	}
	lw = inner - kw - 6
	x := m.menuX[m.menuOpen]
	if x+inner+2 > m.w {
		x = max(0, m.w-inner-2)
	}
	base := lipgloss.NewStyle().Background(colMenuBg).Foreground(colText)
	var lines []string
	for i, it := range mn.items {
		if it.sep {
			lines = append(lines, sepLine(base, it, inner))
			continue
		}
		mark := " "
		if it.checked {
			mark = "✓"
		}
		st := base
		keySt := base.Foreground(colMuted)
		right := it.key
		if right == "" {
			right = it.note
		} else {
			keySt = base
		}
		switch {
		case it.disabled:
			st, keySt = base.Foreground(colMuted), base.Foreground(colMuted)
		case it.label == m.menuSel:
			st = lipgloss.NewStyle().Background(colAccent).Foreground(colHiFg)
			keySt = st
		}
		lines = append(lines, st.Render(" "+mark+padRight(it.label, lw)+"   ")+keySt.Render(fmt.Sprintf("%*s ", kw, right)))
		m.zones = append(m.zones, zone{x, 2 + i, inner + 2, 1, "mi:" + it.label})
	}
	return sMenuBox.Render(strings.Join(lines, "\n")), x
}

// sepLine draws a separator, or a group heading with the key that cycles it.
func sepLine(base lipgloss.Style, it menuItem, w int) string {
	line := base.Foreground(colMuted)
	if it.label == "" {
		return line.Render(strings.Repeat("─", w))
	}
	l := "─ " + it.label + " "
	r := ""
	if it.key != "" {
		r = " " + it.key + " cycles ─"
	}
	fill := max(0, w-lipgloss.Width(l)-lipgloss.Width(r))
	return line.Render("─ ") + line.Bold(true).Render(it.label) + line.Render(" "+strings.Repeat("─", fill)+r)
}

// dialog renders the confirmation question as a centered box with buttons.
func (m *model) dialog() (string, int, int) {
	q := strings.TrimSpace(m.confirm)
	w := min(max(40, lipgloss.Width(q)+4), m.w-4)
	text := lipgloss.NewStyle().Width(w - 4).Render(strings.TrimSpace(q))
	h := lipgloss.Height(text) + 4
	x, y := (m.w-w)/2, max(1, (m.h+1-h)/2)
	yes, no := sButton, sButton
	if m.confirmYes {
		yes = sBtnHot
	} else {
		no = sBtnHot
	}
	if m.hover == "dlg:yes" {
		yes = sBtnHot
	}
	if m.hover == "dlg:no" {
		no = sBtnHot
	}
	by, bn := yes.Render("  Yes  "), no.Render("  No  ")
	btns := by + "  " + bn
	bx := x + 2 + (w-4-lipgloss.Width(btns))/2
	by0 := y + 1 + lipgloss.Height(text) + 1
	m.zones = append(m.zones, zone{bx, by0, lipgloss.Width(by), 1, "dlg:yes"}, zone{bx + lipgloss.Width(by) + 2, by0, lipgloss.Width(bn), 1, "dlg:no"})
	body := text + "\n\n" + lipgloss.PlaceHorizontal(w-4, lipgloss.Center, btns)
	box := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(colWarn).Padding(0, 1).Width(w - 2).Render(body)
	return box, x, y
}

func (m *model) modalBox() (string, int, int) {
	lines := strings.Split(m.modal, "\n")
	if len(lines) > m.h-3 {
		lines = append(lines[:m.h-4], "…")
	}
	text := strings.Join(lines, "\n") + "\n\n" + sMuted.Render("click or press esc to close")
	box := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(colAccent).Padding(0, 2).Render(text)
	w, h := lipgloss.Width(box), lipgloss.Height(box)
	return box, max(0, (m.w-w)/2), max(1, (m.h+1-h)/2)
}

// overlay draws box over base with its top-left corner at (x, y).
func overlay(base, box string, x, y int) string {
	bl := strings.Split(base, "\n")
	for i, line := range strings.Split(box, "\n") {
		row := y + i
		for row >= len(bl) {
			bl = append(bl, "")
		}
		b := bl[row]
		left := ansi.Truncate(b, x, "")
		if pad := x - ansi.StringWidth(left); pad > 0 {
			left += strings.Repeat(" ", pad)
		}
		right := ansi.TruncateLeft(b, x+ansi.StringWidth(line), "")
		bl[row] = left + "\x1b[0m" + line + "\x1b[0m" + right
	}
	return strings.Join(bl, "\n")
}

func formatLeft(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	return fmt.Sprintf("%d:%02d:%02d", int(d.Hours()), int(d.Minutes())%60, int(d.Seconds())%60)
}

const shortcutsText = `Keyboard shortcuts

Everywhere      F10 menu bar · alt+f/e/v/t/m/x/h open a menu · F1 this help
                a active exercise · S started labs · e environments & VMs
                Q quit and stop VMs · ctrl+c quit

Exercise list   ↑↓ move · enter open · / filter · esc clear filter
                tab next track · n/p next/previous module
                u cycle all / solved / in progress / no progress
                L learning paths · X exam · q quit

Exercise        s start the lab · t terminal · w node01 terminal (Kubernetes)
                c check · h next hint · v solution · R reset progress
                ←→, tab or 1–5 switch tabs · n/N next/previous in the list it came from
                esc/q back — leaving the lab that is set up asks whether to stop its VMs

Started labs    ↑↓ move · enter open · r reset selected · A reset all · esc back
Learning paths  ↑↓ move · enter apply · esc back
Exam            setup: ↑↓ field · ←→ change · enter start
                running: ↑↓ move · enter open question · F finish · esc list (timer runs)
                result: ↑↓ move · o open question (solutions unlocked) · esc list

Environments    ↑↓ select · u start · x stop · R reset VMs · B rebuild · D delete
                X stop all · K delete all · r refresh · esc back

Menus           ←→ switch menu · ↑↓ move · enter choose · esc close
                or press the key shown next to an item
Dialogs         y/n, or ←→ and enter`

const mouseText = `Using the mouse

• Click a menu title to open it; hover to move between menus.
  Menus only offer what applies to the screen you are on.
• Click a row to select it and double-click it to open it;
  switch track from the Tracks menu.
• Click the tabs and buttons on the exercise screen.
• The wheel scrolls lists and text.
• Click Yes or No in a confirmation.
• A greyed-out menu item or a dimmed button says why when you click it.

Most terminals select text with shift+drag while the mouse is in use.`

const aboutText = `awesome_labs

Hands-on labs on your own machine for Docker, Kubernetes, Linux,
networking, cloud, data, security, programming and data science.
Real Lima VMs, real breakage, automatic grading.

https://github.com/kpatr1981/awesome_labs`
