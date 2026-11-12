package lab

import (
	"strings"
	"testing"
	"testing/fstest"
)

func baseAssets() fstest.MapFS {
	return fstest.MapFS{
		"vms/docker.yaml":           {Data: []byte("cpus: __CPUS__\nmemory: __MEMORY__\n")},
		"vms/bootstrap/docker.sh":   {Data: []byte("echo pull\n")},
		"vms/kit/reset-docker.sh":   {Data: []byte("echo reset\n")},
		"vms/k8s-node.yaml":         {Data: []byte("x")},
		"vms/offsec.yaml":           {Data: []byte("x")},
		"vms/bootstrap/common.sh":   {Data: []byte("x")},
		"vms/kit/lib.sh":            {Data: []byte("x")},
		"vms/kit/images.txt":        {Data: []byte("nginx:1.29-alpine\n")},
		"vms/bootstrap/k8s-init.sh": {Data: []byte("x")},
	}
}

func TestBaseNameOutsideLabPrefix(t *testing.T) {
	for _, env := range AllEnvs() {
		for _, vm := range env.VMs {
			n := baseName(vm)
			// the lab-VM deletes (app start/exit, stray cleanup) sweep kl-*
			if strings.HasPrefix(n, "kl-") || n == vm.Name {
				t.Errorf("base of %s is %q, which the lab-VM deletes would remove", vm.Name, n)
			}
		}
	}
}

func TestBaseKeyTracksWhatShapesTheVM(t *testing.T) {
	assets := baseAssets()
	m := &Manager{Assets: assets, User: "alice"}
	vm := DockerEnv.Primary()
	key := func(m *Manager, vm VM) string {
		k, err := m.baseKey(DockerEnv, vm)
		if err != nil {
			t.Fatal(err)
		}
		return k
	}
	k0 := key(m, vm)
	if k0 != key(m, vm) {
		t.Fatal("key is not stable")
	}
	bigger := vm
	bigger.Memory = "8GiB"
	if key(m, bigger) == k0 {
		t.Error("a VM size change keeps the old base")
	}
	if key(&Manager{Assets: assets, User: "bob"}, vm) == k0 {
		t.Error("another lab user keeps the old base")
	}
	for _, p := range []string{"vms/bootstrap/docker.sh", "vms/kit/images.txt"} {
		changed := baseAssets()
		changed[p] = &fstest.MapFile{Data: append([]byte("# edited\n"), changed[p].Data...)}
		if key(&Manager{Assets: changed, User: "alice"}, vm) == k0 {
			t.Errorf("editing %s keeps the old base", p)
		}
	}
}
