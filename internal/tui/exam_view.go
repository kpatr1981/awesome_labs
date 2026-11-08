package tui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"kubelab/internal/exam"
	"kubelab/internal/exercise"
	"kubelab/internal/lab"
)

var (
	examCounts  = []int{5, 10, 15, 20, 30}
	examMinutes = []int{30, 60, 90, 120, 180}
	sBigOK      = lipgloss.NewStyle().Bold(true).Foreground(colOK).Padding(0, 1).Border(lipgloss.ThickBorder()).BorderForeground(colOK)
	sBigBad     = lipgloss.NewStyle().Bold(true).Foreground(colBad).Padding(0, 1).Border(lipgloss.ThickBorder()).BorderForeground(colBad)
)

// ---------------- learning paths ----------------

func (m *model) pathProgress(p *exercise.Path) (solved, total int) {
	ids := make([]string, len(p.Exercises))
	for i, e := range p.Exercises {
		ids[i] = e.ID
	}
	s, _ := m.prog.Counts(ids)
	return s, len(ids)
}

func bar(done, total, width int) string {
	if total == 0 {
		return ""
	}
	n := done * width / total
	return sOK.Render(strings.Repeat("█", n)) + sMuted.Render(strings.Repeat("░", width-n))
}

func (m *model) updatePaths(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	n := len(m.paths) + 1 // last row = "all exercises"
	switch msg.String() {
	case "esc", "q", "L":
		m.screen = scrBrowse
	case "up", "k":
		m.pathCursor = (m.pathCursor + n - 1) % n
	case "down", "j":
		m.pathCursor = (m.pathCursor + 1) % n
	case "enter":
		if m.pathCursor == len(m.paths) {
			m.path = nil
		} else {
			m.path = m.paths[m.pathCursor]
		}
		m.trackIdx, m.cursor, m.offset = 0, 0, 0
		m.refilter()
		m.screen = scrBrowse
	}
	return m, nil
}

func (m *model) viewPaths() string {
	var b strings.Builder
	b.WriteString(m.header() + "\n\n")
	b.WriteString(sTitle.Render(" Learning paths") + sMuted.Render("  — pick one to focus the exercise list on it") + "\n\n")
	for i, p := range m.paths {
		s, t := m.pathProgress(p)
		cur := "  "
		if i == m.pathCursor {
			cur = sTitle.Render("▸ ")
		}
		active := ""
		if m.path == p {
			active = sWarn.Render("  (active)")
		}
		fmt.Fprintf(&b, "%s%s %s %s%s\n", cur, padRight(sTitle.Render(p.Title), 42), bar(s, t, 24), sMuted.Render(fmt.Sprintf("%3d/%-3d", s, t)), active)
		fmt.Fprintf(&b, "    %s\n\n", sMuted.Render(p.Description))
	}
	cur := "  "
	if m.pathCursor == len(m.paths) {
		cur = sTitle.Render("▸ ")
	}
	fmt.Fprintf(&b, "%s%s\n\n", cur, sTitle.Render("All exercises"))
	return b.String() + m.statusLine("↑↓ select · enter apply · esc back")
}

// ---------------- exam setup ----------------

type examSource struct {
	label string
	pool  []*exercise.Exercise
}

// examSources lists single-environment pools: whole tracks and every path
// (paths that mix Docker and Kubernetes are split per environment).
func (m *model) examSources() []examSource {
	byProfile := func(list []*exercise.Exercise) map[exercise.Profile][]*exercise.Exercise {
		r := map[exercise.Profile][]*exercise.Exercise{}
		for _, e := range list {
			r[e.Profile] = append(r[e.Profile], e)
		}
		return r
	}
	all := byProfile(m.cat.Exercises)
	src := []examSource{
		{"Kubernetes — everything", all[exercise.ProfileK8s]},
		{"Docker — everything", all[exercise.ProfileDocker]},
	}
	for _, p := range m.paths {
		g := byProfile(p.Exercises)
		if len(g) == 1 {
			for _, list := range g {
				src = append(src, examSource{p.Title, list})
			}
			continue
		}
		for _, prof := range []exercise.Profile{exercise.ProfileK8s, exercise.ProfileDocker} {
			if len(g[prof]) > 0 {
				src = append(src, examSource{fmt.Sprintf("%s (%s)", p.Title, lab.EnvFor(prof).Title), g[prof]})
			}
		}
	}
	return src
}

