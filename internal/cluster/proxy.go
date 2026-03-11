package cluster

import (
	"context"
	"fmt"
	"strings"

	"github.com/pejas/kagen/internal/git"
	"github.com/pejas/kagen/internal/proxy"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	proxyDeploymentName = "egress-proxy"
	proxyConfigMapName  = "egress-proxy-config"
	proxyImage          = "docker.io/alpine:3.22"
)

// EnsureProxy reconciles the repo-scoped proxy metadata and readiness anchor.
//
// This creates a namespaced ConfigMap for the allowlist and a lightweight
// Deployment that represents proxy enforcement state until traffic steering is
// wired through the agent pod.
func (k *KubeManager) EnsureProxy(ctx context.Context, repo *git.Repository, policy *proxy.Policy) error {
	nsName := fmt.Sprintf("kagen-%s", repo.ID())
	if policy == nil || len(policy.AllowedDestinations) == 0 {
		return k.deleteProxyResources(ctx, nsName)
	}

	if err := k.ensureProxyConfig(ctx, nsName, repo.ID(), policy); err != nil {
		return err
	}
	if err := k.ensureProxyDeployment(ctx, nsName, repo.ID()); err != nil {
		return err
	}

	return nil
}

func (k *KubeManager) deleteProxyResources(ctx context.Context, namespace string) error {
	if err := k.client.AppsV1().Deployments(namespace).Delete(ctx, proxyDeploymentName, metav1.DeleteOptions{}); err != nil && !k8serrors.IsNotFound(err) {
		return fmt.Errorf("deleting proxy deployment: %w", err)
	}
	if err := k.client.CoreV1().ConfigMaps(namespace).Delete(ctx, proxyConfigMapName, metav1.DeleteOptions{}); err != nil && !k8serrors.IsNotFound(err) {
		return fmt.Errorf("deleting proxy configmap: %w", err)
	}

	return nil
}

// ProxyReady reports whether the repo-scoped proxy deployment is ready.
func (k *KubeManager) ProxyReady(ctx context.Context, repo *git.Repository) (bool, error) {
	nsName := fmt.Sprintf("kagen-%s", repo.ID())
	deployment, err := k.client.AppsV1().Deployments(nsName).Get(ctx, proxyDeploymentName, metav1.GetOptions{})
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return false, nil
		}
		return false, fmt.Errorf("getting proxy deployment: %w", err)
	}

	return deployment.Status.ReadyReplicas > 0, nil
}

func (k *KubeManager) ensureProxyConfig(ctx context.Context, namespace, repoID string, policy *proxy.Policy) error {
	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      proxyConfigMapName,
			Namespace: namespace,
			Labels: map[string]string{
				"app.kubernetes.io/name": "kagen-proxy",
				"kagen.io/repo-id":       repoID,
			},
		},
		Data: map[string]string{
			"allowlist.txt": strings.Join(policy.AllowedDestinations, "\n"),
		},
	}

	_, err := k.client.CoreV1().ConfigMaps(namespace).Create(ctx, configMap, metav1.CreateOptions{})
	if err == nil {
		return nil
	}
	if !k8serrors.IsAlreadyExists(err) {
		return fmt.Errorf("creating proxy configmap: %w", err)
	}

	current, err := k.client.CoreV1().ConfigMaps(namespace).Get(ctx, proxyConfigMapName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("getting proxy configmap: %w", err)
	}

	current.Data = configMap.Data
	current.Labels = configMap.Labels
	if _, err := k.client.CoreV1().ConfigMaps(namespace).Update(ctx, current, metav1.UpdateOptions{}); err != nil {
		return fmt.Errorf("updating proxy configmap: %w", err)
	}

	return nil
}

func (k *KubeManager) ensureProxyDeployment(ctx context.Context, namespace, repoID string) error {
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      proxyDeploymentName,
			Namespace: namespace,
			Labels: map[string]string{
				"app.kubernetes.io/name": "kagen-proxy",
				"kagen.io/repo-id":       repoID,
			},
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: int32Ptr(1),
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app.kubernetes.io/name": "kagen-proxy",
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app.kubernetes.io/name": "kagen-proxy",
						"kagen.io/repo-id":       repoID,
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:    "proxy",
							Image:   proxyImage,
							Command: []string{"/bin/sh", "-lc"},
							Args: []string{
								"echo 'proxy reconciliation anchor active'; trap : TERM INT; while true; do sleep 3600; done",
							},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "config",
									MountPath: "/etc/kagen-proxy",
									ReadOnly:  true,
								},
							},
						},
					},
					Volumes: []corev1.Volume{
						{
							Name: "config",
							VolumeSource: corev1.VolumeSource{
								ConfigMap: &corev1.ConfigMapVolumeSource{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: proxyConfigMapName,
									},
								},
							},
						},
					},
				},
			},
		},
	}

	_, err := k.client.AppsV1().Deployments(namespace).Create(ctx, deployment, metav1.CreateOptions{})
	if err == nil {
		return nil
	}
	if !k8serrors.IsAlreadyExists(err) {
		return fmt.Errorf("creating proxy deployment: %w", err)
	}

	current, err := k.client.AppsV1().Deployments(namespace).Get(ctx, proxyDeploymentName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("getting proxy deployment: %w", err)
	}

	current.Labels = deployment.Labels
	current.Spec.Template.Labels = deployment.Spec.Template.Labels
	current.Spec.Template.Spec = deployment.Spec.Template.Spec
	if _, err := k.client.AppsV1().Deployments(namespace).Update(ctx, current, metav1.UpdateOptions{}); err != nil {
		return fmt.Errorf("updating proxy deployment: %w", err)
	}

	return nil
}

func int32Ptr(value int32) *int32 {
	return &value
}
