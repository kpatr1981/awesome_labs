// Package config loads ~/.awesome_labs/config.yaml.
package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// VMSize overrides a VM's resources. Changes apply when the VM is (re)created.
type VMSize struct {
	CPUs   int    `yaml:"cpus"`
	Memory string `yaml:"memory"`
}

// Config is the user configuration.
type Config struct {
	// AutoStop stops an exercise's environment once you solve it from the
	// command line (awesome_labs check). Default true. The app always keeps
	// only the environment of the exercise you start, deleting the others.
	AutoStop *bool `yaml:"auto_stop"`

	// OnExit is what leaving the app does to the lab VMs: "delete" (the
	// default) deletes them all, "stop" stops them and "keep" leaves them
	// running. With stop or keep the exercise you were on is still set up
	// when you come back.
	OnExit string `yaml:"on_exit"`
	// AutoCheck checks the exercise whenever its files change (a save in VS
	// Code, leaving the terminal). Toggled with A on the exercise screen.
	AutoCheck bool `yaml:"auto_check"`
	// CheckUpdates looks for a newer release on GitHub once a day.
	CheckUpdates bool `yaml:"check_updates"`
	// Theme is auto, dark or light. NO_COLOR turns colours off.
	Theme string `yaml:"theme"`

	Docker       VMSize `yaml:"docker"`
	ControlPlane VMSize `yaml:"controlplane"`
	Worker       VMSize `yaml:"worker"`
	Offsec       VMSize `yaml:"offsec"`
}

// AutoStopEnabled returns the effective auto_stop setting.
func (c Config) AutoStopEnabled() bool { return c.AutoStop == nil || *c.AutoStop }

// ExitAction returns the effective on_exit setting: delete, stop or keep.
func (c Config) ExitAction() string {
	switch c.OnExit {
	case "stop", "keep":
		return c.OnExit
	}
	return "delete"
}

// Validate reports settings with values awesome_labs doesn't know.
func (c Config) Validate() error {
	switch c.OnExit {
	case "", "delete", "stop", "keep":
	default:
		return fmt.Errorf("on_exit: %q is not delete, stop or keep", c.OnExit)
	}
	switch c.Theme {
	case "", "auto", "dark", "light":
	default:
		return fmt.Errorf("theme: %q is not auto, dark or light", c.Theme)
	}
	return nil
}

const template = `# awesome_labs configuration
#
# Stop an exercise's environment once you solve it with "awesome_labs check".
# The app always keeps only the environment of the exercise you start: the
# others are deleted, and exiting or starting the app deletes every lab VM.
# After its first build each environment keeps a stopped copy (klbase-* in
# "limactl list"), so it comes back in seconds; "env rebuild" replaces it.
auto_stop: true

# What leaving the app does to the lab VMs: delete (default) deletes them all;
# stop stops them, freeing their memory; keep leaves them running. With stop
# or keep the exercise you were on is still set up when you come back.
# on_exit: delete

# Check the exercise whenever its files change: a save in VS Code, or leaving
# the terminal. A on the exercise screen turns it on and off.
# auto_check: false

# Look for a newer awesome_labs on GitHub once a day and say so in the app.
# Nothing about you or your progress is sent. "awesome_labs version --check"
# checks once, any time.
# check_updates: false

# Colours: auto, dark or light. NO_COLOR=1 turns colours off.
# theme: auto

# VM sizes. Changes apply when a VM is created: run
#   awesome_labs env rebuild docker|k8s
# afterwards. Defaults shown; the Kubernetes nodes need at least 2 CPUs / 2GiB.
# docker:       {cpus: 2, memory: 3GiB}
# controlplane: {cpus: 2, memory: 3GiB}
# worker:       {cpus: 2, memory: 2560MiB}
# offsec:       {cpus: 2, memory: 3GiB}
`

// Path returns the config file location inside dir.
func Path(dir string) string { return filepath.Join(dir, "config.yaml") }

// Load reads dir/config.yaml, creating it with defaults if it doesn't exist.
func Load(dir string) (Config, error) {
	var c Config
	p := Path(dir)
	b, err := os.ReadFile(p)
	if os.IsNotExist(err) {
		_ = os.WriteFile(p, []byte(template), 0o644)
		return c, nil
	}
	if err != nil {
		return c, err
	}
	if err := yaml.Unmarshal(b, &c); err != nil {
		return c, fmt.Errorf("%s: %w", p, err)
	}
	if err := c.Validate(); err != nil {
		return c, fmt.Errorf("%s: %w", p, err)
	}
	return c, nil
}
