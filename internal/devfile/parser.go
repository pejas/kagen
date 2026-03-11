package devfile

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v2"
)

// Devfile represents a simplified Devfile v2 structure.
type Devfile struct {
	SchemaVersion string      `yaml:"schemaVersion"`
	Metadata      Metadata    `yaml:"metadata"`
	Components    []Component `yaml:"components"`
}

type Metadata struct {
	Name string `yaml:"name"`
}

type Component struct {
	Name      string     `yaml:"name"`
	Container *Container `yaml:"container,omitempty"`
	Volume    *Volume    `yaml:"volume,omitempty"`
}

type Container struct {
	Image        string        `yaml:"image"`
	Command      []string      `yaml:"command,omitempty"`
	Args         []string      `yaml:"args,omitempty"`
	Env          []Env         `yaml:"env,omitempty"`
	VolumeMounts []VolumeMount `yaml:"volumeMounts,omitempty"`
	MemoryLimit  string        `yaml:"memoryLimit,omitempty"`
}

type Volume struct {
	Size string `yaml:"size"`
}

type Env struct {
	Name  string `yaml:"name"`
	Value string `yaml:"value"`
}

type VolumeMount struct {
	Name string `yaml:"name"`
	Path string `yaml:"path"`
}

// Parse reads a simplified Devfile v2 from the given path.
func Parse(path string) (*Devfile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading devfile: %w", err)
	}

	var d Devfile
	if err := yaml.Unmarshal(data, &d); err != nil {
		return nil, fmt.Errorf("parsing devfile: %w", err)
	}

	return &d, nil
}
