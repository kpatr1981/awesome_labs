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
	"sort"
	"strings"
	"time"

	"kubelab/internal/exercise"
	"kubelab/internal/lab"
	"kubelab/internal/progress"
	"kubelab/internal/tui"
)

//go:embed all:vms all:exercises
var assets embed.FS

func usage() {
	fmt.Fprint(os.Stderr, `kubelab — Docker & Kubernetes exercises on Lima VMs

usage:
  kubelab                         open the interactive exercise browser
  kubelab list [filter]           list exercises
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
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	if len(args) == 0 {
		return tui.Run(cat, mgr, prog)
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
		res := verifyOne(ctx, mgr, ex, logf)
		if ctx.Err() != nil {
			return ctx.Err()
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
