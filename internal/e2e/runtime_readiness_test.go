package e2e

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/pejas/kagen/internal/agent"
	"github.com/pejas/kagen/internal/cluster"
	"github.com/pejas/kagen/internal/git"
	"github.com/pejas/kagen/internal/kubeexec"
	"github.com/pejas/kagen/internal/session"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

const (
	e2eKubeContext        = "colima-kagen"
	e2eProxyDeployment    = "egress-proxy"
	e2eProxyConfigMap     = "egress-proxy-config"
	e2eProxyNetworkPolicy = "egress-proxy-egress"
)

type commandResult struct {
	output   string
	exitCode int
	err      error
}

type asyncCommand struct {
	cmd    *exec.Cmd
	output bytes.Buffer
	done   chan commandResult
}

type readinessHarness struct {
	t        *testing.T
	homeDir  string
	repoDir  string
	repo     *git.Repository
	client   kubernetes.Interface
	executor kubeexec.Runner
}

func TestDetachedStartReadinessForOtherAgents(t *testing.T) {
	requireE2EPrerequisites(t)

	tests := []struct {
		name      string
		agentType agent.Type
	}{
		{name: "claude", agentType: agent.Claude},
		{name: "opencode", agentType: agent.OpenCode},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h := newReadinessHarness(t, "feature/e2e-"+tc.name)

			started := h.startAsync([]string{"start", "--detach", string(tc.agentType)}, nil)
			starting := h.waitForSessionStatus(workflowStatusStarting, 2*time.Minute)
			result := started.wait(t, 10*time.Minute)

			if result.exitCode != 0 {
				t.Fatalf("kagen start --detach %s exit code = %d, want 0\noutput:\n%s", tc.agentType, result.exitCode, result.output)
			}

			summary := h.mustSummary(starting.Session.ID)
			if summary.Session.Status != workflowStatusReady {
				t.Fatalf("session %d status = %q, want %q", summary.Session.ID, summary.Session.Status, workflowStatusReady)
			}

			h.assertRuntimeReady(summary, tc.agentType)
		})
	}
}

func TestDetachedStartCodexLifecycleReadiness(t *testing.T) {
	requireE2EPrerequisites(t)

	h := newReadinessHarness(t, "feature/e2e-codex")

	started := h.startAsync([]string{"start", "--detach", "codex"}, nil)
	starting := h.waitForSessionStatus(workflowStatusStarting, 2*time.Minute)
	result := started.wait(t, 10*time.Minute)

	if result.exitCode != 0 {
		t.Fatalf("kagen start --detach codex exit code = %d, want 0\noutput:\n%s", result.exitCode, result.output)
	}

	summary := h.mustSummary(starting.Session.ID)
	if summary.Session.Status != workflowStatusReady {
		t.Fatalf("session %d status = %q, want %q", summary.Session.ID, summary.Session.Status, workflowStatusReady)
	}
	h.assertRuntimeReady(summary, agent.Codex)

	initialAgentSessions := len(summary.AgentSessions)

	attachResult := h.run([]string{"attach", "codex"}, nil)
	if attachResult.exitCode != 0 {
		t.Fatalf("kagen attach codex exit code = %d, want 0\noutput:\n%s", attachResult.exitCode, attachResult.output)
	}

	attached := h.mustSummary(summary.Session.ID)
	if attached.Session.Status != workflowStatusReady {
		t.Fatalf("session %d status after attach = %q, want %q", attached.Session.ID, attached.Session.Status, workflowStatusReady)
	}
	if len(attached.AgentSessions) != initialAgentSessions+1 {
		t.Fatalf("agent session count after attach = %d, want %d", len(attached.AgentSessions), initialAgentSessions+1)
	}
	h.assertRuntimeReady(attached, agent.Codex)

	downResult := h.run([]string{"down"}, nil)
	if downResult.exitCode != 0 {
		t.Fatalf("kagen down exit code = %d, want 0\noutput:\n%s", downResult.exitCode, downResult.output)
	}

	reattachResult := h.run([]string{"attach", "codex", "--session", fmt.Sprintf("%d", summary.Session.ID)}, nil)
	if reattachResult.exitCode != 0 {
		t.Fatalf("kagen attach codex --session %d exit code = %d, want 0\noutput:\n%s", summary.Session.ID, reattachResult.exitCode, reattachResult.output)
	}

	restarted := h.mustSummary(summary.Session.ID)
	if restarted.Session.Status != workflowStatusReady {
		t.Fatalf("session %d status after down/attach = %q, want %q", restarted.Session.ID, restarted.Session.Status, workflowStatusReady)
	}
	if len(restarted.AgentSessions) != initialAgentSessions+2 {
		t.Fatalf("agent session count after down/attach = %d, want %d", len(restarted.AgentSessions), initialAgentSessions+2)
	}
	h.assertRuntimeReady(restarted, agent.Codex)
}

