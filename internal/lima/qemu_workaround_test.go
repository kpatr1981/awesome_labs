package lima

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// fakeQEMU puts a qemu-system binary on PATH that prints its arguments, one per line.
func fakeQEMU(t *testing.T) (envVar string) {
	t.Helper()
	if runtime.GOOS != "linux" || qemuArch() == "" {
		t.Skip("the workaround only applies on Linux amd64/arm64")
	}
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash not available")
	}
	bin := filepath.Join(t.TempDir(), "usr", "bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	body := "#!/usr/bin/env bash\nprintf '%s\\n' \"$@\"\n"
	if err := os.WriteFile(filepath.Join(bin, "qemu-system-"+qemuArch()), []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	envVar = "QEMU_SYSTEM_" + strings.ToUpper(qemuArch())
	t.Setenv(envVar, "")
	return envVar
}

func TestWrapperRewritesOnlyTheSocketNetdev(t *testing.T) {
	envVar := fakeQEMU(t)
	state := t.TempDir()
	if err := UseQEMUStreamNetdev(state); err != nil {
		t.Fatal(err)
	}
	wrapper := os.Getenv(envVar)
	if !strings.HasPrefix(wrapper, state) {
		t.Fatalf("%s = %q, want a wrapper under %s", envVar, wrapper, state)
	}
	if _, err := os.Lstat(filepath.Join(state, "qemu", "share")); err != nil {
		t.Errorf("share link for firmware lookup: %v", err)
	}

	in := []string{
		"-netdev", "socket,id=net0,fd=3",
		"-chardev", "socket,id=c0,fd=3",
		"-netdev", "user,id=net1,net=192.168.5.0/24",
		"-name", "has space",
	}
	out, err := exec.Command(wrapper, in...).Output()
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"-netdev", "stream,id=net0,addr.type=fd,addr.str=3",
		"-chardev", "socket,id=c0,fd=3",
		"-netdev", "user,id=net1,net=192.168.5.0/24",
		"-name", "has space",
	}
	if got := strings.Split(strings.TrimSuffix(string(out), "\n"), "\n"); strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("args = %q, want %q", got, want)
	}
}

func TestUserChosenQEMUIsLeftAlone(t *testing.T) {
	envVar := fakeQEMU(t)
	t.Setenv(envVar, "/opt/my-qemu")
	state := t.TempDir()
	if err := UseQEMUStreamNetdev(state); err != nil {
		t.Fatal(err)
	}
	if got := os.Getenv(envVar); got != "/opt/my-qemu" {
		t.Errorf("%s = %q, want it unchanged", envVar, got)
	}
	if _, err := os.Stat(filepath.Join(state, "qemu")); !os.IsNotExist(err) {
		t.Errorf("wrapper directory was created: %v", err)
	}
}
