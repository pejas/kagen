// Package cmd defines the Cobra command tree for the kagen CLI.
package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/pejas/kagen/internal/agent"
	"github.com/pejas/kagen/internal/cluster"
	"github.com/pejas/kagen/internal/config"
	"github.com/pejas/kagen/internal/devfile"
	kagerr "github.com/pejas/kagen/internal/errors"
	"github.com/pejas/kagen/internal/forgejo"
	"github.com/pejas/kagen/internal/git"
	"github.com/pejas/kagen/internal/provenance"
	"github.com/pejas/kagen/internal/runtime"
	"github.com/pejas/kagen/internal/ui"
)

// Version information, set by ldflags at build time.
var (
	Version   = "dev"
	Commit    = "none"
	BuildDate = "unknown"
)

// agentFlag holds the --agent flag value.
var agentFlag string

// verboseFlag holds the --verbose flag value.
var verboseFlag bool

// rootCmd is the top-level kagen command.
var rootCmd = &cobra.Command{
	Use:   "kagen",
	Short: "Local, security-first agent runtime for Git repositories",
	Long: `kagen provides an isolated agent runtime for a single Git repository.
It runs on macOS ARM using Colima and K3s, keeps the host checkout canonical,
and uses an in-cluster Forgejo instance as the review and durability boundary
for agent work.`,
	SilenceUsage:  true,
	SilenceErrors: true,
	RunE:          runRoot,
}

func init() {
	rootCmd.PersistentFlags().StringVar(&agentFlag, "agent", "", "agent to use (claude, codex, opencode)")
	rootCmd.PersistentFlags().BoolVarP(&verboseFlag, "verbose", "v", false, "enable verbose output")

	// Bind persistent flags to Viper.
	_ = viper.BindPFlag("agent", rootCmd.PersistentFlags().Lookup("agent"))
	_ = viper.BindPFlag("verbose", rootCmd.PersistentFlags().Lookup("verbose"))

	// Register subcommands.
	rootCmd.AddCommand(initCmd)
	rootCmd.AddCommand(openCmd)
	rootCmd.AddCommand(pullCmd)
	rootCmd.AddCommand(versionCmd)
}

// Execute runs the root command. Called from main().
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		var exitErr *kagerr.ExitError
		if errors.As(err, &exitErr) {
			ui.Error("%v", exitErr.Err)
			os.Exit(exitErr.Code)
		}
		ui.Error("%v", err)
		os.Exit(1)
	}
}

// runRoot implements the default `kagen` flow: discover repo, ensure runtime,
// resolve agent, set up cluster resources, import to Forgejo, and attach.
func runRoot(cmd *cobra.Command, _ []string) error {
	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}

	// 1. Discover the current Git repository.
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting working directory: %w", err)
	}

	repo, err := git.Discover(cwd)
	if err != nil {
		if errors.Is(err, kagerr.ErrNotGitRepo) {
			return fmt.Errorf("%w: run kagen from within a git repository", kagerr.ErrNotGitRepo)
		}
		return fmt.Errorf("discovering repository: %w", err)
	}
	ui.Info("Repository: %s (branch: %s)", repo.Path, repo.CurrentBranch)

	// 2. Load configuration.
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading configuration: %w", err)
	}

	// 4. Ensure local runtime is healthy.
	ui.Info("Ensuring local runtime is healthy...")
	rtm := runtime.NewColimaManager(cfg.Runtime)
	if err := rtm.EnsureRunning(ctx); err != nil {
		return fmt.Errorf("runtime not available: %w", err)
	}
	kubeCtx := rtm.KubeContext()

	// 5. Resolve the agent type.
	agentType, err := resolveAgent(repo, kubeCtx, cfg)
	if err != nil {
		return err
	}
	ui.Info("Agent: %s", agentType)

	cm, err := cluster.NewKubeManager(kubeCtx)
	if err != nil {
		return fmt.Errorf("cluster not available (is the kagen Colima profile running?): %w", err)
	}

	// 5.1 Parse Devfile for resource generation.
	devfilePath := "devfile.yaml"
	if _, err := os.Stat(devfilePath); os.IsNotExist(err) {
		return fmt.Errorf("devfile.yaml not found: run 'kagen init' to bootstrap this repository")
	}

	d, err := devfile.Parse(devfilePath)
	if err != nil {
		return fmt.Errorf("parsing devfile: %w", err)
	}

	if err := cm.EnsureNamespace(ctx, repo); err != nil {
		return fmt.Errorf("ensuring namespace: %w", err)
	}

	if err := cm.EnsureResources(ctx, repo, d); err != nil {
		return fmt.Errorf("ensuring resources: %w", err)
	}

	// 6. Record import provenance.
	rec := provenance.RecordImport(repo)
	ui.Info("Import provenance: %s@%s (%s)", rec.SourceBranch, rec.SourceCommitSHA[:8], rec.ImportedAt.Format("2006-01-02T15:04:05Z"))

	// 7. Import repository to Forgejo.
	ui.Info("Importing repository to Forgejo...")
	clientset, err := cluster.NewClientset(kubeCtx)
	if err != nil {
		return fmt.Errorf("creating kubernetes clientset: %w", err)
	}
	pf := cluster.NewPortForwarder()
	fs := forgejo.NewForgejoService(clientset, pf)

	if err := fs.EnsureRepo(ctx, repo); err != nil {
		return fmt.Errorf("ensuring forgejo repo: %w", err)
	}

	if err := fs.ImportRepo(ctx, repo); err != nil {
		return fmt.Errorf("importing to forgejo: %w", err)
	}

	// 8. Launch and attach agent.
	ui.Info("Launching agent %s...", agentType)
	registry := agent.NewRegistry(repo, kubeCtx)
	a, err := registry.Get(agentType)
	if err != nil {
		return err
	}
	if err := a.Launch(ctx); err != nil {
		return fmt.Errorf("launching agent: %w", err)
	}

	return a.Attach(ctx)
}

// resolveAgent determines which agent to use from the flag, config, or
// interactive prompt.
func resolveAgent(repo *git.Repository, kubeCtx string, cfg *config.Config) (agent.Type, error) {
	// CLI flag takes precedence.
	source := agentFlag
	if source == "" {
		source = cfg.Agent
	}

	if source != "" {
		return agent.TypeFromString(source)
	}

	// Interactive prompt.
	registry := agent.NewRegistry(repo, kubeCtx)
	names := registry.AvailableNames()
	selected, err := ui.Prompt("Select an agent:", names)
	if err != nil {
		return "", fmt.Errorf("agent selection: %w", err)
	}

	// Map display name back to type.
	for _, t := range registry.Available() {
		if string(t) == strings.ToLower(selected) {
			return t, nil
		}
	}

	return "", fmt.Errorf("%w: %q", kagerr.ErrAgentUnknown, selected)
}
