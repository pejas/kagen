package workflow

import (
	"context"
	"fmt"

	"github.com/pejas/kagen/internal/config"
	"github.com/pejas/kagen/internal/forgejo"
	"github.com/pejas/kagen/internal/git"
	"github.com/pejas/kagen/internal/ui"
)

type OpenDependencies struct {
	DiscoverRepository func() (*git.Repository, error)
	LoadConfig         func() (*config.Config, error)
	EnsureRuntime      func(context.Context, *config.Config) (string, error)
	NewForgejoService  func(string) (ReviewService, error)
	OpenBrowser        func(string) error
	WaitForInterrupt   func(context.Context) error
}

type ReviewService interface {
	StartReviewSession(ctx context.Context, repo *git.Repository) (*forgejo.ReviewSession, error)
}

type OpenWorkflow struct {
	deps OpenDependencies
}

func NewOpenWorkflow(deps OpenDependencies) *OpenWorkflow {
	return &OpenWorkflow{deps: deps}
}

func (w *OpenWorkflow) Run(ctx context.Context) error {
	ctx = contextOrBackground(ctx)

	cfg, err := w.deps.LoadConfig()
	if err != nil {
		return err
	}

	repo, err := w.deps.DiscoverRepository()
	if err != nil {
		return err
	}

	kubeCtx, err := w.deps.EnsureRuntime(ctx, cfg)
	if err != nil {
		return err
	}

	svc, err := w.deps.NewForgejoService(kubeCtx)
	if err != nil {
		return err
	}

	return OpenReview(ctx, repo, func(ctx context.Context, repo *git.Repository) (ReviewSession, error) {
		return svc.StartReviewSession(ctx, repo)
	}, w.deps.OpenBrowser, w.deps.WaitForInterrupt)
}

func OpenReview(
	ctx context.Context,
	repo *git.Repository,
	startSession func(context.Context, *git.Repository) (ReviewSession, error),
	openBrowser func(string) error,
	waitForInterrupt func(context.Context) error,
) error {
	session, err := startSession(ctx, repo)
	if err != nil {
		return fmt.Errorf("starting review session: %w", err)
	}
	defer func() {
		_ = session.Stop()
	}()

	hasCommits, err := session.HasNewCommits(ctx, repo)
	if err != nil {
		return fmt.Errorf("checking reviewable changes: %w", err)
	}
	ui.Verbose("Reviewability for %s: has_new_commits=%t", repo.KagenBranch(), hasCommits)
	if !hasCommits {
		ui.Info("No reviewable changes found for %s", repo.KagenBranch())
		return nil
	}

	reviewURL := session.ReviewURL(repo)
	ui.Info("Opening review page: %s", reviewURL)
	if err := openBrowser(reviewURL); err != nil {
		return fmt.Errorf("opening browser: %w", err)
	}

	ui.Info("Review tunnel active. Press Ctrl+C when you are done reviewing.")
	waitErrCh := make(chan error, 1)
	go func() {
		waitErrCh <- waitForInterrupt(ctx)
	}()

	select {
	case err := <-waitErrCh:
		if err != nil {
			return fmt.Errorf("waiting for review session shutdown: %w", err)
		}
		return nil
	case <-session.Done():
		if err := session.Wait(); err != nil {
			return fmt.Errorf("review transport terminated: %w", err)
		}
		return fmt.Errorf("review transport terminated before shutdown")
	}
}
