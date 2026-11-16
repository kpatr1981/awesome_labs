package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func answerYes(t *testing.T, m *model) {
	t.Helper()
	if m.confirm == "" {
		t.Fatalf("no confirmation asked (flash %q)", m.flash)
	}
	m.Update(keyMsg("y"))
	if m.confirm != "" {
		t.Fatalf("confirmation still open: %q", m.confirm)
	}
}

func TestResetMarkedFromTheList(t *testing.T) {
	m := newTestModel(t)
	m.op = ""
	for _, id := range []string{"d-1", "d-2", "k-1"} {
		m.prog.MarkStarted(id)
	}
	m.setProgFilter(progInProgress)
	if len(m.visible) != 3 {
		t.Fatalf("setup: %v", m.visible)
	}
	// mark d-1 and k-1 (space moves down, so skip d-2 with down)
	m.cursor = 0
	keys(m, " ", "down", " ")
	if !m.marked["d-1"] || m.marked["d-2"] || !m.marked["k-1"] {
		t.Fatalf("marks = %v", m.marked)
	}
	out := ansi.Strip(m.View())
	if !strings.Contains(out, "[2 marked]") || !strings.Contains(out, "*● d-1") {
		t.Errorf("marks not shown:\n%s", out)
	}
	keys(m, "R")
	if !strings.Contains(m.confirm, "2 marked") {
		t.Fatalf("confirm = %q", m.confirm)
	}
	answerYes(t, m)
	if !m.prog.Get("d-1").IsZero() || !m.prog.Get("k-1").IsZero() {
		t.Error("marked exercises kept their progress")
	}
	if m.prog.Get("d-2").IsZero() {
		t.Error("unmarked d-2 lost its progress")
	}
	if len(m.marked) != 0 || len(m.visible) != 1 || m.visible[0].ID != "d-2" {
		t.Errorf("after reset: marks %v, list %v", m.marked, m.visible)
	}
}

func TestResetWithoutMarksTakesTheHighlightedOne(t *testing.T) {
	m := newTestModel(t)
	m.op = ""
	m.prog.MarkStarted("d-1")
	m.prog.MarkStarted("d-2")
	m.setProgFilter(progInProgress)
	m.cursor = 1
	keys(m, "R")
	answerYes(t, m)
	if m.prog.Get("d-1").IsZero() || !m.prog.Get("d-2").IsZero() {
		t.Errorf("R reset the wrong one: d-1 %v d-2 %v", m.prog.Get("d-1"), m.prog.Get("d-2"))
	}
}

func TestMarkAllShownAndResetAll(t *testing.T) {
	m := newTestModel(t)
	m.op = ""
	m.prog.MarkStarted("d-1")
	m.prog.RecordCheck("d-2", true)
	m.prog.MarkStarted("k-1")
	keys(m, "tab") // docker only: k-1 is out of sight
	keys(m, "*")
	if len(m.marked) != 2 || !m.marked["d-1"] || !m.marked["d-2"] {
		t.Fatalf("* marked %v", m.marked)
	}
	keys(m, "*") // again: clear
	if len(m.marked) != 0 {
		t.Fatalf("second * left %v", m.marked)
	}
	m.askResetShown()
	if !strings.Contains(m.confirm, "all 2") {
		t.Fatalf("confirm = %q", m.confirm)
	}
	answerYes(t, m)
	if !m.prog.Get("d-1").IsZero() || !m.prog.Get("d-2").IsZero() {
		t.Error("shown exercises kept their progress")
	}
	if m.prog.Get("k-1").IsZero() {
		t.Error("k-1, hidden by the track, lost its progress")
	}
}

func TestMarksNeverCoverHiddenRows(t *testing.T) {
	m := newTestModel(t)
	m.op = ""
	m.prog.MarkStarted("d-1")
	m.prog.MarkStarted("k-1")
	keys(m, "*")
	if len(m.marked) != 2 {
		t.Fatalf("setup: %v", m.marked)
	}
	keys(m, "tab") // docker only
	if m.marked["k-1"] || !m.marked["d-1"] {
		t.Errorf("marks after the track hid k-1: %v", m.marked)
	}
	keys(m, "esc")
	if len(m.marked) != 0 {
		t.Errorf("esc kept marks %v", m.marked)
	}
}

func TestMarkRefusesWhatHasNothingToReset(t *testing.T) {
	m := newTestModel(t)
	m.op = ""
	m.cursor = 0
	id := m.visible[0].ID
	keys(m, " ")
	if m.marked[id] || !strings.Contains(m.flash, "nothing to reset") {
		t.Errorf("marked %s with no progress: marks %v, flash %q", id, m.marked, m.flash)
	}
	m.runningExam("d-1")
	m.prog.MarkStarted("d-1")
	m.refilter()
	m.selectID("d-1")
	keys(m, " ")
	if m.marked["d-1"] {
		t.Error("marked a question of the running exam")
	}
}

func TestResetFromTheListWaitsWhileBusy(t *testing.T) {
	m := newTestModel(t) // busy
	m.prog.MarkStarted("d-1")
	m.refilter()
	keys(m, "R")
	if m.confirm != "" || !strings.Contains(m.flash, "busy") {
		t.Errorf("R while busy: confirm %q, flash %q", m.confirm, m.flash)
	}
}
