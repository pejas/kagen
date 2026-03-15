package agent

import (
	"context"
	"fmt"
	"os"

	"github.com/pejas/kagen/internal/git"
	"github.com/pejas/kagen/internal/kubeexec"
)

// baseAgent provides shared functionality for agent implementations.
type baseAgent struct {
	agentType     Type
	name          string
	repo          *git.Repository
	kubeCtx       string
	containerName string
	statePath     string
	spec          RuntimeSpec
	exec          kubeexec.Runner
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
	if err := b.exec.WaitForPodReady(ctx, ns, "agent", "5m"); err != nil {
		return fmt.Errorf("waiting for agent pod readiness: %w", err)
	}

	return nil
}

// Prepare ensures the runtime-specific state directory exists before attach.
func (b *baseAgent) Prepare(ctx context.Context) error {
	if b.exec == nil {
		return nil
	}

	ns := fmt.Sprintf("kagen-%s", b.repo.ID())
	if err := b.ensureStatePath(ctx, ns); err != nil {
		return err
	}

	return b.configureAgent(ctx, ns)
}

// configureAgent writes agent-specific configuration files.
func (b *baseAgent) configureAgent(ctx context.Context, namespace string) error {
	switch b.spec.Type {
	case OpenCode:
		return b.configureOpenCode(ctx, namespace)
	default:
		return nil
	}
}

// configureOpenCode writes opencode config to disable permission prompts
// inside the insulated container environment.
func (b *baseAgent) configureOpenCode(ctx context.Context, namespace string) error {
	configDir := b.spec.StateRoot() + "/.config"
	configPath := configDir + "/opencode.json"

	checkCmd := []string{"/bin/sh", "-lc", fmt.Sprintf("test -f %s", configPath)}
	if _, err := b.exec.Run(ctx, namespace, "agent", checkCmd, kubeexec.WithContainer(b.containerName)); err == nil {
		return nil
	}

	mkdirCmd := []string{"/bin/mkdir", "-p", configDir}
	if _, err := b.exec.Run(ctx, namespace, "agent", mkdirCmd, kubeexec.WithContainer(b.containerName)); err != nil {
		return fmt.Errorf("creating opencode config directory: %w", err)
	}

	configContent := `{
  "$schema": "https://opencode.ai/config.json",
  "permission": "allow"
}
`
	writeCmd := []string{"/bin/sh", "-lc", fmt.Sprintf("cat > %s << 'EOF'\n%sEOF", configPath, configContent)}
	if _, err := b.exec.Run(ctx, namespace, "agent", writeCmd, kubeexec.WithContainer(b.containerName)); err != nil {
		return fmt.Errorf("writing opencode config: %w", err)
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

	// We use the same logic as KubeManager.AttachAgent but specialized here.
	// In a real implementation, we'd look up the pod by labels.
	podName := "agent" // Simplified for now, should be looked up

	return b.exec.Attach(ctx, fmt.Sprintf("kagen-%s", b.repo.ID()), podName, b.commandArgs(), kubeexec.WithContainer(b.containerName))
}

func (b *baseAgent) commandArgs() []string {
	return []string{"/bin/sh", "-lc", b.spec.AttachShellForStatePath(b.statePath)}
}

func (b *baseAgent) ensureStatePath(ctx context.Context, namespace string) error {
	if b.statePath == "" {
		return nil
	}

	command := []string{"/bin/mkdir", "-p", b.statePath}
	if _, err := b.exec.Run(ctx, namespace, "agent", command, kubeexec.WithContainer(b.containerName)); err != nil {
		return fmt.Errorf("preparing agent state path %s: %w", b.statePath, err)
	}

	return nil
}

// NewClaudeAgent returns a real Claude agent.
func NewClaudeAgent(repo *git.Repository, kubeCtx, containerName, statePath string) Agent {
	spec, _ := SpecFor(Claude)
	return &baseAgent{
		agentType:     Claude,
		name:          "Claude",
		repo:          repo,
		kubeCtx:       kubeCtx,
		containerName: containerName,
		statePath:     statePath,
		spec:          spec,
		exec:          kubeexec.NewRunner(kubeCtx),
	}
}

// NewCodexAgent returns a real Codex agent.
func NewCodexAgent(repo *git.Repository, kubeCtx, containerName, statePath string) Agent {
	spec, _ := SpecFor(Codex)
	return &baseAgent{
		agentType:     Codex,
		name:          "Codex",
		repo:          repo,
		kubeCtx:       kubeCtx,
		containerName: containerName,
		statePath:     statePath,
		spec:          spec,
		exec:          kubeexec.NewRunner(kubeCtx),
	}
}

// NewOpenCodeAgent returns a real OpenCode agent.
func NewOpenCodeAgent(repo *git.Repository, kubeCtx, containerName, statePath string) Agent {
	spec, _ := SpecFor(OpenCode)
	return &baseAgent{
		agentType:     OpenCode,
		name:          "OpenCode",
		repo:          repo,
		kubeCtx:       kubeCtx,
		containerName: containerName,
		statePath:     statePath,
		spec:          spec,
		exec:          kubeexec.NewRunner(kubeCtx),
	}
}
