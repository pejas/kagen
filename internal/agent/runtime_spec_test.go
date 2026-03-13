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

func TestRuntimeSpecReadyCheckSearchesToolboxPathCandidates(t *testing.T) {
	t.Parallel()

	spec, err := SpecFor(Codex)
	if err != nil {
		t.Fatalf("SpecFor(codex) returned error: %v", err)
	}

	check := spec.ReadyCheck()
	if !strings.Contains(check, `command -v "codex"`) {
		t.Fatalf("ReadyCheck() missing command -v probe: %q", check)
	}
	if !strings.Contains(check, `/opt/mise/shims/codex`) {
		t.Fatalf("ReadyCheck() missing mise shim fallback: %q", check)
	}
	if strings.Contains(check, `break;`) || strings.Contains(check, ` then break;`) {
		t.Fatalf("ReadyCheck() should not emit loop control outside a loop: %q", check)
	}
	if !strings.Contains(check, `test -n "$KAGEN_AGENT_BIN"`) {
		t.Fatalf("ReadyCheck() missing resolved binary assertion: %q", check)
	}
}

func TestRuntimeSpecLegacyBootstrapPreservesRuntimeInstallation(t *testing.T) {
	t.Parallel()

	spec, err := SpecFor(Codex)
	if err != nil {
		t.Fatalf("SpecFor(codex) returned error: %v", err)
	}

	if !strings.Contains(strings.Join(spec.LegacyBootstrapArgs(), "\n"), "npm install -g @openai/codex@latest") {
		t.Fatalf("LegacyBootstrapArgs() = %q, want latest npm bootstrap", spec.LegacyBootstrapArgs())
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
	if !strings.Contains(shell, `export PATH="/opt/mise/shims:/opt/mise/bin:/usr/local/bin:/usr/local/sbin:/usr/sbin:/usr/bin:/sbin:/bin"`) {
		t.Fatalf("AttachShellForStatePath() missing toolbox PATH export: %q", shell)
	}
	if !strings.Contains(shell, `export TERM="xterm-256color"`) {
		t.Fatalf("AttachShellForStatePath() missing TERM export: %q", shell)
	}
	if !strings.Contains(shell, `export CODEX_HOME="/home/kagen/.codex/session-1"`) {
		t.Fatalf("AttachShellForStatePath() missing CODEX_HOME override: %q", shell)
	}
	if !strings.Contains(shell, `export KAGEN_AGENT_BIN=`) {
		t.Fatalf("AttachShellForStatePath() missing binary resolution export: %q", shell)
	}
	if strings.Contains(shell, `break;`) || strings.Contains(shell, ` then break;`) {
		t.Fatalf("AttachShellForStatePath() should not emit loop control outside a loop: %q", shell)
	}
	if !strings.Contains(shell, `exec "$KAGEN_AGENT_BIN" --sandbox danger-full-access -a never`) {
		t.Fatalf("AttachShellForStatePath() missing resolved codex exec: %q", shell)
	}
}
