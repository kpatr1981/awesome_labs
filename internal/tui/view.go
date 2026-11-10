package tui

import (
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"

	"awesome_labs/internal/exercise"
	"awesome_labs/internal/lab"
)

var (
	colAccent = lipgloss.AdaptiveColor{Light: "#0066cc", Dark: "#5fafff"}
	colMuted  = lipgloss.AdaptiveColor{Light: "#666666", Dark: "#8a8a8a"}
	colOK     = lipgloss.AdaptiveColor{Light: "#1a7f37", Dark: "#56d364"}
	colWarn   = lipgloss.AdaptiveColor{Light: "#9a6700", Dark: "#e3b341"}
	colBad    = lipgloss.AdaptiveColor{Light: "#cf222e", Dark: "#ff7b72"}
	colSelBg  = lipgloss.AdaptiveColor{Light: "#dbeafe", Dark: "#1f3a5f"}

	sTitle  = lipgloss.NewStyle().Bold(true).Foreground(colAccent)
	sMuted  = lipgloss.NewStyle().Foreground(colMuted)
	sOK     = lipgloss.NewStyle().Foreground(colOK)
	sWarn   = lipgloss.NewStyle().Foreground(colWarn)
	sBad    = lipgloss.NewStyle().Foreground(colBad)
	sSel    = lipgloss.NewStyle().Background(colSelBg).Bold(true)
	sHeader = lipgloss.NewStyle().Bold(true).Foreground(colWarn)
	sTabOn  = lipgloss.NewStyle().Bold(true).Foreground(colAccent).Underline(true).Padding(0, 1)
	sTabOff = lipgloss.NewStyle().Foreground(colMuted).Padding(0, 1)
	sBorder = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(colMuted)
)

func diffStyle(d int) lipgloss.Style {
	switch d {
	case 1, 2:
		return sOK
	case 3:
		return sWarn
	}
	return sBad
}

func stars(d int) string {
	return diffStyle(d).Render(strings.Repeat("●", d)) + sMuted.Render(strings.Repeat("○", 5-d))
}

func trackLabel(t string) string {
	switch t {
	case "":
		return "All"
	case "docker":
		return "Docker"
	case "kubernetes":
		return "Kubernetes"
	case "offsec":
		return "Offensive Sec"
	case "linux":
		return "Linux"
	case "networking":
		return "Networking"
	case "cloud":
		return "Cloud"
	case "data":
		return "Data"
	case "defensive":
		return "Defensive Sec"
	case "programming":
		return "Programming"
	case "datascience":
		return "Data Science"
	}
	return t
}

func (m *model) View() string {
	if m.w == 0 {
		return "loading…"
	}
	m.zones = m.zones[:0]
	var body string
	switch m.screen {
	case scrBrowse:
		body = m.viewBrowse()
	case scrDetail:
		body = m.viewDetail()
	case scrEnv:
		body = m.viewEnv()
	case scrStarted:
		body = m.viewStarted()
	case scrPaths:
		body = m.viewPaths()
	case scrExamSetup:
		body = m.viewExamSetup()
	case scrExam:
		body = m.viewExam()
	case scrExamResult:
		body = m.viewExamResult()
	}
	// keep the frame inside the window so overlays line up
	lines := strings.Split(body, "\n")
	if len(lines) > m.h {
		lines = lines[:m.h]
	}
	mns := m.menus()            // once per frame: the bar and the open menu share it
	if m.menuOpen >= len(mns) { // the screen changed under an open menu
		m.closeMenu()
	}
	out := m.menuBar(mns) + "\n" + strings.Join(lines, "\n")
	if m.menuOpen >= 0 {
		box, x := m.dropdown(mns[m.menuOpen])
		out = overlay(out, box, x, 1)
	}
	if m.confirm != "" {
		box, x, y := m.dialog()
		out = overlay(out, box, x, y)
	}
	if m.modal != "" {
		box, x, y := m.modalBox()
		m.zones = append(m.zones, zone{0, 0, m.w, m.h + 1, "dlg:close"})
		out = overlay(out, box, x, y)
	}
	return out
}

// statusLine shows the running operation or the last message, then help. A
// pending question is in its dialog, not here.
func (m *model) statusLine(help string) string {
	var s string
	if m.op != "" {
		s = " " + m.spin.View() + " " + sWarn.Render(m.op+"…") + "  "
	} else if m.flash != "" {
		st := sMuted
		switch {
		case strings.HasPrefix(m.flash, "✔"), strings.HasPrefix(m.flash, "lab ready"):
			st = sOK
		case strings.HasPrefix(m.flash, "failed"), strings.HasPrefix(m.flash, "not yet"):
			st = sBad
		}
		s = " " + st.Render(m.flash) + "  "
	}
	return s + sMuted.Render(help)
}

