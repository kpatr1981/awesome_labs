package verified

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"awesome_labs/internal/exercise"
)

// Counts are the exercises per track, and the total.
type Counts struct {
	Track map[string]int
	Total int
}

// Count tallies a catalogue.
func Count(cat *exercise.Catalog) Counts {
	c := Counts{Track: map[string]int{}}
	for _, e := range cat.Exercises {
		c.Track[e.Track]++
		c.Total++
	}
	return c
}

// countMark is a number the docs keep current: <!-- count:TRACK -->N<!-- /count -->,
// with TRACK a track id or "total". A number written with a thousands
// separator (1,553) keeps it.
var countMark = regexp.MustCompile(`<!-- count:([a-z]+) -->([0-9,]*)<!-- /count -->`)

// Rewrite puts the current counts into every marker of text. It returns the
// new text and the markers whose number changed ("docker 184 -> 199"), and
// fails on a marker for a track the catalogue doesn't have.
func Rewrite(text string, c Counts) (string, []string, error) {
	var changes []string
	var bad error
	out := countMark.ReplaceAllStringFunc(text, func(m string) string {
		sub := countMark.FindStringSubmatch(m)
		name, old := sub[1], sub[2]
		n, ok := c.Total, true
		if name != "total" {
			n, ok = c.Track[name]
		}
		if !ok {
			if bad == nil {
				bad = fmt.Errorf("count marker for unknown track %q", name)
			}
			return m
		}
		s := strconv.Itoa(n)
		if strings.Contains(old, ",") {
			s = withCommas(n)
		}
		if s != old {
			changes = append(changes, fmt.Sprintf("%s %s -> %s", name, old, s))
		}
		return "<!-- count:" + name + " -->" + s + "<!-- /count -->"
	})
	return out, changes, bad
}

// Marked lists the names of the markers in text.
func Marked(text string) map[string]bool {
	out := map[string]bool{}
	for _, m := range countMark.FindAllStringSubmatch(text, -1) {
		out[m[1]] = true
	}
	return out
}

func withCommas(n int) string {
	s := strconv.Itoa(n)
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + "," + s[i:]
	}
	return s
}
