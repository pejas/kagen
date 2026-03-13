package cmd

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	goruntime "runtime"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/pejas/kagen/internal/git"
	"github.com/pejas/kagen/internal/workflow"
)

type reviewSession = workflow.ReviewSession

var openCmd = &cobra.Command{
	Use:   "open",
	Short: "Open the live Forgejo review page for the current branch",
	Long: `Opens the Forgejo web interface in your browser for the current
repository's review branch and keeps the local review tunnel open until you
interrupt the command.`,
	RunE: runOpen,
}

func runOpen(cmd *cobra.Command, _ []string) error {
	return workflow.NewOpenWorkflow(workflow.OpenDependencies{
		DiscoverRepository: discoverRepository,
		LoadConfig:         loadRunConfig,
		EnsureRuntime:      ensureRuntime,
		NewForgejoService:  func(kubeCtx string) (workflow.ReviewService, error) { return newForgejoService(kubeCtx) },
		OpenBrowser:        openBrowser,
		WaitForInterrupt:   waitForReviewInterrupt,
	}).Run(cmd.Context())
}

func openReview(
	ctx context.Context,
	repo *git.Repository,
	startSession func(context.Context, *git.Repository) (workflow.ReviewSession, error),
	openBrowserFn func(string) error,
	waitFn func(context.Context) error,
) error {
	return workflow.OpenReview(ctx, repo, startSession, openBrowserFn, waitFn)
}

func waitForReviewInterrupt(ctx context.Context) error {
	signalCtx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()

	<-signalCtx.Done()
	if ctx.Err() != nil {
		return ctx.Err()
	}

	return nil
}

// openBrowser opens the given URL in the default browser.
func openBrowser(url string) error {
	var cmd *exec.Cmd

	switch goruntime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "linux":
		cmd = exec.Command("xdg-open", url)
	default:
		return fmt.Errorf("unsupported platform: %s", goruntime.GOOS)
	}

	return cmd.Start()
}
