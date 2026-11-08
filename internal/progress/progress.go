// Package progress records which exercises a user started and solved.
package progress

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Entry is the state of one exercise.
type Entry struct {
	Started     time.Time `json:"started,omitempty"`
	Solved      time.Time `json:"solved,omitempty"`
	Checks      int       `json:"checks,omitempty"`
	HintsUsed   int       `json:"hints_used,omitempty"`
	SawSolution bool      `json:"saw_solution,omitempty"`
}

// Mark is a one-character status marker.
func (e Entry) Mark() string {
	switch {
	case !e.Solved.IsZero():
		return "✔"
	case !e.Started.IsZero():
		return "●"
	}
	return " "
}

// IsZero reports whether nothing is recorded in e.
func (e Entry) IsZero() bool {
	return e.Started.IsZero() && e.Solved.IsZero() && e.Checks == 0 && e.HintsUsed == 0 && !e.SawSolution
}

// Store is a JSON file of entries keyed by exercise id.
type Store struct {
	path string
	mu   sync.Mutex
	data map[string]Entry
}

// Open loads (or creates) dir/progress.json.
func Open(dir string) (*Store, error) {
	s := &Store{path: filepath.Join(dir, "progress.json"), data: map[string]Entry{}}
	b, err := os.ReadFile(s.path)
	if err == nil {
		if err := json.Unmarshal(b, &s.data); err != nil {
			return nil, err
		}
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	return s, nil
}

// Get returns the entry for id.
func (s *Store) Get(id string) Entry {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.data[id]
}

func (s *Store) update(id string, f func(*Entry)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	e := s.data[id]
	f(&e)
	s.data[id] = e
	s.save()
}

// save writes the store to disk; the caller holds s.mu.
func (s *Store) save() {
	b, _ := json.MarshalIndent(s.data, "", "  ")
	tmp := s.path + ".tmp"
	if os.WriteFile(tmp, b, 0o644) == nil {
		_ = os.Rename(tmp, s.path)
	}
}

// MarkStarted records that the lab was prepared for id.
func (s *Store) MarkStarted(id string) {
	s.update(id, func(e *Entry) { e.Started = time.Now() })
}

// RecordCheck records a check result.
func (s *Store) RecordCheck(id string, passed bool) {
	s.update(id, func(e *Entry) {
		e.Checks++
		if passed && e.Solved.IsZero() {
			e.Solved = time.Now()
		}
	})
}

// UseHint records that hint n (1-based) was revealed.
func (s *Store) UseHint(id string, n int) {
	s.update(id, func(e *Entry) {
		if n > e.HintsUsed {
			e.HintsUsed = n
		}
	})
}

// SawSolution records that the solution was opened.
func (s *Store) SawSolution(id string) {
	s.update(id, func(e *Entry) { e.SawSolution = true })
}

// Reset forgets id: started, solved, checks, hints and solution views.
func (s *Store) Reset(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.data, id)
	s.save()
}

// Counts returns (solved, started) totals over ids.
func (s *Store) Counts(ids []string) (solved, started int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, id := range ids {
		e := s.data[id]
		if !e.Solved.IsZero() {
			solved++
		} else if !e.Started.IsZero() {
			started++
		}
	}
	return
}
