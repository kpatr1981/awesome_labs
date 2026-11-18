package main

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"awesome_labs/internal/exercise"
	"awesome_labs/internal/local"
)

// localVerify reports whether verify's arguments ask for the local harness;
// such a run needs no VMs, so it doesn't take the single-instance lock.
func localVerify(args []string) bool {
	for _, a := range args {
		switch a {
		case "-local", "--local", "-local=true", "--local=true":
			return true
		case "--":
			return false
		}
	}
	return false
}

// verifyLocal is `awesome_labs verify -local`: verify without a Lima VM, each
// exercise in a fresh container on this machine's Docker (see internal/local).
func verifyLocal(ctx context.Context, cat *exercise.Catalog, stateDir string, args []string) error {
	fset := flag.NewFlagSet("verify -local", flag.ContinueOnError)
	fset.Bool("local", true, "run in throwaway containers on this machine's Docker instead of the lab VMs")
	from := fset.String("from", "", "start at this exercise id")
	jobs := fset.Int("j", 1, "exercises to run at once")
	rebuild := fset.Bool("rebuild", false, "rebuild the harness's image store first")
	force := fset.Bool("force", false, "also run exercises that reach the host kernel (loop devices, sysctls, clock); risky")
	cache := fset.String("cache", filepath.Join(stateDir, "cache", "images", "docker"), "image archives to seed the image store with")
	if err := fset.Parse(args); err != nil {
		return err
	}
	if *jobs < 1 {
		*jobs = 1
	}
	var exs []*exercise.Exercise
	started := *from == ""
	for _, ex := range cat.Exercises {
		if !started {
			started = ex.ID == *from
		}
		if started && matchIDs(ex.ID, fset.Args()) {
			exs = append(exs, ex)
		}
	}
	if len(exs) == 0 {
		return fmt.Errorf("no exercise matches")
	}
	logf, err := os.Create(filepath.Join(stateDir, "logs", "verify-local-"+time.Now().Format("20060102-150405")+".log"))
	if err != nil {
		return err
	}
	defer logf.Close()
	h := &local.Harness{Assets: assets, CacheDir: *cache, Log: logf, Progress: os.Stdout, Rebuild: *rebuild, Force: *force}
	h.Cleanup(ctx)
	defer h.RemoveOwn()
	if err := h.Prepare(ctx); err != nil {
		fmt.Println("log:", logf.Name())
		return fmt.Errorf("preparing the harness: %w", err)
	}

	var mu sync.Mutex // the log file and stdout
	var failed, skipped []string
	ran := 0
	sem := make(chan struct{}, *jobs)
	var wg sync.WaitGroup
	for _, ex := range exs {
		if ctx.Err() != nil {
			break
		}
		sem <- struct{}{}
		wg.Add(1)
		go func(ex *exercise.Exercise) {
			defer func() { <-sem; wg.Done() }()
			t0 := time.Now()
			var buf bytes.Buffer
			exCtx, cancel := context.WithTimeout(ctx, 25*time.Minute)
			res := h.Verify(exCtx, ex, &buf)
			timedOut := exCtx.Err() != nil && ctx.Err() == nil
			cancel()
			if timedOut {
				res = "timed out"
			}
			mu.Lock()
			defer mu.Unlock()
			fmt.Fprintf(logf, "\n\n######## %s %s\n%s", ex.ID, ex.Title, buf.Bytes())
			if ctx.Err() != nil {
				return
			}
			fmt.Printf("%-10s %-50s %-28s %4.0fs\n", ex.ID, trunc(ex.Title, 50), trunc(res, 28), time.Since(t0).Seconds())
			switch {
			case strings.HasPrefix(res, "skipped"):
				skipped = append(skipped, ex.ID+" ("+strings.TrimPrefix(res, "skipped: ")+")")
			case res != local.ResultOK:
				ran++
				failed = append(failed, ex.ID+" ("+res+")")
			default:
				ran++
			}
		}(ex)
	}
	wg.Wait()
	if ctx.Err() != nil {
		fmt.Println("\ninterrupted: the harness containers are removed")
		return ctx.Err()
	}
	fmt.Printf("\n%d verified locally, %d failed, %d skipped\n", ran, len(failed), len(skipped))
	for _, f := range failed {
		fmt.Println("  FAIL", f)
	}
	for _, s := range skipped {
		fmt.Println("  SKIP", s)
	}
	fmt.Println("log:", logf.Name())
	if len(failed) > 0 {
		return fmt.Errorf("verification failed")
	}
	return nil
}

func matchIDs(id string, prefixes []string) bool {
	if len(prefixes) == 0 {
		return true
	}
	for _, p := range prefixes {
		if strings.HasPrefix(id, p) {
			return true
		}
	}
	return false
}
