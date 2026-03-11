package agent

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"time"

	"github.com/pejas/kagen/internal/git"
)

// BaseAgent provides shared functionality for agent implementations.
type BaseAgent struct {
	agentType     Type
	name          string
	repo          *git.Repository
	kubeCtx       string
	containerName string
	spec          RuntimeSpec
}

func (b *BaseAgent) Name() string    { return b.name }
func (b *BaseAgent) AgentType() Type { return b.agentType }

// Authenticate is a no-op by default, overridden by agents that need it.
func (b *BaseAgent) Authenticate(ctx context.Context) error {
	return nil
}

// Launch ensures the agent process is prepared. For most, this is a no-op
// as the Pod generation already includes the container.
func (b *BaseAgent) Launch(ctx context.Context) error {
	ns := fmt.Sprintf("kagen-%s", b.repo.ID())

	waitArgs := []string{
		"--context", b.kubeCtx,
		"wait",
		"--for=condition=Ready",
		"-n", ns,
		"pod/agent",
		"--timeout=5m",
	}
	waitCmd := exec.CommandContext(ctx, "kubectl", waitArgs...)
	if out, err := waitCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("waiting for agent pod readiness: %s: %w", string(out), err)
	}

	if err := b.waitForRuntime(ctx, ns); err != nil {
		return err
	}

	return nil
}

// Attach connects the user's terminal to the agent process in the cluster.
func (b *BaseAgent) Attach(ctx context.Context) error {
	if os.Getenv("KAGEN_NON_INTERACTIVE") == "true" {
		return nil
	}

	ns := fmt.Sprintf("kagen-%s", b.repo.ID())

	// We use the same logic as KubeManager.AttachAgent but specialized here.
	// In a real implementation, we'd look up the pod by labels.
	podName := "agent" // Simplified for now, should be looked up

	args := []string{
		"--context", b.kubeCtx,
		"exec", "-it",
		"-n", ns,
		podName,
		"--",
	}
	if b.containerName != "" {
		args = []string{
			"--context", b.kubeCtx,
			"exec", "-it",
			"-n", ns,
			podName,
			"-c", b.containerName,
			"--",
		}
	}
	args = append(args, b.commandArgs()...)

	cmd := exec.CommandContext(ctx, "kubectl", args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd.Run()
}

func (b *BaseAgent) commandArgs() []string {
	return []string{"/bin/sh", "-lc", b.spec.AttachShell}
}

func (b *BaseAgent) waitForRuntime(ctx context.Context, namespace string) error {
	for i := 0; i < 90; i++ {
		args := []string{
			"--context", b.kubeCtx,
			"exec",
			"-n", namespace,
			"agent",
		}
		if b.containerName != "" {
			args = append(args, "-c", b.containerName)
		}
		args = append(args, "--", "/bin/sh", "-lc", b.spec.ReadyCheck())
		cmd := exec.CommandContext(ctx, "kubectl", args...)
		if err := cmd.Run(); err == nil {
			return nil
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}

	return fmt.Errorf("timed out waiting for %s runtime bootstrap in pod %s/agent", b.name, namespace)
}

// NewClaudeAgent returns a real Claude agent.
func NewClaudeAgent(repo *git.Repository, kubeCtx, containerName string) Agent {
	spec, _ := SpecFor(Claude)
	return &BaseAgent{
		agentType:     Claude,
		name:          "Claude",
		repo:          repo,
		kubeCtx:       kubeCtx,
		containerName: containerName,
		spec:          spec,
	}
}

// NewCodexAgent returns a real Codex agent.
func NewCodexAgent(repo *git.Repository, kubeCtx, containerName string) Agent {
	spec, _ := SpecFor(Codex)
	return &BaseAgent{
		agentType:     Codex,
		name:          "Codex",
		repo:          repo,
		kubeCtx:       kubeCtx,
		containerName: containerName,
		spec:          spec,
	}
}

// NewOpenCodeAgent returns a real OpenCode agent.
func NewOpenCodeAgent(repo *git.Repository, kubeCtx, containerName string) Agent {
	spec, _ := SpecFor(OpenCode)
	return &BaseAgent{
		agentType:     OpenCode,
		name:          "OpenCode",
		repo:          repo,
		kubeCtx:       kubeCtx,
		containerName: containerName,
		spec:          spec,
	}
}
