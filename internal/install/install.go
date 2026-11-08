// Package install sets up everything awesome_labs needs on macOS and Linux:
// Lima, QEMU on Linux, and the awesome_labs binary itself.
package install

import (
	"archive/tar"
	"bufio"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// LimaVersion is the release installed when Lima is missing.
const (
	LimaVersion  = "2.3.0"
	minLimaMajor = 2
	binaryName   = "awesome_labs"
)

// System describes the machine being set up.
type System struct {
	OS       string // darwin or linux
	Arch     string // amd64 or arm64
	DistroID string // linux only, from /etc/os-release
	IDLike   string
}

// packageSet is how to install QEMU with one Linux package manager.
type packageSet struct {
	Manager  string
	Update   []string // optional command to run first
	Install  []string
	Packages []string
}

// qemuPackages returns the QEMU install recipe for a Linux distribution.
func qemuPackages(s System) (packageSet, error) {
	families := append([]string{s.DistroID}, strings.Fields(s.IDLike)...)
	arm := s.Arch == "arm64"
	for _, f := range families {
		switch f {
		case "fedora", "rhel", "centos", "rocky", "almalinux":
			return packageSet{Manager: "dnf", Install: []string{"dnf", "install", "-y"},
				Packages: append([]string{"qemu-kvm", "qemu-img", "openssh-clients"}, pick(arm, "edk2-aarch64", "edk2-ovmf"))}, nil
		case "debian", "ubuntu":
			sys := "qemu-system-x86"
			if arm {
				sys = "qemu-system-arm"
			}
			return packageSet{Manager: "apt-get", Update: []string{"apt-get", "update"},
				Install:  []string{"apt-get", "install", "-y"},
				Packages: []string{sys, "qemu-utils", "openssh-client", pick(arm, "qemu-efi-aarch64", "ovmf")}}, nil
		case "arch", "manjaro":
			// No -Sy here: on Arch, syncing without upgrading risks a partial upgrade.
			return packageSet{Manager: "pacman", Install: []string{"pacman", "-S", "--needed", "--noconfirm"},
				Packages: []string{pick(arm, "qemu-system-aarch64", "qemu-base"), "qemu-img", "openssh", pick(arm, "edk2-aarch64", "edk2-ovmf")}}, nil
		case "opensuse", "suse", "opensuse-leap", "opensuse-tumbleweed", "sles":
			sys := "qemu-x86"
			if arm {
				sys = "qemu-arm"
			}
			return packageSet{Manager: "zypper", Install: []string{"zypper", "install", "-y"},
				Packages: []string{sys, "qemu-tools"}}, nil
		}
	}
	return packageSet{}, fmt.Errorf("don't know how to install QEMU on %q: install qemu-system and qemu-img yourself, then run this again", s.DistroID)
}

func pick(cond bool, a, b string) string {
	if cond {
		return a
	}
	return b
}

// firmwarePatterns are where Linux distributions put the UEFI firmware Lima boots with.
func firmwarePatterns(arch string) []string {
	if arch == "arm64" {
		return []string{"/usr/share/AAVMF/AAVMF_CODE*", "/usr/share/qemu-efi-aarch64/QEMU_EFI*", "/usr/share/edk2/aarch64/QEMU_EFI*", "/usr/share/edk2/aarch64/QEMU_CODE*"}
	}
	return []string{"/usr/share/OVMF/OVMF_CODE*", "/usr/share/edk2/ovmf/OVMF_CODE*", "/usr/share/edk2/x64/OVMF_CODE*", "/usr/share/edk2-ovmf/x64/OVMF_CODE*", "/usr/share/qemu/ovmf-x86_64*"}
}

func firmwareFound(arch string) bool {
	for _, p := range firmwarePatterns(arch) {
		if m, _ := filepath.Glob(p); len(m) > 0 {
			return true
		}
	}
	return false
}

// parseOSRelease extracts ID and ID_LIKE from the contents of /etc/os-release.
func parseOSRelease(text string) (id, like string) {
	for _, line := range strings.Split(text, "\n") {
		k, v, ok := strings.Cut(strings.TrimSpace(line), "=")
		if !ok {
			continue
		}
		v = strings.Trim(v, `"'`)
		switch k {
		case "ID":
			id = v
		case "ID_LIKE":
			like = v
		}
	}
	return id, like
}

// Detect identifies the current machine.
func Detect() (System, error) {
	s := System{OS: runtime.GOOS, Arch: runtime.GOARCH}
	switch s.OS {
	case "darwin":
	case "linux":
		b, err := os.ReadFile("/etc/os-release")
		if err != nil {
			return s, fmt.Errorf("can't identify this Linux distribution: %w", err)
		}
		s.DistroID, s.IDLike = parseOSRelease(string(b))
	default:
		return s, fmt.Errorf("%s isn't supported: awesome_labs runs on macOS and Linux", s.OS)
	}
	if s.Arch != "amd64" && s.Arch != "arm64" {
		return s, fmt.Errorf("the %s architecture isn't supported", s.Arch)
	}
	return s, nil
}

// limaAsset returns the name of the Lima release archive for a system.
func limaAsset(s System, version string) string {
	osName, arch := "Linux", "x86_64"
	if s.OS == "darwin" {
		osName = "Darwin"
	}
	if s.Arch == "arm64" {
		arch = "aarch64"
		if s.OS == "darwin" {
			arch = "arm64"
		}
	}
	return fmt.Sprintf("lima-%s-%s-%s.tar.gz", version, osName, arch)
}

// qemuBinary is the QEMU system emulator Lima will look for.
func qemuBinary(s System) string {
	if s.Arch == "arm64" {
		return "qemu-system-aarch64"
	}
	return "qemu-system-x86_64"
}

var versionRE = regexp.MustCompile(`(\d+)\.\d+`)

// limaMajor parses the major version from `limactl --version` output.
func limaMajor(out string) int {
	m := versionRE.FindStringSubmatch(out)
	if m == nil {
		return 0
	}
	n, _ := strconv.Atoi(m[1])
	return n
}

// limaUsable reports whether a new enough limactl is on the PATH.
func limaUsable() bool {
	out, err := exec.Command("limactl", "--version").Output()
	return err == nil && limaMajor(string(out)) >= minLimaMajor
}

func found(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

// state is what's already on the machine.
type state struct {
	lima, qemu, brew bool
}

// step is one action of the install plan.
type step struct {
	desc string
	run  func() error
}

// Options controls Run.
type Options struct {
	DryRun bool
	Yes    bool
	BinDir string
	In     io.Reader
	Out    io.Writer
}

type installer struct {
	sys System
	opt Options
	// prefix receives a downloaded Lima, in prefix/bin and prefix/share.
	prefix string
}

// plan works out the steps needed from what's already installed.
func (in *installer) plan(st state) ([]step, error) {
	var steps []step
	if !st.lima {
		switch {
		case in.sys.OS == "darwin" && st.brew:
			steps = append(steps, step{"install Lima with Homebrew (brew install lima)", func() error {
				return in.exec(false, "brew", "install", "lima")
			}})
		default:
			asset := limaAsset(in.sys, LimaVersion)
			steps = append(steps, step{fmt.Sprintf("download Lima %s to %s", LimaVersion, in.prefix), func() error {
				return in.installLima(asset)
			}})
		}
	}
	if in.sys.OS == "linux" && !st.qemu {
		p, err := qemuPackages(in.sys)
		if err != nil {
			return nil, err
		}
		steps = append(steps, step{fmt.Sprintf("install QEMU and its dependencies with %s (%s), using sudo", p.Manager, strings.Join(p.Packages, " ")), func() error {
			if p.Update != nil {
				if err := in.exec(true, p.Update[0], p.Update[1:]...); err != nil {
					return err
				}
			}
			args := append(append([]string{}, p.Install[1:]...), p.Packages...)
			return in.exec(true, p.Install[0], args...)
		}})
	}
	return steps, nil
}

// exec runs a command attached to the terminal, through sudo when asked.
func (in *installer) exec(sudo bool, name string, args ...string) error {
	if sudo && os.Geteuid() != 0 {
		args = append([]string{name}, args...)
		name = "sudo"
	}
	c := exec.Command(name, args...)
	c.Stdin, c.Stdout, c.Stderr = os.Stdin, in.opt.Out, in.opt.Out
	if err := c.Run(); err != nil {
		return fmt.Errorf("%s %s: %w", name, strings.Join(args, " "), err)
	}
	return nil
}

// checksumFor finds a file's hash in the contents of a SHA256SUMS file.
func checksumFor(sums, name string) (string, error) {
	for _, line := range strings.Split(sums, "\n") {
		f := strings.Fields(line)
		if len(f) == 2 && strings.TrimPrefix(f[1], "*") == name {
			return f[0], nil
		}
	}
	return "", fmt.Errorf("no checksum published for %s", name)
}

func download(url string, w io.Writer) error {
	client := &http.Client{Timeout: 15 * time.Minute}
	resp, err := client.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%s: %s", url, resp.Status)
	}
	_, err = io.Copy(w, resp.Body)
	return err
}

// installLima downloads, verifies and unpacks a Lima release into the prefix.
func (in *installer) installLima(asset string) error {
	base := "https://github.com/lima-vm/lima/releases/download/v" + LimaVersion + "/"
	var sums strings.Builder
	if err := download(base+"SHA256SUMS", &sums); err != nil {
		return err
	}
	want, err := checksumFor(sums.String(), asset)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp("", "lima-*.tar.gz")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	defer tmp.Close()
	h := sha256.New()
	if err := download(base+asset, io.MultiWriter(tmp, h)); err != nil {
		return err
	}
	if got := hex.EncodeToString(h.Sum(nil)); got != want {
		return fmt.Errorf("%s failed its checksum: got %s, want %s", asset, got, want)
	}
	if _, err := tmp.Seek(0, io.SeekStart); err != nil {
		return err
	}
	return extract(tmp, in.prefix)
}

// extract unpacks a .tar.gz into dir, refusing entries that would land outside it.
func extract(r io.Reader, dir string) error {
	gz, err := gzip.NewReader(r)
	if err != nil {
		return err
	}
	defer gz.Close()
	root, err := filepath.Abs(dir)
	if err != nil {
		return err
	}
	inside := func(p string) bool {
		return p == root || strings.HasPrefix(p, root+string(filepath.Separator))
	}
	tr := tar.NewReader(gz)
	for {
		h, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		target := filepath.Join(root, filepath.FromSlash(h.Name))
		if !inside(target) {
			return fmt.Errorf("archive entry %q is outside the install directory", h.Name)
		}
		switch h.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := writeFile(target, tr, os.FileMode(h.Mode).Perm()); err != nil {
				return err
			}
		case tar.TypeSymlink:
			link := h.Linkname
			if filepath.IsAbs(link) || !inside(filepath.Join(filepath.Dir(target), link)) {
				return fmt.Errorf("archive link %q points outside the install directory", h.Name)
			}
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			os.Remove(target)
			if err := os.Symlink(link, target); err != nil {
				return err
			}
		}
	}
}

