package agent

import (
	"context"
	"fmt"
	"os"
	"os/exec"

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
	return nil
}

// Attach connects the user's terminal to the agent process in the cluster.
func (b *BaseAgent) Attach(ctx context.Context) error {
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
		b.getCommand(),
	}

	cmd := exec.CommandContext(ctx, "kubectl", args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd.Run()
}

func (b *BaseAgent) getCommand() string {
	switch b.agentType {
	case Claude:
		return "claude-code"
	case Codex:
		return "codex"
	case OpenCode:
		return "opencode"
	default:
		return "/bin/sh"
	}
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