func TestDetachedStartFailureMarksPersistedSessionFailed(t *testing.T) {
	requireE2EPrerequisites(t)

	h := newReadinessHarness(t, "feature/e2e-failure")

	started := h.startAsync([]string{"start", "--detach", "codex"}, map[string]string{
		"KAGEN_PROXY_IMAGE": "busybox:latest",
	})
	starting := h.waitForSessionStatus(workflowStatusStarting, 30*time.Second)
	result := started.wait(t, 4*time.Minute)

	if result.exitCode == 0 {
		t.Fatalf("kagen start --detach codex exit code = 0, want non-zero\noutput:\n%s", result.output)
	}

	summary := h.mustSummary(starting.Session.ID)
	if summary.Session.Status != workflowStatusFailed {
		t.Fatalf("session %d status = %q, want %q", summary.Session.ID, summary.Session.Status, workflowStatusFailed)
	}

	h.assertProxyFailureState(summary.Session.Namespace)
}

func newReadinessHarness(t *testing.T, branch string) *readinessHarness {
	t.Helper()

	realHome, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("resolving real home directory: %v", err)
	}

	homeDir := t.TempDir()
	linkHomeFixture(t, filepath.Join(realHome, ".kube"), filepath.Join(homeDir, ".kube"))
	linkHomeFixture(t, filepath.Join(realHome, ".colima"), filepath.Join(homeDir, ".colima"))

	t.Setenv("HOME", homeDir)

	repoDir := t.TempDir()
	repo := createGitRepository(t, repoDir, branch)

	client, err := cluster.NewClientset(e2eKubeContext)
	if err != nil {
		t.Fatalf("creating kubernetes clientset: %v", err)
	}

	h := &readinessHarness{
		t:        t,
		homeDir:  homeDir,
		repoDir:  repoDir,
		repo:     repo,
		client:   client,
		executor: kubeexec.NewRunner(e2eKubeContext),
	}

	t.Cleanup(func() {
		_ = cleanupNamespace(h.repoNamespace())
	})

	return h
}

func requireE2EPrerequisites(t *testing.T) {
	t.Helper()

	for _, command := range []string{"git", "kubectl", "colima"} {
		if _, err := exec.LookPath(command); err != nil {
			t.Skipf("skipping e2e tests: %s not found in PATH", command)
		}
	}

	binPath, err := kagenBinaryPath()
	if err != nil {
		t.Fatalf("resolving kagen binary path: %v", err)
	}
	if _, err := os.Stat(binPath); err != nil {
		t.Skipf("skipping e2e tests: %s not built", binPath)
	}
}

func createGitRepository(t *testing.T, repoDir, branch string) *git.Repository {
	t.Helper()

	runGit := func(args ...string) {
		t.Helper()

		cmd := exec.Command("git", args...)
		cmd.Dir = repoDir
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v failed: %s: %v", args, string(out), err)
		}
	}

	runGit("init")
	runGit("config", "user.email", "e2e@example.com")
	runGit("config", "user.name", "E2E Tester")
	runGit("config", "commit.gpgsign", "false")

	if err := os.WriteFile(filepath.Join(repoDir, "README.md"), []byte("# Runtime E2E\n"), 0o644); err != nil {
		t.Fatalf("writing README.md: %v", err)
	}
	runGit("add", "README.md")
	runGit("commit", "-m", "initial commit")

	runGit("checkout", "-b", branch)
	if err := os.WriteFile(filepath.Join(repoDir, "branch.txt"), []byte(branch+"\n"), 0o644); err != nil {
		t.Fatalf("writing branch marker: %v", err)
	}
	runGit("add", "branch.txt")
	runGit("commit", "-m", "branch commit")

	repo, err := git.Discover(repoDir)
	if err != nil {
		t.Fatalf("discovering repository: %v", err)
	}

	return repo
}

