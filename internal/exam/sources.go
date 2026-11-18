package exam

import (
	"fmt"

	"awesome_labs/internal/exercise"
)

// The choices an exam is set up with, in both interfaces.
var (
	Counts  = []int{5, 10, 15, 20, 30}
	Minutes = []int{30, 60, 90, 120, 180}
)

// Source is a pool of exercises an exam can draw from.
type Source struct {
	Label string
	Pool  []*exercise.Exercise
}

// fixedTracks are listed first, in this order; the other tracks follow in
// exercise.TrackOrder.
var fixedTracks = []string{"kubernetes", "docker", "offsec", "linux", "networking", "cloud", "data", "defensive", "programming", "datascience"}

// Sources lists single-environment pools: whole tracks and every learning
// path. A path that mixes environments is split into one pool per
// environment, named with envTitle.
func Sources(cat *exercise.Catalog, paths []*exercise.Path, envTitle func(exercise.Profile) string) []Source {
	// One pool per track: several tracks share a lab VM, so a pool per profile
	// would mix them.
	all := map[string][]*exercise.Exercise{}
	for _, e := range cat.Exercises {
		all[e.Track] = append(all[e.Track], e)
	}
	var src []Source
	fixed := map[string]bool{}
	for _, t := range fixedTracks {
		fixed[t] = true
		src = append(src, Source{exercise.TrackName(t) + " — everything", all[t]})
	}
	for _, t := range exercise.TrackOrder {
		if !fixed[t] && len(all[t]) > 0 {
			src = append(src, Source{exercise.TrackName(t) + " — everything", all[t]})
		}
	}
	for _, p := range paths {
		g := map[exercise.Profile][]*exercise.Exercise{}
		for _, e := range p.Exercises {
			g[e.Profile] = append(g[e.Profile], e)
		}
		if len(g) == 1 {
			for _, list := range g {
				src = append(src, Source{p.Title, list})
			}
			continue
		}
		for _, prof := range []exercise.Profile{exercise.ProfileK8s, exercise.ProfileDocker, exercise.ProfileOffsec} {
			if len(g[prof]) > 0 {
				src = append(src, Source{fmt.Sprintf("%s (%s)", p.Title, envTitle(prof)), g[prof]})
			}
		}
	}
	return src
}
