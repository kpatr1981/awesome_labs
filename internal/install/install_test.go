package install

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseOSRelease(t *testing.T) {
	id, like := parseOSRelease("NAME=\"Ubuntu\"\nID=ubuntu\nID_LIKE=\"debian\"\nVERSION_ID=\"24.04\"\n")
	if id != "ubuntu" || like != "debian" {
		t.Fatalf("got %q, %q", id, like)
	}
}

func TestQemuPackages(t *testing.T) {
	cases := []struct {
		sys     System
		manager string
		first   string
	}{
		{System{Arch: "amd64", DistroID: "fedora"}, "dnf", "qemu-kvm"},
		{System{Arch: "amd64", DistroID: "ubuntu", IDLike: "debian"}, "apt-get", "qemu-system-x86"},
		{System{Arch: "arm64", DistroID: "debian"}, "apt-get", "qemu-system-arm"},
		{System{Arch: "amd64", DistroID: "linuxmint", IDLike: "ubuntu debian"}, "apt-get", "qemu-system-x86"},
		{System{Arch: "amd64", DistroID: "arch"}, "pacman", "qemu-base"},
		{System{Arch: "amd64", DistroID: "opensuse-tumbleweed", IDLike: "opensuse suse"}, "zypper", "qemu-x86"},
	}
	for _, c := range cases {
		p, err := qemuPackages(c.sys)
		if err != nil || p.Manager != c.manager || p.Packages[0] != c.first {
			t.Errorf("%+v: got %+v, %v", c.sys, p, err)
		}
	}
	if _, err := qemuPackages(System{DistroID: "gentoo"}); err == nil {
		t.Error("expected an error for an unknown distribution")
	}
}

func TestLimaAsset(t *testing.T) {
	cases := map[string]System{
		"lima-2.3.0-Linux-x86_64.tar.gz":  {OS: "linux", Arch: "amd64"},
		"lima-2.3.0-Linux-aarch64.tar.gz": {OS: "linux", Arch: "arm64"},
		"lima-2.3.0-Darwin-x86_64.tar.gz": {OS: "darwin", Arch: "amd64"},
		"lima-2.3.0-Darwin-arm64.tar.gz":  {OS: "darwin", Arch: "arm64"},
	}
	for want, s := range cases {
		if got := limaAsset(s, "2.3.0"); got != want {
			t.Errorf("%+v: got %s", s, got)
		}
	}
}

func TestLimaMajor(t *testing.T) {
	if got := limaMajor("limactl version 2.3.0\n"); got != 2 {
		t.Errorf("got %d", got)
	}
	if got := limaMajor("nonsense"); got != 0 {
		t.Errorf("got %d", got)
	}
}

func TestPlan(t *testing.T) {
	desc := func(sys System, st state) string {
		steps, err := (&installer{sys: sys, prefix: "/p"}).plan(st)
		if err != nil {
			t.Fatal(err)
		}
		var d []string
		for _, s := range steps {
			d = append(d, s.desc)
		}
		return strings.Join(d, "|")
	}
	fedora := System{OS: "linux", Arch: "amd64", DistroID: "fedora"}
	mac := System{OS: "darwin", Arch: "arm64"}
	if got := desc(fedora, state{lima: true, qemu: true}); got != "" {
		t.Errorf("nothing should be needed, got %q", got)
	}
	if got := desc(fedora, state{}); !strings.Contains(got, "download Lima") || !strings.Contains(got, "dnf") {
		t.Errorf("fedora: %q", got)
	}
	if got := desc(mac, state{brew: true}); !strings.Contains(got, "Homebrew") || strings.Contains(got, "QEMU") {
		t.Errorf("mac with brew: %q", got)
	}
	if got := desc(mac, state{}); !strings.Contains(got, "download Lima") {
		t.Errorf("mac without brew: %q", got)
	}
}

func TestChecksumFor(t *testing.T) {
	sums := "abc  lima-a.tar.gz\ndef *lima-b.tar.gz\n"
	if got, _ := checksumFor(sums, "lima-b.tar.gz"); got != "def" {
		t.Errorf("got %q", got)
	}
	if _, err := checksumFor(sums, "lima-c.tar.gz"); err == nil {
		t.Error("expected an error for a missing file")
	}
}

func archive(t *testing.T, name string, typ byte, link string) *bytes.Buffer {
	t.Helper()
	var b bytes.Buffer
	gz := gzip.NewWriter(&b)
	tw := tar.NewWriter(gz)
	body := []byte("x")
	h := &tar.Header{Name: name, Mode: 0o755, Typeflag: typ, Linkname: link}
	if typ == tar.TypeReg {
		h.Size = int64(len(body))
	}
	if err := tw.WriteHeader(h); err != nil {
		t.Fatal(err)
	}
	if typ == tar.TypeReg {
		tw.Write(body)
	}
	tw.Close()
	gz.Close()
	return &b
}

func TestExtract(t *testing.T) {
	dir := t.TempDir()
	if err := extract(archive(t, "bin/limactl", tar.TypeReg, ""), dir); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(filepath.Join(dir, "bin", "limactl"))
	if err != nil || info.Mode().Perm() != 0o755 {
		t.Fatalf("%v, %v", info, err)
	}
	if err := extract(archive(t, "../evil", tar.TypeReg, ""), dir); err == nil {
		t.Error("a path outside the directory must be refused")
	}
	if err := extract(archive(t, "bin/link", tar.TypeSymlink, "../../evil"), dir); err == nil {
		t.Error("a link outside the directory must be refused")
	}
}

func TestOnPath(t *testing.T) {
	if !onPath("/a/bin", "/x:/a/bin/:/y") || onPath("/a/bin", "/x:/y") {
		t.Error("onPath is wrong")
	}
}
