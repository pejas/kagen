package cmd

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/pejas/kagen/internal/agent"
	"github.com/pejas/kagen/internal/devfile"
	"github.com/pejas/kagen/internal/ui"
)

// defaultKagenConfig is the default project-level kagen config.
const defaultKagenConfigTemplate = `# kagen project configuration
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
	initAgentFlag string
	initForceFlag bool
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Bootstrap local project configuration",
	Long: `Initialize a kagen project in the current repository.
Creates devfile.yaml if it does not already exist. 
Project-specific overrides can be added manually to .kagen.yaml if needed.
This command is idempotent — running it again is a safe no-op.`,
	RunE: runInit,
}

func runInit(_ *cobra.Command, _ []string) error {
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting working directory: %w", err)
	}

	agentType, err := agent.TypeFromString(initAgentFlag)
	if err != nil {
		return err
	}

	devfileContent, err := devfile.DefaultForAgent(agentType)
	if err != nil {
		return err
	}

	devfilePath := filepath.Join(cwd, "devfile.yaml")
	if existingPath, err := devfile.FindPath(cwd); err == nil {
		devfilePath = existingPath
	} else if !errors.Is(err, devfile.ErrDevfileNotFound()) {
		return err
	}
	configPath := filepath.Join(cwd, ".kagen.yaml")
	created := false

	devfileCreated, err := writeProjectFile(devfilePath, devfileContent, initForceFlag)
	if err != nil {
		return err
	}
	if devfileCreated {
		ui.Success("Wrote %s", devfilePath)
		created = true
	}

	configContent := fmt.Sprintf(defaultKagenConfigTemplate, agentType)
	configCreated, err := writeProjectFile(configPath, configContent, initForceFlag)
	if err != nil {
		return err
	}
	if configCreated {
		ui.Success("Wrote %s", configPath)
		created = true
	}

	if !created {
		ui.Info("Project already initialized — nothing to do")
	}

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

func init() {
	initCmd.Flags().StringVar(&initAgentFlag, "agent", string(agent.Codex), "agent runtime to provision in devfile.yaml")
	initCmd.Flags().BoolVar(&initForceFlag, "force", false, "overwrite devfile.yaml and .kagen.yaml if they already exist")
}

func createIfMissing(path, content string) (bool, error) {
	if _, err := os.Stat(path); err == nil {
		return false, nil // Already exists.
	}

	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return false, fmt.Errorf("creating %s: %w", filepath.Base(path), err)
	}

	return true, nil
}
