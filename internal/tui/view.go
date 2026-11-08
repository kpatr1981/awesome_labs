package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"

	"awesome_labs/internal/exam"
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

	sTitle   = lipgloss.NewStyle().Bold(true).Foreground(colAccent)
	sMuted   = lipgloss.NewStyle().Foreground(colMuted)
	sOK      = lipgloss.NewStyle().Foreground(colOK)
	sWarn    = lipgloss.NewStyle().Foreground(colWarn)
	sBad     = lipgloss.NewStyle().Foreground(colBad)
	sSel     = lipgloss.NewStyle().Background(colSelBg).Bold(true)
	sHeader  = lipgloss.NewStyle().Bold(true).Foreground(colWarn)
	sTabOn   = lipgloss.NewStyle().Bold(true).Foreground(colAccent).Underline(true).Padding(0, 1)
	sTabOff  = lipgloss.NewStyle().Foreground(colMuted).Padding(0, 1)
	sBorder  = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(colMuted)
	sConfirm = lipgloss.NewStyle().Bold(true).Foreground(colWarn)
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
	var body string
	switch m.screen {
	case scrBrowse:
		body = m.viewBrowse()
	case scrDetail:
		body = m.viewDetail()
	case scrEnv:
		body = m.viewEnv()
	case scrPaths:
		body = m.viewPaths()
	case scrExamSetup:
		body = m.viewExamSetup()
	case scrExam:
		body = m.viewExam()
	case scrExamResult:
		body = m.viewExamResult()
	}
	return body
}

func (m *model) header() string {
	ids := make([]string, 0, len(m.cat.Exercises))
	for _, e := range m.cat.Exercises {
		ids = append(ids, e.ID)
	}
	solved, started := m.prog.Counts(ids)
	left := sTitle.Render(" awesome_labs ") + sMuted.Render("hands-on labs")
	if m.exam != nil {
		left += "   " + sBigOKInline(m.exam)
	}
	right := sOK.Render(fmt.Sprintf("✔ %d", solved)) + sMuted.Render(fmt.Sprintf("/%d solved  ", len(ids))) +
		sWarn.Render(fmt.Sprintf("● %d", started)) + sMuted.Render(" in progress ")
	gap := max(1, m.w-lipgloss.Width(left)-lipgloss.Width(right))
	return left + strings.Repeat(" ", gap) + right
}

func (m *model) statusLine(help string) string {
	if m.confirm != "" {
		return sConfirm.Render(" " + m.confirm)
	}
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

// fitTabs joins tabs, showing a window around the active one when they don't all
// fit in width w. Hidden tabs on either side are marked with an arrow.
func fitTabs(tabs []string, active, w int) string {
	if lipgloss.Width(strings.Join(tabs, "")) <= w {
		return strings.Join(tabs, "")
	}
	const arrows = 4 // room for "‹ " and " ›"
	lo, hi := active, active
	used := lipgloss.Width(tabs[active])
	for grew := true; grew; {
		grew = false
		if hi+1 < len(tabs) && used+lipgloss.Width(tabs[hi+1]) <= w-arrows {
			hi++
			used += lipgloss.Width(tabs[hi])
			grew = true
		}
		if lo > 0 && used+lipgloss.Width(tabs[lo-1]) <= w-arrows {
			lo--
			used += lipgloss.Width(tabs[lo])
			grew = true
		}
	}
	bar := strings.Join(tabs[lo:hi+1], "")
	if lo > 0 {
		bar = sMuted.Render("‹ ") + bar
	}
	if hi < len(tabs)-1 {
		bar += sMuted.Render(" ›")
	}
	return bar
}

func (m *model) viewBrowse() string {
	var tabs []string
	for i, t := range m.tracks {
		label := trackLabel(t)
		if i == m.trackIdx {
			tabs = append(tabs, sTabOn.Render(label))
		} else {
			tabs = append(tabs, sTabOff.Render(label))
		}
	}
	bar := fitTabs(tabs, m.trackIdx, m.w)
	if m.path != nil {
		ps, pt := m.pathProgress(m.path)
		bar += sWarn.Render("  path: "+m.path.Title) + sMuted.Render(fmt.Sprintf(" %d/%d", ps, pt))
	}
	if m.unsolved {
		bar += sWarn.Render("  [unsolved only]")
	}
	if m.filterOn || m.filter.Value() != "" {
		bar += "   " + m.filter.View()
	}

	listW := min(max(46, m.w*45/100), m.w-20)
	prevW := m.w - listW - 4
	lh := m.listHeight()

	// list rows
	var rows []string
	for i := m.offset; i < len(m.visible) && len(rows) < lh; i++ {
		e := m.visible[i]
		if i == m.offset || e.Module != m.visible[i-1].Module || e.Track != m.visible[i-1].Track {
			rows = append(rows, sHeader.Render(trunc(fmt.Sprintf("▸ %s · %s", trackLabel(e.Track), e.Module), listW-2)))
			if len(rows) >= lh {
				break
			}
		}
		mark := m.prog.Get(e.ID).Mark()
		switch mark {
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

	help := "↑↓ move · enter open · / filter · tab track · n/p module · u unsolved · L paths · X exam · e envs · q quit · Q quit+stop VMs"
	return lipgloss.JoinVertical(lipgloss.Left,
		m.header(), bar,
		lipgloss.JoinHorizontal(lipgloss.Top, list, prevBox),
		m.statusLine(help))
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
	case !p.Started.IsZero():
		status = sWarn.Render("● in progress")
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
		if tab(i) == m.tab {
			tabs = append(tabs, sTabOn.Render(label))
		} else {
			tabs = append(tabs, sTabOff.Render(label))
		}
	}
	box := sBorder.Width(m.w - 2).Render(m.vp.View())
	help := "s start lab · t terminal"
	if e.Profile == exercise.ProfileK8s {
		help += " · w node01"
	}
	if m.examLocked(e) {
		help += " · c check · tab switch · n/N next/prev question · esc exam"
	} else {
		help += " · c check · h hint · v solution · R reset progress · tab switch · n/N next/prev · esc back"
	}
	return lipgloss.JoinVertical(lipgloss.Left, title, strings.Join(tabs, ""), box, m.statusLine(help))
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
	b.WriteString(m.header() + "\n\n")
	b.WriteString(sTitle.Render(" Lab environments") + "\n\n")
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
		var vms []string
		for _, v := range env.VMs {
			vms = append(vms, fmt.Sprintf("%s (%s, %d CPU, %s)", v.Name, v.Hostname, v.CPUs, v.Memory))
		}
		cur := "  "
		if i == m.envCursor {
			cur = sTitle.Render("▸ ")
		}
		tainted := ""
		if m.mgr.Tainted(env) {
			tainted = sWarn.Render("  (will be rebuilt before the next exercise)")
		}
		fmt.Fprintf(&b, "%s%-22s %s%s\n", cur, env.Title, stStyle.Render(st), tainted)
		fmt.Fprintf(&b, "     %s\n\n", sMuted.Render(strings.Join(vms, " · ")))
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
	help := "↑↓ select · u up/create · x stop · R reset · B rebuild · D delete · r refresh · esc back"
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

// sBigOKInline renders the exam countdown for the header.
func sBigOKInline(e *exam.Exam) string {
	left := e.Remaining()
	st := sOK
	if left < 10*time.Minute {
		st = sBad
	} else if left < 30*time.Minute {
		st = sWarn
	}
	p, n, _ := e.Score()
	return st.Bold(true).Render("EXAM "+exam.FormatDuration(left)) + sMuted.Render(fmt.Sprintf(" %d/%d · X", p, n))
}
