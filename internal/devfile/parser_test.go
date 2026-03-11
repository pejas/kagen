package devfile

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/pejas/kagen/internal/agent"
)

func TestParse(t *testing.T) {
	// Create a temporary devfile
	tmpDir := t.TempDir()
	devfilePath := filepath.Join(tmpDir, "devfile.yaml")
	content := `
schemaVersion: 2.2.0
metadata:
  name: test-project
components:
  - name: web
    attributes:
      kagen.agent/runtime: codex
    container:
      image: nginx:latest
      memoryLimit: 512Mi
`
	if err := os.WriteFile(devfilePath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	d, err := Parse(devfilePath)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	if d.Metadata.Name != "test-project" {
		t.Errorf("Metadata.Name = %v, want test-project", d.Metadata.Name)
	}
	if !d.SupportsAgent(agent.Codex) {
		t.Error("SupportsAgent(codex) = false, want true")
	}
}

func TestParse_NotFound(t *testing.T) {
	_, err := Parse("non-existent-file.yaml")
	if err == nil {
		t.Error("Parse() expected error for non-existent file, got nil")
	}
}

func TestFindPath(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "devfile.yml")
	if err := os.WriteFile(path, []byte("schemaVersion: 2.2.0\nmetadata:\n  name: test\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", path, err)
	}

	got, err := FindPath(tmpDir)
	if err != nil {
		t.Fatalf("FindPath(%q) error = %v", tmpDir, err)
	}
	if got != path {
		t.Errorf("FindPath(%q) = %q, want %q", tmpDir, got, path)
	}
}
