// Package exercise loads exercise definitions.
//
// Exercises are YAML files grouped in "module" files: each file holds a
// module header and a list of exercises. See docs/AUTHORING.md.
package exercise

import (
	"fmt"
	"io/fs"
	"path"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// Profile names the lab environment an exercise runs on.
type Profile string

const (
	ProfileDocker Profile = "docker"
	ProfileK8s    Profile = "k8s"
	ProfileOffsec Profile = "offsec"
)

// Exercise is one lab task.
type Exercise struct {
	ID         string   `yaml:"id"`
	Title      string   `yaml:"title"`
	Difficulty int      `yaml:"difficulty"` // 1 (intro) .. 5 (expert)
	Kind       string   `yaml:"kind"`       // build | fix
	Tags       []string `yaml:"tags"`
	Tutorial   string   `yaml:"tutorial"` // concept and commands to read first (markdown)
	Scenario   string   `yaml:"scenario"`
	Hints      []string `yaml:"hints"`
	Solution   string   `yaml:"solution"`

	// Scripts run as root with /opt/kubelab/lib.sh sourced.
	Setup       string `yaml:"setup"`        // primary node (docker host / controlplane)
	SetupWorker string `yaml:"setup_worker"` // k8s worker node01
	Check       string `yaml:"check"`        // primary node; exit 0 == solved
	CheckWorker string `yaml:"check_worker"` // worker; must also pass if set
	Solve       string `yaml:"solve"`        // reference solution, run as the lab user in /lab

	// Deliverables are the files in /lab the check grades (names relative to
	// /lab), in the order the scenario asks for them. In a notebook module they
	// are what the notebook must write (see notebook.go); it may be empty when
	// the check only calls the notebook's functions.
	Deliverables []string `yaml:"deliverables"`
	// Starter is code for the starter notebook of a notebook module (cells
	// split at lines that are just "# %%"): code to fix, or a skeleton.
	Starter string `yaml:"starter"`

	// Destructive exercises (e.g. cluster upgrades) cannot be undone by a
	// reset; the environment is rebuilt before the next exercise.
	Destructive bool `yaml:"destructive"`

	// Filled from the module.
	Notebook bool    `yaml:"-"` // JupyterLab suits it (the web interface offers one)
	Module   string  `yaml:"-"`
	Track    string  `yaml:"-"`
	Profile  Profile `yaml:"-"`
	Order    int     `yaml:"-"`
	Source   string  `yaml:"-"`
}

// Module is a group of related exercises.
type Module struct {
	Track   string  `yaml:"track"`
	Module  string  `yaml:"module"`
	Profile Profile `yaml:"profile"`
	Order   int     `yaml:"order"`
	// Notebook offers JupyterLab on the docker host for the module's exercises
	// (in the web interface); only for the docker profile.
	Notebook  bool       `yaml:"notebook"`
	Exercises []Exercise `yaml:"exercises"`
}

// Catalog is every loaded exercise, in display order.
type Catalog struct {
	Exercises []*Exercise
	byID      map[string]*Exercise
}

// Get returns an exercise by id.
func (c *Catalog) Get(id string) *Exercise { return c.byID[id] }

// DifficultyLabel turns 1..5 into a word.
func DifficultyLabel(d int) string {
	switch d {
	case 1:
		return "Beginner"
	case 2:
		return "Easy"
	case 3:
		return "Medium"
	case 4:
		return "Hard"
	case 5:
		return "Expert"
	}
	return "?"
}

// Load reads every *.yaml file under root in fsys.
func Load(fsys fs.FS, root string) (*Catalog, error) {
	var files []string
	err := fs.WalkDir(fsys, root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && (strings.HasSuffix(p, ".yaml") || strings.HasSuffix(p, ".yml")) {
			files = append(files, p)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(files)

	cat := &Catalog{byID: map[string]*Exercise{}}
	var mods []Module
	for _, f := range files {
		data, err := fs.ReadFile(fsys, f)
		if err != nil {
			return nil, err
		}
		var m Module
		if err := yaml.Unmarshal(data, &m); err != nil {
			return nil, fmt.Errorf("%s: %w", f, err)
		}
		if err := m.validate(path.Base(f)); err != nil {
			return nil, err
		}
		for i := range m.Exercises {
			m.Exercises[i].Source = f
		}
		mods = append(mods, m)
	}
	sort.SliceStable(mods, func(i, j int) bool {
		if mods[i].Track != mods[j].Track {
			return mods[i].Track < mods[j].Track
		}
		return mods[i].Order < mods[j].Order
	})
	for _, m := range mods {
		for i := range m.Exercises {
			e := m.Exercises[i]
			e.Module, e.Track, e.Profile, e.Order, e.Notebook = m.Module, m.Track, m.Profile, i, m.Notebook
			if _, dup := cat.byID[e.ID]; dup {
				return nil, fmt.Errorf("%s: duplicate exercise id %q", e.Source, e.ID)
			}
			ex := e
			cat.byID[e.ID] = &ex
			cat.Exercises = append(cat.Exercises, &ex)
		}
	}
	return cat, nil
}

var kitUserFile = regexp.MustCompile(`\$\{?KL_STATE\}?/user\b`)

func (m *Module) validate(file string) error {
	if m.Track == "" || m.Module == "" {
		return fmt.Errorf("%s: track and module are required", file)
	}
	if m.Profile != ProfileDocker && m.Profile != ProfileK8s && m.Profile != ProfileOffsec {
		return fmt.Errorf("%s: profile must be docker, k8s or offsec, got %q", file, m.Profile)
	}
	if m.Notebook && m.Profile != ProfileDocker {
		return fmt.Errorf("%s: notebook needs the docker profile (JupyterLab runs on the docker host)", file)
	}
	for _, e := range m.Exercises {
		where := fmt.Sprintf("%s: exercise %q", file, e.ID)
		switch {
		case e.ID == "":
			return fmt.Errorf("%s: exercise without id", file)
		case e.Title == "":
			return fmt.Errorf("%s: missing title", where)
		case e.Difficulty < 1 || e.Difficulty > 5:
			return fmt.Errorf("%s: difficulty must be 1..5", where)
		case e.Kind != "build" && e.Kind != "fix":
			return fmt.Errorf("%s: kind must be build or fix", where)
		case strings.TrimSpace(e.Scenario) == "":
			return fmt.Errorf("%s: missing scenario", where)
		case strings.TrimSpace(e.Solution) == "":
			return fmt.Errorf("%s: missing solution", where)
		case strings.TrimSpace(e.Check) == "":
			return fmt.Errorf("%s: missing check", where)
		case strings.TrimSpace(e.Solve) == "":
			return fmt.Errorf("%s: missing solve", where)
		}
		// $KL_STATE/user names the lab user for every script (lib.sh lab_user):
		// an exercise that wrote its own data there would break every later one
		for _, script := range []string{e.Setup, e.SetupWorker, e.Check, e.CheckWorker, e.Solve} {
			if kitUserFile.MatchString(script) {
				return fmt.Errorf("%s: $KL_STATE/user is the kit's (the lab user's name); keep the exercise's data in another file", where)
			}
		}
		if m.Notebook {
			nb := e.ID + ".ipynb"
			switch {
			case !strings.Contains(e.Check, "nb_check "+nb):
				return fmt.Errorf("%s: a notebook exercise's check runs its notebook (nb_check %s)", where, nb)
			case !strings.Contains(e.Solve, "nb_write "+nb):
				return fmt.Errorf("%s: a notebook exercise's solve writes its notebook (nb_write %s)", where, nb)
			}
			for _, d := range e.Deliverables {
				if path.Ext(d) == ".py" || path.Ext(d) == ".ipynb" {
					return fmt.Errorf("%s: deliverable %q: a notebook exercise's deliverables are the files its notebook writes", where, d)
				}
			}
		} else if e.Starter != "" {
			return fmt.Errorf("%s: starter is for notebook modules", where)
		}
		for _, d := range e.Deliverables {
			if d == "" || strings.ContainsAny(d, "/\\") || strings.HasPrefix(d, ".") {
				return fmt.Errorf("%s: deliverable %q must be a plain file name in /lab", where, d)
			}
			if !strings.Contains(e.Scenario, "/lab/"+d) && !strings.Contains(e.Scenario, "`"+d) {
				return fmt.Errorf("%s: deliverable %q isn't named in the scenario", where, d)
			}
		}
		if m.Profile != ProfileK8s && (e.SetupWorker != "" || e.CheckWorker != "") {
			return fmt.Errorf("%s: worker scripts only valid for k8s profile", where)
		}
	}
	return nil
}