func linkHomeFixture(t *testing.T, source, destination string) {
	t.Helper()

	if _, err := os.Stat(source); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return
		}
		t.Fatalf("stating %s: %v", source, err)
	}

	if err := os.Symlink(source, destination); err != nil {
		t.Fatalf("symlinking %s to %s: %v", source, destination, err)
	}
}

func (h *readinessHarness) run(args []string, extraEnv map[string]string) commandResult {
	h.t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	cmd := h.command(ctx, args, extraEnv)
	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output

	err := cmd.Run()
	result := commandResult{
		output: output.String(),
		err:    err,
	}
	if err == nil {
		return result
	}

	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		result.exitCode = exitErr.ExitCode()
		return result
	}

	h.t.Fatalf("running %q: %v\noutput:\n%s", strings.Join(args, " "), err, result.output)
	return commandResult{}
}

func (h *readinessHarness) startAsync(args []string, extraEnv map[string]string) *asyncCommand {
	h.t.Helper()

	ctx, _ := context.WithCancel(context.Background())
	cmd := h.command(ctx, args, extraEnv)

	running := &asyncCommand{
		cmd:  cmd,
		done: make(chan commandResult, 1),
	}
	cmd.Stdout = &running.output
	cmd.Stderr = &running.output

	if err := cmd.Start(); err != nil {
		h.t.Fatalf("starting %q: %v", strings.Join(args, " "), err)
	}

	go func() {
		err := cmd.Wait()
		result := commandResult{
			output: running.output.String(),
			err:    err,
		}
		if err == nil {
			running.done <- result
			return
		}

		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			result.exitCode = exitErr.ExitCode()
		}
		running.done <- result
	}()

	return running
}

func (c *asyncCommand) wait(t *testing.T, timeout time.Duration) commandResult {
	t.Helper()

	select {
	case result := <-c.done:
		return result
	case <-time.After(timeout):
		_ = c.cmd.Process.Kill()
		t.Fatalf("timed out waiting for command %q", strings.Join(c.cmd.Args, " "))
		return commandResult{}
	}
}

func (h *readinessHarness) command(ctx context.Context, args []string, extraEnv map[string]string) *exec.Cmd {
	h.t.Helper()

	binPath, err := kagenBinaryPath()
	if err != nil {
		h.t.Fatalf("resolving kagen binary path: %v", err)
	}

	cmd := exec.CommandContext(ctx, binPath, args...)
	cmd.Dir = h.repoDir
	cmd.Env = append([]string{}, os.Environ()...)
	cmd.Env = append(cmd.Env,
		"KAGEN_NON_INTERACTIVE=true",
		"KAGEN_E2E=true",
	)
	for key, value := range extraEnv {
		cmd.Env = append(cmd.Env, key+"="+value)
	}

	return cmd
}

func (h *readinessHarness) waitForSessionStatus(status string, timeout time.Duration) session.Summary {
	h.t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		summaries := h.repoSummaries()
		for _, summary := range summaries {
			if summary.Session.Status == status {
				return summary
			}
		}
		time.Sleep(250 * time.Millisecond)
	}

	h.t.Fatalf("timed out waiting for session status %q", status)
	return session.Summary{}
}

func (h *readinessHarness) repoSummaries() []session.Summary {
	h.t.Helper()

	store, err := session.OpenDefault()
	if err != nil {
		h.t.Fatalf("opening session store: %v", err)
	}
	defer store.Close()

	summaries, err := store.List(context.Background(), session.ListOptions{RepoPath: h.repo.Path})
	if err != nil {
		h.t.Fatalf("listing session summaries: %v", err)
	}

	return summaries
}

