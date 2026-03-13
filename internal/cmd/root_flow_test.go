package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pejas/kagen/internal/agent"
	"github.com/pejas/kagen/internal/config"
	"github.com/pejas/kagen/internal/git"
	"github.com/pejas/kagen/internal/ui"
	"github.com/pejas/kagen/internal/workload"
)

func TestLoadRunConfigHonoursVerboseFlag(t *testing.T) {
	previousVerbose := verboseFlag
	previousUIVerbose := ui.VerboseEnabled()
	verboseFlag = true
	t.Cleanup(func() {
		verboseFlag = previousVerbose
		ui.SetVerbose(previousUIVerbose)
	})

	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() returned error: %v", err)
	}

	tempDir := t.TempDir()
	if err := os.Chdir(tempDir); err != nil {
		t.Fatalf("Chdir(%q) returned error: %v", tempDir, err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(wd)
	})

	cfg, err := loadRunConfig()
	if err != nil {
		t.Fatalf("loadRunConfig() returned error: %v", err)
	}
	if !cfg.Verbose {
		t.Fatal("loadRunConfig() should enable verbose mode when --verbose is set")
	}
}

func TestBuildRuntimePodUsesInternalWorkloadBuilderWithoutDevfile(t *testing.T) {
	t.Parallel()

	repo := &git.Repository{
		Path:          t.TempDir(),
		CurrentBranch: "main",
	}

	pod, err := buildRuntimePod(repo, config.DefaultConfig(), agent.Codex)
	if err != nil {
		t.Fatalf("buildRuntimePod() returned error: %v", err)
	}

	if pod.Name != runtimePodName {
		t.Fatalf("pod name = %q, want %q", pod.Name, runtimePodName)
	}
	if len(pod.Spec.Containers) != 2 {
		t.Fatalf("container count = %d, want 2", len(pod.Spec.Containers))
	}
	if pod.Spec.Containers[1].Name != "kagen-agent-codex" {
		t.Fatalf("runtime container name = %q, want %q", pod.Spec.Containers[1].Name, "kagen-agent-codex")
	}
	if got := pod.Spec.Containers[1].Image; got != workload.DefaultImages().Toolbox {
		t.Fatalf("runtime container image = %q, want %q", got, workload.DefaultImages().Toolbox)
	}
	if strings.Contains(strings.Join(pod.Spec.Containers[1].Args, "\n"), "npm install -g") {
		t.Fatalf("runtime container args unexpectedly install packages: %q", pod.Spec.Containers[1].Args)
	}
}

func TestBuildRuntimePodIgnoresRepositoryDevfile(t *testing.T) {
	t.Parallel()

	repoDir := t.TempDir()
	repo := &git.Repository{
		Path:          repoDir,
		CurrentBranch: "main",
	}

	devfileContent := `schemaVersion: 2.2.0
metadata:
  name: test-project
components:
  - name: workspace
    container:
      image: custom/workspace:1.0
      command: ["/bin/sh", "-lc"]
      args:
        - exec tail -f /dev/null
`
	if err := os.WriteFile(filepath.Join(repoDir, "devfile.yaml"), []byte(devfileContent), 0o644); err != nil {
		t.Fatalf("WriteFile(devfile.yaml) returned error: %v", err)
	}

	pod, err := buildRuntimePod(repo, config.DefaultConfig(), agent.Codex)
	if err != nil {
		t.Fatalf("buildRuntimePod() returned error: %v", err)
	}

	if pod.Spec.Containers[0].Image != workload.DefaultImages().Workspace {
		t.Fatalf("workspace image = %q, want generated default %q", pod.Spec.Containers[0].Image, workload.DefaultImages().Workspace)
	}
	if pod.Spec.Containers[1].Name != "kagen-agent-codex" {
		t.Fatalf("runtime container name = %q, want %q", pod.Spec.Containers[1].Name, "kagen-agent-codex")
	}
	if got := pod.Spec.Containers[1].Image; got != workload.DefaultImages().Toolbox {
		t.Fatalf("runtime container image = %q, want generated toolbox %q", got, workload.DefaultImages().Toolbox)
	}
}
