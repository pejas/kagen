package config

import (
	"fmt"
	"strings"
	"time"

	"github.com/pejas/kagen/internal/agent"
)

// Validate verifies that configuration values are internally consistent.
func Validate(cfg *Config) error {
	if cfg == nil {
		return fmt.Errorf("config is nil")
	}

	if cfg.Agent != "" {
		if _, err := agent.TypeFromString(cfg.Agent); err != nil {
			return fmt.Errorf("validating agent: %w", err)
		}
	}

	if err := validatePort("forgejo_http_port", cfg.ForgejoHTTPPort); err != nil {
		return err
	}
	if err := validatePort("forgejo_ssh_port", cfg.ForgejoSSHPort); err != nil {
		return err
	}

	if cfg.Runtime.CPU <= 0 {
		return fmt.Errorf("runtime.cpu must be greater than zero")
	}
	if cfg.Runtime.Memory <= 0 {
		return fmt.Errorf("runtime.memory must be greater than zero")
	}
	if cfg.Runtime.Disk <= 0 {
		return fmt.Errorf("runtime.disk must be greater than zero")
	}
	if _, err := time.ParseDuration(cfg.Runtime.StartupTimeout); err != nil {
		return fmt.Errorf("runtime.startup_timeout must be a valid duration: %w", err)
	}

	for _, dest := range cfg.ProxyAllowlist {
		if strings.TrimSpace(dest) == "" {
			return fmt.Errorf("proxy_allowlist cannot contain empty values")
		}
	}

	return nil
}

func validatePort(name string, port int) error {
	if port < 1 || port > 65535 {
		return fmt.Errorf("%s must be between 1 and 65535", name)
	}

	return nil
}
