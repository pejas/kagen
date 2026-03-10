// Package config provides Viper-based configuration loading for kagen.
package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/viper"
)

// Config holds all kagen configuration values.
type Config struct {
	// Agent is the default agent type (claude, codex, opencode).
	Agent string `mapstructure:"agent"`

	// ProxyAllowlist is the list of allowed egress destinations.
	ProxyAllowlist []string `mapstructure:"proxy_allowlist"`

	// ForgejoHTTPPort is the local HTTP port for the in-cluster Forgejo instance.
	ForgejoHTTPPort int `mapstructure:"forgejo_http_port"`

	// ForgejoSSHPort is the local SSH port for the in-cluster Forgejo instance.
	ForgejoSSHPort int `mapstructure:"forgejo_ssh_port"`

	// Verbose enables verbose output.
	Verbose bool `mapstructure:"verbose"`
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() *Config {
	return &Config{
		Agent:           "",
		ProxyAllowlist:  nil,
		ForgejoHTTPPort: 3000,
		ForgejoSSHPort:  2222,
		Verbose:         false,
	}
}

// Load reads configuration from the config file (~/.config/kagen/config.yaml),
// environment variables (KAGEN_*), and returns the merged Config.
func Load() (*Config, error) {
	v := viper.New()

	// Defaults.
	v.SetDefault("agent", "")
	v.SetDefault("proxy_allowlist", []string{})
	v.SetDefault("forgejo_http_port", 3000)
	v.SetDefault("forgejo_ssh_port", 2222)
	v.SetDefault("verbose", false)

	// Config file location.
	configDir, err := configDirectory()
	if err != nil {
		return nil, fmt.Errorf("resolving config directory: %w", err)
	}
	v.SetConfigName("config")
	v.SetConfigType("yaml")
	v.AddConfigPath(configDir)

	// Environment variable overrides.
	v.SetEnvPrefix("KAGEN")
	v.AutomaticEnv()

	// Read config file (ignore "not found" — it's optional).
	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("reading config file: %w", err)
		}
	}

	cfg := DefaultConfig()
	if err := v.Unmarshal(cfg); err != nil {
		return nil, fmt.Errorf("unmarshalling config: %w", err)
	}

	return cfg, nil
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
