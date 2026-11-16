package tui

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/atotto/clipboard"
	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/muesli/termenv"

	"awesome_labs/internal/exercise"
)

// ---- copying code blocks (y) ----

// codeBlocks returns the fenced code blocks of a markdown text, in order.
func codeBlocks(md string) []string {
	var out []string
	var cur []string
	fence := ""
	for _, line := range strings.Split(md, "\n") {
		t := strings.TrimSpace(line)
		switch {
		case fence == "" && (strings.HasPrefix(t, "```") || strings.HasPrefix(t, "~~~")):
			fence, cur = t[:3], nil
		case fence != "" && strings.HasPrefix(t, fence):
			if b := strings.TrimRight(strings.Join(cur, "\n"), "\n "); strings.TrimSpace(b) != "" {
				out = append(out, b)
			}
			fence = ""
		case fence != "":
			cur = append(cur, line)
		}
	}
	return out
}

// tabSource is the markdown the current tab shows, or "" when it shows
// nothing that can be copied (locked, hidden, empty).
func (m *model) tabSource() string {
	e := m.cur
	switch m.tab {
	case tabScenario:
		return e.Scenario
	case tabLearn:
		if !m.examLocked(e) {
			return e.Tutorial
		}
	case tabHints:
		if !m.examLocked(e) {
			return strings.Join(e.Hints[:min(m.hintsShow, len(e.Hints))], "\n\n")
		}
	case tabSolution:
		if m.solShow && !m.examLocked(e) {
			return m.solutionText()
		}
	case tabNotes:
		return m.notes.Get(e.ID)
	}
	return ""
}

// copyNext copies the next code block of the current tab to the clipboard;
// pressing y again on the same tab moves to the block after it. The Output
// and Notes tabs, which have no blocks, copy all of their text.
func (m *model) copyNext() tea.Cmd {
	var blocks []string
	switch m.tab {
	case tabOutput:
		if b := m.output[m.cur.ID]; b != nil && b.Len() > 0 {
			blocks = []string{strings.TrimRight(b.String(), "\n")}
		}
	case tabNotes:
		if n := m.notes.Get(m.cur.ID); n != "" {
			blocks = []string{n}
		}
	default:
		blocks = codeBlocks(m.tabSource())
	}
	if len(blocks) == 0 {
		m.flash = "nothing to copy on this tab"
		return nil
	}
	key := fmt.Sprintf("%s:%d", m.cur.ID, m.tab)
	if key == m.copyKey {
		m.copyIdx = (m.copyIdx + 1) % len(blocks)
	} else {
		m.copyKey, m.copyIdx = key, 0
	}
	b := blocks[m.copyIdx]
	if err := m.clip(b); err != nil {
		m.flash = "copy: " + err.Error()
		return nil
	}
	first := strings.SplitN(b, "\n", 2)[0]
	if len(blocks) > 1 {
		m.flash = fmt.Sprintf("copied block %d/%d: %s · y copies the next", m.copyIdx+1, len(blocks), trunc(first, 50))
	} else {
		m.flash = "copied: " + trunc(first, 60)
	}
	return nil
}

// copyToClipboard puts s on the clipboard: through the system's clipboard
// tool, or, over SSH or without one, through the terminal (OSC 52), which
// most terminals pass on to the computer in front of you.
func copyToClipboard(s string) error {
	if os.Getenv("SSH_CONNECTION") == "" {
		if err := clipboard.WriteAll(s); err == nil {
			return nil
		}
	}
	termenv.Copy(s)
	return nil
}

// ---- your commands, beside the solution ----

// solutionText is the solution, followed by the commands you typed for it.
func (m *model) solutionText() string {
	s := m.cur.Solution
	if h := strings.TrimSpace(m.mgr.History(m.cur.ID)); h != "" {
		s += "\n\n---\n\n### Your commands\n\n```bash\n" + h + "\n```\n"
	} else {
		s += "\n\n---\n\n*Your commands show up here: shells opened from awesome_labs (t, or VS Code's terminal) keep a history per exercise, copied over at each check.*\n"
	}
	return s
}

type historyMsg struct{ id string }

// cmdFetchHistory copies ex's shell history from its VMs in the background.
func (m *model) cmdFetchHistory(ex *exercise.Exercise) tea.Cmd {
	if ex == nil || m.fetchHistory == nil {
		return nil
	}
	return func() tea.Msg {
		m.fetchHistory(ex)
		return historyMsg{ex.ID}
	}
}

// ---- notes ----

func newNoteArea() textarea.Model {
	ta := textarea.New()
	ta.Placeholder = "Your notes on this exercise: commands worth remembering, what tripped you up…"
	ta.ShowLineNumbers = false
	ta.CharLimit = 0
	return ta
}

// editNote starts editing the open exercise's note.
func (m *model) editNote() tea.Cmd {
	m.note.SetValue(m.notes.Get(m.cur.ID))
	m.note.SetWidth(max(20, m.w-6))
	m.note.SetHeight(max(3, m.h-8))
	m.noteEdit = true
	m.flash = "editing notes — esc saves"
	return m.note.Focus()
}

