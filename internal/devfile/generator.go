package devfile

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Generator maps our simplified Devfile structure to a Kubernetes Pod specification.
type Generator struct {
	Namespace string
}

// GeneratePod creates a Pod specification from the Devfile.
func (g *Generator) GeneratePod(name string, d *Devfile) (*corev1.Pod, error) {
	pod := &corev1.Pod{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Pod",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: g.Namespace,
			Labels: map[string]string{
				"app.kubernetes.io/name": "kagen-agent",
			},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{},
			Volumes: []corev1.Volume{
				{
					Name: "git-workspace",
					VolumeSource: corev1.VolumeSource{
						EmptyDir: &corev1.EmptyDirVolumeSource{
							Medium: corev1.StorageMediumMemory,
						},
					},
				},
			},
		},
	}

	for _, comp := range d.Components {
		// Handle Containers
		if comp.Container != nil {
			c := comp.Container
			container := corev1.Container{
				Name:    comp.Name,
				Image:   c.Image,
				Command: c.Command,
				Args:    c.Args,
				Env:     []corev1.EnvVar{},
				VolumeMounts: []corev1.VolumeMount{
					{
						Name:      "git-workspace",
						MountPath: "/projects",
					},
				},
			}

			for _, e := range c.Env {
				container.Env = append(container.Env, corev1.EnvVar{
					Name:  e.Name,
					Value: e.Value,
				})
			}

			for _, v := range c.VolumeMounts {
				container.VolumeMounts = append(container.VolumeMounts, corev1.VolumeMount{
					Name:      v.Name,
					MountPath: v.Path,
				})
			}

			pod.Spec.Containers = append(pod.Spec.Containers, container)
		}

		// Handle Volumes
		if comp.Volume != nil {
			pod.Spec.Volumes = append(pod.Spec.Volumes, corev1.Volume{
				Name: comp.Name,
				VolumeSource: corev1.VolumeSource{
					PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
						ClaimName: name + "-" + comp.Name,
					},
				},
			})
		}
	}

	if len(pod.Spec.Containers) == 0 {
		return nil, fmt.Errorf("no container components found in devfile")
	}

	return pod, nil
}
