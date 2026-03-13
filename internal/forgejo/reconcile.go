package forgejo

import (
	"context"
	"fmt"
	"time"

	"github.com/pejas/kagen/internal/git"
	"github.com/pejas/kagen/internal/ui"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// EnsureRepo ensures the Forgejo deployment and service exist in the namespace.
func (f *ForgejoService) EnsureRepo(ctx context.Context, repo *git.Repository) error {
	ns := forgejoNamespace(repo)
	ui.Verbose("Reconciling Forgejo resources in namespace %s", ns)
	if err := f.ensureBootstrapSecret(ctx, ns); err != nil {
		return fmt.Errorf("ensuring forgejo bootstrap secret: %w", err)
	}
	if err := f.ensurePVC(ctx, ns, repo.ID()); err != nil {
		return fmt.Errorf("ensuring forgejo pvc: %w", err)
	}
	if err := f.ensureDeployment(ctx, ns, repo.ID()); err != nil {
		return err
	}
	if err := f.ensureService(ctx, ns, repo.ID()); err != nil {
		return err
	}

	return f.waitReady(ctx, ns)
}

func (f *ForgejoService) ensureBootstrapSecret(ctx context.Context, ns string) error {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      forgejoBootstrapSecretName,
			Namespace: ns,
		},
		StringData: map[string]string{
			forgejoSecretUsernameKey: forgejoAdminUsername,
			forgejoSecretPasswordKey: forgejoAdminPassword,
		},
	}

	_, err := f.client.CoreV1().Secrets(ns).Create(ctx, secret, metav1.CreateOptions{})
	if err == nil {
		return nil
	}
	if !errors.IsAlreadyExists(err) {
		return fmt.Errorf("creating forgejo bootstrap secret: %w", err)
	}

	current, err := f.client.CoreV1().Secrets(ns).Get(ctx, forgejoBootstrapSecretName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("getting forgejo bootstrap secret: %w", err)
	}
	if current.StringData == nil {
		current.StringData = map[string]string{}
	}
	current.StringData[forgejoSecretUsernameKey] = forgejoAdminUsername
	current.StringData[forgejoSecretPasswordKey] = forgejoAdminPassword
	if _, err := f.client.CoreV1().Secrets(ns).Update(ctx, current, metav1.UpdateOptions{}); err != nil {
		return fmt.Errorf("updating forgejo bootstrap secret: %w", err)
	}

	return nil
}

func (f *ForgejoService) ensurePVC(ctx context.Context, ns, repoID string) error {
	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "forgejo-data",
			Namespace: ns,
			Labels: map[string]string{
				"kagen.io/repo-id":       repoID,
				"app.kubernetes.io/name": "forgejo",
			},
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: resource.MustParse("2Gi"),
				},
			},
		},
	}

	_, err := f.client.CoreV1().PersistentVolumeClaims(ns).Create(ctx, pvc, metav1.CreateOptions{})
	if err != nil && !errors.IsAlreadyExists(err) {
		return err
	}

	return nil
}

func (f *ForgejoService) ensureDeployment(ctx context.Context, ns, repoID string) error {
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "forgejo",
			Namespace: ns,
			Labels: map[string]string{
				"kagen.io/repo-id":       repoID,
				"app.kubernetes.io/name": "forgejo",
			},
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: int32Ptr(1),
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": "forgejo"},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app":                    "forgejo",
						"kagen.io/repo-id":       repoID,
						"app.kubernetes.io/name": "forgejo",
					},
				},
				Spec: corev1.PodSpec{
					SecurityContext: &corev1.PodSecurityContext{
						RunAsUser:  int64Ptr(1000),
						RunAsGroup: int64Ptr(1000),
						FSGroup:    int64Ptr(1000),
					},
					Containers: []corev1.Container{
						{
							Name:  "forgejo",
							Image: "codeberg.org/forgejo/forgejo:1.21-rootless",
							Ports: []corev1.ContainerPort{
								{ContainerPort: 3000, Name: "http"},
								{ContainerPort: 22, Name: "ssh"},
							},
							Env: []corev1.EnvVar{
								{Name: "FORGEJO__database__DB_TYPE", Value: "sqlite3"},
								{Name: "FORGEJO__database__PATH", Value: "/data/gitea.db"},
								{Name: "FORGEJO__security__INSTALL_LOCK", Value: "true"},
							},
							VolumeMounts: []corev1.VolumeMount{
								{Name: "data", MountPath: "/data"},
							},
						},
					},
					Volumes: []corev1.Volume{
						{
							Name: "data",
							VolumeSource: corev1.VolumeSource{
								PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
									ClaimName: "forgejo-data",
								},
							},
						},
					},
				},
			},
		},
	}

	_, err := f.client.AppsV1().Deployments(ns).Create(ctx, deployment, metav1.CreateOptions{})
	if err != nil && !errors.IsAlreadyExists(err) {
		return fmt.Errorf("creating forgejo deployment: %w", err)
	}

	return nil
}

func (f *ForgejoService) ensureService(ctx context.Context, ns, repoID string) error {
	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "forgejo",
			Namespace: ns,
			Labels: map[string]string{
				"kagen.io/repo-id":       repoID,
				"app.kubernetes.io/name": "forgejo",
			},
		},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{"app": "forgejo"},
			Ports: []corev1.ServicePort{
				{Port: 3000, TargetPort: intstr.FromString("http"), Name: "http"},
				{Port: 22, TargetPort: intstr.FromString("ssh"), Name: "ssh"},
			},
			Type: corev1.ServiceTypeClusterIP,
		},
	}

	_, err := f.client.CoreV1().Services(ns).Create(ctx, service, metav1.CreateOptions{})
	if err != nil && !errors.IsAlreadyExists(err) {
		return fmt.Errorf("creating forgejo service: %w", err)
	}

	return nil
}

func (f *ForgejoService) waitReady(ctx context.Context, ns string) error {
	attempt := 0
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			attempt++
			dep, err := f.client.AppsV1().Deployments(ns).Get(ctx, "forgejo", metav1.GetOptions{})
			if err == nil && dep.Status.ReadyReplicas > 0 {
				ui.Verbose("Forgejo deployment %s/forgejo is ready after %d attempt(s)", ns, attempt)
				return nil
			}
			if err != nil && !errors.IsNotFound(err) {
				return fmt.Errorf("getting forgejo deployment: %w", err)
			}
			if ui.VerboseEnabled() && (attempt == 1 || attempt%5 == 0) {
				ui.Verbose("Waiting for Forgejo deployment %s/forgejo readiness (attempt %d)", ns, attempt)
			}
			if err := sleepContext(ctx, 2*time.Second); err != nil {
				return err
			}
		}
	}
}

func (f *ForgejoService) getForgejoPod(ctx context.Context, ns string) (string, error) {
	pods, err := f.client.CoreV1().Pods(ns).List(ctx, metav1.ListOptions{
		LabelSelector: "app=forgejo",
	})
	if err != nil {
		return "", fmt.Errorf("listing forgejo pods: %w", err)
	}
	if len(pods.Items) == 0 {
		return "", fmt.Errorf("forgejo pod not found in namespace %s", ns)
	}

	return pods.Items[0].Name, nil
}

func int32Ptr(value int32) *int32 {
	return &value
}

func int64Ptr(value int64) *int64 {
	return &value
}
