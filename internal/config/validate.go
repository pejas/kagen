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
		dest = strings.TrimSpace(dest)
		if dest == "" {
			return fmt.Errorf("proxy_allowlist cannot contain empty values")
		}
		if strings.Contains(dest, "://") || strings.Contains(dest, "/") {
			return fmt.Errorf("proxy_allowlist entries must be hostnames without scheme or path: %q", dest)
		}
	}

	if err := validateImageRef("images.workspace", cfg.Images.Workspace); err != nil {
		return err
	}
	if err := validateImageRef("images.toolbox", cfg.Images.Toolbox); err != nil {
		return err
	}
	if err := validateImageRef("images.proxy", cfg.Images.Proxy); err != nil {
		return err
	}

	return nil
}

func validatePort(name string, port int) error {
	if port < 1 || port > 65535 {
		return fmt.Errorf("%s must be between 1 and 65535", name)
	}

	return nil
}

func validateImageRef(name, ref string) error {
	trimmed := strings.TrimSpace(ref)
	if trimmed == "" || strings.Contains(trimmed, "://") || strings.ContainsAny(trimmed, " \t\r\n") || strings.HasSuffix(trimmed, ":") || strings.HasSuffix(trimmed, "@") {
		return fmt.Errorf("%s must be a valid image reference", name)
	}

	return nil
}
