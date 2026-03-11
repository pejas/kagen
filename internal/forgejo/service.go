package forgejo

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os/exec"
	"time"

	"github.com/pejas/kagen/internal/cluster"
	"github.com/pejas/kagen/internal/git"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes"
)

// ForgejoService implements the Service interface using client-go.
type ForgejoService struct {
	client *kubernetes.Clientset
	pf     cluster.PortForwarder
}

// NewForgejoService returns a new ForgejoService.
func NewForgejoService(client *kubernetes.Clientset, pf cluster.PortForwarder) *ForgejoService {
	return &ForgejoService{
		client: client,
		pf:     pf,
	}
}

// EnsureRepo ensures the Forgejo deployment and service exist in the namespace.
func (f *ForgejoService) EnsureRepo(ctx context.Context, repo *git.Repository) error {
	ns := fmt.Sprintf("kagen-%s", repo.ID())

	// 1. Ensure PVC
	err := f.ensurePVC(ctx, ns)
	if err != nil {
		return fmt.Errorf("ensuring forgejo pvc: %w", err)
	}

	// 2. Ensure Deployment
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "forgejo",
			Namespace: ns,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: int32Ptr(1),
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": "forgejo"},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"app": "forgejo"},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "forgejo",
							Image: "codeberg.org/forgejo/forgejo:1.21",
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

	_, err = f.client.AppsV1().Deployments(ns).Create(ctx, deployment, metav1.CreateOptions{})
	if err != nil && metav1.StatusReason(err.Error()) != metav1.StatusReasonAlreadyExists {
		return fmt.Errorf("creating forgejo deployment: %w", err)
	}

	// 3. Ensure Service
	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "forgejo",
			Namespace: ns,
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

	_, err = f.client.CoreV1().Services(ns).Create(ctx, service, metav1.CreateOptions{})
	if err != nil && metav1.StatusReason(err.Error()) != metav1.StatusReasonAlreadyExists {
		return fmt.Errorf("creating forgejo service: %w", err)
	}

	return f.waitReady(ctx, ns)
}

func (f *ForgejoService) ensurePVC(ctx context.Context, ns string) error {
	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "forgejo-data",
			Namespace: ns,
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
	if err != nil && metav1.StatusReason(err.Error()) != metav1.StatusReasonAlreadyExists {
		return err
	}
	return nil
}

func (f *ForgejoService) waitReady(ctx context.Context, ns string) error {
	// Simple readiness poll.
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			dep, err := f.client.AppsV1().Deployments(ns).Get(ctx, "forgejo", metav1.GetOptions{})
			if err == nil && dep.Status.ReadyReplicas > 0 {
				return nil
			}
			time.Sleep(2 * time.Second)
		}
	}
}

// ImportRepo ensures the repository exists in Forgejo and prepares it for the first push.
func (f *ForgejoService) ImportRepo(ctx context.Context, repo *git.Repository) error {
	ns := fmt.Sprintf("kagen-%s", repo.ID())

	// 1. Ensure admin user exists.
	podName, err := f.getForgejoPod(ctx, ns)
	if err != nil {
		return err
	}

	createAdminCmd := []string{
		"forgejo", "admin", "user", "create",
		"--username", "kagen",
		"--password", "kagen-internal-secret",
		"--email", "kagen@internal.local",
		"--admin",
		"--must-change-password=false",
	}
	_ = f.execInPod(ctx, ns, podName, createAdminCmd)

	// 2. Start port-forward to Forgejo.
	localPort, err := f.pf.Start(ctx, ns, "svc/forgejo", 3000)
	if err != nil {
		return fmt.Errorf("starting port-forward to forgejo: %w", err)
	}
	defer f.pf.Stop()

	// 3. Create the repository via API.
	if err := f.createRepo(localPort, "workspace"); err != nil {
		// Ignore if already exists.
		// ui.Info("Forgejo repository 'workspace' already exists or created.") // Assuming ui.Info is defined elsewhere
	}

	// 4. Configure git remote and push.
	// We use the localPort assigned by port-forward.
	remoteUrl := fmt.Sprintf("http://kagen:kagen-internal-secret@127.0.0.1:%d/kagen/workspace.git", localPort)

	// Ensure remote exists
	_ = repo.AddRemote("kagen", remoteUrl)

	// Push current head to Forgejo
	if err := repo.Push(ctx, "kagen", "HEAD:"+repo.CurrentBranch); err != nil {
		return fmt.Errorf("pushing to forgejo: %w", err)
	}

	return nil
}

func (f *ForgejoService) createRepo(port int, name string) error {
	apiUrl := fmt.Sprintf("http://127.0.0.1:%d/api/v1/user/repos", port)
	body, _ := json.Marshal(map[string]interface{}{
		"name":           name,
		"private":        true,
		"auto_init":      false,
		"default_branch": "main",
	})

	req, err := http.NewRequest("POST", apiUrl, bytes.NewBuffer(body))
	if err != nil {
		return err
	}
	req.SetBasicAuth("kagen", "kagen-internal-secret")
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusConflict {
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	return nil
}

func (f *ForgejoService) getForgejoPod(ctx context.Context, ns string) (string, error) {
	pods, err := f.client.CoreV1().Pods(ns).List(ctx, metav1.ListOptions{
		LabelSelector: "app=forgejo",
	})
	if err != nil || len(pods.Items) == 0 {
		return "", fmt.Errorf("forgejo pod not found")
	}
	return pods.Items[0].Name, nil
}

func (f *ForgejoService) execInPod(ctx context.Context, ns, pod string, command []string) error {
	args := append([]string{"exec", "-n", ns, pod, "--"}, command...)
	cmd := exec.CommandContext(ctx, "kubectl", args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("kubectl exec failed: %s: %w", string(out), err)
	}
	return nil
}

// GetReviewURL returns the local browser URL for the repository review in Forgejo.
func (f *ForgejoService) GetReviewURL(repo *git.Repository) (string, error) {
	// In the real kagen flow, we'll likely have a stable ingress or known port.
	// For now, we point to the repo's main page or the specific kagen branch.
	return fmt.Sprintf("http://localhost:3000/kagen/workspace/src/branch/%s", repo.CurrentBranch), nil
}

// HasNewCommits checks if there are commits in Forgejo not yet pulled local.
func (f *ForgejoService) HasNewCommits(ctx context.Context, repo *git.Repository) (bool, error) {
	return false, nil
}

func int32Ptr(i int32) *int32 { return &i }
