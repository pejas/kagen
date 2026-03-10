package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

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
	_ = cmd.Context()

	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting working directory: %w", err)
	}

	repo, err := git.Discover(cwd)
	if err != nil {
		return fmt.Errorf("discovering repository: %w", err)
	}

	ui.Header("kagen pull")
	ui.Info("Repository:   %s", repo.Path)
	ui.Info("Local branch: %s", repo.CurrentBranch)
	ui.Info("Kagen branch: %s", repo.KagenBranch())

	// TODO: Implement the actual pull workflow:
	// 1. Fetch kagen/<branch> from Forgejo remote.
	// 2. Merge or rebase into the current local branch.
	// 3. Report the result.
	ui.Warn("Pull workflow not yet implemented")
	ui.Info("Once implemented, this will fetch and merge %s into %s",
		repo.KagenBranch(), repo.CurrentBranch)

	return nil
}
