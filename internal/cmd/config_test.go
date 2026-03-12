package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunConfigWriteWritesOnlyProjectConfig(t *testing.T) {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() returned error: %v", err)
	}

	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("Chdir(tmpDir) returned error: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(cwd)
	})

	originalAgentFlag := configWriteAgentFlag
	originalForceFlag := configWriteForceFlag
	configWriteAgentFlag = "codex"
	configWriteForceFlag = false
	t.Cleanup(func() {
		configWriteAgentFlag = originalAgentFlag
		configWriteForceFlag = originalForceFlag
	})

	if err := runConfigWrite(nil, nil); err != nil {
		t.Fatalf("runConfigWrite() returned error: %v", err)
	}

	configPath := filepath.Join(tmpDir, ".kagen.yaml")
	content, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile(.kagen.yaml) returned error: %v", err)
	}

	if strings.TrimSpace(string(content)) == "" {
		t.Fatal(".kagen.yaml should not be empty")
	}
	if !strings.Contains(string(content), "agent: codex") {
		t.Fatalf(".kagen.yaml = %q, want codex default agent", string(content))
	}
	if !strings.Contains(string(content), "This file is not required before 'kagen start' or 'kagen attach'.") {
		t.Fatalf(".kagen.yaml = %q, want optional config guidance", string(content))
	}
	if _, err := os.Stat(filepath.Join(tmpDir, "devfile.yaml")); !os.IsNotExist(err) {
		t.Fatalf("devfile.yaml stat error = %v, want file to remain absent", err)
	}
}

func TestRunConfigWriteForceOverwritesOnlyProjectConfig(t *testing.T) {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() returned error: %v", err)
	}

	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("Chdir(tmpDir) returned error: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(cwd)
	})

	configPath := filepath.Join(tmpDir, ".kagen.yaml")
	if err := os.WriteFile(configPath, []byte("agent: claude\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(.kagen.yaml) returned error: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "devfile.yaml"), []byte("legacy: true\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(devfile.yaml) returned error: %v", err)
	}

	originalAgentFlag := configWriteAgentFlag
	originalForceFlag := configWriteForceFlag
	configWriteAgentFlag = "opencode"
	configWriteForceFlag = true
	t.Cleanup(func() {
		configWriteAgentFlag = originalAgentFlag
		configWriteForceFlag = originalForceFlag
	})

	if err := runConfigWrite(nil, nil); err != nil {
		t.Fatalf("runConfigWrite() returned error: %v", err)
	}

	content, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile(.kagen.yaml) returned error: %v", err)
	}
	if !strings.Contains(string(content), "agent: opencode") {
		t.Fatalf(".kagen.yaml = %q, want opencode default agent", string(content))
	}

	devfileContent, err := os.ReadFile(filepath.Join(tmpDir, "devfile.yaml"))
	if err != nil {
		t.Fatalf("ReadFile(devfile.yaml) returned error: %v", err)
	}
	if string(devfileContent) != "legacy: true\n" {
		t.Fatalf("devfile.yaml = %q, want legacy content left untouched", string(devfileContent))
	}
}

func TestConfigCommandReplacesInitCommand(t *testing.T) {
	var hasConfig bool
	var hasInit bool

	for _, cmd := range rootCmd.Commands() {
		switch cmd.Name() {
		case "config":
			hasConfig = true
		case "init":
			hasInit = true
		}
	}

	if !hasConfig {
		t.Fatal("root command should register 'config'")
	}
	if hasInit {
		t.Fatal("root command should not register removed 'init' command")
	}
	if rootCmd.PersistentFlags().Lookup("agent") != nil {
		t.Fatal("root command should not expose removed '--agent' flag")
	}

	cmd := newConfigWriteCommand()
	if !strings.Contains(cmd.Long, "not required before 'kagen start' or 'kagen attach'") {
		t.Fatalf("config write help = %q, want optional config guidance", cmd.Long)
	}
}
