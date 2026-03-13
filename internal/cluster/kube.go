package cluster

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/pejas/kagen/internal/agent"
	"github.com/pejas/kagen/internal/git"
	"github.com/pejas/kagen/internal/proxy"
	"github.com/pejas/kagen/internal/ui"
	"github.com/pejas/kagen/internal/workload"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

const (
	forgejoBootstrapSecretName = "forgejo-bootstrap-auth"
	forgejoSecretUsernameKey   = "username"
	forgejoSecretPasswordKey   = "password"
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
	ui.Verbose("Ensuring namespace %s exists", nsName)
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
func (k *KubeManager) EnsureResources(ctx context.Context, repo *git.Repository, agentType string, pod *corev1.Pod, policy *proxy.Policy) error {
	nsName := fmt.Sprintf("kagen-%s", repo.ID())
	pod.Labels["kagen.io/repo-id"] = repo.ID()
	injectWorkspaceSync(pod, repo)
	injectAgentRuntime(pod, agentType, nsName, policy)
	ui.Verbose("Reconciling pod %s/%s", nsName, pod.Name)

	// 2. Ensure PVCs for volumes requested by the generated runtime pod.
	for _, v := range pod.Spec.Volumes {
		if v.PersistentVolumeClaim != nil {
			if err := k.ensurePVC(ctx, nsName, v.PersistentVolumeClaim.ClaimName); err != nil {
				return err
			}
		}
	}

	// 3. Reconcile Pod.
	_, err := k.client.CoreV1().Pods(nsName).Create(ctx, pod, metav1.CreateOptions{})
	if err == nil {
		return nil
	}

	if !errors.IsAlreadyExists(err) {
		return fmt.Errorf("creating pod %s/%s: %w", nsName, pod.Name, err)
	}

	if err := k.replacePod(ctx, nsName, pod); err != nil {
		return fmt.Errorf("replacing pod %s/%s: %w", nsName, pod.Name, err)
	}

	return nil
}

func injectWorkspaceSync(pod *corev1.Pod, repo *git.Repository) {
	pod.Spec.InitContainers = append(pod.Spec.InitContainers, corev1.Container{
		Name:    "workspace-sync",
		Image:   workload.DefaultImages().Workspace,
		Command: []string{"/bin/sh", "-lc"},
		Args: []string{fmt.Sprintf(`set -eu
export GIT_TERMINAL_PROMPT=0
cd /
repo_url="http://forgejo:3000/kagen/workspace.git"
auth_header="$(printf '%%s' "${FORGEJO_USERNAME}:${FORGEJO_PASSWORD}" | base64 | tr -d '\n')"
worktree=/projects/workspace
rm -rf "$worktree"
mkdir -p /projects
mkdir -p /home/kagen
for _ in $(seq 1 90); do
  if git -c "http.extraHeader=Authorization: Basic ${auth_header}" ls-remote "$repo_url" >/dev/null 2>&1; then
    break
  fi
  sleep 2
done
git -c "http.extraHeader=Authorization: Basic ${auth_header}" clone "$repo_url" "$worktree"
cd "$worktree"
git checkout %q 2>/dev/null || git checkout --track -b %q "origin/%s"
chown -R 1000:1000 /projects /home/kagen
`, repo.KagenBranch(), repo.KagenBranch(), repo.KagenBranch())},
		SecurityContext: &corev1.SecurityContext{
			RunAsUser:  int64Ptr(0),
			RunAsGroup: int64Ptr(0),
		},
		Env: []corev1.EnvVar{
			{
				Name: "FORGEJO_USERNAME",
				ValueFrom: &corev1.EnvVarSource{
					SecretKeyRef: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{Name: forgejoBootstrapSecretName},
						Key:                  forgejoSecretUsernameKey,
					},
				},
			},
			{
				Name: "FORGEJO_PASSWORD",
				ValueFrom: &corev1.EnvVarSource{
					SecretKeyRef: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{Name: forgejoBootstrapSecretName},
						Key:                  forgejoSecretPasswordKey,
					},
				},
			},
		},
		VolumeMounts: []corev1.VolumeMount{
			{
				Name:      "git-workspace",
				MountPath: "/projects",
			},
			{
				Name:      "agent-home",
				MountPath: agent.DefaultHomeDir(),
			},
		},
	})
}

