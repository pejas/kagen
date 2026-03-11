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
	agentType Type
	name      string
	repo      *git.Repository
	kubeCtx   string
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

	if b.agentType == Codex {
		if err := b.waitForCodex(ctx, ns); err != nil {
			return err
		}
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
	args = append(args, b.commandArgs()...)

	cmd := exec.CommandContext(ctx, "kubectl", args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd.Run()
}

func (b *BaseAgent) commandArgs() []string {
	switch b.agentType {
	case Claude:
		return []string{"claude-code"}
	case Codex:
		return []string{
			"codex",
			"-C", "/projects/workspace",
			"--sandbox", "danger-full-access",
			"-a", "never",
		}
	case OpenCode:
		return []string{"opencode"}
	default:
		return []string{"/bin/sh"}
	}
}

func (b *BaseAgent) waitForCodex(ctx context.Context, namespace string) error {
	for i := 0; i < 90; i++ {
		args := []string{
			"--context", b.kubeCtx,
			"exec",
			"-n", namespace,
			"agent",
			"--",
			"/bin/sh", "-lc",
			"test -d /projects/workspace/.git && command -v codex >/dev/null 2>&1",
		}
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

	return fmt.Errorf("timed out waiting for Codex runtime bootstrap in pod %s/agent", namespace)
}

// NewClaudeAgent returns a real Claude agent.
func NewClaudeAgent(repo *git.Repository, kubeCtx string) Agent {
	return &BaseAgent{
		agentType: Claude,
		name:      "Claude",
		repo:      repo,
		kubeCtx:   kubeCtx,
	}
}

// NewCodexAgent returns a real Codex agent.
func NewCodexAgent(repo *git.Repository, kubeCtx string) Agent {
	return &BaseAgent{
		agentType: Codex,
		name:      "Codex",
		repo:      repo,
		kubeCtx:   kubeCtx,
	}
}

// NewOpenCodeAgent returns a real OpenCode agent.
func NewOpenCodeAgent(repo *git.Repository, kubeCtx string) Agent {
	return &BaseAgent{
		agentType: OpenCode,
		name:      "OpenCode",
		repo:      repo,
		kubeCtx:   kubeCtx,
	}
}
