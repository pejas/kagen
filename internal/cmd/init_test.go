package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pejas/kagen/internal/devfile"
)

func TestDefaultDevfileContent(t *testing.T) {
	tmpDir := t.TempDir()
	devfilePath := filepath.Join(tmpDir, "devfile.yaml")

	if err := os.WriteFile(devfilePath, []byte(defaultDevfileContent), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	parsed, err := devfile.Parse(devfilePath)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	if len(parsed.Components) != 1 || parsed.Components[0].Container == nil {
		t.Fatalf("expected one container component, got %#v", parsed.Components)
	}

	container := parsed.Components[0].Container
	if container.Image != "vxcontrol/codebase:latest" {
		t.Fatalf("Container.Image = %q, want %q", container.Image, "vxcontrol/codebase:latest")
	}

	foundCodexMount := false
	for _, mount := range container.VolumeMounts {
		if mount.Name == "agent-auth" && mount.Path == "/home/vscode/.codex" {
			foundCodexMount = true
			break
		}
	}
	if !foundCodexMount {
		t.Fatalf("agent-auth mount not found in %#v", container.VolumeMounts)
	}

	if !strings.Contains(defaultDevfileContent, `command: ["tail", "-f", "/dev/null"]`) {
		t.Fatal("default devfile is missing the keepalive command")
	}
}
