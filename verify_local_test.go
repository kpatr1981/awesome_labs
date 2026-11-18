package main

import (
	"strings"
	"testing"
)

func TestLocalVerify(t *testing.T) {
	cases := map[string]bool{
		"-local tf-001":         true,
		"--local":               true,
		"-j 2 -local tf-":       true,
		"tf-001":                false,
		"-from tf-010 tf-":      false,
		"-- -local":             false,
		"-local=true docker-00": true,
	}
	for args, want := range cases {
		if got := localVerify(strings.Fields(args)); got != want {
			t.Errorf("localVerify(%q) = %v, want %v", args, got, want)
		}
	}
}

func TestMatchIDs(t *testing.T) {
	if !matchIDs("tf-001", nil) || !matchIDs("tf-001", []string{"docker-", "tf-"}) || matchIDs("tf-001", []string{"docker-"}) {
		t.Error("matchIDs")
	}
}
