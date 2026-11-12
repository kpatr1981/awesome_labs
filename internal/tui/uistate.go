package tui

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
)

// uiState is where the exercise list was left: the app reopens on it, so
// quitting and coming back lands on the same exercise with the same filters.
type uiState struct {
	Track    string `json:"track,omitempty"`
	Progress int    `json:"progress,omitempty"`
	Filter   string `json:"filter,omitempty"`
	Path     string `json:"path,omitempty"`
	Selected string `json:"selected,omitempty"`
}

func (m *model) uiPath() string { return filepath.Join(m.mgr.StateDir, "ui.json") }

// currentUI is the list state as it is now.
func (m *model) currentUI() uiState {
	s := uiState{
		Track:    m.tracks[m.trackIdx],
		Progress: int(m.progFilter),
		Filter:   m.filter.Value(),
	}
	if m.path != nil {
		s.Path = m.path.ID
	}
	if m.cursor < len(m.visible) {
		s.Selected = m.visible[m.cursor].ID
	}
	return s
}

// loadUI puts the list back the way the last run left it. Anything that no
// longer exists (a renamed track, path or exercise) is skipped.
func (m *model) loadUI() {
	b, err := os.ReadFile(m.uiPath())
	if err != nil {
		return
	}
	var s uiState
	if json.Unmarshal(b, &s) != nil {
		return
	}
	if i := slices.Index(m.tracks, s.Track); i >= 0 {
		m.trackIdx = i
	}
	if s.Progress >= 0 && s.Progress < int(progFilterCount) {
		m.progFilter = progFilter(s.Progress)
	}
	m.filter.SetValue(s.Filter)
	for i, p := range m.paths {
		if p.ID == s.Path {
			m.path, m.pathCursor = p, i
		}
	}
	m.refilter()
	m.selectID(s.Selected)
	m.saved = m.currentUI()
}

// saveUI writes the list state when it changed since the last write.
func (m *model) saveUI() {
	s := m.currentUI()
	if s == m.saved {
		return
	}
	b, _ := json.Marshal(s)
	tmp := m.uiPath() + ".tmp"
	if os.WriteFile(tmp, b, 0o644) == nil && os.Rename(tmp, m.uiPath()) == nil {
		m.saved = s
	}
}

// selectID moves the list cursor onto exercise id, if the list shows it.
func (m *model) selectID(id string) bool {
	for i, e := range m.visible {
		if e.ID == id {
			m.cursor = i
			m.clampOffset()
			return true
		}
	}
	return false
}

// refilterKeep refilters after progress changed, keeping the cursor on the
// exercise it was on. One the filter now drops leaves the cursor where it
// was, on the exercise after it.
func (m *model) refilterKeep() {
	id := ""
	if m.cursor < len(m.visible) {
		id = m.visible[m.cursor].ID
	}
	m.refilter()
	m.selectID(id)
}