// writeFile writes through a temporary file, so a running binary can be replaced.
func writeFile(target string, r io.Reader, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(target), ".install-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := io.Copy(tmp, r); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmp.Name(), mode); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), target)
}

// installSelf copies the running binary into BinDir unless it's already there.
func (in *installer) installSelf(exe string) error {
	target := filepath.Join(in.opt.BinDir, binaryName)
	if a, err := os.Stat(exe); err == nil {
		if b, err := os.Stat(target); err == nil && os.SameFile(a, b) {
			return nil
		}
	}
	f, err := os.Open(exe)
	if err != nil {
		return err
	}
	defer f.Close()
	return writeFile(target, f, 0o755)
}

func username() string {
	if u, err := user.Current(); err == nil {
		return u.Username
	}
	return "$USER"
}

func onPath(dir, path string) bool {
	for _, p := range filepath.SplitList(path) {
		if filepath.Clean(p) == filepath.Clean(dir) {
			return true
		}
	}
	return false
}

// Run is the `awesome_labs install` command.
func Run(args []string, in io.Reader, out io.Writer) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	opt := Options{In: in, Out: out}
	fs := flag.NewFlagSet("install", flag.ContinueOnError)
	fs.SetOutput(out)
	fs.BoolVar(&opt.DryRun, "dry-run", false, "show what would be done, and change nothing")
	fs.BoolVar(&opt.Yes, "yes", false, "don't ask for confirmation")
	fs.StringVar(&opt.BinDir, "bin-dir", filepath.Join(home, ".local", "bin"), "where to put the awesome_labs binary")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if os.Geteuid() == 0 {
		return errors.New("run this as your normal user, not root: Lima refuses to run as root, and sudo is used only for installing packages")
	}
	sys, err := Detect()
	if err != nil {
		return err
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	inst := &installer{sys: sys, opt: opt, prefix: filepath.Join(home, ".local")}

	name := "macOS"
	if sys.OS == "linux" {
		name = "Linux (" + sys.DistroID + ")"
	}
	fmt.Fprintf(out, "Detected %s on %s\n", name, sys.Arch)

	st := state{lima: limaUsable(), qemu: found(qemuBinary(sys)) && found("qemu-img") && found("ssh") && firmwareFound(sys.Arch), brew: found("brew")}
	steps, err := inst.plan(st)
	if err != nil {
		return err
	}
	steps = append(steps, step{"copy the awesome_labs binary to " + opt.BinDir, func() error {
		return inst.installSelf(exe)
	}})

	fmt.Fprintln(out, "\nThis will:")
	for _, s := range steps {
		fmt.Fprintln(out, "  -", s.desc)
	}
	if opt.DryRun {
		fmt.Fprintln(out, "\nDry run: nothing was changed.")
		return nil
	}
	if !opt.Yes {
		fmt.Fprint(out, "\nContinue? [y/N] ")
		line, _ := bufio.NewReader(in).ReadString('\n')
		if a := strings.ToLower(strings.TrimSpace(line)); a != "y" && a != "yes" {
			return errors.New("cancelled: nothing was changed")
		}
	}
	for _, s := range steps {
		fmt.Fprintln(out, "==>", s.desc)
		if err := s.run(); err != nil {
			return err
		}
	}

	path := os.Getenv("PATH")
	for _, d := range []string{opt.BinDir, filepath.Join(inst.prefix, "bin")} {
		if !onPath(d, path) && (d == opt.BinDir || !st.lima) {
			fmt.Fprintf(out, "\nNote: %s isn't on your PATH. Add it to your shell profile:\n  export PATH=\"%s:$PATH\"\n", d, d)
			break
		}
	}
	if sys.OS == "linux" {
		if f, err := os.OpenFile("/dev/kvm", os.O_RDWR, 0); err != nil {
			fmt.Fprintf(out, "\nWarning: this user can't open /dev/kvm, so VMs will be very slow or won't start.\nTry: sudo usermod -aG kvm %s (then log in again)\n", username())
		} else {
			f.Close()
		}
	}
	fmt.Fprintln(out, "\nDone. Start with: awesome_labs")
	return nil
}
