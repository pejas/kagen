package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/pejas/kagen/internal/agent"
	"github.com/pejas/kagen/internal/cluster"
	"github.com/pejas/kagen/internal/config"
	kagerr "github.com/pejas/kagen/internal/errors"
	"github.com/pejas/kagen/internal/forgejo"
	"github.com/pejas/kagen/internal/git"
	"github.com/pejas/kagen/internal/kubeexec"
	"github.com/pejas/kagen/internal/provenance"
	"github.com/pejas/kagen/internal/proxy"
	"github.com/pejas/kagen/internal/runtime"
	"github.com/pejas/kagen/internal/session"
	"github.com/pejas/kagen/internal/ui"
	"github.com/pejas/kagen/internal/workload"
	corev1 "k8s.io/api/core/v1"
)

const (
	runtimePodName = "agent"
)

func rootContext(ctx context.Context) context.Context {
	if ctx != nil {
		return ctx
	}

	return context.Background()
}

func discoverRepository() (*git.Repository, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("getting working directory: %w", err)
	}

	ui.Verbose("Discovering repository from %s", cwd)

	repo, err := git.Discover(cwd)
	if err != nil {
		if errors.Is(err, kagerr.ErrNotGitRepo) {
			return nil, fmt.Errorf("%w: run kagen from within a git repository", kagerr.ErrNotGitRepo)
		}
		return nil, fmt.Errorf("discovering repository: %w", err)
	}

	ui.Info("Repository: %s (branch: %s)", repo.Path, repo.CurrentBranch)
	ui.Verbose("Resolved repo id=%s head=%s", repo.ID(), repo.HeadSHA)
	return repo, nil
}

func loadRunConfig() (*config.Config, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, fmt.Errorf("loading configuration: %w", err)
	}

	cfg.Verbose = cfg.Verbose || verboseFlag
	ui.SetVerbose(cfg.Verbose)
	ui.Verbose(
		"Loaded config: default agent=%q forgejo_http_port=%d startup_timeout=%s",
		cfg.Agent,
		cfg.ForgejoHTTPPort,
		cfg.Runtime.StartupTimeout,
	)

	return cfg, nil
}

func ensureRuntime(ctx context.Context, cfg *config.Config) (string, error) {
	ui.Info("Ensuring local runtime is healthy...")
	manager := runtime.NewColimaManager(cfg.Runtime)
	if err := manager.EnsureRunning(ctx); err != nil {
		return "", fmt.Errorf("runtime not available: %w", err)
	}

	ui.Verbose("Runtime healthy with kube context %s", manager.KubeContext())
	return manager.KubeContext(), nil
}

func newForgejoService(kubeCtx string) (*forgejo.ForgejoService, error) {
	clientset, err := cluster.NewClientset(kubeCtx)
	if err != nil {
		return nil, fmt.Errorf("creating kubernetes clientset: %w", err)
	}

	return forgejo.NewForgejoService(
		clientset,
		cluster.NewPortForwarder(),
		kubeexec.NewRunner(kubeCtx),
	), nil
}

func ensureForgejoImport(ctx context.Context, svc *forgejo.ForgejoService, repo *git.Repository) error {
	rec := provenance.RecordImport(repo)
	ui.Info("Import provenance: %s@%s (%s)", rec.SourceBranch, rec.SourceCommitSHA[:8], rec.ImportedAt.Format("2006-01-02T15:04:05Z"))

	ui.Info("Importing repository to Forgejo...")
	ui.Verbose("Ensuring Forgejo repo boundary for namespace %s", fmt.Sprintf("kagen-%s", repo.ID()))
	if err := svc.EnsureRepo(ctx, repo); err != nil {
		return fmt.Errorf("ensuring forgejo repo: %w", err)
	}
	if err := svc.ImportRepo(ctx, repo); err != nil {
		return fmt.Errorf("importing to forgejo: %w", err)
	}

	ui.Verbose("Forgejo import finished for %s", repo.KagenBranch())
	return nil
}

