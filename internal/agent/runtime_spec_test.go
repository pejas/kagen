package agent

import (
	"strings"
	"testing"
)

func TestRuntimeSpecToolboxBootstrapAvoidsRuntimeInstallation(t *testing.T) {
	t.Parallel()

	spec, err := SpecFor(Codex)
	if err != nil {
		t.Fatalf("SpecFor(codex) returned error: %v", err)
	}

	if got := spec.ToolboxBootstrapCommand(); len(got) != 2 || got[0] != "/bin/sh" || got[1] != "-lc" {
		t.Fatalf("ToolboxBootstrapCommand() = %q, want [/bin/sh -lc]", got)
	}
	if got := spec.ToolboxBootstrapArgs(); len(got) != 1 || got[0] != "exec tail -f /dev/null" {
		t.Fatalf("ToolboxBootstrapArgs() = %q, want install-free keepalive", got)
	}
	if strings.Contains(strings.Join(spec.ToolboxBootstrapArgs(), "\n"), "npm install -g") {
		t.Fatalf("ToolboxBootstrapArgs() unexpectedly install packages: %q", spec.ToolboxBootstrapArgs())
	}
}

func TestRuntimeSpecLegacyBootstrapPreservesRuntimeInstallation(t *testing.T) {
	t.Parallel()

	spec, err := SpecFor(Codex)
	if err != nil {
		t.Fatalf("SpecFor(codex) returned error: %v", err)
	}

	if !strings.Contains(strings.Join(spec.LegacyBootstrapArgs(), "\n"), "npm install -g @openai/codex") {
		t.Fatalf("LegacyBootstrapArgs() = %q, want npm bootstrap", spec.LegacyBootstrapArgs())
	}
}

func TestRuntimeSpecAttachShellForStatePathIsolatesRuntimeState(t *testing.T) {
	t.Parallel()

	spec, err := SpecFor(Codex)
	if err != nil {
		t.Fatalf("SpecFor(codex) returned error: %v", err)
	}

	shell := spec.AttachShellForStatePath("/home/kagen/.codex/session-1")
	if !strings.Contains(shell, `mkdir -p "/home/kagen/.codex/session-1"`) {
		t.Fatalf("AttachShellForStatePath() missing mkdir: %q", shell)
	}
	if !strings.Contains(shell, `export HOME="/home/kagen/.codex/session-1"`) {
		t.Fatalf("AttachShellForStatePath() missing HOME override: %q", shell)
	}
	if !strings.Contains(shell, `export CODEX_HOME="/home/kagen/.codex/session-1"`) {
		t.Fatalf("AttachShellForStatePath() missing CODEX_HOME override: %q", shell)
	}
	if !strings.Contains(shell, "exec codex --sandbox danger-full-access -a never") {
		t.Fatalf("AttachShellForStatePath() missing codex exec: %q", shell)
	}
}