func (h *readinessHarness) mustSummary(id int64) session.Summary {
	h.t.Helper()

	store, err := session.OpenDefault()
	if err != nil {
		h.t.Fatalf("opening session store: %v", err)
	}
	defer store.Close()

	summary, found, err := store.GetSummary(context.Background(), id)
	if err != nil {
		h.t.Fatalf("getting session %d summary: %v", id, err)
	}
	if !found {
		h.t.Fatalf("session %d not found", id)
	}

	return summary
}

func (h *readinessHarness) repoNamespace() string {
	return fmt.Sprintf("kagen-%s", h.repo.ID())
}

func (h *readinessHarness) assertRuntimeReady(summary session.Summary, agentType agent.Type) {
	h.t.Helper()

	spec, err := agent.SpecFor(agentType)
	if err != nil {
		h.t.Fatalf("loading runtime spec for %s: %v", agentType, err)
	}

	if summary.Session.Namespace != h.repoNamespace() {
		h.t.Fatalf("session namespace = %q, want %q", summary.Session.Namespace, h.repoNamespace())
	}
	if summary.Session.Status != workflowStatusReady {
		h.t.Fatalf("session status = %q, want %q", summary.Session.Status, workflowStatusReady)
	}
	if summary.Session.WorkspaceBranch != h.repo.KagenBranch() {
		h.t.Fatalf("workspace branch = %q, want %q", summary.Session.WorkspaceBranch, h.repo.KagenBranch())
	}

	pod, err := h.client.CoreV1().Pods(summary.Session.Namespace).Get(context.Background(), summary.Session.PodName, metav1.GetOptions{})
	if err != nil {
		h.t.Fatalf("getting pod %s/%s: %v", summary.Session.Namespace, summary.Session.PodName, err)
	}
	if pod.Status.Phase != corev1.PodRunning {
		h.t.Fatalf("pod %s/%s phase = %s, want %s", pod.Namespace, pod.Name, pod.Status.Phase, corev1.PodRunning)
	}

	h.assertInitContainerCompleted(pod, "workspace-sync")
	h.assertContainerReady(pod, "workspace")
	h.assertContainerReady(pod, spec.ContainerName())

	workspaceBranch := strings.TrimSpace(h.execInRuntime(summary.Session.Namespace, spec.ContainerName(), `git -C /projects/workspace rev-parse --abbrev-ref HEAD`))
	if workspaceBranch != h.repo.KagenBranch() {
		h.t.Fatalf("workspace branch in pod = %q, want %q", workspaceBranch, h.repo.KagenBranch())
	}

	workspaceHead := strings.TrimSpace(h.execInRuntime(summary.Session.Namespace, spec.ContainerName(), `git -C /projects/workspace rev-parse HEAD`))
	if workspaceHead != h.repo.HeadSHA {
		h.t.Fatalf("workspace HEAD in pod = %q, want %q", workspaceHead, h.repo.HeadSHA)
	}

	stateSession := latestAgentSession(summary, agentType)
	if stateSession.ID == "" {
		h.t.Fatalf("missing persisted %s agent session for session %d", agentType, summary.Session.ID)
	}

	expectedStatePath := path.Join(spec.StateRoot(), stateSession.ID)
	if stateSession.StatePath != expectedStatePath {
		h.t.Fatalf("state path = %q, want %q", stateSession.StatePath, expectedStatePath)
	}

	h.execInRuntime(summary.Session.Namespace, spec.ContainerName(), fmt.Sprintf(`test -d %q`, stateSession.StatePath))
	h.assertProxyResources(summary.Session.Namespace, requiredProxyHosts(agentType))
}

