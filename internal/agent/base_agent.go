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
	repo          *git.Repository
	kubeCtx       string
	containerName string
	statePath     string
	spec          RuntimeSpec
	exec          kubeexec.Runner
}

func newBaseAgent(spec RuntimeSpec, deps AgentDependencies) Agent {
	containerName := deps.ContainerName
	if containerName == "" {
		containerName = ContainerName(spec)
	}

	return &baseAgent{
		repo:          deps.Repo,
		kubeCtx:       deps.KubeCtx,
		containerName: containerName,
		statePath:     deps.StatePath,
		spec:          spec,
		exec:          kubeexec.NewRunner(deps.KubeCtx),
	}
}

func registerBaseAgent(spec RuntimeSpec) {
	RegisterFactory(spec, func(deps AgentDependencies) Agent {
		return newBaseAgent(spec, deps)
	})
}

func (b *baseAgent) Name() string {
	return b.spec.DisplayName()
}

func (b *baseAgent) AgentType() Type {
	return b.spec.Type()
}

// Authenticate is a no-op by default, overridden by agents that need it.
func (b *baseAgent) Authenticate(context.Context) error {
	return nil
}

// Launch ensures the agent process is prepared. For most, this is a no-op
// as the pod generation already includes the container.
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

	return b.spec.Configure(ctx, ns, b.containerName, b.exec)
}

// Attach connects the user's terminal to the agent process in the cluster.
func (b *baseAgent) Attach(ctx context.Context) error {
	if os.Getenv("KAGEN_NON_INTERACTIVE") == "true" {
		return nil
	}
	if b.exec == nil {
		return nil
	}

	podName := "agent"
	return b.exec.Attach(ctx, fmt.Sprintf("kagen-%s", b.repo.ID()), podName, b.commandArgs(), kubeexec.WithContainer(b.containerName))
}

func (b *baseAgent) commandArgs() []string {
	return []string{"/bin/sh", "-lc", AttachShellForStatePath(b.spec, b.statePath)}
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
