package cmd

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	goruntime "runtime"

	"github.com/spf13/cobra"

	kagerr "github.com/pejas/kagen/internal/errors"
	"github.com/pejas/kagen/internal/forgejo"
	"github.com/pejas/kagen/internal/git"
	"github.com/pejas/kagen/internal/ui"
)

var openCmd = &cobra.Command{
	Use:   "open",
	Short: "Open the Forgejo review page for the current branch",
	Long: `Opens the Forgejo web interface in your browser for the current
repository's kagen branch. If no reviewable changes are found, a clear
message is displayed instead of opening a dead page.`,
	RunE: runOpen,
}

func runOpen(cmd *cobra.Command, _ []string) error {
	ctx := cmd.Context()

	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting working directory: %w", err)
	}

	repo, err := git.Discover(cwd)
	if err != nil {
		return fmt.Errorf("discovering repository: %w", err)
	}

	svc := forgejo.NewStubService()

	// Check for reviewable changes first.
	hasCommits, err := svc.HasNewCommits(ctx, repo)
	if err != nil {
		if errors.Is(err, kagerr.ErrNotImplemented) {
			ui.Warn("Forgejo integration not yet implemented")
			ui.Info("Would open review page for branch: %s", repo.KagenBranch())
			return nil
		}
		return fmt.Errorf("checking for new commits: %w", err)
	}

	if !hasCommits {
		ui.Info("No reviewable changes found for %s", repo.KagenBranch())
		return nil
	}

	reviewURL, err := svc.GetReviewURL(repo)
	if err != nil {
		return fmt.Errorf("getting review URL: %w", err)
	}

	ui.Info("Opening review page: %s", reviewURL)
	return openBrowser(reviewURL)
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