func injectAgentRuntime(pod *corev1.Pod, agentType, namespace string, policy *proxy.Policy) {
	if len(pod.Spec.Containers) == 0 || agentType == "" {
		return
	}

	spec, err := agent.SpecFor(agent.Type(agentType))
	if err != nil {
		return
	}

	container := runtimeContainer(pod, spec.ContainerName())
	if container == nil {
		container = &pod.Spec.Containers[0]
	}

	for _, variable := range spec.RequiredEnv {
		setContainerEnv(container, variable.Name, variable.Value)
	}

	// Inject git authorship configuration
	injectGitAuthorship(container, spec)

	if policyEnabled(policy) {
		injectProxyEnv(container, namespace)
	}
}

func injectProxyEnv(container *corev1.Container, namespace string) {
	proxyURL := fmt.Sprintf("http://%s.%s.svc.cluster.local:%d", proxyServiceName, namespace, proxyPort)
	noProxy := strings.Join([]string{
		"127.0.0.1",
		"localhost",
		".svc",
		".svc.cluster.local",
		"forgejo",
		fmt.Sprintf("forgejo.%s.svc.cluster.local", namespace),
		fmt.Sprintf("%s.%s.svc.cluster.local", proxyServiceName, namespace),
	}, ",")

	container.Env = append(container.Env,
		corev1.EnvVar{Name: "HTTP_PROXY", Value: proxyURL},
		corev1.EnvVar{Name: "HTTPS_PROXY", Value: proxyURL},
		corev1.EnvVar{Name: "ALL_PROXY", Value: proxyURL},
		corev1.EnvVar{Name: "http_proxy", Value: proxyURL},
		corev1.EnvVar{Name: "https_proxy", Value: proxyURL},
		corev1.EnvVar{Name: "all_proxy", Value: proxyURL},
		corev1.EnvVar{Name: "NO_PROXY", Value: noProxy},
		corev1.EnvVar{Name: "no_proxy", Value: noProxy},
	)
}

// injectGitAuthorship configures git author and committer environment variables.
// Author is the AI agent (for attribution), committer is the human user (for notifications).
func injectGitAuthorship(container *corev1.Container, spec agent.RuntimeSpec) {
	// Read host git config for committer info
	hostUser, err := git.GetHostUser()
	if err != nil {
		// If we can't read host config, use reasonable defaults
		hostUser = &git.UserConfig{Name: "kagen-user", Email: ""}
	}

	// Build agent author email using subaddressing (RFC 5233)
	authorEmail := git.AddSubaddress(hostUser.Email, spec.GitAuthorName)

	// Set git environment variables
	setContainerEnv(container, "GIT_AUTHOR_NAME", spec.GitAuthorName)
	setContainerEnv(container, "GIT_AUTHOR_EMAIL", authorEmail)
	setContainerEnv(container, "GIT_COMMITTER_NAME", hostUser.Name)
	setContainerEnv(container, "GIT_COMMITTER_EMAIL", hostUser.Email)
}

func runtimeContainer(pod *corev1.Pod, name string) *corev1.Container {
	for i := range pod.Spec.Containers {
		if pod.Spec.Containers[i].Name == name {
			return &pod.Spec.Containers[i]
		}
	}

	return nil
}

func setContainerEnv(container *corev1.Container, name, value string) {
	for i := range container.Env {
		if container.Env[i].Name == name {
			container.Env[i].Value = value
			return
		}
	}

	container.Env = append(container.Env, corev1.EnvVar{Name: name, Value: value})
}

func policyEnabled(policy *proxy.Policy) bool {
	return policy != nil && len(policy.AllowedDestinations) > 0
}

func int64Ptr(value int64) *int64 {
	return &value
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
	if err != nil && !errors.IsAlreadyExists(err) {
		return fmt.Errorf("creating pvc %s/%s: %w", ns, name, err)
	}

	return nil
}

func (k *KubeManager) replacePod(ctx context.Context, namespace string, pod *corev1.Pod) error {
	gracePeriodSeconds := int64(0)
	if err := k.client.CoreV1().Pods(namespace).Delete(ctx, pod.Name, metav1.DeleteOptions{
		GracePeriodSeconds: &gracePeriodSeconds,
	}); err != nil && !errors.IsNotFound(err) {
		return fmt.Errorf("deleting existing pod: %w", err)
	}

	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		_, err := k.client.CoreV1().Pods(namespace).Get(ctx, pod.Name, metav1.GetOptions{})
		if errors.IsNotFound(err) {
			break
		}
		if err != nil {
			return fmt.Errorf("checking pod deletion: %w", err)
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}

	if _, err := k.client.CoreV1().Pods(namespace).Create(ctx, pod, metav1.CreateOptions{}); err != nil {
		return fmt.Errorf("creating replacement pod: %w", err)
	}

	return nil
}
