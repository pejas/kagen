package workflow

import (
	"context"
	"fmt"

	"github.com/pejas/kagen/internal/config"
	kagerr "github.com/pejas/kagen/internal/errors"
	"github.com/pejas/kagen/internal/git"
	"github.com/pejas/kagen/internal/ui"
)

type PullDependencies struct {
	DiscoverRepository func() (*git.Repository, error)
	LoadConfig         func() (*config.Config, error)
	EnsureRuntime      func(context.Context, *config.Config) (string, error)
	NewForgejoService  func(string) (PullService, error)
}

type PullService interface {
	FetchReviewRefs(ctx context.Context, repo *git.Repository) error
}

type PullWorkflow struct {
	deps PullDependencies
}

func NewPullWorkflow(deps PullDependencies) *PullWorkflow {
	return &PullWorkflow{deps: deps}
}

func (w *PullWorkflow) Run(ctx context.Context) error {
	ctx = contextOrBackground(ctx)

	cfg, err := w.deps.LoadConfig()
	if err != nil {
		return err
	}

	repo, err := w.deps.DiscoverRepository()
	if err != nil {
		return err
	}
	localBaseSHA := repo.HeadSHA

	if repo.HasUncommittedChanges() {
		ui.Warn("You have uncommitted changes.")
		ui.Info("Creating a WIP commit to protect your work...")
		if err := repo.Commit("kagen: WIP local changes before pull"); err != nil {
			return fmt.Errorf("creating WIP commit: %w", err)
		}
		repo, err = git.Discover(repo.Path)
		if err != nil {
			return fmt.Errorf("refreshing repository after WIP commit: %w", err)
		}
		ui.Verbose("Repository refreshed after WIP commit; new HEAD=%s", repo.HeadSHA)
	}

	kubeCtx, err := w.deps.EnsureRuntime(ctx, cfg)
	if err != nil {
		return err
	}
	svc, err := w.deps.NewForgejoService(kubeCtx)
	if err != nil {
		return err
	}

	ui.Info("Fetching changes from %s...", repo.KagenBranch())
	if err := svc.FetchReviewRefs(ctx, repo); err != nil {
		return fmt.Errorf("fetching from forgejo: %w", err)
	}

	mergeRef := repo.KagenRemoteTrackingBranch("kagen")
	baseRef := repo.RemoteTrackingBranch("kagen")
	ui.Verbose("Validating review ref %s against canonical ref %s", mergeRef, baseRef)
	if err := ValidatePullRefs(repo, mergeRef, baseRef, localBaseSHA); err != nil {
		return err
	}

	ui.Info("Fast-forwarding %s from %s...", repo.CurrentBranch, repo.KagenBranch())
	if err := repo.MergeFFOnly(ctx, mergeRef); err != nil {
		return fmt.Errorf("fast-forwarding changes: %w", err)
	}

	ui.Success("Successfully fast-forwarded reviewed changes.")
	return nil
}

func ValidatePullRefs(repo *git.Repository, reviewRef, baseRef, localBaseSHA string) error {
	if !repo.HasRef(reviewRef) {
		if repo.HasRef(baseRef) {
			return fmt.Errorf(
				"%w: expected reviewed changes on %s but only found %s; agent work may have been pushed to the canonical branch",
				kagerr.ErrNoReviewableChanges,
				reviewRef,
				baseRef,
			)
		}

		return fmt.Errorf("%w: remote branch %s not found", kagerr.ErrNoReviewableChanges, reviewRef)
	}

	if !repo.HasRef(baseRef) {
		return nil
	}

	reviewSHA, err := repo.ResolveRef(reviewRef)
	if err != nil {
		return fmt.Errorf("resolving review ref %s: %w", reviewRef, err)
	}
	baseSHA, err := repo.ResolveRef(baseRef)
	if err != nil {
		return fmt.Errorf("resolving base ref %s: %w", baseRef, err)
	}
	if localBaseSHA == "" {
		localBaseSHA, err = repo.ResolveRef("HEAD")
		if err != nil {
			return fmt.Errorf("resolving HEAD: %w", err)
		}
	}

	if baseSHA != reviewSHA && baseSHA != localBaseSHA {
		return fmt.Errorf(
			"unexpected remote branch state: %s advanced independently of %s; refusing to merge ambiguous review state",
			baseRef,
			reviewRef,
		)
	}

	return nil
}
