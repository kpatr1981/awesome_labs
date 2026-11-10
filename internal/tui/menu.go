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
	sep        bool
	disabled   bool
	checked    bool
	run        func() tea.Cmd
}

type menu struct {
	title string
	acc   rune // alt+acc opens it
	items []menuItem
}

func sep() menuItem { return menuItem{sep: true} }

// keyMsg builds the key message a real key press would produce.
func keyMsg(k string) tea.KeyMsg {
	switch k {
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEsc}
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
	}
	return nil
}

// onExercise runs key k on the exercise screen of the target exercise.
func (m *model) onExercise(k string) func() tea.Cmd {
	return func() tea.Cmd {
		if m.screen != scrDetail {
			ex := m.target()
			if ex == nil {
				return nil
			}
			m.openDetail(ex)
		}
		_, c := m.updateDetail(keyMsg(k))
		return c
	}
}

func (m *model) onScreen(s screen, k string) func() tea.Cmd {
	return func() tea.Cmd {
		m.screen = s
		if s == scrEnv {
			return tea.Batch(m.press(k), m.cmdEnvStatus())
		}
		return m.press(k)
	}
}

func (m *model) menus() []menu {
	ex := m.target()
	noEx := ex == nil
	locked := m.examLocked(ex)
	onBrowse, onDetail, onEnv := m.screen == scrBrowse, m.screen == scrDetail, m.screen == scrEnv

	file := menu{"File", 'f', []menuItem{
		{label: "Browse exercises", key: "esc", run: func() tea.Cmd { m.screen = scrBrowse; return nil }},
		{label: "Started labs", key: "S", run: func() tea.Cmd { m.openStarted(); return nil }},
		{label: "Learning paths", key: "L", run: func() tea.Cmd { m.screen = scrPaths; return nil }},
		{label: "Lab environments & VMs", key: "e", run: func() tea.Cmd { m.screen = scrEnv; return m.cmdEnvStatus() }},
		sep(),
		{label: "Quit", key: "q", run: func() tea.Cmd { return tea.Quit }},
		{label: "Quit and stop all VMs", key: "Q", run: m.quitAndStop},
	}}

	exm := menu{"Exercise", 'e', []menuItem{
		{label: "Open", key: "enter", disabled: !onBrowse || noEx, run: func() tea.Cmd { m.openDetail(ex); return nil }},
		{label: "Go to the active exercise", key: "a", disabled: !m.hasActive, run: func() tea.Cmd { m.goActive(); return nil }},
		sep(),
		{label: "Start / set up the lab", key: "s", disabled: noEx, run: m.onExercise("s")},
		{label: "Open a terminal", key: "t", disabled: noEx, run: m.onExercise("t")},
		{label: "Terminal on node01", key: "w", disabled: noEx || ex.Profile != exercise.ProfileK8s, run: m.onExercise("w")},
		{label: "Check my work", key: "c", disabled: noEx, run: m.onExercise("c")},
		sep(),
		{label: "Show the next hint", key: "h", disabled: noEx || locked || len(ex.Hints) == 0, run: m.onExercise("h")},
		{label: "Reveal the solution…", key: "v", disabled: noEx || locked, run: m.onExercise("v")},
		{label: "Reset my progress…", key: "R", disabled: noEx, run: m.onExercise("R")},
		sep(),
		{label: "Next exercise", key: "n", disabled: !onDetail, run: m.onExercise("n")},
		{label: "Previous exercise", key: "N", disabled: !onDetail, run: m.onExercise("N")},
	}}

	view := menu{"View", 'v', []menuItem{
		{label: "Filter…", key: "/", run: func() tea.Cmd {
			m.screen = scrBrowse
			m.filterOn = true
			m.filter.Focus()
			return textinput.Blink
		}},
		{label: "Clear the filter", key: "esc", disabled: m.filter.Value() == "", run: func() tea.Cmd {
			m.filter.SetValue("")
			m.refilter()
			return nil
		}},
		{label: "Unsolved only", key: "u", checked: m.unsolved, run: func() tea.Cmd {
			m.unsolved = !m.unsolved
			m.refilter()
			return nil
		}},
		sep(),
		{label: "Next module", key: "n", disabled: !onBrowse, run: func() tea.Cmd { return m.press("n") }},
		{label: "Previous module", key: "p", disabled: !onBrowse, run: func() tea.Cmd { return m.press("p") }},
		sep(),
		{label: "Clear the learning path", disabled: m.path == nil, run: func() tea.Cmd {
			m.path = nil
			m.cursor, m.offset = 0, 0
			m.refilter()
			return nil
		}},
	}}

	tracks := menu{title: "Tracks", acc: 't'}
	for i, t := range m.tracks {
		i := i
		key := ""
		if i == 0 {
			key = "tab cycles"
		}
		tracks.items = append(tracks.items, menuItem{label: trackLabel(t), key: key, checked: i == m.trackIdx, run: func() tea.Cmd {
			m.screen = scrBrowse
			m.trackIdx, m.cursor, m.offset = i, 0, 0
			m.refilter()
			return nil
		}})
	}

	envs := menu{"VMs", 'm', []menuItem{
		{label: "Show environments & VMs", key: "e", run: func() tea.Cmd { m.screen = scrEnv; return m.cmdEnvStatus() }},
		{label: "Refresh status", key: "r", disabled: !onEnv, run: func() tea.Cmd { return m.press("r") }},
		sep(),
		{label: "Start / create selected", key: "u", disabled: !onEnv, run: func() tea.Cmd { return m.press("u") }},
		{label: "Stop selected", key: "x", disabled: !onEnv, run: func() tea.Cmd { return m.press("x") }},
		{label: "Reset selected", key: "R", disabled: !onEnv, run: func() tea.Cmd { return m.press("R") }},
		{label: "Rebuild selected…", key: "B", disabled: !onEnv, run: func() tea.Cmd { return m.press("B") }},
		{label: "Delete selected…", key: "D", disabled: !onEnv, run: func() tea.Cmd { return m.press("D") }},
		sep(),
		{label: "Stop all lab VMs", key: "X", run: m.onScreen(scrEnv, "X")},
		{label: "Delete all lab VMs…", key: "K", run: m.onScreen(scrEnv, "K")},
	}}

	exams := menu{"Exam", 'x', []menuItem{
		{label: "New exam…", disabled: m.exam != nil, run: func() tea.Cmd { m.screen = scrExamSetup; return nil }},
		{label: "Return to the exam", key: "X", disabled: m.exam == nil, run: func() tea.Cmd { m.screen = scrExam; return nil }},
		{label: "Finish the exam…", key: "F", disabled: m.exam == nil, run: m.onScreen(scrExam, "F")},
		sep(),
		{label: "Last result", disabled: m.finished == nil, run: func() tea.Cmd { m.screen = scrExamResult; return nil }},
	}}

	help := menu{"Help", 'h', []menuItem{
		{label: "Keyboard shortcuts", key: "F1", run: func() tea.Cmd { m.modal = shortcutsText; return nil }},
		{label: "Using the mouse", run: func() tea.Cmd { m.modal = mouseText; return nil }},
		sep(),
		{label: "About awesome_labs", run: func() tea.Cmd { m.modal = aboutText; return nil }},
	}}
	return []menu{file, exm, view, tracks, envs, exams, help}
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

func (m *model) goActive() {
	if ex := m.cat.Get(m.active.ID); m.hasActive && ex != nil {
		m.openDetail(ex)
	} else {
		m.flash = "no exercise is set up on the VMs — open one and press s to start it (S lists labs started earlier)"
	}
}

// ---- menu state ----

func (m *model) openMenu(i int) {
	m.menuOpen, m.menuItem = i, -1
	m.menuStep(1)
}

func (m *model) closeMenu() { m.menuOpen = -1 }

// menuStep moves the highlight to the next enabled item in direction d.
func (m *model) menuStep(d int) {
	items := m.menus()[m.menuOpen].items
	i := m.menuItem
	for range items {
		i = (i + d + len(items)) % len(items)
		if !items[i].sep && !items[i].disabled {
			m.menuItem = i
			return
		}
	}
}

func (m *model) runMenuItem(i int) tea.Cmd {
	items := m.menus()[m.menuOpen].items
	m.closeMenu()
	if i < 0 || i >= len(items) || items[i].sep || items[i].disabled {
		return nil
	}
	m.flash = ""
	return items[i].run()
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
	case "enter", " ":
		return m.runMenuItem(m.menuItem)
	default:
		if i := m.menuForAlt(k); i >= 0 {
			m.openMenu(i)
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
	return nil, false
}

func (m *model) answer(yes bool) tea.Cmd {
	m.confirm, m.confirmYes = "", false
	f := m.onYes
	m.onYes = nil
	if yes && f != nil {
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
			if sc(id, "mi:", &n) {
				m.menuItem = n
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
		case sc(id, "mi:", &n):
			return m, m.runMenuItem(n)
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
			m.openDetail(m.visible[m.cursor])
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
			m.openDetail(m.visible[n])
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

type button struct{ label, key string }

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

func (m *model) menuBar() string {
	var b strings.Builder
	b.WriteString(sBar.Bold(true).Foreground(colAccent).Render(" awesome_labs "))
	x := lipgloss.Width(b.String())
	m.menuX = m.menuX[:0]
	for i, mn := range m.menus() {
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

	ids := make([]string, 0, len(m.cat.Exercises))
	for _, e := range m.cat.Exercises {
		ids = append(ids, e.ID)
	}
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

// dropdown renders the open menu and registers its item zones.
func (m *model) dropdown() (string, int) {
	mn := m.menus()[m.menuOpen]
	lw, kw := 0, 0
	for _, it := range mn.items {
		lw = max(lw, lipgloss.Width(it.label))
		kw = max(kw, lipgloss.Width(it.key))
	}
	inner := 2 + lw + 3 + kw + 1
	x := m.menuX[m.menuOpen]
	if x+inner+2 > m.w {
		x = max(0, m.w-inner-2)
	}
	base := lipgloss.NewStyle().Background(colMenuBg).Foreground(colText)
	var lines []string
	for i, it := range mn.items {
		if it.sep {
			lines = append(lines, base.Foreground(colMuted).Render(strings.Repeat("─", inner)))
			continue
		}
		mark := " "
		if it.checked {
			mark = "✓"
		}
		text := " " + mark + padRight(it.label, lw) + "   " + fmt.Sprintf("%*s", kw, it.key) + " "
		st := base
		switch {
		case it.disabled:
			st = base.Foreground(colMuted)
		case i == m.menuItem:
			st = lipgloss.NewStyle().Background(colAccent).Foreground(colHiFg)
		}
		lines = append(lines, st.Width(inner).Render(text))
		m.zones = append(m.zones, zone{x, 2 + i, inner + 2, 1, fmt.Sprintf("mi:%d", i)})
	}
	return sMenuBox.Render(strings.Join(lines, "\n")), x
}

// dialog renders the confirmation question as a centered box with buttons.
func (m *model) dialog() (string, int, int) {
	q := strings.TrimSuffix(strings.TrimSpace(m.confirm), "(y/n)")
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
                ctrl+c quit

Exercise list   ↑↓ move · enter open · / filter · esc clear filter
                tab next track · n/p next/previous module · u unsolved only
                a active exercise · S started labs · L learning paths
                e environments & VMs · X exam · q quit · Q quit and stop VMs

Exercise        s start the lab · t terminal · w node01 terminal (Kubernetes)
                c check · h next hint · v solution · R reset progress
                1–5 or tab switch tabs · n/N next/previous · esc back

Environments    ↑↓ select · u start · x stop · R reset · B rebuild · D delete
                X stop all · K delete all · r refresh · esc back

Menus           ←→ switch menu · ↑↓ move · enter choose · esc close
Dialogs         y/n, or ←→ and enter`

const mouseText = `Using the mouse

• Click a menu title to open it; hover to move between menus.
• Click a row to select it and double-click it to open it;
  switch track from the Tracks menu.
• Click the tabs and buttons on the exercise screen.
• The wheel scrolls lists and text.
• Click Yes or No in a confirmation.

Most terminals select text with shift+drag while the mouse is in use.`

const aboutText = `awesome_labs

Hands-on labs on your own machine for Docker, Kubernetes, Linux,
networking, cloud, data, security, programming and data science.
Real Lima VMs, real breakage, automatic grading.

https://github.com/kpatr1981/awesome_labs`
