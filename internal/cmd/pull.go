package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/pejas/kagen/internal/cluster"
	"github.com/pejas/kagen/internal/git"
	"github.com/pejas/kagen/internal/ui"
)

var pullCmd = &cobra.Command{
	Use:   "pull",
	Short: "Pull reviewed changes from Forgejo into the current branch",
	Long: `Fetches and merges the reviewed kagen/<current-branch> from the
in-cluster Forgejo instance back into the current local branch.

This is the explicit bridge from reviewed in-cluster work back to the
host repository.`,
	RunE: runPull,
}

func runPull(cmd *cobra.Command, _ []string) error {
	ctx := cmd.Context() // Changed to assign to ctx

	// 1. Discover repo
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting working directory: %w", err)
	}
	repo, err := git.Discover(cwd)
	if err != nil {
		return fmt.Errorf("discovering repository: %w", err)
	}

	// 2. Clear state/WIP protection
	if repo.HasUncommittedChanges() {
		ui.Warn("You have uncommitted changes.")
		ui.Info("Creating a WIP commit to protect your work...")
		if err := repo.Commit("kagen: WIP local changes before pull"); err != nil {
			return fmt.Errorf("creating WIP commit: %w", err)
		}
	}

	// 4. Setup Forgejo service
	pf := cluster.NewPortForwarder()

	// 5. Start port-forward to Forgejo to resolve remote URL
	ui.Info("Connecting to in-cluster Forgejo...")
	localPort, err := pf.Start(ctx, fmt.Sprintf("kagen-%s", repo.ID()), "svc/forgejo", 3000)
	if err != nil {
		return fmt.Errorf("starting port-forward: %w", err)
	}
	defer pf.Stop()

	remoteUrl := fmt.Sprintf("http://kagen:kagen-internal-secret@127.0.0.1:%d/kagen/workspace.git", localPort)
	if err := repo.AddRemote("kagen", remoteUrl); err != nil {
		return err
	}

	// 6. Fetch and Merge
	ui.Info("Fetching changes from %s...", repo.KagenBranch())
	if err := repo.Fetch(ctx, "kagen"); err != nil {
		return fmt.Errorf("fetching from forgejo: %w", err)
	}

	ui.Info("Fast-forwarding %s from %s...", repo.CurrentBranch, repo.KagenBranch())
	if err := repo.MergeFFOnly(ctx, repo.KagenBranch()); err != nil {
		return fmt.Errorf("fast-forwarding changes: %w", err)
	}

	ui.Success("Successfully fast-forwarded reviewed changes.")
	return nil
}