// ---- browse ----

func (m *model) viewBrowse() string {
	// the track is picked from the Track menu (or tab); this line only shows
	// the active path, the progress filter and the search box when in use
	var bar string
	if m.path != nil {
		ps, pt := m.pathProgress(m.path)
		bar += sWarn.Render("path: "+m.path.Title) + sMuted.Render(fmt.Sprintf(" %d/%d  ", ps, pt))
	}
	if m.progFilter != progAll {
		bar += sWarn.Render("[" + strings.ToLower(progFilterNames[m.progFilter]) + "]  ")
	}
	if m.filterOn || m.filter.Value() != "" {
		m.addZone("filter", lipgloss.Width(bar), 0, m.w, 1)
		bar += m.filter.View()
	}
	top := 0 // rows taken by the info line
	if m.hasBrowseBar() {
		top = 1
	}

	listW := min(max(46, m.w*45/100), m.w-20)
	prevW := m.w - listW - 4
	lh := m.listHeight()

	// list rows
	var rows []string
	for i := m.offset; i < len(m.visible) && len(rows) < lh; i++ {
		e := m.visible[i]
		if i == m.offset || e.Module != m.visible[i-1].Module || e.Track != m.visible[i-1].Track { // module header row
			rows = append(rows, sHeader.Render(trunc(fmt.Sprintf("▸ %s · %s", trackLabel(e.Track), e.Module), listW-2)))
			if len(rows) >= lh {
				break
			}
		}
		mark := m.prog.Get(e.ID).Mark()
		if m.isActive(e) {
			mark = "▶"
		}
		switch mark {
		case "▶":
			mark = sTitle.Render(mark)
		case "✔":
			mark = sOK.Render(mark)
		case "●":
			mark = sWarn.Render(mark)
		}
		kind := sMuted.Render("build")
		if e.Kind == "fix" {
			kind = sBad.Render("fix  ")
		}
		titleW := listW - 2 - 2 - 10 - 6 - 6 - 2
		line := fmt.Sprintf(" %s %-9s %s %s %s", mark, e.ID, stars(e.Difficulty), kind, trunc(e.Title, titleW))
		if i == m.cursor {
			line = sSel.Render(padRight(line, listW-2))
		}
		m.addZone(fmt.Sprintf("row:%d", i), 0, top+1+len(rows), listW, 1)
		rows = append(rows, line)
	}
	if len(m.visible) == 0 {
		rows = append(rows, sMuted.Render("  no exercises match"))
	}
	for len(rows) < lh {
		rows = append(rows, "")
	}
	list := sBorder.Width(listW - 2).Height(lh).Render(strings.Join(rows, "\n"))

	// preview
	var prev string
	if len(m.visible) > 0 {
		e := m.visible[m.cursor]
		prev = m.preview(e, prevW-2, lh)
	}
	prevBox := sBorder.Width(prevW - 2).Height(lh).Render(prev)
	m.addZone("preview", listW, top, prevW, lh+2)

	help := "↑↓ move · enter open · / filter · tab next track · F10 menu · F1 keys · q quit"
	body := lipgloss.JoinHorizontal(lipgloss.Top, list, prevBox)
	if top > 0 {
		body = lipgloss.JoinVertical(lipgloss.Left, bar, body)
	}
	return lipgloss.JoinVertical(lipgloss.Left, body, m.statusLine(help))
}

func (m *model) meta(e *exercise.Exercise) string {
	env := lab.EnvFor(e.Profile).Title
	kind := "Build something"
	if e.Kind == "fix" {
		kind = "Fix something"
	}
	return stars(e.Difficulty) + " " + diffStyle(e.Difficulty).Render(exercise.DifficultyLabel(e.Difficulty)) +
		sMuted.Render(" · "+kind+" · "+env)
}

func (m *model) preview(e *exercise.Exercise, w, h int) string {
	p := m.prog.Get(e.ID)
	status := sMuted.Render("not started")
	switch {
	case !p.Solved.IsZero():
		status = sOK.Render("✔ solved " + p.Solved.Format("2006-01-02"))
	case m.isActive(e):
		status = sWarn.Render("▶ set up on the VMs now")
	case !p.Started.IsZero():
		status = sWarn.Render("● started, not solved")
	}
	head := sTitle.Render(e.ID+" — "+e.Title) + "\n" + m.meta(e) + "\n" + status
	if len(e.Tags) > 0 {
		head += sMuted.Render("  #" + strings.Join(e.Tags, " #"))
	}
	md := m.markdown("prev:"+e.ID, e.Scenario, w)
	lines := strings.Split(head+"\n"+md, "\n")
	if len(lines) > h {
		lines = append(lines[:h-1], sMuted.Render("… press enter to read more"))
	}
	return strings.Join(lines, "\n")
}

