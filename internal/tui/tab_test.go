package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"
)

// The spinner on the Output tab must not leak escape codes as text: the
// selected tab is underlined, and lipgloss styles underlined text rune by rune.
func TestOutputTabSpinnerHasNoRawEscapes(t *testing.T) {
	prev := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(prev)

	m := newTestModel(t)
	e := m.ex(t, "d-1")
	m.openDetail(e, scrBrowse)
	m.op, m.opID, m.tab = "starting", e.ID, tabOutput
	if out := ansi.Strip(m.View()); strings.Contains(out, "[38;") || strings.Contains(out, "[0m") {
		t.Fatalf("escape codes shown as text:\n%s", out)
	}
}
