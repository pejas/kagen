// Package cmd defines the Cobra command tree for the kagen CLI.
package cmd

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/pejas/kagen/internal/agent"
	"github.com/pejas/kagen/internal/config"
	kagerr "github.com/pejas/kagen/internal/errors"
	"github.com/pejas/kagen/internal/git"
	"github.com/pejas/kagen/internal/proxy"
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
	ctx := rootContext(cmd.Context())

	cfg, err := loadRunConfig()
	if err != nil {
		return err
	}

	repo, err := discoverRepository()
	if err != nil {
		return err
	}

	kubeCtx, err := ensureRuntime(ctx, cfg)
	if err != nil {
		return err
	}

	if err := ensureProjectDevfileExists(); err != nil {
		return err
	}

	agentType, err := resolveAgent(repo, kubeCtx, cfg)
	if err != nil {
		return err
	}
	ui.Info("Agent: %s", agentType)
	policy := proxy.LoadPolicy(cfg, string(agentType))
	if verboseFlag && len(policy.AllowedDestinations) > 0 {
		ui.Info("Required egress hosts: %s", strings.Join(policy.AllowedDestinations, ", "))
	}

	d, err := loadProjectDevfile(agentType)
	if err != nil {
		return err
	}

	forgejoService, err := newForgejoService(kubeCtx)
	if err != nil {
		return err
	}
	if err := ensureClusterResources(ctx, kubeCtx, repo, cfg, agentType, d); err != nil {
		return err
	}
	if err := ensureForgejoImport(ctx, forgejoService, repo); err != nil {
		return err
	}
	if err := validateProxyPolicy(ctx, kubeCtx, repo, cfg, agentType); err != nil {
		return err
	}

	return launchAgent(ctx, repo, kubeCtx, agentType)
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
