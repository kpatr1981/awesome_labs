package main

import (
	"os"
	"regexp"
	"testing"
)

// lib.sh is one file sourced by every script: a function defined twice silently replaces the
// first definition, breaking every exercise that used it (it happened with kj).
func TestKitLibHasNoDuplicateFunctions(t *testing.T) {
	b, err := os.ReadFile("vms/kit/lib.sh")
	if err != nil {
		t.Fatal(err)
	}
	re := regexp.MustCompile(`(?m)^([A-Za-z_][A-Za-z0-9_]*)\(\)\s*\{`)
	seen := map[string]bool{}
	for _, m := range re.FindAllSubmatch(b, -1) {
		name := string(m[1])
		if seen[name] {
			t.Errorf("vms/kit/lib.sh defines %s() more than once", name)
		}
		seen[name] = true
	}
	if len(seen) < 100 {
		t.Fatalf("found only %d functions; the pattern is probably wrong", len(seen))
	}
}
