package cmd

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	goruntime "runtime"

	"github.com/spf13/cobra"

	"github.com/pejas/kagen/internal/cluster"
	"github.com/pejas/kagen/internal/config"
	kagerr "github.com/pejas/kagen/internal/errors"
	"github.com/pejas/kagen/internal/forgejo"
	"github.com/pejas/kagen/internal/git"
	"github.com/pejas/kagen/internal/runtime"
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

	// 1. Discover repo
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting working directory: %w", err)
	}
	repo, err := git.Discover(cwd)
	if err != nil {
		return fmt.Errorf("discovering repository: %w", err)
	}

	// 2. Load config
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	// 3. Setup runtime to get kube context
	rtm := runtime.NewColimaManager(cfg.Runtime)
	kubeCtx := rtm.KubeContext()

	// 4. Setup Forgejo service
	clientset, err := cluster.NewClientset(kubeCtx)
	if err != nil {
		return fmt.Errorf("creating kubernetes clientset: %w", err)
	}
	pf := cluster.NewPortForwarder()
	svc := forgejo.NewForgejoService(clientset, pf)

	// In Stage 4, we assume the environment is running if we're calling open.

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
