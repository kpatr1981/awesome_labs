package exercise

import (
	"fmt"
	"io/fs"
	"strings"

	"gopkg.in/yaml.v3"
)

// Path is a curated learning path: a named selection of exercises.
type Path struct {
	ID          string   `yaml:"id"`
	Title       string   `yaml:"title"`
	Description string   `yaml:"description"`
	Include     []Filter `yaml:"include"` // an exercise matching ANY filter is included
	Exclude     []string `yaml:"exclude"` // exercise ids

	Exercises []*Exercise `yaml:"-"` // resolved, in catalog order
}

// Filter selects exercises; all set fields must match.
type Filter struct {
	Track         string   `yaml:"track"`
	Modules       []string `yaml:"modules"`
	Kinds         []string `yaml:"kinds"`
	Tags          []string `yaml:"tags"` // any of
	IDs           []string `yaml:"ids"`
	MinDifficulty int      `yaml:"min_difficulty"`
	MaxDifficulty int      `yaml:"max_difficulty"`
}

func (f Filter) match(e *Exercise) bool {
	if f.Track != "" && e.Track != f.Track {
		return false
	}
	if len(f.Modules) > 0 && !contains(f.Modules, e.Module) {
		return false
	}
	if len(f.Kinds) > 0 && !contains(f.Kinds, e.Kind) {
		return false
	}
	if len(f.IDs) > 0 && !contains(f.IDs, e.ID) {
		return false
	}
	if f.MinDifficulty > 0 && e.Difficulty < f.MinDifficulty {
		return false
	}
	if f.MaxDifficulty > 0 && e.Difficulty > f.MaxDifficulty {
		return false
	}
	if len(f.Tags) > 0 {
		ok := false
		for _, t := range e.Tags {
			if contains(f.Tags, t) {
				ok = true
				break
			}
		}
		if !ok {
			return false
		}
	}
	return true
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if strings.EqualFold(x, s) {
			return true
		}
	}
	return false
}

// LoadPaths reads a paths file and resolves it against the catalog.
func LoadPaths(fsys fs.FS, file string, cat *Catalog) ([]*Path, error) {
	data, err := fs.ReadFile(fsys, file)
	if err != nil {
		return nil, err
	}
	var doc struct {
		Paths []*Path `yaml:"paths"`
	}
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("%s: %w", file, err)
	}
	for _, p := range doc.Paths {
		for _, e := range cat.Exercises {
			if contains(p.Exclude, e.ID) {
				continue
			}
			for _, f := range p.Include {
				if f.match(e) {
					p.Exercises = append(p.Exercises, e)
					break
				}
			}
		}
		if len(p.Exercises) == 0 {
			return nil, fmt.Errorf("%s: path %q matches no exercises", file, p.ID)
		}
	}
	return doc.Paths, nil
}