func (m *model) updateExamSetup(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	srcs := m.examSources()
	step := func(d int) {
		switch m.setupField {
		case 0:
			m.setupSource = (m.setupSource + len(srcs) + d) % len(srcs)
		case 1:
			m.setupCount = (m.setupCount + len(examCounts) + d) % len(examCounts)
		case 2:
			m.setupMins = (m.setupMins + len(examMinutes) + d) % len(examMinutes)
		case 3:
			m.setupHints = !m.setupHints
		}
	}
	switch msg.String() {
	case "esc", "q":
		m.screen = scrBrowse
	case "up", "k":
		m.setupField = (m.setupField + 3) % 4
	case "down", "j", "tab":
		m.setupField = (m.setupField + 1) % 4
	case "left", "h":
		step(-1)
	case "right", "l", " ":
		step(1)
	case "enter":
		s := srcs[m.setupSource]
		e, err := exam.New(exam.Options{
			Title:      fmt.Sprintf("%s — %d questions", s.label, examCounts[m.setupCount]),
			Source:     s.label,
			Pool:       s.pool,
			Count:      examCounts[m.setupCount],
			Duration:   time.Duration(examMinutes[m.setupMins]) * time.Minute,
			AllowHints: m.setupHints,
		})
		if err != nil {
			m.flash = "failed: " + err.Error()
			return m, nil
		}
		if err := m.exams.Save(e); err != nil {
			m.flash = "failed: " + err.Error()
			return m, nil
		}
		m.exam, m.examCursor = e, 0
		m.screen = scrExam
		m.flash = "exam started — good luck! open a question with enter"
	}
	return m, nil
}

func (m *model) viewExamSetup() string {
	srcs := m.examSources()
	var b strings.Builder
	b.WriteString(m.header() + "\n\n")
	b.WriteString(sTitle.Render(" New exam") + "\n\n")
	rows := []struct{ label, value string }{
		{"Pool", fmt.Sprintf("%s  (%d exercises)", srcs[m.setupSource].label, len(srcs[m.setupSource].pool))},
		{"Questions", fmt.Sprint(examCounts[m.setupCount])},
		{"Time limit", fmt.Sprintf("%d minutes", examMinutes[m.setupMins])},
		{"Hints & solutions", map[bool]string{true: "allowed", false: "locked (exam conditions)"}[m.setupHints]},
	}
	for i, r := range rows {
		cur, val := "  ", sMuted.Render(r.value)
		if i == m.setupField {
			cur, val = sTitle.Render("▸ "), sSel.Render(" ◀ "+r.value+" ▶ ")
		}
		fmt.Fprintf(&b, "%s%-20s %s\n", cur, r.label, val)
	}
	b.WriteString("\n" + sMuted.Render(strings.Join([]string{
		"  How it works",
		"  • Questions are drawn at random: about a third easy, a third medium, a third hard.",
		"  • Each question gets a fresh lab when you start it (s) — finish one before moving on.",
		"  • Your last check (c) per question counts. Starting a question again (s) resets its lab.",
		fmt.Sprintf("  • Pass mark: %.0f%%. When time runs out, checking stops and you get your result.", exam.PassMark*100),
	}, "\n")) + "\n\n")
	return b.String() + m.statusLine("↑↓ field · ←→ change · enter start · esc back")
}

// ---------------- running exam ----------------

func (m *model) updateExam(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	e := m.exam
	if e == nil {
		m.screen = scrBrowse
		return m, nil
	}
	switch msg.String() {
	case "esc", "q":
		m.screen = scrBrowse
		m.flash = "exam still running — press X to return to it"
	case "up", "k":
		m.examCursor = max(0, m.examCursor-1)
	case "down", "j":
		m.examCursor = min(len(e.Questions)-1, m.examCursor+1)
	case "enter":
		if ex := m.cat.Get(e.Questions[m.examCursor].ID); ex != nil {
			m.openDetail(ex)
			m.fromExam = true
		}
	case "F":
		m.confirm = "Finish the exam now and see your result? (y/n)"
		m.onYes = func() tea.Cmd {
			m.finishExam()
			m.screen = scrExamResult
			return nil
		}
	}
	return m, nil
}

func (m *model) finishExam() {
	if m.exam == nil {
		return
	}
	_ = m.exams.Finish(m.exam)
	m.finished, m.exam = m.exam, nil
}

