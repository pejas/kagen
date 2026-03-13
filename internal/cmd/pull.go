package cmd

import (
	"github.com/spf13/cobra"

	"github.com/pejas/kagen/internal/git"
	"github.com/pejas/kagen/internal/workflow"
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
	return workflow.NewPullWorkflow(workflow.PullDependencies{
		DiscoverRepository: discoverRepository,
		LoadConfig:         loadRunConfig,
		EnsureRuntime:      ensureRuntime,
		NewForgejoService:  func(kubeCtx string) (workflow.PullService, error) { return newForgejoService(kubeCtx) },
	}).Run(cmd.Context())
}

func validatePullRefs(repo *git.Repository, reviewRef, baseRef, localBaseSHA string) error {
	return workflow.ValidatePullRefs(repo, reviewRef, baseRef, localBaseSHA)
}
