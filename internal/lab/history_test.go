package lab

import "testing"

func TestHistoryOutlivesARebuiltVM(t *testing.T) {
	m := &Manager{StateDir: t.TempDir()}
	m.recordHistory("d-1", "docker ps\n")
	m.recordHistory("d-1", "docker ps\ndocker logs db\n") // the same VM, later
	if got := m.History("d-1"); got != "docker ps\ndocker logs db\n" {
		t.Fatalf("history = %q", got)
	}
	m.recordHistory("d-1", "docker inspect db\n") // a VM built again
	if got := m.History("d-1"); got != "docker ps\ndocker logs db\ndocker inspect db\n" {
		t.Errorf("history after a rebuild = %q", got)
	}
	m.recordHistory("d-1", "")
	if got := m.History("d-1"); got == "" {
		t.Error("an unreachable VM wiped the history")
	}
	m.ForgetHistory("d-1")
	if got := m.History("d-1"); got != "" {
		t.Errorf("after forget: %q", got)
	}
	if historyName("a/b c") != ".kl_history_a_b_c" {
		t.Errorf("unsafe id in file name: %s", historyName("a/b c"))
	}
}
