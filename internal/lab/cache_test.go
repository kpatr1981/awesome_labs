package lab

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"testing/fstest"
)

func TestCacheFileRoundTrip(t *testing.T) {
	for _, ref := range []string{"nginx:1.27-alpine", "quay.io/frrouting/frr:9.1.0", "docker.io/library/busybox:1.37", "kl-datascience:1"} {
		f := cacheFile(ref)
		if filepath.Base(f) != f {
			t.Fatalf("%q: file name %q has a path separator", ref, f)
		}
		got, ok := refOf(f)
		if !ok || got != ref {
			t.Fatalf("refOf(cacheFile(%q)) = %q, %v", ref, got, ok)
		}
	}
	if _, ok := refOf(".partial-123"); ok {
		t.Fatal("a partial download must not count as cached")
	}
}

func TestShortRef(t *testing.T) {
	cases := map[string]string{
		"docker.io/library/nginx:1.27-alpine": "nginx:1.27-alpine",
		"docker.io/traefik/whoami:v1.10":      "traefik/whoami:v1.10",
		"quay.io/frrouting/frr:9.1.0":         "quay.io/frrouting/frr:9.1.0",
		"nginx:1.27-alpine":                   "nginx:1.27-alpine",
	}
	for in, want := range cases {
		if got := shortRef(in); got != want {
			t.Errorf("shortRef(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestCachedRefsSkipsPartials(t *testing.T) {
	m := &Manager{StateDir: t.TempDir()}
	dir := m.ImageCacheDir(rtDocker)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{cacheFile("redis:7.4-alpine"), cacheFile("alpine:3.22"), ".partial-9"} {
		if err := os.WriteFile(filepath.Join(dir, f), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if got, want := m.cachedRefs(rtDocker), []string{"alpine:3.22", "redis:7.4-alpine"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("cachedRefs = %v, want %v", got, want)
	}
	if got := m.cachedRefs(rtContainerd); len(got) != 0 {
		t.Fatalf("containerd cache should be empty, got %v", got)
	}
}

func TestListedImages(t *testing.T) {
	m := &Manager{Assets: fstest.MapFS{"vms/kit/images.txt": {Data: []byte(
		"# image   where\nnginx:1.27-alpine  dk\nubuntu:24.04  d\nquay.io/metallb/speaker:v0.16.1  k\n")}}}
	got := m.listedImages("k")
	want := map[string]bool{"nginx:1.27-alpine": true, "quay.io/metallb/speaker:v0.16.1": true}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("listedImages(k) = %v, want %v", got, want)
	}
}
