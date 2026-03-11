package cluster

import (
	"context"
	"fmt"
	"os"

	"github.com/pejas/kagen/internal/agent"
	"github.com/pejas/kagen/internal/devfile"
	"github.com/pejas/kagen/internal/git"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// KubeManager implements ClusterManager interface using client-go.
type KubeManager struct {
	client  *kubernetes.Clientset
	kubeCtx string
}

// NewKubeManager returns a new KubeManager for the given context.
func NewKubeManager(kubeCtx string) (*KubeManager, error) {
	client, err := NewClientset(kubeCtx)
	if err != nil {
		return nil, err
	}
	return &KubeManager{
		client:  client,
		kubeCtx: kubeCtx,
	}, nil
}

// EnsureNamespace ensures the repo-scoped namespace exists.
func (k *KubeManager) EnsureNamespace(ctx context.Context, repo *git.Repository) error {
	nsName := fmt.Sprintf("kagen-%s", repo.ID())
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: nsName,
			Labels: map[string]string{
				"kagen.io/scope": "repo",
			},
		},
	}
	if os.Getenv("KAGEN_E2E") == "true" {
		ns.Labels["kagen.io/e2e"] = "true"
	}

	_, err := k.client.CoreV1().Namespaces().Create(ctx, ns, metav1.CreateOptions{})
	if err != nil {
		if errors.IsAlreadyExists(err) {
			return nil
		}
		// Try to get it to be sure.
		_, err = k.client.CoreV1().Namespaces().Get(ctx, nsName, metav1.GetOptions{})
		if err == nil {
			return nil
		}
		return fmt.Errorf("creating namespace %s: %w", nsName, err)
	}

	return nil
}

// EnsureResources orchestrates the PVCs and Pod for the repository.
func (k *KubeManager) EnsureResources(ctx context.Context, repo *git.Repository, agentType agent.Type, d *devfile.Devfile) error {
	nsName := fmt.Sprintf("kagen-%s", repo.ID())

	// 1. Generate Pod spec.
	gen := &devfile.Generator{Namespace: nsName}
	pod, err := gen.GeneratePod("agent", d)
	if err != nil {
		return fmt.Errorf("generating pod spec: %w", err)
	}
	pod.Labels["kagen.io/repo-id"] = repo.ID()
	injectWorkspaceSync(pod, repo)
	injectAgentRuntime(pod, agentType)

	// 2. Ensure PVCs (Stage 3 focuses on simple PVC existence).
	// For this stage, we assume PVCs mentioned in devfile volumes are handled or we create simple ones.
	for _, v := range pod.Spec.Volumes {
		if v.PersistentVolumeClaim != nil {
			err := k.ensurePVC(ctx, nsName, v.PersistentVolumeClaim.ClaimName)
			if err != nil {
				return err
			}
		}
	}

	// 3. Reconcile Pod.
	_, err = k.client.CoreV1().Pods(nsName).Create(ctx, pod, metav1.CreateOptions{})
	if err != nil {
		// Ignore if already exists, but Stage 3 doesn't handle updates yet.
		// For now, if it exists, we just return nil.
		return nil
	}

	return nil
}

func injectWorkspaceSync(pod *corev1.Pod, repo *git.Repository) {
	pod.Spec.InitContainers = append(pod.Spec.InitContainers, corev1.Container{
		Name:    "workspace-sync",
		Image:   "alpine/git:2.47.2",
		Command: []string{"/bin/sh", "-lc"},
		Args: []string{fmt.Sprintf(`set -eu
worktree=/projects/workspace
rm -rf "$worktree"
git clone "http://kagen:kagen-internal-secret@forgejo:3000/kagen/workspace.git" "$worktree"
cd "$worktree"
git checkout %q 2>/dev/null || git checkout -b %q "origin/%s"
`, repo.CurrentBranch, repo.CurrentBranch, repo.CurrentBranch)},
		VolumeMounts: []corev1.VolumeMount{
			{
				Name:      "git-workspace",
				MountPath: "/projects",
			},
		},
	})
}

func injectAgentRuntime(pod *corev1.Pod, agentType agent.Type) {
	if len(pod.Spec.Containers) == 0 {
		return
	}

	switch agentType {
	case agent.Codex:
		pod.Spec.Containers[0].Env = append(pod.Spec.Containers[0].Env,
			corev1.EnvVar{Name: "HOME", Value: "/home/kagen"},
			corev1.EnvVar{Name: "CODEX_HOME", Value: "/home/kagen/.codex"},
		)
	default:
	}
}

// AttachAgent connects the current terminal to the agent process.
func (k *KubeManager) AttachAgent(ctx context.Context, repo *git.Repository) error {
	// This will be implemented using os/exec to call kubectl exec -it.
	// It's the most reliable way to handle TUI and terminal resizing.
	nsName := fmt.Sprintf("kagen-%s", repo.ID())

	// find the pod
	pods, err := k.client.CoreV1().Pods(nsName).List(ctx, metav1.ListOptions{
		LabelSelector: "app.kubernetes.io/name=kagen-agent",
	})
	if err != nil || len(pods.Items) == 0 {
		return fmt.Errorf("agent pod not found in namespace %s", nsName)
	}
	podName := pods.Items[0].Name

	fmt.Printf("Attaching to agent in pod %s/%s...\n", nsName, podName)

	// cmd := exec.Command("kubectl", "--context", k.kubeCtx, "exec", "-it", "-n", nsName, podName, "--", "/bin/sh")
	// cmd.Stdin = os.Stdin
	// cmd.Stdout = os.Stdout
	// cmd.Stderr = os.Stderr
	// return cmd.Run()

	return nil // Stub for now, will implement properly in Stage 3 completion.
}

func (k *KubeManager) ensurePVC(ctx context.Context, ns, name string) error {
	// Simple PVC creation for Stage 3.
	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: resource.MustParse("1Gi"),
				},
			},
		},
	}
	_, err := k.client.CoreV1().PersistentVolumeClaims(ns).Create(ctx, pvc, metav1.CreateOptions{})
	if err != nil {
		// ignore already exists
		return nil
	}
	return nil
}
