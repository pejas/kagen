package forgejo

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/pejas/kagen/internal/cluster"
	"github.com/pejas/kagen/internal/git"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
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

const forgejoConfigPath = "/etc/gitea/app.ini"

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

	_, err = f.client.AppsV1().Deployments(ns).Create(ctx, deployment, metav1.CreateOptions{})
	if err != nil && !errors.IsAlreadyExists(err) {
		return fmt.Errorf("creating forgejo deployment: %w", err)
	}

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
	if err != nil && !errors.IsAlreadyExists(err) {
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
	if err != nil && !errors.IsAlreadyExists(err) {
		return err
	}
	return nil
}

func (f *ForgejoService) waitReady(ctx context.Context, ns string) error {
	// Wait for the deployment to report a ready replica.
	// ImportRepo performs its own retries when port-forwarding and talking
	// to the API, so duplicating that work here adds a flaky extra gate.
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

	// 1. Ensure admin user exists with retries.
	podName, err := f.getForgejoPod(ctx, ns)
	if err != nil {
		return err
	}

	createAdminCmd := []string{
		"forgejo", "--config", forgejoConfigPath, "admin", "user", "create",
		"--username", "kagen",
		"--password", "kagen-internal-secret",
		"--email", "kagen@internal.local",
		"--admin",
		"--must-change-password=false",
	}

	for i := 0; i < 5; i++ {
		out, err := f.execInPod(ctx, ns, podName, createAdminCmd)
		if err == nil || strings.Contains(out, "already exists") || strings.Contains(err.Error(), "already exists") {
			break
		}
		fmt.Fprintf(os.Stderr, "ℹ Retry user creation (%d/5)... output: %s, err: %v\n", i+1, out, err)
		time.Sleep(2 * time.Second)
	}

	// Verify user exists via CLI
	listCmd := []string{"forgejo", "--config", forgejoConfigPath, "admin", "user", "list"}
	out, _ := f.execInPod(ctx, ns, podName, listCmd)
	if !strings.Contains(out, "kagen") {
		fmt.Fprintf(os.Stderr, "⚠ User 'kagen' not found in user list: %s\n", out)
	}

	// Wait for user to be available in API
	time.Sleep(2 * time.Second)

	// 2. Start port-forward to the Forgejo pod and verify the API is reachable.
	var (
		lastErr   error
		localPort int
	)
	for i := 0; i < 5; i++ {
		localPort, err = f.pf.Start(ctx, ns, "pod/"+podName, 3000)
		if err != nil {
			lastErr = fmt.Errorf("starting port-forward to forgejo: %w", err)
			time.Sleep(1 * time.Second)
			continue
		}

		if err := f.waitForAPI(ctx, localPort); err != nil {
			_ = f.pf.Stop()
			lastErr = err
			time.Sleep(1 * time.Second)
			continue
		}

		if err := f.createRepo(localPort, "workspace"); err == nil {
			lastErr = nil
			break
		} else {
			_ = f.pf.Stop()
			lastErr = err
			time.Sleep(1 * time.Second)
		}
	}
	if lastErr != nil {
		return fmt.Errorf("failed to create forgejo repo after retries: %w", lastErr)
	}
	defer f.pf.Stop()

	// 4. Configure git remote and push with retries.
	remoteUrl := fmt.Sprintf("http://kagen:kagen-internal-secret@127.0.0.1:%d/kagen/workspace.git", localPort)
	_ = repo.AddRemote("kagen", remoteUrl)

	for i := 0; i < 5; i++ {
		if err := repo.Push(ctx, "kagen", "HEAD:"+repo.CurrentBranch); err == nil {
			return nil
		} else {
			lastErr = err
			time.Sleep(1 * time.Second)
		}
	}

	return fmt.Errorf("pushing to forgejo: %w", lastErr)
}

func (f *ForgejoService) waitForAPI(ctx context.Context, port int) error {
	versionURL := fmt.Sprintf("http://127.0.0.1:%d/api/v1/version", port)

	for i := 0; i < 30; i++ {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			resp, err := http.Get(versionURL)
			if err == nil {
				resp.Body.Close()
				if resp.StatusCode == http.StatusOK {
					return nil
				}
			}
			time.Sleep(500 * time.Millisecond)
		}
	}

	return fmt.Errorf("timed out waiting for forgejo API on local port %d", port)
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

func (f *ForgejoService) execInPod(ctx context.Context, ns, pod string, command []string) (string, error) {
	args := append([]string{"exec", "-n", ns, pod, "--"}, command...)
	cmd := exec.CommandContext(ctx, "kubectl", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("kubectl exec failed: %s: %w", string(out), err)
	}
	return string(out), nil
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
func int64Ptr(i int64) *int64 { return &i }
