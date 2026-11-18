package local

import (
	"regexp"

	"awesome_labs/internal/exercise"
)

// The harness container is privileged and shares the host's kernel, devices
// and clock. A lab VM is a safe place for an exercise to make loop devices,
// change kernel settings or set the time; the author's machine is not. Any
// exercise whose scripts do one of these is skipped (unless forced).
var hostRisks = []struct {
	re     *regexp.Regexp
	reason string
}{
	{regexp.MustCompile(`\blosetup\b|\bmount\b[^\n]*-o[^\n]*\bloop\b`), "loop devices"},
	{regexp.MustCompile(`\b(pv|vg|lv)(create|remove|extend|reduce|change|rename)\b`), "LVM"},
	{regexp.MustCompile(`\bmkfs(\.\w+)?\b|\bwipefs\b|\bparted\b|\bfdisk\b|\bsfdisk\b`), "filesystems on block devices"},
	{regexp.MustCompile(`\bswap(on|off)\b`), "swap"},
	{regexp.MustCompile(`\bsysctl\b[^\n]*(--system|\s-p\b)`), "applying sysctl files"},
	{regexp.MustCompile(`\bsysctl\b[^\n]*-w\s+["']?(kernel|vm|fs|user|dev|abi)\.`), "kernel sysctls"},
	{regexp.MustCompile(`/proc/sys/(kernel|vm|fs|user|dev|abi)/`), "kernel sysctls"},
	{regexp.MustCompile(`\btimedatectl\s+set|\bdate\s+-s\b|\bdate\s+--set\b|\bhwclock\b`), "the system clock"},
	{regexp.MustCompile(`\brmmod\b|\bmodprobe\s+-r\b`), "unloading kernel modules"},
}

// HostRisk names what an exercise does that would reach past the harness
// container to the host ("" when nothing does).
func HostRisk(ex *exercise.Exercise) string {
	for _, s := range []string{ex.Setup, ex.Check, ex.Solve} {
		for _, r := range hostRisks {
			if r.re.MatchString(s) {
				return r.reason
			}
		}
	}
	return ""
}

// Unsupported says why the harness can't run an exercise at all ("" when it can).
func Unsupported(ex *exercise.Exercise) string {
	if ex.Profile != exercise.ProfileDocker {
		return "needs the " + string(ex.Profile) + " lab (VM only)"
	}
	if ex.SetupWorker != "" || ex.CheckWorker != "" {
		return "needs a worker node (VM only)"
	}
	if ex.Destructive {
		return "destructive (VM only)"
	}
	return ""
}
