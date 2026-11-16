package tui

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"awesome_labs/internal/exercise"
)

func TestCodeBlocks(t *testing.T) {
	md := "Run this:\n\n```bash\nkubectl get pods\nkubectl get svc\n```\n\nthen\n\n~~~\necho hi\n~~~\n\n```\n```\n"
	got := codeBlocks(md)
	if len(got) != 2 || got[0] != "kubectl get pods\nkubectl get svc" || got[1] != "echo hi" {
		t.Errorf("codeBlocks = %q", got)
	}
}

func TestYCopiesEachCodeBlockInTurn(t *testing.T) {
	m := build(t, named("exercise set up on the VMs"), false)
	m.cur.Scenario = "First:\n\n```\none\n```\n\nThen:\n\n```\ntwo\n```"
	var copied []string
	m.clip = func(s string) error { copied = append(copied, s); return nil }
	for range 3 {
		m.Update(keyMsg("y"))
	}
	if strings.Join(copied, ",") != "one,two,one" {
		t.Errorf("copied %q", copied)
	}
	if !strings.Contains(m.flash, "block 1/2") {
		t.Errorf("flash %q", m.flash)
	}
	// a locked solution copies nothing
	copied = nil
	m.setTab(tabSolution)
	m.Update(keyMsg("y"))
	if len(copied) != 0 || !strings.Contains(m.flash, "nothing to copy") {
		t.Errorf("hidden solution: copied %q, flash %q", copied, m.flash)
	}
}

func TestNotesAreEditedAndKept(t *testing.T) {
	m := build(t, named("exercise set up on the VMs"), false)
	m.Update(keyMsg("6"))
	if m.tab != tabNotes {
		t.Fatalf("6: tab %v", m.tab)
	}
	m.Update(keyMsg("enter"))
	if !m.noteEdit {
		t.Fatal("enter didn't start editing")
	}
	for _, k := range []string{"u", "s", "e", " ", "-", "o"} {
		m.Update(keyMsg(k)) // u, s, e… would be commands outside the editor
	}
	if m.screen != scrDetail || m.op != "" {
		t.Fatalf("typing in the note ran commands: screen %v op %q", m.screen, m.op)
	}
	m.Update(keyMsg("esc"))
	if m.noteEdit || m.screen != scrDetail {
		t.Fatalf("esc: editing %v, screen %v", m.noteEdit, m.screen)
	}
	if got := m.notes.Get("d-1"); got != "use -o" {
		t.Errorf("note = %q", got)
	}
	if v := m.View(); !strings.Contains(v, "Notes ✎") {
		t.Error("the Notes tab doesn't show that there is a note")
	}
}

func TestAPassSuggestsTheNextUnsolvedExercise(t *testing.T) {
	m := build(t, named("exercise set up on the VMs"), false)
	m.prog.RecordCheck("d-2", true) // solved already: skipped
	m.Update(opDoneMsg{kind: "check", id: "d-1", passed: true})
	if m.suggest == nil || m.suggest.ID != "k-1" {
		t.Fatalf("suggest = %v, want k-1", m.suggest)
	}
	if !strings.Contains(m.flash, "next up: k-1") {
		t.Errorf("flash %q", m.flash)
	}
	m.Update(keyMsg("enter"))
	if m.screen != scrDetail || m.cur.ID != "k-1" {
		t.Errorf("enter: screen %v cur %s, want k-1 open", m.screen, m.cur.ID)
	}
}

func TestFullTextSearch(t *testing.T) {
	m := newTestModel(t)
	m.op = ""
	m.ex(t, "d-2").Tutorial = "Uses iptables to drop packets."
	m.Update(keyMsg("/"))
	for _, r := range "iptables" {
		m.Update(keyMsg(string(r)))
	}
	if len(m.visible) != 0 {
		t.Fatalf("titles-only search matched %d", len(m.visible))
	}
	m.Update(tea.KeyMsg{Type: tea.KeyCtrlT})
	if len(m.visible) != 1 || m.visible[0].ID != "d-2" {
		t.Errorf("full-text search: %d matches", len(m.visible))
	}
}

func TestWeakSpots(t *testing.T) {
	m := newTestModel(t)
	if !strings.Contains(m.weakSpotsText(), "Nothing to go on yet") {
		t.Error("empty progress should say there's nothing yet")
	}
	m.prog.MarkStarted("d-2")
	m.prog.UseHint("d-2", 2)
	m.prog.SawSolution("d-2")
	m.prog.RecordCheck("d-2", false)
	m.prog.RecordCheck("k-1", true)
	s := m.weakSpotsText()
	if !strings.Contains(s, "Docker · Basics") || !strings.Contains(s, "d-2") {
		t.Errorf("weak spots:\n%s", s)
	}
	if strings.Contains(s, "Kubernetes · Pods") {
		t.Errorf("a first-time pass counted as a weak spot:\n%s", s)
	}
}

