// Package workload builds typed Kubernetes workload specifications for kagen.
package workload

import (
	"fmt"

	"github.com/pejas/kagen/internal/agent"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	defaultWorkspaceName  = "workspace"
	defaultAgentHomeName  = "agent-home"
	defaultWorkspaceMount = "/projects"
)

// Images describes the pinned baseline images for the generated pod.
type Images struct {
	Workspace string
	Toolbox   string
}

// Request describes the typed inputs required to build the runtime pod.
type Request struct {
	Name      string
	Namespace string
	Runtime   agent.RuntimeSpec
	Images    Images
}

// Builder produces the baseline runtime pod without any CLI orchestration.
type Builder struct{}

// NewBuilder returns a workload Builder.
func NewBuilder() *Builder {
	return &Builder{}
}

// BuildPod creates the baseline pod specification for the requested runtime.
func (b *Builder) BuildPod(req Request) (*corev1.Pod, error) {
	if req.Name == "" {
		return nil, fmt.Errorf("workload name is required")
	}
	if req.Namespace == "" {
		return nil, fmt.Errorf("workload namespace is required")
	}
	if req.Runtime == nil || req.Runtime.Type() == "" {
		return nil, fmt.Errorf("runtime type is required")
	}

	if req.Images.Workspace == "" {
		return nil, fmt.Errorf("workspace image is required")
	}
	if req.Images.Toolbox == "" {
		return nil, fmt.Errorf("toolbox image is required")
	}

	images := req.Images
	pod := &corev1.Pod{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Pod",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      req.Name,
			Namespace: req.Namespace,
			Labels: map[string]string{
				"app.kubernetes.io/name": "kagen-agent",
			},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				workspaceContainer(images),
				runtimeContainer(req.Runtime, images),
			},
			Volumes: baselineVolumes(req.Name),
		},
	}

	return pod, nil
}

func workspaceContainer(images Images) corev1.Container {
	return corev1.Container{
		Name:    defaultWorkspaceName,
		Image:   images.Workspace,
		Command: []string{"/bin/sh", "-lc"},
		Args:    []string{"exec tail -f /dev/null"},
		VolumeMounts: []corev1.VolumeMount{
			{
				Name:      "git-workspace",
				MountPath: defaultWorkspaceMount,
			},
		},
	}
}

func runtimeContainer(spec agent.RuntimeSpec, images Images) corev1.Container {
	return corev1.Container{
		Name:       agent.ContainerName(spec),
		Image:      images.Toolbox,
		Command:    []string{"/bin/sh", "-lc"},
		Args:       []string{"exec tail -f /dev/null"},
		Env:        requiredEnv(spec.RequiredEnv()),
		WorkingDir: defaultWorkspaceMount + "/workspace",
		VolumeMounts: []corev1.VolumeMount{
			{
				Name:      "git-workspace",
				MountPath: defaultWorkspaceMount,
			},
			{
				Name:      defaultAgentHomeName,
				MountPath: agent.DefaultHomeDir(),
			},
		},
	}
}

func baselineVolumes(workloadName string) []corev1.Volume {
	return []corev1.Volume{
		{
			Name: "git-workspace",
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{
					Medium: corev1.StorageMediumMemory,
				},
			},
		},
		{
			Name: defaultAgentHomeName,
			VolumeSource: corev1.VolumeSource{
				PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
					ClaimName: workloadName + "-" + defaultAgentHomeName,
				},
			},
		},
	}
}

func requiredEnv(input []agent.EnvVar) []corev1.EnvVar {
	env := make([]corev1.EnvVar, 0, len(input))
	for _, variable := range input {
		env = append(env, corev1.EnvVar{
			Name:  variable.Name,
			Value: variable.Value,
		})
	}

	return env
}
