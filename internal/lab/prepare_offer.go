package lab

import (
	"context"
	"os"
	"path/filepath"
)

// The first launch (of the terminal UI or the web interface) offers to build
// the environments that have no saved copy yet, so that their first exercise
// starts in seconds. The offer is made once, whichever interface makes it.

func (m *Manager) prepareMarker() string { return filepath.Join(m.StateDir, "prepare-offered") }

// PrepareOffered reports whether the first-launch offer was already made.
func (m *Manager) PrepareOffered() bool {
	_, err := os.Stat(m.prepareMarker())
	return err == nil
}

// MarkPrepareOffered records that the offer was made (answered either way).
func (m *Manager) MarkPrepareOffered() { _ = os.WriteFile(m.prepareMarker(), []byte("1\n"), 0o644) }

// Unprepared lists the environments without an up-to-date saved copy.
func (m *Manager) Unprepared(ctx context.Context) []Env {
	var out []Env
	for _, env := range AllEnvs() {
		if !m.HasBase(ctx, env) {
			out = append(out, env)
		}
	}
	return out
}
