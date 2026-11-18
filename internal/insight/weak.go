// Package insight computes views of a learner's progress that both the
// terminal UI and the web interface show, such as weak spots.
package insight

import (
	"sort"

	"awesome_labs/internal/exercise"
)

// Effort is what one exercise took: the fields weak spots are computed from.
type Effort struct {
	Hints       int
	Checks      int
	SawSolution bool
	Solved      bool
	Started     bool
}

// Module is one module's share of the help needed.
type Module struct {
	Name      string   `json:"name"`      // "Track · Module"
	Tried     int      `json:"tried"`     // exercises with any progress
	Solved    int      `json:"solved"`    // of those, solved
	Hints     int      `json:"hints"`     // hints revealed
	Solutions int      `json:"solutions"` // solutions revealed
	Retries   int      `json:"retries"`   // checks beyond the first passing one, and failed checks
	Score     float64  `json:"score"`     // help needed per exercise tried; higher is weaker
	Retry     []string `json:"retry"`     // exercises whose solution was revealed: worth doing again
}

// Report is the weak-spot ranking.
type Report struct {
	Any     bool     // some exercise has progress at all
	Modules []Module // modules that needed help, weakest first (at most MaxModules)
	Retry   []string // exercises worth doing again without the solution (at most MaxRetry)
}

// Limits of a Report.
const (
	MaxModules = 10
	MaxRetry   = 12
)

// WeakSpots ranks the modules where you needed the most help: hints,
// solutions and failed checks, over the exercises you tried.
func WeakSpots(cat *exercise.Catalog, get func(id string) Effort) Report {
	mods := map[string]*Module{}
	var order []string
	for _, e := range cat.Exercises {
		f := get(e.ID)
		if !f.Started && !f.Solved && f.Hints == 0 && !f.SawSolution {
			continue
		}
		k := exercise.TrackName(e.Track) + " · " + e.Module
		md := mods[k]
		if md == nil {
			md = &Module{Name: k}
			mods[k] = md
			order = append(order, k)
		}
		md.Tried++
		md.Hints += f.Hints
		if f.SawSolution {
			md.Solutions++
			md.Retry = append(md.Retry, e.ID)
		}
		if f.Solved {
			md.Solved++
			md.Retries += max(0, f.Checks-1)
		} else {
			md.Retries += f.Checks
		}
	}
	r := Report{Any: len(order) > 0}
	for _, k := range order {
		md := mods[k]
		md.Score = float64(md.Hints+3*md.Solutions+md.Retries+2*(md.Tried-md.Solved)) / float64(md.Tried)
	}
	sort.SliceStable(order, func(i, j int) bool { return mods[order[i]].Score > mods[order[j]].Score })
	for _, k := range order {
		if md := mods[k]; md.Score > 0 && len(r.Modules) < MaxModules {
			r.Modules = append(r.Modules, *md)
		}
		r.Retry = append(r.Retry, mods[k].Retry...)
	}
	if len(r.Retry) > MaxRetry {
		r.Retry = r.Retry[:MaxRetry]
	}
	return r
}
