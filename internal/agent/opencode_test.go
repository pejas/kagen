package agent

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestOpenCodeConfigFilesReturnsConfig(t *testing.T) {
	t.Parallel()

	spec := openCodeRuntimeSpec{}
	configFiles := spec.ConfigFiles()

	if len(configFiles) != 1 {
		t.Fatalf("ConfigFiles() returned %d files, want 1", len(configFiles))
	}

	cf := configFiles[0]
	if cf.Name != "opencode.json" {
		t.Errorf("ConfigFile.Name = %q, want %q", cf.Name, "opencode.json")
	}

	expectedPath := filepath.Join(spec.StateRoot(), ".config", "opencode.json")
	if cf.MountPath != expectedPath {
		t.Errorf("ConfigFile.MountPath = %q, want %q", cf.MountPath, expectedPath)
	}

	if !strings.Contains(cf.Content, "opencode.ai/config.json") {
		t.Errorf("ConfigFile.Content missing expected schema reference")
	}
	if !strings.Contains(cf.Content, `"permission": "allow"`) {
		t.Errorf("ConfigFile.Content missing permission setting")
	}
}

func TestOpenCodeConfigFilesMountPathIsAbsolute(t *testing.T) {
	t.Parallel()

	spec := openCodeRuntimeSpec{}
	configFiles := spec.ConfigFiles()

	if len(configFiles) == 0 {
		t.Fatal("ConfigFiles() returned empty slice")
	}

	cf := configFiles[0]
	if !filepath.IsAbs(cf.MountPath) {
		t.Errorf("ConfigFile.MountPath = %q, want absolute path", cf.MountPath)
	}
}
