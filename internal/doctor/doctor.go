// Package doctor checks the machine and awesome_labs's own state for the
// usual causes of trouble, and says how to fix each one.
package doctor

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"

	"awesome_labs/internal/install"
	"awesome_labs/internal/lab"
)

// Level is how bad a finding is.
type Level int

const (
	OK Level = iota
	Info
	Warn
	Bad
)

// Finding is the result of one check.
type Finding struct {
	Level  Level
	Title  string
	Detail string
	Fix    string
}

// Probe is what the checks read from the machine; tests replace it.
type Probe struct {
	Lima      func() (line string, major int, ok bool)
	QEMU      func() (needed, ok bool)
	FreeDisk  func(path string) (uint64, error)
	TotalRAM  func() (uint64, error)
	OnPath    func() (string, error)           // the awesome_labs found on the PATH
	VersionOf func(bin string) (string, error) // `bin version`
	Self      func() (string, error)
	VMs       func(ctx context.Context) ([]lab.VMInfo, error)
	HasBase   func(ctx context.Context, env lab.Env) bool
	Code      func() (string, error)
}

// System is the real machine.
func System(mgr *lab.Manager) Probe {
	return Probe{
		Lima: install.InstalledLima,
		QEMU: func() (bool, bool) {
			if runtime.GOOS != "linux" {
				return false, true
			}
			s, err := install.Detect()
			return true, err == nil && install.QEMUReady(s)
		},
		FreeDisk: freeDisk,
		TotalRAM: totalRAM,
		OnPath:   func() (string, error) { return exec.LookPath("awesome_labs") },
		VersionOf: func(bin string) (string, error) {
			out, err := exec.Command(bin, "version").Output()
			return strings.TrimSpace(string(out)), err
		},
		Self:    os.Executable,
		VMs:     mgr.VMs,
		HasBase: mgr.HasBase,
		Code:    lab.CodeBinary,
	}
}

const gib = 1 << 30

