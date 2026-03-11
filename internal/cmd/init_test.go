package cmd

import (
	"os"
	"path/filepath"
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

	if len(parsed.Components) != 1 || parsed.Components[0].Container == nil {
		t.Fatalf("expected one container component, got %#v", parsed.Components)
	}

	container := parsed.Components[0].Container
	if container.Image != "vxcontrol/codebase:latest" {
		t.Fatalf("Container.Image = %q, want %q", container.Image, "vxcontrol/codebase:latest")
	}

	if parsed.Components[0].Name != "workspace" {
		t.Fatalf("Components[0].Name = %q, want %q", parsed.Components[0].Name, "workspace")
	}
	if len(parsed.Components) != 1 {
		t.Fatalf("len(Components) = %d, want 1", len(parsed.Components))
	}
	if len(container.Command) != 2 || container.Command[0] != "/bin/sh" {
		t.Fatalf("Container.Command = %#v, want shell keepalive", container.Command)
	}
}
