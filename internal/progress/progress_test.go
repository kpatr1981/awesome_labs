package progress

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResetRemovesEntryFromDisk(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	s.MarkStarted("a")
	s.RecordCheck("b", true)
	s.Reset("a")

	b, err := os.ReadFile(filepath.Join(dir, "progress.json"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), `"a"`) {
		t.Errorf("entry a still on disk: %s", b)
	}

	r, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !r.Get("a").IsZero() {
		t.Errorf("a not forgotten: %+v", r.Get("a"))
	}
	if r.Get("b").Solved.IsZero() {
		t.Error("resetting a also changed b")
	}
	if solved, started := r.Counts([]string{"a", "b"}); solved != 1 || started != 0 {
		t.Errorf("Counts = %d solved, %d started; want 1, 0", solved, started)
	}
}
