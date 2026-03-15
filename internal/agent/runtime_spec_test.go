package agent

import (
	"fmt"
	"strings"
	"testing"
)

func TestRuntimeSpecToolboxBootstrapAvoidsRuntimeInstallation(t *testing.T) {
	t.Parallel()

	spec, err := SpecFor(Codex)
	if err != nil {
		t.Fatalf("SpecFor(codex) returned error: %v", err)
	}

	shell := AttachShellForStatePath(spec, "")
	if strings.Contains(shell, "npm install -g") {
		t.Fatalf("AttachShellForStatePath() unexpectedly installs packages: %q", shell)
	}
}

func TestRuntimeSpecReadyCheckSearchesToolboxPathCandidates(t *testing.T) {
	t.Parallel()

	spec, err := SpecFor(Codex)
	if err != nil {
		t.Fatalf("SpecFor(codex) returned error: %v", err)
	}

	check := ReadyCheck(spec)
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
	if !strings.Contains(check, `test -x "$KAGEN_AGENT_BIN"`) {
		t.Fatalf("ReadyCheck() missing executable assertion: %q", check)
	}
}

func TestRuntimeSpecAttachShellForStatePathIsolatesRuntimeState(t *testing.T) {
	t.Parallel()

	spec, err := SpecFor(Codex)
	if err != nil {
		t.Fatalf("SpecFor(codex) returned error: %v", err)
	}

	shell := AttachShellForStatePath(spec, "/home/kagen/.codex/session-1")
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

func TestRuntimeSpecAttachShellForEmptyStatePathKeepsDeclaredDefaults(t *testing.T) {
	t.Parallel()

	spec, err := SpecFor(Codex)
	if err != nil {
		t.Fatalf("SpecFor(codex) returned error: %v", err)
	}

	shell := AttachShellForStatePath(spec, "")
	if !strings.Contains(shell, `export HOME="/home/kagen"`) {
		t.Fatalf("AttachShellForStatePath() missing default HOME export: %q", shell)
	}
	if !strings.Contains(shell, `export CODEX_HOME="/home/kagen/.codex"`) {
		t.Fatalf("AttachShellForStatePath() missing default CODEX_HOME export: %q", shell)
	}
}

func TestRuntimeSpecBinaryPreflightCheckPrintsResolvedPath(t *testing.T) {
	t.Parallel()

	spec, err := SpecFor(Codex)
	if err != nil {
		t.Fatalf("SpecFor(codex) returned error: %v", err)
	}

	check := BinaryPreflightCheck(spec)
	if !strings.Contains(check, `printf '%s' "$KAGEN_AGENT_BIN"`) {
		t.Fatalf("BinaryPreflightCheck() missing resolved path output: %q", check)
	}
}

func TestRuntimeSpecStateRootUsesAgentScopedDirectory(t *testing.T) {
	t.Parallel()

	spec, err := SpecFor(Codex)
	if err != nil {
		t.Fatalf("SpecFor(codex) returned error: %v", err)
	}

	if got := spec.StateRoot(); got != "/home/kagen/.codex" {
		t.Fatalf("StateRoot() = %q, want /home/kagen/.codex", got)
	}
}

func TestResolveEnvVarsUsesLiteralAndSessionSources(t *testing.T) {
	t.Parallel()

	resolved := resolveEnvVars([]EnvVar{
		{Name: "PATH", Value: "/usr/bin", Source: EnvValueLiteral},
		{Name: "HOME", Value: "/home/kagen", Source: EnvValueSessionRoot},
		{Name: "CLAUDE_CONFIG_DIR", Value: ".claude", Source: EnvValueSessionPathJoin},
	}, "/home/kagen/.state/session-1")

	if len(resolved) != 3 {
		t.Fatalf("resolveEnvVars() returned %d variables, want 3", len(resolved))
	}
	if resolved[0].Value != "/usr/bin" {
		t.Fatalf("PATH value = %q, want /usr/bin", resolved[0].Value)
	}
	if resolved[1].Value != "/home/kagen/.state/session-1" {
		t.Fatalf("HOME value = %q, want session root", resolved[1].Value)
	}
	if resolved[2].Value != "/home/kagen/.state/session-1/.claude" {
		t.Fatalf("CLAUDE_CONFIG_DIR value = %q, want joined session path", resolved[2].Value)
	}
}

func TestResolveEnvVarsKeepsDefaultValuesWhenStatePathIsEmpty(t *testing.T) {
	t.Parallel()

	resolved := resolveEnvVars([]EnvVar{
		{Name: "HOME", Value: "/home/kagen", Source: EnvValueSessionRoot},
		{Name: "CLAUDE_CONFIG_DIR", Value: "/home/kagen/.claude", Source: EnvValueSessionPathJoin},
	}, "")

	if resolved[0].Value != "/home/kagen" {
		t.Fatalf("HOME value = %q, want declared default", resolved[0].Value)
	}
	if resolved[1].Value != "/home/kagen/.claude" {
		t.Fatalf("CLAUDE_CONFIG_DIR value = %q, want declared default", resolved[1].Value)
	}
}

func TestResolveEnvVarsRejectsDuplicateNames(t *testing.T) {
	t.Parallel()

	defer func() {
		recovered := recover()
		if recovered == nil {
			t.Fatal("resolveEnvVars() did not panic for duplicate names")
		}
		if got := fmt.Sprint(recovered); !strings.Contains(got, `duplicate environment variable "HOME"`) {
			t.Fatalf("panic = %q, want duplicate env message", got)
		}
	}()

	_ = resolveEnvVars([]EnvVar{
		{Name: "HOME", Value: "/home/kagen", Source: EnvValueSessionRoot},
		{Name: "HOME", Value: "/tmp/home", Source: EnvValueLiteral},
	}, "/home/kagen/.state/session-1")
}

func TestResolveEnvVarsPreservesDeclarationOrder(t *testing.T) {
	t.Parallel()

	resolved := resolveEnvVars([]EnvVar{
		{Name: "TERM", Value: defaultTerm, Source: EnvValueLiteral},
		{Name: "HOME", Value: DefaultHomeDir(), Source: EnvValueSessionRoot},
		{Name: "PATH", Value: defaultToolboxPath, Source: EnvValueLiteral},
	}, "/home/kagen/.state/session-1")

	if resolved[0].Name != "TERM" || resolved[1].Name != "HOME" || resolved[2].Name != "PATH" {
		t.Fatalf("resolveEnvVars() order = %#v, want TERM, HOME, PATH", resolved)
	}
}
