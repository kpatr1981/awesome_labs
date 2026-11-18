package local

import (
	"archive/tar"
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"testing/fstest"

	"awesome_labs/internal/exercise"
)

func TestHostRisk(t *testing.T) {
	cases := map[string]string{
		"truncate -s 100M /lab/d.img\nlosetup -fP /lab/d.img": "loop devices",
		"mount -o loop /lab/d.img /mnt":                       "loop devices",
		"vgcreate data /dev/loop0":                            "LVM",
		"mkfs.ext4 -q /dev/loop0":                             "filesystems on block devices",
		"swapoff -a":                                          "swap",
		"sysctl --system":                                     "applying sysctl files",
		"sysctl -w kernel.pid_max=4096":                       "kernel sysctls",
		"echo 1 > /proc/sys/vm/drop_caches":                   "kernel sysctls",
		"date -s '2030-01-01'":                                "the system clock",
		"modprobe -r wireguard":                               "unloading kernel modules",
		// fine in the container's own namespaces
		"sysctl -w net.ipv4.ip_forward=1":                         "",
		"ip netns add a && ip link add v0 type veth peer name v1": "",
		"docker run -d --name web nginx:1.27-alpine":              "",
		"date +%s > /lab/now":                                     "",
		"mount | grep overlay":                                    "",
	}
	for script, want := range cases {
		ex := &exercise.Exercise{Profile: exercise.ProfileDocker, Setup: script}
		if got := HostRisk(ex); got != want {
			t.Errorf("HostRisk(%q) = %q, want %q", script, got, want)
		}
	}
}

func TestUnsupported(t *testing.T) {
	cases := []struct {
		ex   exercise.Exercise
		want bool
	}{
		{exercise.Exercise{Profile: exercise.ProfileDocker}, false},
		{exercise.Exercise{Profile: exercise.ProfileK8s}, true},
		{exercise.Exercise{Profile: exercise.ProfileOffsec}, true},
		{exercise.Exercise{Profile: exercise.ProfileDocker, Destructive: true}, true},
		{exercise.Exercise{Profile: exercise.ProfileDocker, CheckWorker: "true"}, true},
	}
	for _, c := range cases {
		if got := Unsupported(&c.ex) != ""; got != c.want {
			t.Errorf("Unsupported(%+v) = %v, want %v", c.ex, got, c.want)
		}
	}
}

// fake runs the exercise "scripts" as Go: an exit code per script body marker.
type fake struct {
	solved bool
	codes  map[string]int // substring -> exit code
	calls  []string
}

func (f *fake) script(_ context.Context, asRoot bool, wd, body string, _ map[string]string, _ io.Writer) (int, error) {
	who := "user"
	if asRoot {
		who = "root"
	}
	f.calls = append(f.calls, who+"@"+wd+":"+firstLine(body[strings.Index(body, "\n")+1:]))
	switch {
	case strings.Contains(body, "SOLVE"):
		f.solved = true
		return f.codes["SOLVE"], nil
	case strings.Contains(body, "CHECK"):
		if f.solved {
			return f.codes["CHECK-after"], nil
		}
		return f.codes["CHECK-before"], nil
	case strings.Contains(body, "SETUP"):
		return f.codes["SETUP"], nil
	}
	return 0, nil
}

func firstLine(s string) string { return strings.SplitN(s, "\n", 2)[0] }

func TestVerifySteps(t *testing.T) {
	ex := &exercise.Exercise{ID: "x-1", Title: "t", Profile: exercise.ProfileDocker, Setup: "SETUP", Check: "CHECK", Solve: "SOLVE", Scenario: "s"}
	cases := []struct {
		codes map[string]int
		want  string
	}{
		{map[string]int{"CHECK-before": 1}, ResultOK},
		{map[string]int{"CHECK-before": 0}, ResultPassEarly},
		{map[string]int{"SETUP": 2, "CHECK-before": 1}, ResultSetupError},
		{map[string]int{"CHECK-before": 1, "SOLVE": 1}, ResultSolveError},
		{map[string]int{"CHECK-before": 1, "CHECK-after": 1}, ResultFailsAfter},
	}
	for _, c := range cases {
		f := &fake{codes: c.codes}
		if got := verifySteps(context.Background(), ex, f, io.Discard); got != c.want {
			t.Errorf("codes %v: got %q, want %q (calls %v)", c.codes, got, c.want, f.calls)
		}
	}
	// setup and checks run as root from /, the solve as the lab user in /lab
	f := &fake{codes: map[string]int{"CHECK-before": 1}}
	verifySteps(context.Background(), ex, f, io.Discard)
	want := []string{"root@/:SETUP", "root@/:source /opt/kubelab/lib.sh", "root@/:CHECK", "user@/lab:SOLVE", "root@/:CHECK"}
	if strings.Join(f.calls, "|") != strings.Join(want, "|") {
		t.Errorf("calls:\n got %v\nwant %v", f.calls, want)
	}
}

