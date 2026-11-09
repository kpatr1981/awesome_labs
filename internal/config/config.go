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
	// AutoStop stops the other lab environment when you start an exercise, so
	// only one runs at a time (saves ~3-6 GiB of RAM), and stops an exercise's
	// environment once you solve it. Default true.
	AutoStop *bool `yaml:"auto_stop"`

	Docker       VMSize `yaml:"docker"`
	ControlPlane VMSize `yaml:"controlplane"`
	Worker       VMSize `yaml:"worker"`
	Offsec       VMSize `yaml:"offsec"`
}

// AutoStopEnabled returns the effective auto_stop setting.
func (c Config) AutoStopEnabled() bool { return c.AutoStop == nil || *c.AutoStop }

const template = `# awesome_labs configuration
#
# Stop the other lab environment when you start an exercise, so that only one
# of them (Docker host or Kubernetes cluster) uses memory at a time, and stop
# an exercise's environment once you solve it.
auto_stop: true

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
	return c, nil
}