// ---- detail ----

func (m *model) viewDetail() string {
	e := m.cur
	title := sTitle.Render(" "+e.ID+" — "+e.Title) + "   " + m.meta(e)
	if r := m.result[e.ID]; r == "passed" {
		title += "  " + sOK.Render("✔ PASSED")
	} else if r == "failed" {
		title += "  " + sBad.Render("✘ not yet")
	}
	var tabs []string
	for i, n := range tabNames {
		label := fmt.Sprintf("%d %s", i+1, n)
		if tab(i) == tabHints {
			label = fmt.Sprintf("%d Hints %d/%d", i+1, m.hintsShow, len(e.Hints))
		}
		if tab(i) == tabSolution && !m.solShow {
			label += " 🔒"
		}
		if tab(i) == tabOutput && m.op != "" && m.opID == e.ID {
			label += " " + m.spin.View()
		}
		st := sTabOff
		if tab(i) == m.tab {
			st = sTabOn
		} else if m.hover == fmt.Sprintf("tab:%d", i) {
			st = sTabOff.Foreground(colAccent)
		}
		tabs = append(tabs, st.Render(label))
	}
	x := 0
	for i, t := range tabs {
		m.addZone(fmt.Sprintf("tab:%d", i), x, 1, lipgloss.Width(t), 1)
		x += lipgloss.Width(t)
	}
	tabRow := strings.Join(tabs, "")
	// terminals and checks need the lab set up, which only the active
	// exercise has: dimmed otherwise, they say why when pressed
	active := m.isActive(e)
	start := "▶ Start"
	if active {
		start = "↻ Restart"
	}
	bs := []button{{label: start, key: "s"}, {label: "Terminal", key: "t", off: !active}}
	if e.Profile == exercise.ProfileK8s {
		bs = append(bs, button{label: "node01", key: "w", off: !active})
	}
	bs = append(bs, button{label: "✔ Check", key: "c", off: !active})
	if !m.examLocked(e) {
		bs = append(bs, button{label: "Hint", key: "h"}, button{label: "Solution", key: "v"})
	}
	bs = append(bs, button{label: "Back", key: "esc"})
	if bare, bw, ok := fitButtons(m.w-x-2, bs); ok {
		gap := m.w - x - bw
		tabRow += strings.Repeat(" ", gap) + m.buttons(x+gap, 1, bare, bs...)
	}
	box := sBorder.Width(m.w - 2).Render(m.vp.View())
	// the buttons above and the menus cover the actions; list only the rest
	back := map[screen]string{scrStarted: "started labs", scrExam: "exam", scrExamResult: "exam result"}[m.back]
	if back == "" {
		back = "list"
	}
	help := "←→ switch tab · n/N next/previous " + m.siblingNoun() + " · esc back to the " + back + " · F10 menu · F1 keys"
	return lipgloss.JoinVertical(lipgloss.Left, title, tabRow, box, m.statusLine(help))
}

