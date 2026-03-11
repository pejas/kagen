package agent

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"time"

	"github.com/pejas/kagen/internal/git"
	"github.com/pejas/kagen/internal/kubeexec"
)

// baseAgent provides shared functionality for agent implementations.
type baseAgent struct {
	agentType Type
	name      string
	repo      *git.Repository
	kubeCtx   string
	exec      kubeexec.Runner
}

func (b *baseAgent) Name() string    { return b.name }
func (b *baseAgent) AgentType() Type { return b.agentType }

// Authenticate is a no-op by default, overridden by agents that need it.
func (b *baseAgent) Authenticate(ctx context.Context) error {
	return nil
}

// Launch ensures the agent process is prepared. For most, this is a no-op
// as the Pod generation already includes the container.
func (b *baseAgent) Launch(ctx context.Context) error {
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
func (b *baseAgent) Attach(ctx context.Context) error {
	if os.Getenv("KAGEN_NON_INTERACTIVE") == "true" {
		return nil
	}
	if b.exec == nil {
		return nil
	}

	ns := fmt.Sprintf("kagen-%s", b.repo.ID())

	// We use the same logic as KubeManager.AttachAgent but specialized here.
	// In a real implementation, we'd look up the pod by labels.
	podName := "agent" // Simplified for now, should be looked up

	return b.exec.Attach(ctx, ns, podName, b.commandArgs())
}

func (b *baseAgent) commandArgs() []string {
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

func (b *baseAgent) waitForCodex(ctx context.Context, namespace string) error {
	for i := 0; i < 90; i++ {
		command := []string{
			"/bin/sh", "-lc",
			"test -d /projects/workspace/.git && command -v codex >/dev/null 2>&1",
		}
		if _, err := b.exec.Run(ctx, namespace, "agent", command); err == nil {
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
	return &baseAgent{
		agentType: Claude,
		name:      "Claude",
		repo:      repo,
		kubeCtx:   kubeCtx,
		exec:      kubeexec.NewRunner(kubeCtx),
	}
}

// NewCodexAgent returns a real Codex agent.
func NewCodexAgent(repo *git.Repository, kubeCtx string) Agent {
	return &baseAgent{
		agentType: Codex,
		name:      "Codex",
		repo:      repo,
		kubeCtx:   kubeCtx,
		exec:      kubeexec.NewRunner(kubeCtx),
	}
}

// NewOpenCodeAgent returns a real OpenCode agent.
func NewOpenCodeAgent(repo *git.Repository, kubeCtx string) Agent {
	return &baseAgent{
		agentType: OpenCode,
		name:      "OpenCode",
		repo:      repo,
		kubeCtx:   kubeCtx,
		exec:      kubeexec.NewRunner(kubeCtx),
	}
}
