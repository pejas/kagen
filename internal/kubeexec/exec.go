package kubeexec

import (
	"context"
	"fmt"
	"os"
	"os/exec"
)

// Runner manages remote command execution in cluster pods.
type Runner interface {
	Run(ctx context.Context, namespace, pod string, command []string) (string, error)
	Attach(ctx context.Context, namespace, pod string, command []string) error
	WaitForPodReady(ctx context.Context, namespace, pod, timeout string) error
}

// KubectlRunner implements Runner using kubectl exec.
type KubectlRunner struct {
	kubeCtx string
}

// NewRunner returns a new kubectl-backed Runner.
func NewRunner(kubeCtx string) *KubectlRunner {
	return &KubectlRunner{kubeCtx: kubeCtx}
}

// Run executes a non-interactive command in a pod and returns combined output.
func (r *KubectlRunner) Run(ctx context.Context, namespace, pod string, command []string) (string, error) {
	args := []string{"exec"}
	if r.kubeCtx != "" {
		args = append(args, "--context", r.kubeCtx)
	}
	args = append(args, "-n", namespace, pod, "--")
	args = append(args, command...)

	cmd := exec.CommandContext(ctx, "kubectl", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("kubectl exec %s/%s: %s: %w", namespace, pod, string(out), err)
	}

	return string(out), nil
}

// Attach opens an interactive kubectl exec session in a pod.
func (r *KubectlRunner) Attach(ctx context.Context, namespace, pod string, command []string) error {
	args := []string{"exec"}
	if r.kubeCtx != "" {
		args = append(args, "--context", r.kubeCtx)
	}
	args = append(args, "-it", "-n", namespace, pod, "--")
	args = append(args, command...)

	cmd := exec.CommandContext(ctx, "kubectl", args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("kubectl attach %s/%s: %w", namespace, pod, err)
	}

	return nil
}

// WaitForPodReady blocks until the target pod reports Ready.
func (r *KubectlRunner) WaitForPodReady(ctx context.Context, namespace, pod, timeout string) error {
	args := []string{"wait"}
	if r.kubeCtx != "" {
		args = append(args, "--context", r.kubeCtx)
	}
	args = append(args, "--for=condition=Ready", "-n", namespace, "pod/"+pod, "--timeout="+timeout)

	cmd := exec.CommandContext(ctx, "kubectl", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("kubectl wait %s/%s: %s: %w", namespace, pod, string(out), err)
	}

	return nil
}