func ensureClusterResources(
	ctx context.Context,
	kubeCtx string,
	repo *git.Repository,
	cfg *config.Config,
	agentType agent.Type,
) error {
	ui.Info("Ensuring cluster resources for %s/%s...", agentType, repo.CurrentBranch)
	ui.Verbose("Reconciling namespace kagen-%s", repo.ID())
	manager, err := cluster.NewKubeManager(kubeCtx)
	if err != nil {
		return fmt.Errorf("cluster not available (is the kagen Colima profile running?): %w", err)
	}
	if err := manager.EnsureNamespace(ctx, repo); err != nil {
		return fmt.Errorf("ensuring namespace: %w", err)
	}
	policy := proxy.LoadPolicy(cfg, string(agentType))
	if err := manager.EnsureProxy(ctx, repo, policy); err != nil {
		return fmt.Errorf("ensuring proxy: %w", err)
	}
	if len(policy.AllowedDestinations) == 0 {
		ui.Verbose("Proxy policy disabled for %s", agentType)
	} else {
		ui.Verbose("Proxy policy requires %d allowed destination(s)", len(policy.AllowedDestinations))
	}
	pod, err := buildRuntimePod(repo, cfg, agentType)
	if err != nil {
		return err
	}
	containerNames := make([]string, 0, len(pod.Spec.Containers))
	for _, container := range pod.Spec.Containers {
		containerNames = append(containerNames, container.Name)
	}
	ui.Verbose("Built runtime pod %s/%s with containers: %s", pod.Namespace, pod.Name, strings.Join(containerNames, ", "))
	if err := manager.EnsureResources(ctx, repo, string(agentType), pod, policy); err != nil {
		return fmt.Errorf("ensuring resources: %w", err)
	}

	return nil
}

func validateProxyPolicy(ctx context.Context, kubeCtx string, repo *git.Repository, cfg *config.Config, agentType agent.Type) error {
	policy := proxy.LoadPolicy(cfg, string(agentType))
	if len(policy.AllowedDestinations) == 0 {
		ui.Verbose("Skipping proxy readiness validation: no allowlist configured for %s", agentType)
		return nil
	}

	manager, err := cluster.NewKubeManager(kubeCtx)
	if err != nil {
		return fmt.Errorf("creating cluster manager for proxy validation: %w", err)
	}

	policy.Enforced, err = manager.ProxyReady(ctx, repo)
	if err != nil {
		return fmt.Errorf("checking proxy readiness: %w", err)
	}
	ui.Verbose("Proxy readiness for kagen-%s: enforced=%t", repo.ID(), policy.Enforced)
	if err := policy.Validate(); err != nil {
		return fmt.Errorf("validating proxy policy: %w", err)
	}

	return nil
}

func launchAgentRuntime(ctx context.Context, repo *git.Repository, kubeCtx string, agentType agent.Type) error {
	spec, err := agent.SpecFor(agentType)
	if err != nil {
		return err
	}

	ui.Info("Launching agent %s...", agentType)
	registry := agent.NewRegistry(repo, kubeCtx).WithContainer(spec.ContainerName())
	ui.Verbose("Launching container %s in namespace kagen-%s", spec.ContainerName(), repo.ID())
	a, err := registry.Get(agentType)
	if err != nil {
		return err
	}
	if err := a.Launch(ctx); err != nil {
		return fmt.Errorf("launching agent: %w", err)
	}

	return nil
}

func attachAgent(ctx context.Context, repo *git.Repository, kubeCtx string, agentType agent.Type, agentSession session.AgentSession) error {
	spec, err := agent.SpecFor(agentType)
	if err != nil {
		return err
	}

	registry := agent.NewRegistry(repo, kubeCtx).
		WithContainer(spec.ContainerName()).
		WithStatePath(agentSession.StatePath)
	a, err := registry.Get(agentType)
	if err != nil {
		return err
	}

	return a.Attach(ctx)
}

func buildRuntimePod(repo *git.Repository, _ *config.Config, agentType agent.Type) (*corev1.Pod, error) {
	namespace := fmt.Sprintf("kagen-%s", repo.ID())

	spec, err := agent.SpecFor(agentType)
	if err != nil {
		return nil, err
	}

	builder := workload.NewBuilder()
	pod, err := builder.BuildPod(workload.Request{
		Name:      runtimePodName,
		Namespace: namespace,
		Runtime:   spec,
		Images:    workload.DefaultImages(),
	})
	if err != nil {
		return nil, fmt.Errorf("building workload pod: %w", err)
	}

	return pod, nil
}