func TestOnExitStopKeepsTheLab(t *testing.T) {
	m := build(t, named("exercise set up on the VMs"), false)
	m.onExit = "stop"
	stopped, deleted := false, false
	m.stopVMs = func(context.Context, io.Writer) error { stopped = true; return nil }
	m.deleteVMs = func(context.Context, io.Writer) error { deleted = true; return nil }
	c := m.quitAndStop()
	c()
	if !stopped || deleted {
		t.Errorf("on_exit stop: stopped %v deleted %v", stopped, deleted)
	}
	if !m.hasActive {
		t.Error("stopping forgot the active exercise")
	}
}

func TestAutoCheckRunsOnceAfterSavesSettle(t *testing.T) {
	m := build(t, named("exercise set up on the VMs"), false)
	clock := time.Now()
	m.now = func() time.Time { return clock }
	checks := make(chan string, 4)
	m.check = func(_ context.Context, ex *exercise.Exercise, _ io.Writer) (bool, error) {
		checks <- ex.ID
		return false, nil
	}
	m.Update(keyMsg("A"))
	if !m.autoCheck {
		t.Fatal("A didn't turn auto-check on")
	}
	pushed := filepath.Join(m.mgr.StateDir, "vscode", "docker.pushed")
	if err := os.MkdirAll(filepath.Dir(pushed), 0o755); err != nil {
		t.Fatal(err)
	}
	save := func() {
		clock = clock.Add(time.Second)
		_ = os.WriteFile(pushed, nil, 0o644)
		_ = os.Chtimes(pushed, clock, clock)
	}
	save()
	if c := m.autoCheckTick(); c != nil {
		t.Fatal("checked at once, before the saves settled")
	}
	save() // another save pushes it back
	clock = clock.Add(time.Second)
	if c := m.autoCheckTick(); c != nil {
		t.Fatal("checked while saves were still coming")
	}
	clock = clock.Add(autoCheckDelay)
	if c := m.autoCheckTick(); c == nil {
		t.Fatal("no check after the saves settled")
	}
	select {
	case id := <-checks:
		if id != "d-1" {
			t.Errorf("checked %s", id)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the check never ran")
	}
	if m.tab == tabOutput {
		t.Error("auto-check switched to the Output tab")
	}
	if c := m.autoCheckTick(); c != nil {
		t.Error("a second check for the same saves")
	}
}

func TestSolutionShowsYourCommands(t *testing.T) {
	m := build(t, named("exercise set up on the VMs"), false)
	m.solShow = true
	if s := m.solutionText(); !strings.Contains(s, "Your commands show up here") {
		t.Errorf("no history yet:\n%s", s)
	}
	dir := filepath.Join(m.mgr.StateDir, "history")
	_ = os.MkdirAll(dir, 0o755)
	_ = os.WriteFile(filepath.Join(dir, "d-1.cur"), []byte("docker ps -a\n"), 0o644)
	s := m.solutionText()
	if !strings.Contains(s, "### Your commands") || !strings.Contains(s, "docker ps -a") {
		t.Errorf("history not shown:\n%s", s)
	}
	if b := codeBlocks(s); len(b) == 0 || b[len(b)-1] != "docker ps -a" {
		t.Errorf("y can't copy your commands: %q", b)
	}
}

// The offer is made by the message under the screen: once another message
// replaces it, enter must not jump to an exercise nothing mentions any more.
func TestNextUpOfferEndsWithItsMessage(t *testing.T) {
	m := build(t, named("exercise set up on the VMs"), false)
	m.Update(opDoneMsg{kind: "check", id: "d-1", passed: true})
	if m.suggest == nil {
		t.Fatal("setup: no suggestion")
	}
	m.flash = "copied" // any later message, a copy say
	m.Update(keyMsg("enter"))
	if m.cur.ID != "d-1" {
		t.Errorf("enter opened %s after the offer was gone from the screen", m.cur.ID)
	}
}

// Solving an exercise drops it out of the "in progress" list: next up and n
// must still go on in that list, not in the whole catalog.
func TestNextStaysInAListTheExerciseLeft(t *testing.T) {
	m := newTestModel(t)
	m.prog.MarkStarted("d-1")
	m.prog.MarkStarted("k-1") // d-2 isn't started: not in the list
	m.setProgFilter(progInProgress)
	keys(m, "enter")
	if m.cur == nil || m.cur.ID != "d-1" {
		t.Fatalf("setup: opened %v", m.cur)
	}
	m.Update(opDoneMsg{kind: "check", id: "d-1", passed: true})
	if m.suggest == nil || m.suggest.ID != "k-1" {
		t.Errorf("next up = %v, want k-1 (the next in progress), not the next in the catalog", m.suggest)
	}
	m.Update(keyMsg("n"))
	if m.cur.ID != "k-1" {
		t.Errorf("n opened %s, want k-1", m.cur.ID)
	}
}