func (m *model) questionMark(q exam.Question) string {
	switch {
	case q.Passed:
		return sOK.Render("✔")
	case q.Checks > 0:
		return sBad.Render("✘")
	}
	return sMuted.Render("·")
}

func (m *model) viewExam() string {
	e := m.exam
	var b strings.Builder
	b.WriteString(m.header() + "\n\n")
	p, n, _ := e.Score()
	left := e.Remaining()
	ts := sOK
	if left < 10*time.Minute {
		ts = sBad
	} else if left < 30*time.Minute {
		ts = sWarn
	}
	fmt.Fprintf(&b, " %s   %s %s   %s\n\n", sTitle.Render(e.Title), sMuted.Render("time left"), ts.Bold(true).Render(exam.FormatDuration(left)),
		sMuted.Render(fmt.Sprintf("passing so far: %d/%d", p, n)))
	for i, q := range e.Questions {
		ex := m.cat.Get(q.ID)
		if ex == nil {
			continue
		}
		line := fmt.Sprintf(" %2d. %s %-9s %s  %s", i+1, m.questionMark(q), q.ID, stars(ex.Difficulty), trunc(ex.Title, m.w-40))
		if i == m.examCursor {
			line = sSel.Render(padRight(line, m.w-2))
		}
		b.WriteString(line + "\n")
	}
	b.WriteString("\n")
	return b.String() + m.statusLine("↑↓ move · enter open question · F finish exam · esc back to browser (timer keeps running)")
}

// ---------------- result ----------------

func (m *model) updateExamResult(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	e := m.finished
	switch msg.String() {
	case "esc", "q", "enter":
		m.screen = scrBrowse
	case "up", "k":
		m.examCursor = max(0, m.examCursor-1)
	case "down", "j":
		if e != nil {
			m.examCursor = min(len(e.Questions)-1, m.examCursor+1)
		}
	case "o":
		if e != nil {
			if ex := m.cat.Get(e.Questions[m.examCursor].ID); ex != nil {
				m.openDetail(ex)
			}
		}
	}
	return m, nil
}

func (m *model) viewExamResult() string {
	e := m.finished
	var b strings.Builder
	b.WriteString(m.header() + "\n\n")
	if e == nil {
		return b.String() + m.statusLine("esc back")
	}
	p, n, sc := e.Score()
	verdict := sBigBad.Render(fmt.Sprintf("NOT PASSED  %d/%d · %.0f%%", p, n, sc*100))
	if e.Pass() {
		verdict = sBigOK.Render(fmt.Sprintf("PASSED  %d/%d · %.0f%%", p, n, sc*100))
	}
	fmt.Fprintf(&b, " %s\n%s\n\n", sTitle.Render(e.Title), lipgloss.NewStyle().MarginLeft(1).Render(verdict))
	fmt.Fprintf(&b, " %s\n\n", sMuted.Render(fmt.Sprintf("time used %s of %s · pass mark %.0f%%", exam.FormatDuration(e.TimeUsed()), exam.FormatDuration(e.Duration), exam.PassMark*100)))
	for i, q := range e.Questions {
		ex := m.cat.Get(q.ID)
		if ex == nil {
			continue
		}
		res := sBad.Render("not solved")
		if q.Passed {
			res = sOK.Render("solved")
		} else if q.Checks == 0 {
			res = sMuted.Render("not attempted")
		}
		line := fmt.Sprintf(" %2d. %s %-9s %s  %-46s %s", i+1, m.questionMark(q), q.ID, stars(ex.Difficulty), trunc(ex.Title, 46), res)
		if i == m.examCursor {
			line = sSel.Render(padRight(line, m.w-2))
		}
		b.WriteString(line + "\n")
	}
	if hist := m.exams.History(); len(hist) > 1 {
		best := 0.0
		for _, h := range hist[:len(hist)-1] {
			if h.Source == e.Source {
				if _, _, s := h.Score(); s > best {
					best = s
				}
			}
		}
		if best > 0 {
			fmt.Fprintf(&b, "\n %s\n", sMuted.Render(fmt.Sprintf("your previous best on this pool: %.0f%%", best*100)))
		}
	}
	b.WriteString("\n")
	return b.String() + m.statusLine("↑↓ move · o open question (solutions unlocked) · enter back to the browser")
}
