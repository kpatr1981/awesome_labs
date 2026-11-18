package verified

import (
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"awesome_labs/internal/exercise"
)

const modA = `track: docker
module: A
profile: docker
order: 1
exercises:
- id: d-1
  title: One
  difficulty: 1
  kind: build
  scenario: s
  solution: s
  setup: echo setup1
  check: echo check1
  solve: echo solve1
- id: d-2
  title: Two
  difficulty: 2
  kind: fix
  scenario: s
  solution: s
  check: echo check2
  solve: echo solve2
`

func load(t *testing.T, files map[string]string) *exercise.Catalog {
	t.Helper()
	fsys := fstest.MapFS{}
	for k, v := range files {
		fsys["ex/"+k] = &fstest.MapFile{Data: []byte(v)}
	}
	cat, err := exercise.Load(fsys, "ex")
	if err != nil {
		t.Fatal(err)
	}
	return cat
}

func reasons(p []Pending) string {
	var s []string
	for _, x := range p {
		s = append(s, x.ID+":"+x.Reason)
	}
	return strings.Join(s, " ")
}

func TestHashIgnoresTextAndPlace(t *testing.T) {
	cat := load(t, map[string]string{"a.yaml": modA})
	h := Hash(cat.Get("d-1"))
	// reworded, re-tagged, moved after d-2 in another file name: same hash
	moved := strings.Replace(modA, "title: One", "title: One, reworded\n  tags: [x]\n  tutorial: more", 1)
	parts := strings.SplitN(moved, "- id: d-2", 2)
	head, one := parts[0][:strings.Index(parts[0], "- id: d-1")], parts[0][strings.Index(parts[0], "- id: d-1"):]
	moved = head + "- id: d-2" + parts[1] + one
	cat2 := load(t, map[string]string{"z.yaml": moved})
	if cat2.Get("d-1").Title == cat.Get("d-1").Title || cat2.Exercises[0].ID != "d-2" {
		t.Fatal("test setup: the exercise was not reworded and moved")
	}
	if got := Hash(cat2.Get("d-1")); got != h {
		t.Errorf("rewording or moving changed the hash")
	}
	// a script change does change it
	cat3 := load(t, map[string]string{"a.yaml": strings.Replace(modA, "echo check1", "echo check1b", 1)})
	if Hash(cat3.Get("d-1")) == h {
		t.Errorf("a changed check kept the hash")
	}
	cat4 := load(t, map[string]string{"a.yaml": strings.Replace(modA, "profile: docker", "profile: offsec", 1)})
	if Hash(cat4.Get("d-1")) == h {
		t.Errorf("a changed profile kept the hash")
	}
}

func TestRecordGate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "verified.json")
	cat := load(t, map[string]string{"a.yaml": modA})
	r, err := Load(path) // missing: empty
	if err != nil {
		t.Fatal(err)
	}
	if got := reasons(r.Unverified(cat)); got != "d-1:new d-2:new" {
		t.Errorf("empty record: %s", got)
	}
	r.Mark(cat.Get("d-1"), "2026-11-18", "v0.20.0", "")
	if err := r.Save(path); err != nil {
		t.Fatal(err)
	}
	r, err = Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := reasons(r.Unverified(cat)); got != "d-2:new" {
		t.Errorf("after marking d-1: %s", got)
	}
	changed := load(t, map[string]string{"a.yaml": strings.Replace(modA, "echo solve1", "echo solve1 && true", 1)})
	p := r.Unverified(changed)
	if got := reasons(p); got != "d-1:changed d-2:new" {
		t.Errorf("after changing d-1: %s", got)
	}
	if p[0].Last.Date != "2026-11-18" {
		t.Errorf("changed entry lost its last date: %+v", p[0].Last)
	}
	// an id that went away
	only2 := load(t, map[string]string{"a.yaml": modA[:strings.Index(modA, "- id: d-1")] + modA[strings.Index(modA, "- id: d-2"):]})
	if g := r.Gone(only2); len(g) != 1 || g[0] != "d-1" {
		t.Errorf("gone: %v", g)
	}
	r.Prune(only2)
	if _, ok := r.Exercises["d-1"]; ok {
		t.Error("prune kept d-1")
	}
}

func TestRewriteCounts(t *testing.T) {
	c := Counts{Track: map[string]int{"docker": 199, "git": 12}, Total: 1553}
	in := "| **Docker** | x | <!-- count:docker -->184<!-- /count --> |\n" +
		"| Git | <!-- count:git -->12<!-- /count --> |\n" +
		"That's <!-- count:total -->1546<!-- /count --> so far (<!-- count:total -->1,546<!-- /count -->)."
	out, ch, err := Rewrite(in, c)
	if err != nil {
		t.Fatal(err)
	}
	want := "| **Docker** | x | <!-- count:docker -->199<!-- /count --> |\n" +
		"| Git | <!-- count:git -->12<!-- /count --> |\n" +
		"That's <!-- count:total -->1553<!-- /count --> so far (<!-- count:total -->1,553<!-- /count -->)."
	if out != want {
		t.Errorf("rewrite:\n%s\nwant:\n%s", out, want)
	}
	if len(ch) != 3 {
		t.Errorf("changes: %v", ch)
	}
	if _, _, err := Rewrite("<!-- count:nope -->1<!-- /count -->", c); err == nil {
		t.Error("an unknown track was accepted")
	}
	if m := Marked(in); !m["docker"] || !m["git"] || !m["total"] || len(m) != 3 {
		t.Errorf("marked: %v", m)
	}
}