// Check runs every check. version is this binary's version.
func Check(ctx context.Context, mgr *lab.Manager, p Probe, version string) []Finding {
	var out []Finding
	add := func(l Level, title, detail, fix string) {
		out = append(out, Finding{l, title, detail, fix})
	}

	// Lima and QEMU
	if line, major, ok := p.Lima(); !ok {
		add(Bad, "Lima is not installed", "awesome_labs runs its labs in Lima VMs.", "awesome_labs install")
	} else if major < install.MinLimaMajor {
		add(Bad, "Lima is too old: "+line, fmt.Sprintf("awesome_labs needs Lima %d or newer.", install.MinLimaMajor), "awesome_labs install (or upgrade Lima with your package manager)")
	} else {
		add(OK, line, "", "")
	}
	if needed, ok := p.QEMU(); needed && !ok {
		add(Bad, "QEMU or its UEFI firmware is missing", "On Linux the VMs run on QEMU.", "awesome_labs install")
	} else if needed {
		add(OK, "QEMU and its firmware are installed", "", "")
	}

	// the machine
	home, _ := os.UserHomeDir()
	if free, err := p.FreeDisk(home); err == nil {
		g := float64(free) / gib
		switch {
		case g < 15:
			add(Bad, fmt.Sprintf("%.0f GiB free on the disk with %s", g, home), "A full lab needs ~40 GiB: builds fail and VMs pause on a full disk.", "free some space, or delete saved copies you don't use: limactl delete klbase-<name>")
		case g < 40:
			add(Warn, fmt.Sprintf("%.0f GiB free on the disk with %s", g, home), "~40 GiB is comfortable for every environment and its saved copy.", "")
		default:
			add(OK, fmt.Sprintf("%.0f GiB free disk", g), "", "")
		}
	}
	if ram, err := p.TotalRAM(); err == nil {
		g := float64(ram) / gib
		switch {
		case g < 7.5:
			add(Bad, fmt.Sprintf("%.0f GiB of RAM", g), "The Kubernetes lab alone takes 5.5 GiB.", "lower the VM sizes in "+filepath.Join(mgr.StateDir, "config.yaml")+" (then awesome_labs env rebuild)")
		case g < 12:
			add(Warn, fmt.Sprintf("%.0f GiB of RAM", g), "Enough for one environment at a time; close other big apps while the Kubernetes lab runs.", "")
		default:
			add(OK, fmt.Sprintf("%.0f GiB of RAM", g), "", "")
		}
	}

	// which awesome_labs runs
	self, _ := p.Self()
	if r, err := filepath.EvalSymlinks(self); err == nil {
		self = r
	}
	if bin, err := p.OnPath(); err != nil {
		add(Warn, "awesome_labs is not on your PATH", "VS Code's terminal and task find it by its full path, but typing awesome_labs won't.", "awesome_labs install")
	} else {
		if r, err := filepath.EvalSymlinks(bin); err == nil {
			bin = r
		}
		if bin != self {
			v, err := p.VersionOf(bin)
			switch {
			case err != nil:
				add(Warn, "the awesome_labs on your PATH is an old build: "+bin, "It has no version command, so it predates this one ("+version+"). Typing awesome_labs runs it, not this one.", "awesome_labs install (from this build) replaces it")
			case v != "awesome_labs "+version:
				add(Warn, "the awesome_labs on your PATH is a different build: "+bin, fmt.Sprintf("It is %s; this one is %s. Typing awesome_labs runs that one.", strings.TrimPrefix(v, "awesome_labs "), version), "awesome_labs install (from the build you want) replaces it")
			default:
				add(OK, "awesome_labs "+version+" (also on your PATH)", "", "")
			}
		} else {
			add(OK, "awesome_labs "+version+" is the one on your PATH", "", "")
		}
	}

	// awesome_labs's own state
	if pid := mgr.AppLockHolder(); pid > 0 && pid != os.Getpid() {
		add(Info, fmt.Sprintf("awesome_labs is running (pid %d)", pid), "Commands that change the VMs or your progress wait for it to exit.", "")
	}
	for _, env := range mgr.StaleBusy() {
		add(Warn, env.Title+" is marked busy by a process that is gone", "A verify run crashed; auto-stop and the VS Code sync skip this environment.", "awesome_labs doctor --fix")
	}
	vms, err := p.VMs(ctx)
	if err == nil {
		var running, stray []string
		mem := int64(0)
		for _, v := range vms {
			if v.Running() {
				running = append(running, v.Name)
				mem += v.Memory
			}
			if v.Env == "" && v.Status != "" {
				stray = append(stray, v.Name)
			}
		}
		if len(running) > 0 {
			add(Info, fmt.Sprintf("running: %s (%.1f GiB)", strings.Join(running, ", "), float64(mem)/gib), "", "")
		}
		if len(stray) > 0 {
			add(Warn, "other lab VMs: "+strings.Join(stray, ", "), "They aren't part of an environment and only take up space.", "awesome_labs vms delete <name>")
		}
	}
	for _, env := range lab.AllEnvs() {
		if mgr.Tainted(env) {
			add(Info, env.Title+" will be rebuilt before its next exercise", "A destructive exercise changed it beyond a reset.", "")
		}
		if !p.HasBase(ctx, env) {
			add(Info, env.Title+" has no saved copy yet", "Its first exercise builds it (5–20 minutes); later ones start in seconds.", "awesome_labs env prepare "+envArg(env))
		}
	}
	if c, err := p.Code(); err == nil {
		add(OK, "VS Code: "+c, "", "")
	} else {
		add(Info, "VS Code's code command was not found", "Only needed to work on a lab in VS Code (o on the exercise screen).", "in VS Code: Shell Command: Install 'code' command in PATH")
	}
	return out
}

func envArg(env lab.Env) string {
	switch env.Profile {
	case lab.K8sEnv.Profile:
		return "k8s"
	case lab.OffsecEnv.Profile:
		return "offsec"
	}
	return "docker"
}

// Print writes the findings and returns the worst level.
func Print(w io.Writer, fs []Finding) Level {
	worst := OK
	for _, f := range fs {
		mark := map[Level]string{OK: "✔", Info: "·", Warn: "⚠", Bad: "✘"}[f.Level]
		fmt.Fprintf(w, "  %s %s\n", mark, f.Title)
		if f.Detail != "" {
			fmt.Fprintf(w, "      %s\n", f.Detail)
		}
		if f.Fix != "" {
			fmt.Fprintf(w, "      fix: %s\n", f.Fix)
		}
		worst = max(worst, f.Level)
	}
	return worst
}

func freeDisk(path string) (uint64, error) {
	var st syscall.Statfs_t
	if err := syscall.Statfs(path, &st); err != nil {
		return 0, err
	}
	return uint64(st.Bavail) * uint64(st.Bsize), nil
}

func totalRAM() (uint64, error) {
	if runtime.GOOS == "darwin" {
		out, err := exec.Command("sysctl", "-n", "hw.memsize").Output()
		if err != nil {
			return 0, err
		}
		return strconv.ParseUint(strings.TrimSpace(string(out)), 10, 64)
	}
	f, err := os.Open("/proc/meminfo")
	if err != nil {
		return 0, err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if rest, ok := strings.CutPrefix(sc.Text(), "MemTotal:"); ok {
			kb, err := strconv.ParseUint(strings.Fields(rest)[0], 10, 64)
			return kb * 1024, err
		}
	}
	return 0, fmt.Errorf("no MemTotal in /proc/meminfo")
}
