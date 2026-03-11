package devfile

import (
	"fmt"

	"github.com/pejas/kagen/internal/agent"
)

const (
	defaultAgentHomeVolumeName = "agent-home"
	defaultAgentHomeVolumeSize = "5Gi"
	defaultWorkspaceImage      = "vxcontrol/codebase:latest"
)

// FindRuntimeComponent returns the container component for the requested agent.
func (d *Devfile) FindRuntimeComponent(agentType agent.Type) *Component {
	for i := range d.Components {
		component := &d.Components[i]
		if component.Attributes["kagen.agent/runtime"] == string(agentType) {
			return component
		}
	}

	return nil
}

// EnsureRuntimeComponent mutates the devfile so the requested runtime exists
// and returns the container name that should be used for attachment.
func EnsureRuntimeComponent(d *Devfile, spec agent.RuntimeSpec) (string, error) {
	component := d.FindRuntimeComponent(spec.Type)
	if component == nil {
		d.Components = append(d.Components, Component{
			Name: spec.ContainerName(),
			Attributes: Attributes{
				"kagen.agent/runtime": string(spec.Type),
			},
			Container: &Container{
				Image:        defaultWorkspaceImage,
				Command:      spec.BootstrapCommand(),
				Args:         spec.BootstrapArgs(),
				Env:          []Env{},
				VolumeMounts: []VolumeMount{},
			},
		})
		component = &d.Components[len(d.Components)-1]
	}

	if component.Container == nil {
		return "", fmt.Errorf("runtime component %s for %s has no container", component.Name, spec.Type)
	}

	if component.Attributes == nil {
		component.Attributes = Attributes{}
	}
	component.Attributes["kagen.agent/runtime"] = string(spec.Type)

	if len(component.Container.Command) == 0 && len(component.Container.Args) == 0 {
		component.Container.Command = spec.BootstrapCommand()
		component.Container.Args = spec.BootstrapArgs()
	}

	for name, value := range spec.RequiredEnvMap() {
		setEnv(component.Container, name, value)
	}

	if !hasVolumeMountPath(component.Container, agent.DefaultHomeDir()) {
		component.Container.VolumeMounts = append(component.Container.VolumeMounts, VolumeMount{
			Name: defaultAgentHomeVolumeName,
			Path: agent.DefaultHomeDir(),
		})
		ensureVolumeComponent(d, defaultAgentHomeVolumeName, defaultAgentHomeVolumeSize)
	}

	return component.Name, nil
}

func setEnv(container *Container, name, value string) {
	for i := range container.Env {
		if container.Env[i].Name == name {
			container.Env[i].Value = value
			return
		}
	}

	container.Env = append(container.Env, Env{Name: name, Value: value})
}

func hasVolumeMountPath(container *Container, path string) bool {
	for _, mount := range container.VolumeMounts {
		if mount.Path == path {
			return true
		}
	}

	return false
}

func ensureVolumeComponent(d *Devfile, name, size string) {
	for _, component := range d.Components {
		if component.Name == name && component.Volume != nil {
			return
		}
	}

	d.Components = append(d.Components, Component{
		Name:   name,
		Volume: &Volume{Size: size},
	})
}
