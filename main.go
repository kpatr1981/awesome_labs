// kubelab — hands-on Docker & Kubernetes exercises on local Lima VMs.
package main

import (
	"context"
	"embed"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"kubelab/internal/config"
	"kubelab/internal/exam"
	"kubelab/internal/exercise"
	"kubelab/internal/lab"
	"kubelab/internal/progress"
	"kubelab/internal/tui"
)

//go:embed all:vms all:exercises learning-paths.yaml
var assets embed.FS

func usage() {
	fmt.Fprint(os.Stderr, `kubelab — Docker & Kubernetes exercises on Lima VMs

usage:
  kubelab                         open the interactive exercise browser
  kubelab list [filter]           list exercises
  kubelab paths [path-id]         learning paths and your progress
  kubelab exam [status|history|abandon]  timed exams (start one from the UI with X)
  kubelab show <id>               print scenario (add --solution for the solution)
  kubelab start <id>              prepare the lab for an exercise
  kubelab check <id>              check your work
  kubelab shell [docker|k8s|node01]  open a shell in a lab VM
  kubelab env status|up|stop|destroy|rebuild|reset [docker|k8s]
  kubelab verify [-from id] [ids or prefixes...]
                                  author tool: setup -> check fails -> solve -> check passes
  kubelab lint                    validate all exercise files

environment:
  KUBELAB_EXERCISES=dir           load exercises from disk instead of the built-in set
`)
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func exercisesFS() (fs.FS, string) {
	if dir := os.Getenv("KUBELAB_EXERCISES"); dir != "" {
		return os.DirFS(dir), "."
	}
	return assets, "exercises"
}

func run(args []string) error {
	fsys, root := exercisesFS()
	cat, err := exercise.Load(fsys, root)
	if err != nil {
		return err
	}
	mgr, err := lab.NewManager(assets)
	if err != nil {
		return err
	}
	prog, err := progress.Open(mgr.StateDir)
	if err != nil {
		return err
	}
	cfg, err := config.Load(mgr.StateDir)
	if err != nil {
		return err
	}
	lab.ApplySizes([2]string{itoa(cfg.Docker.CPUs), cfg.Docker.Memory},
		[2]string{itoa(cfg.ControlPlane.CPUs), cfg.ControlPlane.Memory},
		[2]string{itoa(cfg.Worker.CPUs), cfg.Worker.Memory})
	pfs, pfile := fs.FS(assets), "learning-paths.yaml"
	if f := os.Getenv("KUBELAB_PATHS"); f != "" {
		pfs, pfile = os.DirFS(filepath.Dir(f)), filepath.Base(f)
	}
	paths, err := exercise.LoadPaths(pfs, pfile, cat)
	if err != nil {
		return err
	}
	exams := exam.NewStore(mgr.StateDir)
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	if len(args) == 0 {
		mgr.AutoStop = cfg.AutoStopEnabled()
		return tui.Run(tui.Deps{Catalog: cat, Manager: mgr, Progress: prog, Paths: paths, Exams: exams})
	}
	cmd, rest := args[0], args[1:]
	get := func() (*exercise.Exercise, error) {
		if len(rest) == 0 {
			return nil, fmt.Errorf("%s needs an exercise id", cmd)
		}
		ex := cat.Get(rest[0])
		if ex == nil {
			return nil, fmt.Errorf("no exercise %q", rest[0])
		}
		return ex, nil
	}
	switch cmd {
	case "-h", "--help", "help":
		usage()
	case "lint":
		fmt.Printf("%d exercises OK\n", len(cat.Exercises))
		counts := map[string]int{}
		for _, e := range cat.Exercises {
			counts[e.Track+" / "+e.Module]++
		}
		keys := make([]string, 0, len(counts))
		for k := range counts {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			fmt.Printf("  %3d  %s\n", counts[k], k)
		}
	case "list":
		filter := strings.ToLower(strings.Join(rest, " "))
		for _, e := range cat.Exercises {
			line := fmt.Sprintf("%-10s %s %-5s %-42s %s", e.ID, stars(e.Difficulty), e.Kind, e.Title, e.Module)
			if filter == "" || strings.Contains(strings.ToLower(line), filter) {
				fmt.Printf("%s %s\n", prog.Get(e.ID).Mark(), line)
			}
		}
	case "show":
		ex, err := get()
		if err != nil {
			return err
		}
		fmt.Printf("# %s — %s\n%s · %s · %s\n\n%s\n", ex.ID, ex.Title, ex.Module, exercise.DifficultyLabel(ex.Difficulty), ex.Kind, ex.Scenario)
		if len(rest) > 1 && rest[1] == "--solution" {
			fmt.Printf("\n## Solution\n\n%s\n", ex.Solution)
		}
	case "start":
		ex, err := get()
		if err != nil {
			return err
		}
		mgr.AutoStop = cfg.AutoStopEnabled()
		if err := mgr.Start(ctx, ex, os.Stdout); err != nil {
			return err
		}
		prog.MarkStarted(ex.ID)
		fmt.Printf("\n%s\n\nOpen a shell with: kubelab shell %s\n", ex.Scenario, ex.Profile)
	case "check":
		ex, err := get()
		if err != nil {
			return err
		}
		ok, err := mgr.Check(ctx, ex, os.Stdout)
		if err != nil {
			return err
		}
		prog.RecordCheck(ex.ID, ok)
		if !ok {
			os.Exit(1)
		}
	case "solve":
		ex, err := get()
		if err != nil {
			return err
		}
		return mgr.Solve(ctx, ex, os.Stdout)
	case "shell":
		target := "docker"
		if len(rest) > 0 {
			target = rest[0]
		}
		env := lab.DockerEnv
		worker := false
		switch target {
		case "k8s", "controlplane", "cp":
			env = lab.K8sEnv
		case "node01", "worker":
			env, worker = lab.K8sEnv, true
		}
		c := mgr.ShellCommand(env, worker)
		return c.Run()
	case "paths":
		return pathsCmd(paths, prog, rest)
	case "exam":
		return examCmd(exams, cat, rest)
	case "env":
		return envCmd(ctx, mgr, rest)
	case "verify":
		return verify(ctx, cat, mgr, rest)
	default:
		usage()
		return fmt.Errorf("unknown command %q", cmd)
	}
	return nil
}

func stars(d int) string { return strings.Repeat("★", d) + strings.Repeat("☆", 5-d) }

func envCmd(ctx context.Context, mgr *lab.Manager, args []string) error {
	if len(args) == 0 {
		args = []string{"status"}
	}
	envs := []lab.Env{lab.DockerEnv, lab.K8sEnv}
	if len(args) > 1 {
		switch args[1] {
		case "docker":
			envs = []lab.Env{lab.DockerEnv}
		case "k8s", "kubernetes":
			envs = []lab.Env{lab.K8sEnv}
		default:
			return fmt.Errorf("unknown environment %q (docker|k8s)", args[1])
		}
	}
	mgr.Verbose = os.Getenv("KUBELAB_VERBOSE") != ""
	for _, env := range envs {
		var err error
		switch args[0] {
		case "status":
			fmt.Printf("%-20s %s\n", env.Title, mgr.EnvStatus(ctx, env))
		case "up":
			err = mgr.Ensure(ctx, env, os.Stdout)
		case "stop":
			err = mgr.Stop(ctx, env, os.Stdout)
		case "destroy":
			err = mgr.Destroy(ctx, env, os.Stdout)
		case "rebuild":
			err = mgr.Rebuild(ctx, env, os.Stdout)
		case "reset":
			err = mgr.Reset(ctx, env, os.Stdout)
		default:
			return fmt.Errorf("unknown env action %q", args[0])
		}
		if err != nil {
			return err
		}
	}
	return nil
}

// verify runs each exercise end to end: setup, check must fail, solve, check must pass.
func verify(ctx context.Context, cat *exercise.Catalog, mgr *lab.Manager, args []string) error {
	fset := flag.NewFlagSet("verify", flag.ContinueOnError)
	from := fset.String("from", "", "start at this exercise id")
	if err := fset.Parse(args); err != nil {
		return err
	}
	match := func(id string) bool {
		if fset.NArg() == 0 {
			return true
		}
		for _, p := range fset.Args() {
			if strings.HasPrefix(id, p) {
				return true
			}
		}
		return false
	}
	logf, err := os.Create(mgr.StateDir + "/logs/verify-" + time.Now().Format("20060102-150405") + ".log")
	if err != nil {
		return err
	}
	defer logf.Close()
	started := *from == ""
	var failed []string
	n := 0
	for _, ex := range cat.Exercises {
		if !started {
			started = ex.ID == *from
		}
		if !started || !match(ex.ID) {
			continue
		}
		n++
		t0 := time.Now()
		fmt.Printf("%-10s %-50s ", ex.ID, trunc(ex.Title, 50))
		fmt.Fprintf(logf, "\n\n######## %s %s\n", ex.ID, ex.Title)
		// a hung step (a wait without a timeout in some script) must not stall the whole run
		limit := 25 * time.Minute
		if ex.Destructive {
			limit = 50 * time.Minute
		}
		exCtx, cancel := context.WithTimeout(ctx, limit)
		res := verifyOne(exCtx, mgr, ex, logf)
		timedOut := exCtx.Err() != nil
		cancel()
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if timedOut {
			res = "timed out"
			fmt.Fprintf(logf, "TIMED OUT after %s\n", limit)
		}
		fmt.Printf("%-28s %4.0fs\n", res, time.Since(t0).Seconds())
		if res != "ok" {
			failed = append(failed, ex.ID+" ("+res+")")
		}
	}
	fmt.Printf("\n%d verified, %d failed\n", n, len(failed))
	for _, f := range failed {
		fmt.Println("  FAIL", f)
	}
	fmt.Println("log:", logf.Name())
	if len(failed) > 0 {
		return fmt.Errorf("verification failed")
	}
	return nil
}

func verifyOne(ctx context.Context, mgr *lab.Manager, ex *exercise.Exercise, log io.Writer) string {
	if err := mgr.Start(ctx, ex, log); err != nil {
		fmt.Fprintln(log, "START ERROR:", err)
		return "setup error"
	}
	ok, err := mgr.Check(ctx, ex, log)
	if err != nil {
		return "check error"
	}
	if ok {
		return "passes before solving"
	}
	if err := mgr.Solve(ctx, ex, log); err != nil {
		fmt.Fprintln(log, "SOLVE ERROR:", err)
		return "solve error"
	}
	ok, err = mgr.Check(ctx, ex, log)
	if err != nil {
		return "check error"
	}
	if !ok {
		return "fails after solving"
	}
	return "ok"
}

func trunc(s string, n int) string {
	if len([]rune(s)) <= n {
		return s
	}
	return string([]rune(s)[:n-1]) + "…"
}

func itoa(n int) string {
	if n <= 0 {
		return ""
	}
	return fmt.Sprint(n)
}

func pathsCmd(paths []*exercise.Path, prog *progress.Store, args []string) error {
	for _, p := range paths {
		if len(args) > 0 && args[0] != p.ID {
			continue
		}
		ids := make([]string, len(p.Exercises))
		for i, e := range p.Exercises {
			ids[i] = e.ID
		}
		solved, _ := prog.Counts(ids)
		fmt.Printf("%-22s %3d/%-3d %s\n%22s %s\n", p.ID, solved, len(ids), p.Title, "", p.Description)
		if len(args) > 0 {
			for _, e := range p.Exercises {
				fmt.Printf("  %s %-10s %s %s\n", prog.Get(e.ID).Mark(), e.ID, stars(e.Difficulty), e.Title)
			}
		}
	}
	return nil
}

func examCmd(store *exam.Store, cat *exercise.Catalog, args []string) error {
	action := "status"
	if len(args) > 0 {
		action = args[0]
	}
	switch action {
	case "status":
		e := store.Current()
		if e == nil {
			fmt.Println("no exam running — start one from the UI (press X)")
			return nil
		}
		p, n, _ := e.Score()
		fmt.Printf("%s — %s left, %d/%d passed so far\n", e.Title, exam.FormatDuration(e.Remaining()), p, n)
		for i, q := range e.Questions {
			mark := "·"
			if q.Passed {
				mark = "✔"
			} else if q.Checks > 0 {
				mark = "✘"
			}
			t := ""
			if ex := cat.Get(q.ID); ex != nil {
				t = ex.Title
			}
			fmt.Printf("  %2d. %s %-10s %s\n", i+1, mark, q.ID, t)
		}
	case "history":
		for _, e := range store.History() {
			p, n, sc := e.Score()
			res := "FAIL"
			if e.Pass() {
				res = "PASS"
			}
			fmt.Printf("%s  %-40s %2d/%-2d %3.0f%%  %s  %s\n", e.Started.Format("2006-01-02 15:04"), e.Title, p, n, sc*100, res, exam.FormatDuration(e.TimeUsed()))
		}
	case "abandon":
		if e := store.Current(); e != nil {
			return store.Finish(e)
		}
	default:
		return fmt.Errorf("unknown exam action %q", action)
	}
	return nil
}
