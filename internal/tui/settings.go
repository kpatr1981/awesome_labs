package tui

import (
	"context"
	"os"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"awesome_labs/internal/lab"
	"awesome_labs/internal/update"
)

// applyTheme sets the colours for theme (auto, dark or light) and returns the
// matching markdown style. NO_COLOR turns colours off: the selection and the
// lit buttons are shown reversed instead.
func applyTheme(theme string) string {
	if os.Getenv("NO_COLOR") != "" {
		lipgloss.SetColorProfile(termenv.Ascii)
		sSel = sSel.Reverse(true)
		sBtnHot = sBtnHot.Reverse(true)
		return "notty"
	}
	dark := lipgloss.HasDarkBackground()
	switch theme {
	case "dark":
		dark = true
	case "light":
		dark = false
	}
	lipgloss.SetHasDarkBackground(dark)
	if dark {
		return "dark"
	}
	return "light"
}

// cmdCheckUpdates asks, at most once a day, whether a newer release is out.
func (m *model) cmdCheckUpdates() tea.Cmd {
	if !m.checkUpdates {
		return nil
	}
	dir, current := m.mgr.StateDir, m.version
	return func() tea.Msg {
		r, err := update.Daily(context.Background(), dir)
		if err != nil || !update.Newer(current, r.Tag) {
			return nil
		}
		return updateMsg{r.Tag, r.URL}
	}
}

// autoCheckDelay is how long auto-check waits after the last save, so a burst
// of saves is checked once.
const autoCheckDelay = 2 * time.Second

// autoCheckTick runs on every tick: with auto-check on, a save from VS Code
// into the active exercise's lab schedules a check, which runs once things
// are quiet.
func (m *model) autoCheckTick() tea.Cmd {
	if !m.autoCheck || !m.hasActive {
		return nil
	}
	ex := m.cat.Get(m.active.ID)
	if ex == nil {
		return nil
	}
	if p := m.mgr.CodePushed(lab.CodeTargetOf(lab.EnvFor(ex.Profile))); p.After(m.pushSeen) {
		m.pushSeen = p
		m.checkDue = m.now().Add(autoCheckDelay)
	}
	if m.checkDue.IsZero() || m.now().Before(m.checkDue) || m.op != "" {
		return nil
	}
	m.checkDue = time.Time{}
	m.quiet, m.autoRun = true, true
	return m.cmdCheckEx(ex)
}
