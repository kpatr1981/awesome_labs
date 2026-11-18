// Package verified records which exercises passed `awesome_labs verify` in
// their current form, so a release can refuse exercises that never did.
//
// The record (verified.json at the repo root) maps each exercise id to a hash
// of what the run proved: the scripts and the files they grade. A change to
// any of them makes the entry stale until the exercise is verified again.
// Text a learner reads (title, tutorial, scenario, hints, solution, tags) is
// left out, so rewording an exercise doesn't call for a new VM run, and so is
// the file the exercise sits in and its place there: moving an exercise
// keeps its entry. The shared kit (vms/kit/lib.sh, the bootstrap) is not part
// of any hash; verify the tracks it touches after changing it.
package verified

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"

	"awesome_labs/internal/exercise"
)

// Entry is one exercise's last recorded pass.
type Entry struct {
	Hash    string `json:"hash"`
	Date    string `json:"date"`              // YYYY-MM-DD
	Version string `json:"version,omitempty"` // the awesome_labs that ran it
	Note    string `json:"note,omitempty"`    // e.g. "baseline": accepted, not run by verify
}

// Record is the whole file.
type Record struct {
	Exercises map[string]Entry `json:"exercises"`
}

// graded is what Hash covers. Keep the field set in sync with the package
// comment; changing it makes every entry stale.
type graded struct {
	ID           string           `json:"id"`
	Profile      exercise.Profile `json:"profile"`
	Notebook     bool             `json:"notebook"`
	Destructive  bool             `json:"destructive"`
	Setup        string           `json:"setup"`
	SetupWorker  string           `json:"setup_worker"`
	Check        string           `json:"check"`
	CheckWorker  string           `json:"check_worker"`
	Solve        string           `json:"solve"`
	Deliverables []string         `json:"deliverables"`
	Starter      string           `json:"starter"`
}

// Hash is the hex SHA-256 of an exercise's graded content.
func Hash(e *exercise.Exercise) string {
	b, _ := json.Marshal(graded{e.ID, e.Profile, e.Notebook, e.Destructive,
		e.Setup, e.SetupWorker, e.Check, e.CheckWorker, e.Solve, e.Deliverables, e.Starter})
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// Load reads a record; a missing file is an empty record.
func Load(path string) (*Record, error) {
	r := &Record{Exercises: map[string]Entry{}}
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return r, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(b, r); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if r.Exercises == nil {
		r.Exercises = map[string]Entry{}
	}
	return r, nil
}

// Save writes the record one exercise per line, sorted by id, so a verify
// run shows up as a small diff. It writes a temporary file and renames it
// into place.
func (r *Record) Save(path string) error {
	ids := make([]string, 0, len(r.Exercises))
	for id := range r.Exercises {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	var b strings.Builder
	b.WriteString("{\n  \"exercises\": {")
	for i, id := range ids {
		k, _ := json.Marshal(id)
		v, err := json.Marshal(r.Exercises[id])
		if err != nil {
			return err
		}
		if i > 0 {
			b.WriteByte(',')
		}
		fmt.Fprintf(&b, "\n    %s: %s", k, v)
	}
	b.WriteString("\n  }\n}\n")
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(b.String()), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// Mark records that e passed (or was accepted) now.
func (r *Record) Mark(e *exercise.Exercise, date, version, note string) {
	r.Exercises[e.ID] = Entry{Hash: Hash(e), Date: date, Version: version, Note: note}
}

// Pending is an exercise without a matching entry.
type Pending struct {
	ID     string
	Reason string // "new" (never recorded) or "changed" (recorded with another hash)
	Last   Entry  // the old entry when changed
}

// Unverified lists the catalogue's exercises that are new or changed since
// their recorded pass, in catalogue order.
func (r *Record) Unverified(cat *exercise.Catalog) []Pending {
	var out []Pending
	for _, e := range cat.Exercises {
		got, ok := r.Exercises[e.ID]
		switch {
		case !ok:
			out = append(out, Pending{ID: e.ID, Reason: "new"})
		case got.Hash != Hash(e):
			out = append(out, Pending{ID: e.ID, Reason: "changed", Last: got})
		}
	}
	return out
}

// Gone lists recorded ids the catalogue no longer has, sorted.
func (r *Record) Gone(cat *exercise.Catalog) []string {
	var out []string
	for id := range r.Exercises {
		if cat.Get(id) == nil {
			out = append(out, id)
		}
	}
	sort.Strings(out)
	return out
}

// Prune drops entries for ids the catalogue no longer has.
func (r *Record) Prune(cat *exercise.Catalog) {
	for _, id := range r.Gone(cat) {
		delete(r.Exercises, id)
	}
}
