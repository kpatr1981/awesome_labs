package lab

import (
	"errors"
	"testing"
)

func TestLockAppAllowsOnlyOne(t *testing.T) {
	m := &Manager{StateDir: t.TempDir()}
	release, err := m.LockApp()
	if err != nil {
		t.Fatal(err)
	}
	// a second open file description conflicts like another process would
	if _, err := m.LockApp(); !errors.Is(err, ErrRunning) {
		t.Fatalf("second lock: %v, want ErrRunning", err)
	}
	release()
	again, err := m.LockApp()
	if err != nil {
		t.Fatalf("after release: %v", err)
	}
	again()
}
