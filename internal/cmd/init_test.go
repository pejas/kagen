package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pejas/kagen/internal/agent"
	"github.com/pejas/kagen/internal/devfile"
)

func TestDefaultDevfileContent(t *testing.T) {
	tmpDir := t.TempDir()
	devfilePath := filepath.Join(tmpDir, "devfile.yaml")

	content, err := devfile.DefaultForAgent(agent.Codex)
	if err != nil {
		t.Fatalf("DefaultForAgent() error = %v", err)
	}

	if err := os.WriteFile(devfilePath, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	parsed, err := devfile.Parse(devfilePath)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	if len(parsed.Components) != 2 || parsed.Components[0].Container == nil || parsed.Components[1].Volume == nil {
		t.Fatalf("expected one container and one volume component, got %#v", parsed.Components)
	}

	container := parsed.Components[0].Container
	if container.Image != "vxcontrol/codebase:latest" {
		t.Fatalf("Container.Image = %q, want %q", container.Image, "vxcontrol/codebase:latest")
	}

	foundCodexMount := false
	for _, mount := range container.VolumeMounts {
		if mount.Name == "agent-home" && mount.Path == "/home/kagen" {
			foundCodexMount = true
			break
		}
	}
	if !foundCodexMount {
		t.Fatalf("agent-home mount not found in %#v", container.VolumeMounts)
	}

	if !strings.Contains(content, "exec tail -f /dev/null") {
		t.Fatal("default devfile is missing the keepalive command")
	}
}
