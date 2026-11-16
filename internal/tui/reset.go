package tui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"

	"awesome_labs/internal/exercise"
)

// Resetting progress from the exercise list: space marks rows, * marks every
// row shown, and R resets the marked rows — or the highlighted one when none
// is marked. Marks only ever cover rows on screen: a filter that hides a row
// drops its mark, so a reset never touches something out of sight.

// resettable reports whether ex's progress may be erased now.
func (m *model) resettable(ex *exercise.Exercise) bool {
	return m.hasProgress(ex) && !m.inExam(ex)
}

// toggleMark marks or unmarks the highlighted row and moves to the next.
func (m *model) toggleMark() {
	if len(m.visible) == 0 {
		return
	}
	ex := m.visible[m.cursor]
	switch {
	case m.marked[ex.ID]:
		delete(m.marked, ex.ID)
	case m.inExam(ex):
		m.flash = "progress can't be reset during an exam on " + ex.ID
		return
	case !m.hasProgress(ex):
		m.flash = "no progress recorded for " + ex.ID + " — nothing to reset"
		return
	default:
		if m.marked == nil {
			m.marked = map[string]bool{}
		}
		m.marked[ex.ID] = true
	}
	m.move(1)
}

// markAllShown marks every row shown that has progress to reset; when all of
// them are marked already it clears the marks instead.
func (m *model) markAllShown() {
	var can []string
	for _, e := range m.visible {
		if m.resettable(e) {
			can = append(can, e.ID)
		}
	}
	if len(can) == 0 {
		m.flash = "nothing shown has progress to reset"
		return
	}
	all := true
	for _, id := range can {
		all = all && m.marked[id]
	}
	if all {
		m.marked = nil
		m.flash = "marks cleared"
		return
	}
	m.marked = map[string]bool{}
	for _, id := range can {
		m.marked[id] = true
	}
	m.flash = fmt.Sprintf("%d marked — R resets their progress", len(can))
}

// pruneMarks drops the marks of rows the list no longer shows.
func (m *model) pruneMarks() {
	if len(m.marked) == 0 {
		return
	}
	shown := make(map[string]bool, len(m.visible))
	for _, e := range m.visible {
		shown[e.ID] = true
	}
	for id := range m.marked {
		if !shown[id] {
			delete(m.marked, id)
		}
	}
	if len(m.marked) == 0 {
		m.marked = nil
	}
}

// markedShown returns the marked exercises in list order.
func (m *model) markedShown() []*exercise.Exercise {
	var out []*exercise.Exercise
	for _, e := range m.visible {
		if m.marked[e.ID] {
			out = append(out, e)
		}
	}
	return out
}

// askResetList offers to reset the marked rows, or the highlighted one.
func (m *model) askResetList() {
	if m.op != "" {
		m.flash = "busy: " + m.op
		return
	}
	exs := m.markedShown()
	if len(exs) == 0 {
		if len(m.visible) == 0 {
			return
		}
		ex := m.visible[m.cursor]
		m.askForget(ex, "Reset progress on "+ex.ID+"? Your notes are kept.", func() {
			m.flash = "reset " + ex.ID
		})
		return
	}
	q := fmt.Sprintf("Reset progress on the %d marked exercises? Your notes are kept.", len(exs))
	if len(exs) == 1 {
		q = "Reset progress on " + exs[0].ID + "? Your notes are kept."
	}
	m.ask(q, func() tea.Cmd {
		n := m.forgetAll(exs)
		m.marked = nil
		m.flash = fmt.Sprintf("reset %d exercises", n)
		if n == 1 {
			m.flash = "reset " + exs[0].ID
		}
		return nil
	}, nil)
}

// askResetShown offers to reset every row shown that has progress.
func (m *model) askResetShown() tea.Cmd {
	if m.op != "" {
		m.flash = "busy: " + m.op
		return nil
	}
	var exs []*exercise.Exercise
	for _, e := range m.visible {
		if m.resettable(e) {
			exs = append(exs, e)
		}
	}
	if len(exs) == 0 {
		m.flash = "nothing shown has progress to reset"
		return nil
	}
	m.ask(fmt.Sprintf("Reset progress on all %d exercises shown? Your notes are kept.", len(exs)), func() tea.Cmd {
		n := m.forgetAll(exs)
		m.marked = nil
		m.flash = fmt.Sprintf("reset %d exercises", n)
		return nil
	}, nil)
	return nil
}

// forgetAll erases the progress of exs, skipping questions of a running
// exam, and returns how many it reset.
func (m *model) forgetAll(exs []*exercise.Exercise) int {
	n := 0
	for _, e := range exs {
		if m.inExam(e) {
			continue
		}
		m.prog.Reset(e.ID)
		m.mgr.ForgetHistory(e.ID)
		delete(m.result, e.ID)
		n++
	}
	m.refilterKeep() // the progress filter may no longer keep them
	return n
}