func (m *model) refreshViewport(follow bool) {
	if m.cur == nil && m.screen != scrEnv {
		return
	}
	m.vp.Width = m.w - 4
	m.vp.Height = max(3, m.h-6)
	w := m.vp.Width - 2
	var content string
	switch {
	case m.screen == scrEnv:
		return
	case m.tab == tabScenario:
		intro := sMuted.Render("Press s to prepare the lab, then t to open a terminal. The task is also in /lab/TASK.md.")
		content = intro + "\n" + m.markdown("sc:"+m.cur.ID, m.cur.Scenario, w)
	case m.tab == tabLearn && m.examLocked(m.cur):
		content = sMuted.Render("\n  🔒 Tutorials are locked during the exam.")
	case m.tab == tabLearn:
		if strings.TrimSpace(m.cur.Tutorial) == "" {
			content = sMuted.Render("\n  No tutorial for this one yet.")
			break
		}
		content = m.markdown("learn:"+m.cur.ID, m.cur.Tutorial, w)
	case m.tab == tabHints && m.examLocked(m.cur):
		content = sMuted.Render("\n  🔒 Hints are locked during the exam.")
	case m.tab == tabSolution && m.examLocked(m.cur):
		content = sMuted.Render("\n  🔒 The solution is locked during the exam. It unlocks on the result screen.")
	case m.tab == tabHints:
		if len(m.cur.Hints) == 0 {
			content = sMuted.Render("\n  No hints for this one.")
			break
		}
		var b strings.Builder
		for i := 0; i < m.hintsShow; i++ {
			fmt.Fprintf(&b, "**Hint %d.** %s\n\n", i+1, m.cur.Hints[i])
		}
		content = m.markdown(fmt.Sprintf("h:%s:%d", m.cur.ID, m.hintsShow), b.String(), w)
		if m.hintsShow < len(m.cur.Hints) {
			content += "\n" + sMuted.Render(fmt.Sprintf("  press h to reveal hint %d of %d", m.hintsShow+1, len(m.cur.Hints)))
		}
	case m.tab == tabSolution:
		if !m.solShow {
			content = sMuted.Render("\n  🔒 The solution is hidden. Press v to reveal it.")
			break
		}
		content = m.markdown("sol:"+m.cur.ID, m.cur.Solution, w)
	case m.tab == tabOutput:
		b := m.output[m.cur.ID]
		if b == nil || b.Len() == 0 {
			content = sMuted.Render("\n  No output yet. Press s to prepare the lab or c to check your work.")
		} else {
			content = lipgloss.NewStyle().Width(w).Render(b.String())
		}
	}
	m.vp.SetContent(content)
	if follow && m.tab == tabOutput {
		m.vp.GotoBottom()
	}
}

func (m *model) viewStarted() string {
	title := sTitle.Render(" Your labs ") + sMuted.Render("one exercise is set up on the VMs at a time") + "   "
	title = m.titleButtons(title, 0, button{label: "Open", key: "enter"}, button{label: "Reset selected", key: "r"}, button{label: "Reset all", key: "A"}, button{label: "Back", key: "esc"})
	if len(m.started) == 0 {
		return lipgloss.JoinVertical(lipgloss.Left, title, "",
			sMuted.Render("  No started labs. Start a lab with s on an exercise."),
			m.statusLine("esc back"))
	}
	var b strings.Builder
	if !m.hasActive {
		b.WriteString(sMuted.Render("  No exercise is set up right now.") + "\n\n")
	}
	for i, e := range m.started {
		p := m.prog.Get(e.ID)
		if m.isActive(e) && i == 0 {
			b.WriteString(sHeader.Render("  ▶ Working on") + "\n")
		} else if i == 0 || m.isActive(m.started[i-1]) {
			b.WriteString(sHeader.Render("  Started earlier, not solved") + sMuted.Render(" — their lab was reset; press enter, then s to start one again") + "\n")
		}
		info := "· started " + p.Started.Format("2006-01-02 15:04")
		if m.isActive(e) {
			info = fmt.Sprintf("· since %s · %d checks · %s", m.active.Since.Format("2006-01-02 15:04"), p.Checks, lab.EnvFor(e.Profile).Title)
		}
		line := fmt.Sprintf("%-10s %s %s", e.ID, trunc(e.Title, 40), sMuted.Render(info))
		m.addZone(fmt.Sprintf("row:%d", i), 0, 2+strings.Count(b.String(), "\n"), m.w, 1)
		if i == m.startedCursor {
			b.WriteString(sTitle.Render("▸ ") + line + "\n")
		} else {
			b.WriteString("  " + line + "\n")
		}
	}
	help := "↑↓ move · F10 menu · F1 keys"
	return lipgloss.JoinVertical(lipgloss.Left, title, "", b.String(), m.statusLine(help))
}

func (m *model) markdown(key, src string, w int) string {
	m.mdMu.Lock()
	defer m.mdMu.Unlock()
	ck := fmt.Sprintf("%s@%d", key, w)
	if s, ok := m.mdCache[ck]; ok {
		return s
	}
	r, err := glamour.NewTermRenderer(glamour.WithStandardStyle(m.style), glamour.WithWordWrap(max(20, w-2)))
	out := src
	if err == nil {
		if s, err := r.Render(src); err == nil {
			out = strings.TrimRight(s, "\n")
		}
	}
	m.mdCache[ck] = out
	return out
}

// ---- environments ----

