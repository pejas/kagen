package cmd

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/pejas/kagen/internal/config"
	"github.com/pejas/kagen/internal/runtime"
	"github.com/pejas/kagen/internal/ui"
)

type runtimeStopper interface {
	Stop(ctx context.Context) error
}

var (
	loadRunConfigForDown = loadRunConfig
	newRuntimeManager    = func(cfg config.RuntimeConfig) runtimeStopper {
		return runtime.NewColimaManager(cfg)
	}
)

func newDownCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "down",
		Short: "Shut down the local kagen runtime environment",
		Long: `Shuts down the whole local kagen runtime environment by stopping the
underlying Colima VM and K3s cluster.

This does not delete persisted kagen sessions or agent sessions. Leaving an
agent TUI with /exit or /quit only detaches from that tool; use 'kagen down'
when you want to stop the runtime itself.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runDown(cmd.Context())
		},
	}
}

func runDown(ctx context.Context) error {
	ctx = rootContext(ctx)

	cfg, err := loadRunConfigForDown()
	if err != nil {
		return err
	}

	ui.Info("Shutting down the local kagen runtime environment...")
	if err := newRuntimeManager(cfg.Runtime).Stop(ctx); err != nil {
		return fmt.Errorf("shutting down runtime: %w", err)
	}

	ui.Success("Stopped the local kagen runtime environment. Persisted sessions remain available in 'kagen list'.")
	return nil
}
