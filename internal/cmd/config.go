package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/pejas/kagen/internal/agent"
	"github.com/pejas/kagen/internal/ui"
)

// defaultKagenConfigTemplate is the default optional project-level kagen config.
const defaultKagenConfigTemplate = `# Optional kagen project configuration
# This file is not required before 'kagen start' or 'kagen attach'.
# See https://github.com/pejas/kagen for documentation.

agent: %s

# Additional egress destinations beyond required runtime and provider hosts.
# agent_providers:
#   opencode:
#     - anthropic
# proxy_allowlist:
#   - registry.npmjs.org
#   - github.com
`

var (
	configWriteAgentFlag string
	configWriteForceFlag bool
)

func newConfigCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Manage optional repository configuration",
		Long: `Manage optional repository configuration such as the repo-specific
defaults stored in .kagen.yaml.

This command does not initialise the runtime. 'kagen start' and 'kagen attach'
work without .kagen.yaml.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
	}

	cmd.AddCommand(newConfigWriteCommand())
	return cmd
}

func newConfigWriteCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "write",
		Short: "Write optional repository defaults to .kagen.yaml",
		Long: `Write an optional .kagen.yaml with repository-specific defaults and
overrides.

This file is not required before 'kagen start' or 'kagen attach'. Running the
command again is a safe no-op unless '--force' is supplied.`,
		RunE: runConfigWrite,
	}

	cmd.Flags().StringVar(&configWriteAgentFlag, "agent", string(agent.Codex), "default agent to write to .kagen.yaml")
	cmd.Flags().BoolVar(&configWriteForceFlag, "force", false, "overwrite .kagen.yaml if it already exists")

	return cmd
}

func runConfigWrite(_ *cobra.Command, _ []string) error {
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting working directory: %w", err)
	}

	agentType, err := agent.TypeFromString(configWriteAgentFlag)
	if err != nil {
		return err
	}

	configPath := filepath.Join(cwd, ".kagen.yaml")
	configContent := fmt.Sprintf(defaultKagenConfigTemplate, agentType)
	created, err := writeProjectFile(configPath, configContent, configWriteForceFlag)
	if err != nil {
		return err
	}
	if created {
		ui.Success("Wrote %s", configPath)
		return nil
	}

	ui.Info("Optional project config already exists — nothing to do")
	return nil
}

// writeProjectFile writes content to path if it does not exist, or overwrites it
// when force is enabled.
func writeProjectFile(path, content string, force bool) (bool, error) {
	if !force {
		if _, err := os.Stat(path); err == nil {
			return false, nil
		}
	}

	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return false, fmt.Errorf("creating %s: %w", filepath.Base(path), err)
	}

	return true, nil
}
