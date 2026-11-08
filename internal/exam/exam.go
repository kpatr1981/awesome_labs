// Package exam implements timed exams: a random, balanced set of exercises
// solved against the clock, graded by the normal exercise checks.
package exam

import (
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"sort"
	"time"

	"awesome_labs/internal/exercise"
)

// PassMark is the share of questions needed to pass (like the CKA's 66%).
const PassMark = 0.66

// Question is one exercise in an exam.
type Question struct {
	ID       string    `json:"id"`
	Passed   bool      `json:"passed"`
	Checks   int       `json:"checks"`
	PassedAt time.Time `json:"passed_at,omitempty"`
}

// Exam is a running or finished exam.
type Exam struct {
	Title      string        `json:"title"`
	Source     string        `json:"source"` // pool description
	Started    time.Time     `json:"started"`
	Duration   time.Duration `json:"duration"`
	AllowHints bool          `json:"allow_hints"`
	Questions  []Question    `json:"questions"`
	Finished   time.Time     `json:"finished,omitempty"`
}

// Options configure a new exam.
type Options struct {
	Title      string
	Source     string
	Pool       []*exercise.Exercise
	Count      int
	Duration   time.Duration
	AllowHints bool
	Rand       *rand.Rand
}

// New draws a balanced random set of questions from the pool: roughly a
// third easy (1-2), a third medium (3), a third hard (4-5), easiest first.
// Destructive exercises are never chosen, and the pool must use one profile.
func New(o Options) (*Exam, error) {
	r := o.Rand
	if r == nil {
		r = rand.New(rand.NewSource(time.Now().UnixNano()))
	}
	var easy, mid, hard []*exercise.Exercise
	profiles := map[exercise.Profile]bool{}
	for _, e := range o.Pool {
		if e.Destructive {
			continue
		}
		profiles[e.Profile] = true
		switch {
		case e.Difficulty <= 2:
			easy = append(easy, e)
		case e.Difficulty == 3:
			mid = append(mid, e)
		default:
			hard = append(hard, e)
		}
	}
	if len(profiles) > 1 {
		return nil, errors.New("an exam must use a single environment (Docker or Kubernetes)")
	}
	total := len(easy) + len(mid) + len(hard)
	if total == 0 {
		return nil, errors.New("no eligible exercises in this pool")
	}
	n := o.Count
	if n > total {
		n = total
	}
	for _, b := range [][]*exercise.Exercise{easy, mid, hard} {
		r.Shuffle(len(b), func(i, j int) { b[i], b[j] = b[j], b[i] })
	}
	want := [3]int{n / 3, n / 3, n - 2*(n/3)} // hard gets the remainder
	buckets := [][]*exercise.Exercise{easy, mid, hard}
	var picked []*exercise.Exercise
	for i := range buckets {
		k := min(want[i], len(buckets[i]))
		picked = append(picked, buckets[i][:k]...)
		buckets[i] = buckets[i][k:]
	}
	// top up from whatever is left if a bucket was too small
	for _, b := range buckets {
		for len(picked) < n && len(b) > 0 {
			picked = append(picked, b[0])
			b = b[1:]
		}
	}
	sort.SliceStable(picked, func(i, j int) bool { return picked[i].Difficulty < picked[j].Difficulty })
	ex := &Exam{Title: o.Title, Source: o.Source, Started: time.Now(), Duration: o.Duration, AllowHints: o.AllowHints}
	for _, e := range picked {
		ex.Questions = append(ex.Questions, Question{ID: e.ID})
	}
	return ex, nil
}

// Remaining is the time left (0 when over).
func (e *Exam) Remaining() time.Duration {
	if !e.Finished.IsZero() {
		return 0
	}
	left := time.Until(e.Started.Add(e.Duration))
	if left < 0 {
		return 0
	}
	return left
}

// Over reports whether the exam is finished or out of time.
func (e *Exam) Over() bool { return !e.Finished.IsZero() || e.Remaining() == 0 }

// Has reports whether id is one of the exam's questions.
func (e *Exam) Has(id string) bool {
	for _, q := range e.Questions {
		if q.ID == id {
			return true
		}
	}
	return false
}

// Record stores a check result. The last result counts (like a real exam,
// where only the final state is graded). Ignored once the exam is over.
func (e *Exam) Record(id string, passed bool) bool {
	if e.Over() {
		return false
	}
	for i := range e.Questions {
		q := &e.Questions[i]
		if q.ID != id {
			continue
		}
		q.Checks++
		q.Passed = passed
		if passed {
			q.PassedAt = time.Now()
		}
		return true
	}
	return false
}

// Score returns passed, total and the percentage.
func (e *Exam) Score() (int, int, float64) {
	p := 0
	for _, q := range e.Questions {
		if q.Passed {
			p++
		}
	}
	if len(e.Questions) == 0 {
		return 0, 0, 0
	}
	return p, len(e.Questions), float64(p) / float64(len(e.Questions))
}

// Pass reports whether the score reaches the pass mark.
func (e *Exam) Pass() bool { _, _, s := e.Score(); return s >= PassMark }

// TimeUsed is how long the exam took (or has taken so far).
func (e *Exam) TimeUsed() time.Duration {
	end := e.Finished
	if end.IsZero() {
		end = time.Now()
		if limit := e.Started.Add(e.Duration); end.After(limit) {
			end = limit
		}
	}
	return end.Sub(e.Started).Round(time.Second)
}

// ---- persistence ----

// Store keeps the current exam and the history under ~/.awesome_labs.
type Store struct{ dir string }

// NewStore uses dir (usually ~/.awesome_labs).
func NewStore(dir string) *Store { return &Store{dir: dir} }

func (s *Store) current() string { return filepath.Join(s.dir, "exam.json") }
func (s *Store) history() string { return filepath.Join(s.dir, "exam-history.json") }

// Current returns the running exam, or nil.
func (s *Store) Current() *Exam {
	b, err := os.ReadFile(s.current())
	if err != nil {
		return nil
	}
	var e Exam
	if json.Unmarshal(b, &e) != nil {
		return nil
	}
	return &e
}

// Save writes the running exam.
func (s *Store) Save(e *Exam) error {
	b, _ := json.MarshalIndent(e, "", "  ")
	tmp := s.current() + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, s.current())
}

// Finish marks the exam finished, appends it to the history and clears it.
func (s *Store) Finish(e *Exam) error {
	if e.Finished.IsZero() {
		e.Finished = time.Now()
		if limit := e.Started.Add(e.Duration); e.Finished.After(limit) {
			e.Finished = limit
		}
	}
	hist := s.History()
	hist = append(hist, *e)
	b, _ := json.MarshalIndent(hist, "", "  ")
	if err := os.WriteFile(s.history(), b, 0o644); err != nil {
		return err
	}
	_ = os.Remove(s.current())
	return nil
}

// History returns all finished exams, oldest first.
func (s *Store) History() []Exam {
	b, err := os.ReadFile(s.history())
	if err != nil {
		return nil
	}
	var h []Exam
	_ = json.Unmarshal(b, &h)
	return h
}

// FormatDuration renders 1h02m03s as "1:02:03" / "02:03".
func FormatDuration(d time.Duration) string {
	d = d.Round(time.Second)
	h, m, s := int(d.Hours()), int(d.Minutes())%60, int(d.Seconds())%60
	if h > 0 {
		return fmt.Sprintf("%d:%02d:%02d", h, m, s)
	}
	return fmt.Sprintf("%02d:%02d", m, s)
}
