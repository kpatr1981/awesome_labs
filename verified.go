package main

import (
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"awesome_labs/internal/exercise"
	"awesome_labs/internal/verified"
)

// defaultRecord is the verified-content record, at the repo root.
const defaultRecord = "verified.json"

// recordPass notes in the record at path that ex passed verify just now.
func recordPass(path string, ex *exercise.Exercise) error {
	r, err := verified.Load(path)
	if err != nil {
		return err
	}
	r.Mark(ex, time.Now().Format("2006-01-02"), version, "")
	return r.Save(path)
}

// verifiedCmd is `awesome_labs verified`: the release gate. It lists the
// exercises that are new or changed since their last recorded pass and fails
// when there are any. -accept marks exercises as verified without running
// them (the baseline, or a pass recorded elsewhere).
func verifiedCmd(cat *exercise.Catalog, args []string) error {
	fset := flag.NewFlagSet("verified", flag.ContinueOnError)
	file := fset.String("file", defaultRecord, "the verified-content record")
	accept := fset.Bool("accept", false, "mark the exercises named by the arguments (ids, prefixes, or all) as verified without running them")
	except := fset.String("except", "", "with -accept: comma-separated ids or prefixes to leave out")
	note := fset.String("note", "accepted", "with -accept: the note to record")
	prune := fset.Bool("prune", false, "drop entries for exercises that no longer exist")
	quiet := fset.Bool("q", false, "print only the summary line")
	idsOnly := fset.Bool("ids", false, "print only the ids of the unverified exercises, one per line (for scripts)")
	if err := fset.Parse(args); err != nil {
		return err
	}
	r, err := verified.Load(*file)
	if err != nil {
		return err
	}
	if *accept || *prune {
		if *accept {
			if fset.NArg() == 0 {
				return fmt.Errorf("-accept needs ids, prefixes or all")
			}
			skip := splitList(*except)
			n := 0
			for _, e := range cat.Exercises {
				if matchAny(e.ID, fset.Args()) && !matchAny(e.ID, skip) {
					r.Mark(e, time.Now().Format("2006-01-02"), version, *note)
					n++
				}
			}
			fmt.Printf("accepted %d exercises as verified (%s)\n", n, *note)
		}
		if *prune {
			gone := r.Gone(cat)
			r.Prune(cat)
			fmt.Printf("pruned %d entries\n", len(gone))
		}
		return r.Save(*file)
	}
	pend := r.Unverified(cat)
	if *idsOnly {
		for _, p := range pend {
			fmt.Println(p.ID)
		}
		return nil
	}
	if !*quiet {
		for _, p := range pend {
			ex := cat.Get(p.ID)
			line := fmt.Sprintf("  %-8s %-10s %s", p.Reason, p.ID, trunc(ex.Title, 50))
			if p.Reason == "changed" {
				line += fmt.Sprintf("  (last pass %s)", p.Last.Date)
			}
			fmt.Println(line)
		}
		if gone := r.Gone(cat); len(gone) > 0 {
			fmt.Printf("  %d recorded ids no longer exist (verified -prune drops them): %s\n", len(gone), strings.Join(gone, " "))
		}
	}
	if len(pend) > 0 {
		ids := make([]string, len(pend))
		for i, p := range pend {
			ids[i] = p.ID
		}
		fmt.Printf("%d of %d exercises are not verified in their current form; run: awesome_labs verify -record %s %s\n",
			len(pend), len(cat.Exercises), *file, strings.Join(ids, " "))
		return fmt.Errorf("unverified exercises")
	}
	fmt.Printf("all %d exercises are verified in their current form\n", len(cat.Exercises))
	return nil
}

// countsCmd is `awesome_labs counts`: it rewrites the exercise counts the
// docs keep between <!-- count:TRACK --> markers. -check only reports, and
// fails when a file is out of date or a track has no marker in the first file.
func countsCmd(cat *exercise.Catalog, args []string) error {
	fset := flag.NewFlagSet("counts", flag.ContinueOnError)
	check := fset.Bool("check", false, "don't write: fail when a file is out of date")
	if err := fset.Parse(args); err != nil {
		return err
	}
	files := fset.Args()
	if len(files) == 0 {
		files = []string{"README.md", "docs/PENDING.md"}
	}
	c := verified.Count(cat)
	stale := false
	for i, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			return err
		}
		out, changes, err := verified.Rewrite(string(b), c)
		if err != nil {
			return fmt.Errorf("%s: %w", f, err)
		}
		marked := verified.Marked(string(b))
		if len(marked) == 0 {
			return fmt.Errorf("%s has no <!-- count:... --> markers", f)
		}
		if i == 0 { // the README lists every track
			for t := range c.Track {
				if !marked[t] {
					fmt.Printf("%s: no count marker for track %s\n", f, t)
					stale = true
				}
			}
		}
		for _, ch := range changes {
			fmt.Printf("%s: %s\n", f, ch)
		}
		if len(changes) == 0 {
			continue
		}
		if *check {
			stale = true
			continue
		}
		if err := os.WriteFile(f, []byte(out), 0o644); err != nil {
			return err
		}
	}
	fmt.Printf("%d exercises in %d tracks\n", c.Total, len(c.Track))
	if stale {
		return fmt.Errorf("the counts are out of date: run awesome_labs counts (make docs)")
	}
	return nil
}

func splitList(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// matchAny: id is one of the names, starts with one of them, or a name is "all".
func matchAny(id string, names []string) bool {
	for _, n := range names {
		if n == "all" || strings.HasPrefix(id, n) {
			return true
		}
	}
	return false
}