func (h *readinessHarness) assertProxyResources(namespace string, expectedHosts []string) {
	h.t.Helper()

	deployment, err := h.client.AppsV1().Deployments(namespace).Get(context.Background(), e2eProxyDeployment, metav1.GetOptions{})
	if err != nil {
		h.t.Fatalf("getting proxy deployment %s/%s: %v", namespace, e2eProxyDeployment, err)
	}
	if deployment.Status.ReadyReplicas < 1 {
		h.t.Fatalf("proxy deployment %s/%s ready replicas = %d, want at least 1", namespace, deployment.Name, deployment.Status.ReadyReplicas)
	}

	if _, err := h.client.NetworkingV1().NetworkPolicies(namespace).Get(context.Background(), e2eProxyNetworkPolicy, metav1.GetOptions{}); err != nil {
		h.t.Fatalf("getting proxy network policy %s/%s: %v", namespace, e2eProxyNetworkPolicy, err)
	}

	configMap, err := h.client.CoreV1().ConfigMaps(namespace).Get(context.Background(), e2eProxyConfigMap, metav1.GetOptions{})
	if err != nil {
		h.t.Fatalf("getting proxy configmap %s/%s: %v", namespace, e2eProxyConfigMap, err)
	}

	allowlist := configMap.Data["allowlist"]
	for _, host := range expectedHosts {
		if !strings.Contains(allowlist, regexp.QuoteMeta(host)) {
			h.t.Fatalf("proxy allowlist missing %q in %s/%s", host, namespace, configMap.Name)
		}
	}
}

func (h *readinessHarness) assertProxyFailureState(namespace string) {
	h.t.Helper()

	deployment, err := h.client.AppsV1().Deployments(namespace).Get(context.Background(), e2eProxyDeployment, metav1.GetOptions{})
	if err != nil {
		h.t.Fatalf("getting failed proxy deployment %s/%s: %v", namespace, e2eProxyDeployment, err)
	}
	if deployment.Status.ReadyReplicas != 0 {
		h.t.Fatalf("failed proxy deployment ready replicas = %d, want 0", deployment.Status.ReadyReplicas)
	}

	_, err = h.client.CoreV1().Pods(namespace).Get(context.Background(), "agent", metav1.GetOptions{})
	if err == nil {
		h.t.Fatal("agent pod exists after proxy bootstrap failure, want no pod")
	}
	if !apierrors.IsNotFound(err) {
		h.t.Fatalf("getting agent pod after proxy bootstrap failure: %v", err)
	}
}

func (h *readinessHarness) assertInitContainerCompleted(pod *corev1.Pod, name string) {
	h.t.Helper()

	for _, status := range pod.Status.InitContainerStatuses {
		if status.Name != name {
			continue
		}
		if status.State.Terminated == nil {
			h.t.Fatalf("init container %s has no terminated state", name)
		}
		if status.State.Terminated.ExitCode != 0 {
			h.t.Fatalf("init container %s exit code = %d, want 0", name, status.State.Terminated.ExitCode)
		}
		return
	}

	h.t.Fatalf("init container %s not found", name)
}

func (h *readinessHarness) assertContainerReady(pod *corev1.Pod, name string) {
	h.t.Helper()

	for _, status := range pod.Status.ContainerStatuses {
		if status.Name != name {
			continue
		}
		if !status.Ready {
			h.t.Fatalf("container %s is not ready", name)
		}
		if status.State.Running == nil {
			h.t.Fatalf("container %s is not running", name)
		}
		return
	}

	h.t.Fatalf("container %s not found", name)
}

func (h *readinessHarness) execInRuntime(namespace, container, script string) string {
	h.t.Helper()

	output, err := h.executor.Run(context.Background(), namespace, "agent", []string{"/bin/sh", "-lc", script}, kubeexec.WithContainer(container))
	if err != nil {
		h.t.Fatalf("executing %q in %s/%s[%s]: %v", script, namespace, "agent", container, err)
	}

	return output
}

func latestAgentSession(summary session.Summary, agentType agent.Type) session.AgentSession {
	var (
		latest session.AgentSession
		found  bool
	)

	for _, agentSession := range summary.AgentSessions {
		if agentSession.AgentType != string(agentType) {
			continue
		}
		if !found || agentSession.LastUsedAt.After(latest.LastUsedAt) || agentSession.CreatedAt.After(latest.CreatedAt) {
			latest = agentSession
			found = true
		}
	}

	return latest
}

func requiredProxyHosts(agentType agent.Type) []string {
	switch agentType {
	case agent.Codex:
		return []string{"api.openai.com", "auth.openai.com"}
	case agent.Claude:
		return []string{"api.anthropic.com", "platform.claude.com"}
	case agent.OpenCode:
		return []string{"opencode.ai"}
	default:
		return nil
	}
}

const (
	workflowStatusFailed   = "failed"
	workflowStatusReady    = "ready"
	workflowStatusStarting = "starting"
)
