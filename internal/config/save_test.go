package config

import (
	"os"
	"strings"
	"testing"
)

func TestSetKeepsCommentsAndOtherKeys(t *testing.T) {
	dir := t.TempDir()
	if _, err := Load(dir); err != nil { // writes the template
		t.Fatal(err)
	}
	old, _ := os.ReadFile(Path(dir))
	c, err := Set(dir, Setting{"on_exit", "delete"}, Setting{"auto_stop", false}, Setting{"delete_stopped_after_days", 7})
	if err != nil {
		t.Fatal(err)
	}
	if c.ExitAction() != "delete" || c.AutoStopEnabled() || c.DeleteStoppedAfterDays != 7 {
		t.Fatalf("returned config %+v", c)
	}
	b, _ := os.ReadFile(Path(dir))
	s := string(b)
	for _, want := range []string{"on_exit: delete\n", "auto_stop: false\n", "delete_stopped_after_days: 7\n", "# What leaving the app does", "# docker:       {cpus: 2, memory: 4GiB}"} {
		if !strings.Contains(s, want) {
			t.Errorf("missing %q in:\n%s", want, s)
		}
	}
	if strings.Contains(s, "# on_exit:") || strings.Contains(s, "# delete_stopped_after_days:") {
		t.Errorf("commented-out key left behind:\n%s", s)
	}
	if strings.Count(s, "\n") != strings.Count(string(old), "\n") {
		t.Errorf("lines added or lost: %d -> %d", strings.Count(string(old), "\n"), strings.Count(s, "\n"))
	}
	got, err := Load(dir)
	if err != nil || got.ExitAction() != "delete" {
		t.Fatalf("reload: %+v, %v", got, err)
	}
}

func TestSetReplacesValueKeepsTrailingCommentAndAppends(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(Path(dir), []byte("theme: dark   # mine\ndocker: {cpus: 4, memory: 8GiB}\n"), 0o644)
	if _, err := Set(dir, Setting{"theme", "light"}, Setting{"check_updates", true}); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(Path(dir))
	want := "theme: light   # mine\ndocker: {cpus: 4, memory: 8GiB}\ncheck_updates: true\n"
	if string(b) != want {
		t.Fatalf("got:\n%s\nwant:\n%s", b, want)
	}
}

func TestSetRefusesInvalid(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(Path(dir), []byte("theme: dark\n"), 0o644)
	if _, err := Set(dir, Setting{"theme", "neon"}); err == nil {
		t.Fatal("an invalid theme was accepted")
	}
	if _, err := Set(dir, Setting{"delete_stopped_after_days", -1}); err == nil {
		t.Fatal("a negative day count was accepted")
	}
	b, _ := os.ReadFile(Path(dir))
	if string(b) != "theme: dark\n" {
		t.Fatalf("the file changed after a refused write: %q", b)
	}
	if _, err := os.Stat(Path(dir) + ".tmp"); !os.IsNotExist(err) {
		t.Fatal("temporary file left behind")
	}
}
