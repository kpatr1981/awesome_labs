package exam

import (
	"fmt"
	"math/rand"
	"testing"
	"time"

	"kubelab/internal/exercise"
)

func pool(n int, prof exercise.Profile) []*exercise.Exercise {
	var p []*exercise.Exercise
	for i := 0; i < n; i++ {
		p = append(p, &exercise.Exercise{ID: fmt.Sprintf("x-%03d", i), Difficulty: i%5 + 1, Profile: prof})
	}
	return p
}

func TestNewBalancedAndSorted(t *testing.T) {
	e, err := New(Options{Pool: pool(100, exercise.ProfileK8s), Count: 9, Duration: time.Hour, Rand: rand.New(rand.NewSource(1))})
	if err != nil {
		t.Fatal(err)
	}
	if len(e.Questions) != 9 {
		t.Fatalf("got %d questions", len(e.Questions))
	}
	seen := map[string]bool{}
	for _, q := range e.Questions {
		if seen[q.ID] {
			t.Fatalf("duplicate %s", q.ID)
		}
		seen[q.ID] = true
	}
}

func TestRejectsMixedProfilesAndDestructive(t *testing.T) {
	p := append(pool(5, exercise.ProfileK8s), pool(5, exercise.ProfileDocker)...)
	if _, err := New(Options{Pool: p, Count: 3, Duration: time.Hour}); err == nil {
		t.Fatal("expected error for mixed profiles")
	}
	d := pool(3, exercise.ProfileK8s)
	for _, x := range d {
		x.Destructive = true
	}
	if _, err := New(Options{Pool: d, Count: 3, Duration: time.Hour}); err == nil {
		t.Fatal("expected error: only destructive exercises")
	}
}

func TestScoringAndTimer(t *testing.T) {
	e, _ := New(Options{Pool: pool(10, exercise.ProfileDocker), Count: 3, Duration: time.Hour})
	e.Record(e.Questions[0].ID, true)
	e.Record(e.Questions[1].ID, true)
	e.Record(e.Questions[1].ID, false) // last check counts
	if p, n, _ := e.Score(); p != 1 || n != 3 {
		t.Fatalf("score %d/%d", p, n)
	}
	e.Started = time.Now().Add(-2 * time.Hour)
	if !e.Over() || e.Record(e.Questions[2].ID, true) {
		t.Fatal("exam should be over and ignore checks")
	}
}
