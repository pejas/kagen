package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/pejas/kagen/internal/cluster"
	"github.com/pejas/kagen/internal/config"
	"github.com/pejas/kagen/internal/diagnostics"
	kagerr "github.com/pejas/kagen/internal/errors"
	"github.com/pejas/kagen/internal/git"
	"github.com/pejas/kagen/internal/runtime"
	"github.com/pejas/kagen/internal/session"
	"github.com/pejas/kagen/internal/workflow"
)

type doctorSessionStore interface {
	Close() error
	GetSummary(context.Context, int64) (session.Summary, bool, error)
	List(context.Context, session.ListOptions) ([]session.Summary, error)
}

var (
	openSessionStoreForDoctor = func() (doctorSessionStore, error) {
		return session.OpenDefault()
	}
	discoverRepositoryForDoctor = func() (*git.Repository, error) {
		cwd, err := os.Getwd()
		if err != nil {
			return nil, fmt.Errorf("getting working directory: %w", err)
		}

		repo, err := git.Discover(cwd)
		if err != nil {
			if errors.Is(err, kagerr.ErrNotGitRepo) {
				return nil, fmt.Errorf("%w: run kagen from within a git repository or pass --session", kagerr.ErrNotGitRepo)
			}
			return nil, fmt.Errorf("discovering repository: %w", err)
		}

		return repo, nil
	}
	loadRunConfigForDoctor       = loadRunConfig
	loadLatestOperationForDoctor = func(sessionID int64) (diagnostics.Operation, bool, error) {
		return diagnostics.NewLatestOperationStore().LoadLatestOperation(sessionID)
	}
	findFailureArtefactDirForDoctor = func(sessionID int64) (string, bool, error) {
		return diagnostics.NewLatestOperationStore().FailureArtefactDirectory(sessionID)
	}
	runtimeStatusForDoctor = func(ctx context.Context, cfg *config.Config) (runtime.Status, string, error) {
		manager := runtime.NewColimaManager(cfg.Runtime)
		status, err := manager.Status(ctx)
		return status, manager.KubeContext(), err
	}
	newDiagnosticsInspectorForDoctor = func(kubeCtx string) (workflow.DoctorClusterInspector, error) {
		return cluster.NewDiagnosticsInspector(kubeCtx)
	}
)

func newDoctorCommand() *cobra.Command {
	var sessionID int64

	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Summarise persisted session diagnostics",
		Long: `Summarises the latest persisted diagnostics for a kagen session.

Without --session, the most recently used session for the current repository
is selected.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runDoctor(cmd.Context(), sessionID, cmd.Flags().Changed("session"))
		},
	}

	cmd.Flags().Int64Var(&sessionID, "session", 0, "persisted session ID to inspect")

	return cmd
}

func runDoctor(ctx context.Context, sessionID int64, sessionSelected bool) error {
	return workflow.NewDoctorWorkflow(workflow.DoctorDependencies{
		LoadConfig:              loadRunConfigForDoctor,
		DiscoverRepository:      discoverRepositoryForDoctor,
		OpenSessionStore:        func() (workflow.DoctorSessionStore, error) { return openSessionStoreForDoctor() },
		LoadLatestOperation:     loadLatestOperationForDoctor,
		FindFailureArtefactDir:  findFailureArtefactDirForDoctor,
		RuntimeStatus:           runtimeStatusForDoctor,
		NewDiagnosticsInspector: newDiagnosticsInspectorForDoctor,
	}).Run(ctx, sessionID, sessionSelected)
}
