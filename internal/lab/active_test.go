package lab

import (
	"testing"

	"awesome_labs/internal/exercise"
)

func TestActiveExercise(t *testing.T) {
	m := &Manager{StateDir: t.TempDir()}
	if _, ok := m.Active(); ok {
		t.Fatal("fresh state has an active exercise")
	}
	m.setActive(&exercise.Exercise{ID: "k8s-101", Profile: exercise.ProfileK8s})
	a, ok := m.Active()
	if !ok || a.ID != "k8s-101" || a.Profile != exercise.ProfileK8s || a.Since.IsZero() {
		t.Fatalf("got %+v %v", a, ok)
	}
	// wiping another environment leaves it alone
	m.clearActiveIn(DockerEnv)
	if _, ok := m.Active(); !ok {
		t.Fatal("clearing the docker env dropped a k8s exercise")
	}
	// clearing a different id leaves it alone
	m.ClearActive("docker-001")
	if _, ok := m.Active(); !ok {
		t.Fatal("ClearActive of another id dropped it")
	}
	m.clearActiveIn(K8sEnv)
	if _, ok := m.Active(); ok {
		t.Fatal("wiping its environment kept the exercise active")
	}
	// a new start replaces the previous one
	m.setActive(&exercise.Exercise{ID: "docker-001", Profile: exercise.ProfileDocker})
	m.setActive(&exercise.Exercise{ID: "docker-002", Profile: exercise.ProfileDocker})
	if a, _ := m.Active(); a.ID != "docker-002" {
		t.Fatalf("active = %s, want docker-002", a.ID)
	}
	m.ClearActive("")
	if _, ok := m.Active(); ok {
		t.Fatal("ClearActive(\"\") kept it")
	}
}

func TestStopDeleteRefuseNonLabVMs(t *testing.T) {
	m := &Manager{StateDir: t.TempDir()}
	if err := m.DeleteVM(t.Context(), "default", nil); err == nil {
		t.Fatal("DeleteVM accepted a non-lab instance")
	}
	if err := m.StopVM(t.Context(), "my-vm", nil); err == nil {
		t.Fatal("StopVM accepted a non-lab instance")
	}
}

// An environment cleaned when its exercise ended lets exactly the next start
// skip the reset; a shell into it, or another environment, doesn't count.
func TestCleanMark(t *testing.T) {
	m := &Manager{StateDir: t.TempDir()}
	if m.takeClean(DockerEnv) {
		t.Fatal("fresh state is marked clean")
	}
	m.setClean(DockerEnv, true)
	if m.takeClean(K8sEnv) {
		t.Fatal("cleaning the docker env marked k8s clean")
	}
	if !m.takeClean(DockerEnv) {
		t.Fatal("cleaned env not marked clean")
	}
	if m.takeClean(DockerEnv) {
		t.Fatal("the clean mark served two starts")
	}
	m.setClean(DockerEnv, true)
	_ = m.ShellCommand(DockerEnv, false)
	if m.takeClean(DockerEnv) {
		t.Fatal("still clean after a shell was opened")
	}
}
