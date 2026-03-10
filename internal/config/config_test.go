package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	t.Parallel()

	cfg := DefaultConfig()
	if cfg.ForgejoHTTPPort != 3000 {
		t.Errorf("expected default ForgejoHTTPPort=3000, got %d", cfg.ForgejoHTTPPort)
	}
	if cfg.ForgejoSSHPort != 2222 {
		t.Errorf("expected default ForgejoSSHPort=2222, got %d", cfg.ForgejoSSHPort)
	}
	if cfg.Agent != "" {
		t.Errorf("expected default Agent to be empty, got %q", cfg.Agent)
	}
	if cfg.Verbose {
		t.Error("expected default Verbose=false")
	}
}

func TestLoadWithEnvOverrides(t *testing.T) {
	// Cannot use t.Parallel() with t.Setenv.
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)
	t.Setenv("KAGEN_AGENT", "claude")
	t.Setenv("KAGEN_VERBOSE", "true")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() returned error: %v", err)
	}

	if cfg.Agent != "claude" {
		t.Errorf("expected Agent=claude from env, got %q", cfg.Agent)
	}
	if !cfg.Verbose {
		t.Error("expected Verbose=true from env")
	}
}

func TestLoadFromConfigFile(t *testing.T) {
	// Cannot use t.Parallel() with t.Setenv.
	tmpDir := t.TempDir()
	configDir := filepath.Join(tmpDir, ".config", "kagen")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("failed to create config dir: %v", err)
	}

	configContent := []byte("agent: opencode\nforgejo_http_port: 4000\n")
	if err := os.WriteFile(filepath.Join(configDir, "config.yaml"), configContent, 0o644); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	t.Setenv("HOME", tmpDir)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() returned error: %v", err)
	}

	if cfg.Agent != "opencode" {
		t.Errorf("expected Agent=opencode from file, got %q", cfg.Agent)
	}
	if cfg.ForgejoHTTPPort != 4000 {
		t.Errorf("expected ForgejoHTTPPort=4000 from file, got %d", cfg.ForgejoHTTPPort)
	}
}
