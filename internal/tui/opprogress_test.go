package tui

import (
	"strings"
	"testing"
	"time"
)

func TestBuildProgressEstimatesFromTheLastRun(t *testing.T) {
	m := newTestModel(t)
	clock := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	m.now = func() time.Time { return clock }

	// first build: no estimate, just the step and the time
	m.beginRun("start:docker")
	m.runLine("==> creating VM kl-docker (docker-host)")
	for i := range 9 {
		m.runLine("==> [docker-host] step " + string(rune('a'+i)))
	}
	clock = clock.Add(90 * time.Second)
	if s := m.runStatus(); !strings.Contains(s, "step 10") || !strings.Contains(s, "1:30") || strings.Contains(s, "%") {
		t.Errorf("first build status = %q", s)
	}
	clock = clock.Add(210 * time.Second) // 5:00 in all
	m.endRun(true)

	// a boot isn't estimated from a build
	m.beginRun("start:docker")
	m.runLine("==> starting VM kl-docker from its saved copy")
	clock = clock.Add(10 * time.Second)
	if s := m.runStatus(); strings.Contains(s, "%") {
		t.Errorf("a boot used the build's estimate: %q", s)
	}
	m.endRun(true) // too short to keep

	// the next build: half the steps and half the time is about 50 %
	m.beginRun("start:docker")
	m.runLine("==> creating VM kl-docker (docker-host)")
	for range 4 {
		m.runLine("==> [docker-host] pulling images")
	}
	clock = clock.Add(150 * time.Second)
	s := m.runStatus()
	for _, want := range []string{"50%", "step 5/~10", "2:30 of ~5:00", "pulling images"} {
		if !strings.Contains(s, want) {
			t.Errorf("second build status = %q, missing %q", s, want)
		}
	}
	// running long never claims to be done
	clock = clock.Add(time.Hour)
	if s := m.runStatus(); !strings.Contains(s, "97%") {
		t.Errorf("overrunning build = %q, want it capped at 97%%", s)
	}
}
