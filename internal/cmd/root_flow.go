package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/pejas/kagen/internal/agent"
	"github.com/pejas/kagen/internal/cluster"
	"github.com/pejas/kagen/internal/config"
	"github.com/pejas/kagen/internal/devfile"
	kagerr "github.com/pejas/kagen/internal/errors"
	"github.com/pejas/kagen/internal/forgejo"
	"github.com/pejas/kagen/internal/git"
	"github.com/pejas/kagen/internal/kubeexec"
	"github.com/pejas/kagen/internal/provenance"
	"github.com/pejas/kagen/internal/proxy"
	"github.com/pejas/kagen/internal/runtime"
	"github.com/pejas/kagen/internal/ui"
)

const devfilePath = "devfile.yaml"

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

	repo, err := git.Discover(cwd)
	if err != nil {
		if errors.Is(err, kagerr.ErrNotGitRepo) {
			return nil, fmt.Errorf("%w: run kagen from within a git repository", kagerr.ErrNotGitRepo)
		}
		return nil, fmt.Errorf("discovering repository: %w", err)
	}

	ui.Info("Repository: %s (branch: %s)", repo.Path, repo.CurrentBranch)
	return repo, nil
}

func loadRunConfig() (*config.Config, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, fmt.Errorf("loading configuration: %w", err)
	}

	return cfg, nil
}

func loadProjectDevfile(agentType agent.Type) (*devfile.Devfile, error) {
	if err := ensureProjectDevfileExists(); err != nil {
		return nil, err
	}

	d, err := devfile.Parse(devfilePath)
	if err != nil {
		return nil, fmt.Errorf("parsing devfile: %w", err)
	}
	spec, err := agent.SpecFor(agentType)
	if err != nil {
		return nil, err
	}
	if _, err := devfile.EnsureRuntimeComponent(d, spec); err != nil {
		return nil, fmt.Errorf("ensuring runtime component: %w", err)
	}

	return d, nil
}

func ensureProjectDevfileExists() error {
	if _, err := os.Stat(devfilePath); os.IsNotExist(err) {
		return fmt.Errorf("devfile.yaml not found: run 'kagen init' to bootstrap this repository")
	}

	return nil
}

func ensureRuntime(ctx context.Context, cfg *config.Config) (string, error) {
	ui.Info("Ensuring local runtime is healthy...")
	manager := runtime.NewColimaManager(cfg.Runtime)
	if err := manager.EnsureRunning(ctx); err != nil {
		return "", fmt.Errorf("runtime not available: %w", err)
	}

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
	if err := svc.EnsureRepo(ctx, repo); err != nil {
		return fmt.Errorf("ensuring forgejo repo: %w", err)
	}
	if err := svc.ImportRepo(ctx, repo); err != nil {
		return fmt.Errorf("importing to forgejo: %w", err)
	}

	return nil
}

func ensureClusterResources(
	ctx context.Context,
	kubeCtx string,
	repo *git.Repository,
	cfg *config.Config,
	agentType agent.Type,
	d *devfile.Devfile,
) error {
	ui.Info("Ensuring cluster resources for %s/%s...", agentType, repo.CurrentBranch)
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
	if err := manager.EnsureResources(ctx, repo, string(agentType), d, policy); err != nil {
		return fmt.Errorf("ensuring resources: %w", err)
	}

	return nil
}

func validateProxyPolicy(ctx context.Context, kubeCtx string, repo *git.Repository, cfg *config.Config, agentType agent.Type) error {
	policy := proxy.LoadPolicy(cfg, string(agentType))
	if len(policy.AllowedDestinations) == 0 {
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
	if err := policy.Validate(); err != nil {
		return fmt.Errorf("validating proxy policy: %w", err)
	}

	return nil
}

func launchAgent(ctx context.Context, repo *git.Repository, kubeCtx string, agentType agent.Type) error {
	spec, err := agent.SpecFor(agentType)
	if err != nil {
		return err
	}

	ui.Info("Launching agent %s...", agentType)
	registry := agent.NewRegistry(repo, kubeCtx).WithContainer(spec.ContainerName())
	a, err := registry.Get(agentType)
	if err != nil {
		return err
	}
	if err := a.Launch(ctx); err != nil {
		return fmt.Errorf("launching agent: %w", err)
	}

	return a.Attach(ctx)
}
