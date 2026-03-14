// Package config provides Viper-based configuration loading for kagen.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/viper"
)

// Config holds all kagen configuration values.
type Config struct {
	// Agent is the default agent type (claude, codex, opencode).
	Agent string `mapstructure:"agent"`

	// AgentProviders declares the upstream API providers required per agent.
	AgentProviders map[string][]string `mapstructure:"agent_providers"`

	// ProxyAllowlist is the list of allowed egress destinations.
	ProxyAllowlist []string `mapstructure:"proxy_allowlist"`

	// ForgejoHTTPPort is the local HTTP port for the in-cluster Forgejo instance.
	ForgejoHTTPPort int `mapstructure:"forgejo_http_port"`

	// ForgejoSSHPort is the local SSH port for the in-cluster Forgejo instance.
	ForgejoSSHPort int `mapstructure:"forgejo_ssh_port"`

	// Verbose enables verbose output.
	Verbose bool `mapstructure:"verbose"`

	// Runtime contains configuration for the Colima runtime.
	Runtime RuntimeConfig `mapstructure:"runtime"`

	// Images contains runtime image references for the generated workloads.
	Images ImagesConfig `mapstructure:"images"`
}

// RuntimeConfig holds Colima-specific settings.
type RuntimeConfig struct {
	CPU            int    `mapstructure:"cpu"`
	Memory         int    `mapstructure:"memory"`
	Disk           int    `mapstructure:"disk"`
	StartupTimeout string `mapstructure:"startup_timeout"`
}

// ImagesConfig holds container image references used on the runtime path.
type ImagesConfig struct {
	Workspace string `mapstructure:"workspace"`
	Toolbox   string `mapstructure:"toolbox"`
	Proxy     string `mapstructure:"proxy"`
}

var defaultConfig = Config{
	Agent:           "",
	AgentProviders:  map[string][]string{},
	ProxyAllowlist:  nil,
	ForgejoHTTPPort: 3000,
	ForgejoSSHPort:  2222,
	Verbose:         false,
	Runtime: RuntimeConfig{
		CPU:            4,
		Memory:         8,
		Disk:           60,
		StartupTimeout: "5m",
	},
	Images: ImagesConfig{
		Workspace: "ghcr.io/pejas/kagen-workspace:0.1.4",
		Toolbox:   "ghcr.io/pejas/kagen-toolbox:0.1.4",
		Proxy:     "ghcr.io/pejas/kagen-proxy:0.1.4",
	},
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() *Config {
	cfg := defaultConfig
	cfg.AgentProviders = map[string][]string{}
	return &cfg
}

// ProvidersForAgent returns the configured providers for the given agent.
func (c *Config) ProvidersForAgent(agent string) []string {
	if c == nil || c.AgentProviders == nil {
		return nil
	}

	return c.AgentProviders[agent]
}

// Load reads configuration from:
// 1. Global config (~/.config/kagen/main.yml)
// 2. Project config (.kagen.yaml in repo root)
// 3. Environment variables (KAGEN_*)
// 4. CLI flags (bound in cmd package)
func Load() (*Config, error) {
	v := viper.New()

	// 1. Set Defaults
	setDefaults(v)

	// 2. Load Global Config (~/.config/kagen/main.yml)
	configDir, err := configDirectory()
	if err == nil {
		v.AddConfigPath(configDir)
		v.SetConfigName("main")
		v.SetConfigType("yaml")
		if err := v.ReadInConfig(); err != nil {
			if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
				return nil, fmt.Errorf("reading global config: %w", err)
			}
		}
	}

	// 3. Load Project Config (.kagen.yaml)
	// We search in the current directory.
	v.AddConfigPath(".")
	v.SetConfigName(".kagen")
	v.SetConfigType("yaml")
	// MergeInConfig merges the current file into the existing config.
	if err := v.MergeInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("merging project config: %w", err)
		}
	}

	// 4. Environment variable overrides
	v.SetEnvPrefix("KAGEN")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	cfg := DefaultConfig()
	if err := v.Unmarshal(cfg); err != nil {
		return nil, fmt.Errorf("unmarshalling config: %w", err)
	}
	if err := Validate(cfg); err != nil {
		return nil, fmt.Errorf("validating config: %w", err)
	}

	return cfg, nil
}

func setDefaults(v *viper.Viper) {
	cfg := DefaultConfig()
	v.SetDefault("agent", cfg.Agent)
	v.SetDefault("agent_providers", cfg.AgentProviders)
	v.SetDefault("proxy_allowlist", []string{})
	v.SetDefault("forgejo_http_port", cfg.ForgejoHTTPPort)
	v.SetDefault("forgejo_ssh_port", cfg.ForgejoSSHPort)
	v.SetDefault("verbose", cfg.Verbose)
	v.SetDefault("runtime.cpu", cfg.Runtime.CPU)
	v.SetDefault("runtime.memory", cfg.Runtime.Memory)
	v.SetDefault("runtime.disk", cfg.Runtime.Disk)
	v.SetDefault("runtime.startup_timeout", cfg.Runtime.StartupTimeout)
	v.SetDefault("images.workspace", cfg.Images.Workspace)
	v.SetDefault("images.toolbox", cfg.Images.Toolbox)
	v.SetDefault("images.proxy", cfg.Images.Proxy)
}

// configDirectory returns the path to the kagen config directory,
// creating it if it does not exist.
func configDirectory() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("finding home directory: %w", err)
	}

	dir := filepath.Join(home, ".config", "kagen")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("creating config directory: %w", err)
	}

	return dir, nil
}
