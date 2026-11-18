package web

import (
	"fmt"
	"net/http"
	"sync"

	"awesome_labs/internal/config"
)

// live holds the settings the running server acts on and that the Settings
// screen can change without a restart.
type live struct {
	mu         sync.Mutex
	autoStop   bool
	deleteDays int
	changed    chan struct{} // closed and replaced when deleteDays changes
}

func newLive(autoStop bool, deleteDays int) *live {
	return &live{autoStop: autoStop, deleteDays: deleteDays, changed: make(chan struct{})}
}

func (l *live) get() (autoStop bool, deleteDays int) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.autoStop, l.deleteDays
}

func (l *live) set(c config.Config) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.autoStop = c.AutoStopEnabled()
	if c.DeleteStoppedAfterDays != l.deleteDays {
		l.deleteDays = c.DeleteStoppedAfterDays
		close(l.changed)
		l.changed = make(chan struct{})
	}
}

func (l *live) wait() <-chan struct{} {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.changed
}

// settingJSON describes one setting for the Settings screen.
type settingJSON struct {
	Key     string   `json:"key"`
	Label   string   `json:"label"`
	Help    string   `json:"help"`
	Type    string   `json:"type"` // choice, bool or int
	Options []string `json:"options,omitempty"`
	Value   any      `json:"value"`
	// Applies says when a change takes effect: "now", "next start" (of the
	// web interface or the terminal UI) or "terminal UI" (only it uses it).
	Applies string `json:"applies"`
}

func settingsOf(c config.Config) []settingJSON {
	theme := c.Theme
	if theme == "" {
		theme = "auto"
	}
	return []settingJSON{
		{Key: "on_exit", Label: "When awesome_labs exits", Type: "choice", Options: []string{"stop", "delete", "keep"}, Value: c.ExitAction(),
			Help:    "stop: stop the lab VMs, keeping the exercise you were on set up · delete: delete every lab VM (saved copies are kept) · keep: leave them running.",
			Applies: "next start"},
		{Key: "auto_stop", Label: "Stop lab VMs nothing needs", Type: "bool", Value: c.AutoStopEnabled(),
			Help:    "Stop running lab VMs after 10 minutes with no exercise set up, or 30 minutes with no page open; and after solving an exercise with awesome_labs check. Stopping keeps the lab.",
			Applies: "now"},
		{Key: "delete_stopped_after_days", Label: "Delete VMs stopped for more than (days)", Type: "int", Value: c.DeleteStoppedAfterDays,
			Help:    "Free the disk of lab VMs nobody has used for a while. Saved copies are kept, so the next exercise still starts in seconds; the exercise set up on a deleted VM is forgotten. 0 never deletes. Checked when the web interface starts and once a day.",
			Applies: "now"},
		{Key: "auto_check", Label: "Auto-check", Type: "bool", Value: c.AutoCheck,
			Help:    "Check the exercise whenever its files change (a save in VS Code, leaving the terminal). A on the exercise screen toggles it.",
			Applies: "terminal UI"},
		{Key: "check_updates", Label: "Look for a newer release", Type: "bool", Value: c.CheckUpdates,
			Help:    "Once a day, ask GitHub whether a newer awesome_labs is out. Nothing about you or your progress is sent.",
			Applies: "terminal UI"},
		{Key: "theme", Label: "Colours", Type: "choice", Options: []string{"auto", "dark", "light"}, Value: theme,
			Help:    "The web page follows your browser's light or dark mode.",
			Applies: "terminal UI"},
	}
}

func (s *server) handleSettings(w http.ResponseWriter, r *http.Request) {
	dir := s.Manager.StateDir
	c, err := config.Load(dir)
	out := map[string]any{"path": config.Path(dir), "settings": settingsOf(c), "runningOnExit": s.OnExit}
	if err != nil {
		out["error"] = err.Error()
	}
	writeJSON(w, http.StatusOK, out)
}

// handleSetSettings writes the settings in the body ({"key": value, …}) to
// the config file, keeping its comments, and applies the live ones.
func (s *server) handleSetSettings(w http.ResponseWriter, r *http.Request) {
	var body map[string]any
	if err := decodeJSON(r, &body); err != nil {
		fail(w, http.StatusBadRequest, "bad request: %v", err)
		return
	}
	types := map[string]string{}
	for _, st := range settingsOf(config.Config{}) {
		types[st.Key] = st.Type
	}
	var sets []config.Setting
	for k, v := range body {
		var val any
		switch types[k] {
		case "choice":
			str, ok := v.(string)
			if !ok {
				fail(w, http.StatusBadRequest, "%s: want a string", k)
				return
			}
			val = str
		case "bool":
			b, ok := v.(bool)
			if !ok {
				fail(w, http.StatusBadRequest, "%s: want true or false", k)
				return
			}
			val = b
		case "int":
			f, ok := v.(float64)
			if !ok || f != float64(int(f)) {
				fail(w, http.StatusBadRequest, "%s: want a whole number", k)
				return
			}
			val = int(f)
		default:
			fail(w, http.StatusBadRequest, "unknown setting %q", k)
			return
		}
		sets = append(sets, config.Setting{Key: k, Value: val})
	}
	c, err := config.Set(s.Manager.StateDir, sets...)
	if err != nil {
		fail(w, http.StatusBadRequest, "%v", err)
		return
	}
	s.live.set(c)
	writeJSON(w, http.StatusOK, map[string]any{"path": config.Path(s.Manager.StateDir), "settings": settingsOf(c), "runningOnExit": s.OnExit,
		"saved": fmt.Sprintf("%d setting(s) saved", len(sets))})
}