// noteKey handles a key while the note is edited: esc (or ctrl+s) saves.
func (m *model) noteKey(msg tea.KeyMsg) tea.Cmd {
	switch msg.String() {
	case "esc", "ctrl+s":
		if err := m.notes.Set(m.cur.ID, m.note.Value()); err != nil {
			m.flash = "notes not saved: " + err.Error()
			return nil
		}
		if msg.String() == "esc" {
			m.noteEdit = false
			m.note.Blur()
		}
		m.flash = "notes saved"
		m.refreshViewport(false)
		return nil
	case "ctrl+c":
		return nil // handled by the app: never lose the note to a stray key
	}
	var c tea.Cmd
	m.note, c = m.note.Update(msg)
	return c
}

// ---- what to do next ----

// nextUnsolved is the first unsolved exercise after the open one in the list
// it came from, or nil.
func (m *model) nextUnsolved() *exercise.Exercise {
	list, _ := m.siblings()
	after := false
	for _, e := range list {
		if e == nil {
			continue
		}
		if e == m.cur {
			after = true
			continue
		}
		if after && m.prog.Get(e.ID).Solved.IsZero() {
			return e
		}
	}
	return nil
}

// ---- full-text search ----

// fullTextOf is the lower-cased scenario, tutorial and hints of e.
func (m *model) fullTextOf(e *exercise.Exercise) string {
	if s, ok := m.textIndex[e.ID]; ok {
		return s
	}
	s := strings.ToLower(e.Scenario + "\n" + e.Tutorial + "\n" + strings.Join(e.Hints, "\n"))
	m.textIndex[e.ID] = s
	return s
}

func (m *model) filterPlaceholder() string {
	if m.fullText {
		return "search the full text: scenarios, tutorials, hints… (ctrl+t: titles only)"
	}
	return "type to filter (id, title, tag, module)… ctrl+t searches the full text"
}

// ---- weak spots ----

// weakSpots ranks the modules where you needed the most help: hints,
// solutions and failed checks, over the exercises you tried.
func weakSpots(cat *exercise.Catalog, get func(string) (hints, checks int, sawSolution, solved, started bool)) string {
	type mod struct {
		name                                  string
		tried, solved, hints, sols, extraChks int
		retry                                 []string
	}
	mods := map[string]*mod{}
	var order []string
	for _, e := range cat.Exercises {
		h, c, sol, ok, st := get(e.ID)
		if !st && !ok && h == 0 && !sol {
			continue
		}
		k := trackName(e.Track) + " · " + e.Module
		md := mods[k]
		if md == nil {
			md = &mod{name: k}
			mods[k] = md
			order = append(order, k)
		}
		md.tried++
		md.hints += h
		if sol {
			md.sols++
			md.retry = append(md.retry, e.ID)
		}
		if ok {
			md.solved++
			md.extraChks += max(0, c-1)
		} else {
			md.extraChks += c
		}
	}
	if len(order) == 0 {
		return "Weak spots\n\nNothing to go on yet: this fills in as you solve exercises,\nfrom the hints, solutions and failed checks each one took."
	}
	score := func(md *mod) float64 {
		return float64(md.hints+3*md.sols+md.extraChks+2*(md.tried-md.solved)) / float64(md.tried)
	}
	sort.SliceStable(order, func(i, j int) bool { return score(mods[order[i]]) > score(mods[order[j]]) })
	var b strings.Builder
	b.WriteString("Weak spots — where you needed the most help\n\n")
	b.WriteString(fmt.Sprintf("  %-44s %7s %6s %10s %8s\n", "Module", "solved", "hints", "solutions", "retries"))
	shown := 0
	for _, k := range order {
		md := mods[k]
		if score(md) == 0 {
			continue
		}
		b.WriteString(fmt.Sprintf("  %-44s %3d/%-3d %6d %10d %8d\n", trunc(md.name, 44), md.solved, md.tried, md.hints, md.sols, md.extraChks))
		if shown++; shown == 10 {
			break
		}
	}
	if shown == 0 {
		b.WriteString("  No weak spots: every exercise you tried passed first time, without help.\n")
	}
	var retry []string
	for _, k := range order {
		retry = append(retry, mods[k].retry...)
	}
	if len(retry) > 0 {
		if len(retry) > 12 {
			retry = retry[:12]
		}
		b.WriteString("\nWorth doing again without the solution (R resets one):\n  " + strings.Join(retry, " · ") + "\n")
	}
	return b.String()
}

func (m *model) weakSpotsText() string {
	return weakSpots(m.cat, func(id string) (int, int, bool, bool, bool) {
		p := m.prog.Get(id)
		return p.HintsUsed, p.Checks, p.SawSolution, !p.Solved.IsZero(), !p.Started.IsZero()
	})
}
