package tui

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Long operations (building or booting an environment) show how far they
// are: the step they're on, the time so far and, once the same kind of
// operation finished before, an estimate with a bar. The estimate comes from
// the last run of the same kind, kept in build-stats.json.

type opRun struct {
	start time.Time
	steps int    // "==> " lines so far
	step  string // the latest one
	build bool   // a VM is being created or bootstrapped, not just booted
	key   string // which kind of run, for the estimate: "<what>/start" or "<what>/build"
}

type runStat struct {
	Steps   int     `json:"steps"`
	Seconds float64 `json:"seconds"`
}

func (m *model) statsPath() string { return filepath.Join(m.mgr.StateDir, "build-stats.json") }

func (m *model) loadStats() map[string]runStat {
	out := map[string]runStat{}
	if b, err := os.ReadFile(m.statsPath()); err == nil {
		_ = json.Unmarshal(b, &out)
	}
	return out
}

// beginRun starts timing an operation on what (an environment or an exercise's).
func (m *model) beginRun(what string) {
	m.run = opRun{start: m.now(), key: what + "/start"}
}

// runLine notes a line of operation output.
func (m *model) runLine(line string) {
	rest, ok := strings.CutPrefix(line, "==> ")
	if !ok {
		return
	}
	if strings.HasPrefix(rest, "[") { // "==> [host] step" from inside a VM
		if i := strings.Index(rest, "] "); i > 0 {
			rest = rest[i+2:]
		}
	}
	m.run.steps++
	m.run.step = rest
	if !m.run.build && (strings.HasPrefix(rest, "creating VM") || strings.HasPrefix(rest, "bootstrapping") ||
		strings.Contains(rest, "building it from scratch")) {
		m.run.build = true
		m.run.key = strings.TrimSuffix(m.run.key, "/start") + "/build"
	}
}

// endRun records how long a finished operation took, for the next estimate.
// Short ones aren't worth estimating.
func (m *model) endRun(ok bool) {
	r := m.run
	m.run = opRun{}
	el := m.now().Sub(r.start).Seconds()
	if !ok || r.start.IsZero() || el < 20 || r.steps < 2 {
		return
	}
	st := m.loadStats()
	st[r.key] = runStat{Steps: r.steps, Seconds: el}
	if b, err := json.MarshalIndent(st, "", "  "); err == nil {
		_ = os.WriteFile(m.statsPath(), b, 0o644)
	}
}

// runStatus is the progress part of the status line, "" when there's
// nothing worth showing yet.
func (m *model) runStatus() string {
	r := m.run
	if r.start.IsZero() {
		return ""
	}
	el := m.now().Sub(r.start)
	if el < 3*time.Second && r.steps == 0 {
		return ""
	}
	var parts []string
	est, ok := m.loadStats()[r.key]
	if ok && est.Seconds > 0 {
		frac := el.Seconds() / est.Seconds
		if est.Steps > 0 {
			frac = max(frac, float64(r.steps)/float64(est.Steps))
		}
		frac = min(frac, 0.97) // it isn't done until it says so
		parts = append(parts, progressBar(frac, 12)+fmt.Sprintf(" %d%%", int(frac*100)))
		if est.Steps > 0 {
			parts = append(parts, fmt.Sprintf("step %d/~%d", r.steps, est.Steps))
		}
		parts = append(parts, clock(el)+" of ~"+clock(time.Duration(est.Seconds)*time.Second))
	} else {
		if r.steps > 0 {
			parts = append(parts, fmt.Sprintf("step %d", r.steps))
		}
		parts = append(parts, clock(el))
	}
	if r.step != "" {
		parts = append(parts, r.step)
	}
	return strings.Join(parts, " · ")
}

func progressBar(frac float64, w int) string {
	n := int(frac*float64(w) + 0.5)
	n = min(max(n, 0), w)
	return strings.Repeat("█", n) + strings.Repeat("░", w-n)
}

func clock(d time.Duration) string {
	s := int(d.Seconds())
	return fmt.Sprintf("%d:%02d", s/60, s%60)
}
