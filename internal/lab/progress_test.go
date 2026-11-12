package lab

import (
	"strings"
	"testing"
)

func TestStepWriterForwardsOnlyStepLines(t *testing.T) {
	var b strings.Builder
	w := stepWriter(&b, "docker-host")
	w.Write([]byte("+ echo '==> pulling 2 images'\n==> pulling 2 images\n+ docker pull -q nginx\ndocker.io/library/nginx\n==> pul"))
	w.Write([]byte("led nginx (1/2)\n"))
	want := "==> [docker-host] pulling 2 images\n==> [docker-host] pulled nginx (1/2)\n"
	if b.String() != want {
		t.Fatalf("got %q, want %q", b.String(), want)
	}
}