type errStepper struct{}

func (errStepper) script(context.Context, bool, string, string, map[string]string, io.Writer) (int, error) {
	return -1, errors.New("container gone")
}

func TestVerifyStepsHarnessError(t *testing.T) {
	ex := &exercise.Exercise{ID: "x-1", Check: "CHECK", Solve: "SOLVE"}
	if got := verifySteps(context.Background(), ex, errStepper{}, io.Discard); got != ResultCheckError {
		t.Errorf("got %q, want %q", got, ResultCheckError)
	}
}

func TestExScript(t *testing.T) {
	if got := exScript("echo hi", true); got != "source /opt/kubelab/lib.sh\necho hi\nkl_finish\n" {
		t.Errorf("exScript with finish: %q", got)
	}
	if got := exScript("echo hi", false); strings.Contains(got, "kl_finish") {
		t.Errorf("exScript without finish: %q", got)
	}
}

func TestKitTar(t *testing.T) {
	assets := fstest.MapFS{
		"vms/kit/lib.sh":          {Data: []byte("# lib")},
		"vms/kit/images.txt":      {Data: []byte("nginx:1.27-alpine d")},
		"vms/bootstrap/docker.sh": {Data: []byte("# not kit")},
		"vms/bootstrap/common.sh": {Data: []byte("# not kit")},
	}
	b, err := kitTar(assets)
	if err != nil {
		t.Fatal(err)
	}
	tr := tar.NewReader(bytes.NewReader(b))
	var names []string
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		if h.Mode != 0o755 {
			t.Errorf("%s: mode %o", h.Name, h.Mode)
		}
		names = append(names, h.Name)
	}
	if strings.Join(names, ",") != "images.txt,lib.sh" {
		t.Errorf("kit files %v", names)
	}
}

func TestStoreHashFollowsInputs(t *testing.T) {
	mk := func(images string) fstest.MapFS {
		return fstest.MapFS{
			"vms/bootstrap/common.sh": {Data: []byte("c")},
			"vms/bootstrap/docker.sh": {Data: []byte("d")},
			"vms/kit/images.txt":      {Data: []byte(images)},
		}
	}
	a, err := storeHash(mk("nginx:1.27-alpine d"))
	if err != nil {
		t.Fatal(err)
	}
	b, _ := storeHash(mk("nginx:1.27-alpine d"))
	c, _ := storeHash(mk("nginx:1.28-alpine d"))
	if a != b || a == c {
		t.Errorf("store hash: same inputs %q %q, changed images %q", a, b, c)
	}
	if !strings.HasPrefix(imageTag(), "kl-local-host:") {
		t.Errorf("image tag %q", imageTag())
	}
}

func TestSanitize(t *testing.T) {
	if got := sanitize("offsec-cry-001"); got != "offsec-cry-001" {
		t.Errorf("sanitize kept %q", got)
	}
	if got := sanitize("Ab/c d"); got != "ab-c-d" {
		t.Errorf("sanitize: %q", got)
	}
}

func TestLineFilter(t *testing.T) {
	var progress, log bytes.Buffer
	w := stepWriter(&progress, &log)
	io.WriteString(w, "+ apt-get update\n==> pulling 3 ima")
	io.WriteString(w, "ges\nnoise\n")
	if progress.String() != "   ==> pulling 3 images\n" {
		t.Errorf("progress %q", progress.String())
	}
	if !strings.Contains(log.String(), "noise") {
		t.Errorf("log lost lines: %q", log.String())
	}
}
