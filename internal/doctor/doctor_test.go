package doctor

import (
	"context"
	"errors"
	"strings"
	"testing"

	"awesome_labs/internal/lab"
)

func probe() Probe {
	return Probe{
		Lima:      func() (string, int, bool) { return "limactl version 2.3.0", 2, true },
		QEMU:      func() (bool, bool) { return false, true },
		FreeDisk:  func(string) (uint64, error) { return 100 * gib, nil },
		TotalRAM:  func() (uint64, error) { return 16 * gib, nil },
		OnPath:    func() (string, error) { return "/self", nil },
		VersionOf: func(string) (string, error) { return "awesome_labs v1", nil },
		Self:      func() (string, error) { return "/self", nil },
		VMs:       func(context.Context) ([]lab.VMInfo, error) { return nil, nil },
		HasBase:   func(context.Context, lab.Env) bool { return true },
		Code:      func() (string, error) { return "/usr/bin/code", nil },
	}
}

func worst(fs []Finding) (Level, string) {
	l, titles := OK, []string{}
	for _, f := range fs {
		if f.Level >= Warn {
			titles = append(titles, f.Title)
		}
		l = max(l, f.Level)
	}
	return l, strings.Join(titles, "; ")
}

func TestHealthyMachine(t *testing.T) {
	m := &lab.Manager{StateDir: t.TempDir()}
	if l, w := worst(Check(context.Background(), m, probe(), "v1")); l > Info {
		t.Errorf("healthy machine: %v %s", l, w)
	}
}

func TestFindsTheUsualTrouble(t *testing.T) {
	m := &lab.Manager{StateDir: t.TempDir()}
	p := probe()
	p.Lima = func() (string, int, bool) { return "limactl version 1.0.4", 1, true }
	p.FreeDisk = func(string) (uint64, error) { return 10 * gib, nil }
	p.OnPath = func() (string, error) { return "/usr/local/bin/awesome_labs", nil }
	p.VersionOf = func(string) (string, error) { return "", errors.New("unknown command") }
	l, w := worst(Check(context.Background(), m, p, "v1"))
	if l != Bad {
		t.Errorf("level %v, want Bad", l)
	}
	for _, want := range []string{"Lima is too old", "GiB free", "old build"} {
		if !strings.Contains(w, want) {
			t.Errorf("missing %q in %q", want, w)
		}
	}
}