func (m *model) viewEnv() string {
	var b strings.Builder
	b.WriteString("\n")
	t := sTitle.Render(" Lab environments & VMs") + "   "
	b.WriteString(m.titleButtons(t, 1, button{label: "Start", key: "u"}, button{label: "Stop", key: "x"}, button{label: "Reset", key: "R"},
		button{label: "Rebuild", key: "B"}, button{label: "Delete", key: "D"}, button{label: "Stop all", key: "X"}, button{label: "Delete all", key: "K"}, button{label: "Refresh", key: "r"}, button{label: "Back", key: "esc"}) + "\n\n")
	for i, env := range lab.AllEnvs() {
		st, ok := m.envStatus[env.Profile]
		if !ok {
			st = "checking…"
		}
		stStyle := sMuted
		switch {
		case st == lab.StatusReady:
			stStyle = sOK
		case st == lab.StatusStopped || st == lab.StatusMissing:
			stStyle = sMuted
		default:
			stStyle = sWarn
		}
		active := ""
		if m.hasActive && m.active.Profile == env.Profile {
			active = sTitle.Render("  ▶ " + m.active.ID)
		}
		var vms []string
		for _, v := range env.VMs {
			live, ok := m.vmInfo[v.Name]
			switch {
			case !ok:
				vms = append(vms, sMuted.Render(fmt.Sprintf("%s (%s, %d CPU, %s)", v.Name, v.Hostname, v.CPUs, v.Memory)))
			case live.Running():
				vms = append(vms, sOK.Render("● ")+fmt.Sprintf("%-12s running   %d CPU %s", v.Name, live.CPUs, live.MemoryLabel()))
			case live.Status == "":
				vms = append(vms, sMuted.Render(fmt.Sprintf("○ %-12s not created (%d CPU, %s)", v.Name, v.CPUs, v.Memory)))
			default:
				vms = append(vms, sMuted.Render(fmt.Sprintf("○ %-12s %-9s %d CPU %s", v.Name, strings.ToLower(live.Status), live.CPUs, live.MemoryLabel())))
			}
		}
		cur := "  "
		if i == m.envCursor {
			cur = sTitle.Render("▸ ")
		}
		tainted := ""
		if m.mgr.Tainted(env) {
			tainted = sWarn.Render("  (will be rebuilt before the next exercise)")
		}
		m.addZone(fmt.Sprintf("row:%d", i), 0, strings.Count(b.String(), "\n"), m.w, 1+len(vms))
		fmt.Fprintf(&b, "%s%-22s %s%s%s\n", cur, env.Title, stStyle.Render(st), active, tainted)
		for _, v := range vms {
			fmt.Fprintf(&b, "     %s\n", v)
		}
		b.WriteString("\n")
	}
	var stray []string
	n, cpus, mem := 0, 0, int64(0)
	for _, v := range m.vmInfo {
		if v.Running() {
			n, cpus, mem = n+1, cpus+v.CPUs, mem+v.Memory
		}
		if v.Env == "" {
			stray = append(stray, v.Name+" ("+strings.ToLower(v.Status)+")")
		}
	}
	if len(stray) > 0 {
		sort.Strings(stray)
		b.WriteString(sWarn.Render("  Other lab VMs: "+strings.Join(stray, ", ")+" — K deletes them too") + "\n\n")
	}
	if m.vmInfo != nil {
		b.WriteString(fmt.Sprintf("  %s\n\n", sMuted.Render(fmt.Sprintf("%d running · %d CPU · %.0f GiB in use", n, cpus, float64(mem)/(1<<30)))))
	}
	b.WriteString(sMuted.Render(fmt.Sprintf("  Kubernetes v%s · Calico %s · Traefik ingress/gateway · metrics-server · local-path storage", lab.K8sVersion, lab.CalicoVersion)) + "\n")
	b.WriteString(sMuted.Render("  First start of an environment takes 5–15 minutes. Full log: "+m.mgr.LogPath()) + "\n\n")
	if o := m.output["env"]; o != nil && o.Len() > 0 {
		lines := strings.Split(strings.TrimRight(o.String(), "\n"), "\n")
		keep := max(3, m.h-18)
		if len(lines) > keep {
			lines = lines[len(lines)-keep:]
		}
		b.WriteString(sBorder.Width(m.w - 2).Render(strings.Join(lines, "\n")))
		b.WriteString("\n")
	}
	help := "↑↓ select · F10 menu · F1 keys"
	return b.String() + m.statusLine(help)
}

// ---- utils ----

func trunc(s string, n int) string {
	if n <= 1 {
		return ""
	}
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}

func padRight(s string, w int) string {
	if d := w - lipgloss.Width(s); d > 0 {
		return s + strings.Repeat(" ", d)
	}
	return s
}
