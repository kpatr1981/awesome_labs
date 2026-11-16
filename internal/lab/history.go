package lab

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"awesome_labs/internal/exercise"
	"awesome_labs/internal/lima"
)

// The commands you typed for an exercise: shells opened by awesome_labs keep
// a history per exercise in the lab user's home on each VM (a reset leaves
// home alone), and the app copies it to history/<id>.txt on this computer,
// so it outlives the VMs. A VM built again starts an empty history; what the
// old one had is kept above it.

var safeID = regexp.MustCompile(`[^A-Za-z0-9._-]`)

func historyName(id string) string { return ".kl_history_" + safeID.ReplaceAllString(id, "_") }

func (m *Manager) historyDir() string { return filepath.Join(m.StateDir, "history") }

// History returns the commands recorded for id, oldest first.
func (m *Manager) History(id string) string {
	base := filepath.Join(m.historyDir(), safeID.ReplaceAllString(id, "_"))
	old, _ := os.ReadFile(base + ".txt")
	cur, _ := os.ReadFile(base + ".cur")
	return strings.TrimLeft(string(old)+string(cur), "\n")
}

// FetchHistory copies ex's shell history from its VMs to this computer and
// returns all of it.
func (m *Manager) FetchHistory(ctx context.Context, ex *exercise.Exercise) string {
	env := EnvFor(ex.Profile)
	var got strings.Builder
	for _, vm := range env.VMs {
		var out bytes.Buffer
		code, err := lima.Run(ctx, vm.Name, lima.Script{Body: `cat "$HOME/` + historyName(ex.ID) + `" 2>/dev/null; true`, Stderr: &bytes.Buffer{}}, &out)
		if err != nil || code != 0 || out.Len() == 0 {
			continue
		}
		if len(env.VMs) > 1 && vm.Role == "worker" {
			got.WriteString("# on node01\n")
		}
		got.Write(out.Bytes())
		if !bytes.HasSuffix(out.Bytes(), []byte("\n")) {
			got.WriteByte('\n')
		}
	}
	m.recordHistory(ex.ID, got.String())
	return m.History(ex.ID)
}

// recordHistory stores what the VMs hold now. When it doesn't carry on from
// what was stored last time (the VMs were built again), the stored part is
// kept and the new one follows it.
func (m *Manager) recordHistory(id, now string) {
	if now == "" {
		return
	}
	_ = os.MkdirAll(m.historyDir(), 0o755)
	base := filepath.Join(m.historyDir(), safeID.ReplaceAllString(id, "_"))
	prev, _ := os.ReadFile(base + ".cur")
	if len(prev) > 0 && !strings.HasPrefix(now, string(prev)) {
		f, err := os.OpenFile(base+".txt", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
		if err == nil {
			_, _ = f.Write(prev)
			f.Close()
		}
	}
	_ = os.WriteFile(base+".cur", []byte(now), 0o644)
}

// ForgetHistory deletes what is kept on this computer for id.
func (m *Manager) ForgetHistory(id string) {
	base := filepath.Join(m.historyDir(), safeID.ReplaceAllString(id, "_"))
	_ = os.Remove(base + ".txt")
	_ = os.Remove(base + ".cur")
}
