package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func keys(m *model, ks ...string) {
	for _, k := range ks {
		msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(k)}
		switch k {
		case "tab":
			msg = tea.KeyMsg{Type: tea.KeyTab}
		case "down":
			msg = tea.KeyMsg{Type: tea.KeyDown}
		case "enter":
			msg = tea.KeyMsg{Type: tea.KeyEnter}
		case "esc":
			msg = tea.KeyMsg{Type: tea.KeyEsc}
		}
		m.Update(msg)
	}
}

func TestListComesBackAfterRelaunch(t *testing.T) {
	dir := t.TempDir()
	m := newTestModelIn(t, dir)
	keys(m, "tab", "down") // docker track, second exercise
	keys(m, "/", "t", "a", "s", "k", "enter", "down")
	if got := m.visible[m.cursor].ID; got != "d-2" {
		t.Fatalf("setup: cursor on %s, want d-2", got)
	}

	r := newTestModelIn(t, dir)
	if r.tracks[r.trackIdx] != "docker" {
		t.Errorf("track = %q, want docker", r.tracks[r.trackIdx])
	}
	if r.filter.Value() != "task" {
		t.Errorf("filter = %q, want task", r.filter.Value())
	}
	if len(r.visible) == 0 || r.visible[r.cursor].ID != "d-2" {
		t.Errorf("cursor not back on d-2: %v", r.visible)
	}
}

func TestProgressFilterComesBackAfterRelaunch(t *testing.T) {
	dir := t.TempDir()
	m := newTestModelIn(t, dir)
	m.prog.MarkStarted("k-1")
	keys(m, "u", "u") // "In progress"
	if m.progFilter != progInProgress {
		t.Fatalf("setup: filter %v", m.progFilter)
	}
	r := newTestModelIn(t, dir)
	if r.progFilter != progInProgress || len(r.visible) != 1 || r.visible[0].ID != "k-1" {
		t.Errorf("progress filter not restored: %v %v", r.progFilter, r.visible)
	}
}

func TestCursorStaysOnTheExerciseWhenTheListChanges(t *testing.T) {
	m := newTestModel(t)
	m.prog.RecordCheck("k-1", true)
	keys(m, "u") // "Solved": only k-1
	keys(m, "enter")
	if m.cur == nil || m.cur.ID != "k-1" {
		t.Fatalf("setup: opened %v", m.cur)
	}
	// d-2 gets solved meanwhile (say from the command line) and a check
	// refilters: d-2 sorts before k-1, but the cursor must stay on k-1
	m.prog.RecordCheck("d-2", true)
	m.refilterKeep()
	keys(m, "esc")
	if got := m.visible[m.cursor].ID; got != "k-1" {
		t.Errorf("cursor on %s after the list changed, want k-1", got)
	}
}
