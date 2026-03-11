package devfile

import (
	"os"
	"path/filepath"
	"testing"
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
}

func TestParse_NotFound(t *testing.T) {
	_, err := Parse("non-existent-file.yaml")
	if err == nil {
		t.Error("Parse() expected error for non-existent file, got nil")
	}
}
