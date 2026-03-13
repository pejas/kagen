package agent

import (
	"fmt"
	"slices"
	"strings"

	kagerr "github.com/pejas/kagen/internal/errors"
)

const defaultHomeDir = "/home/kagen"
const defaultTerm = "xterm-256color"
const defaultToolboxPath = "/opt/mise/shims:/opt/mise/bin:/usr/local/bin:/usr/local/sbin:/usr/sbin:/usr/bin:/sbin:/bin"

type runtimeBootstrap struct {
	command []string
	args    []string
}

// EnvVar defines a required environment variable for a runtime.
type EnvVar struct {
	Name  string
	Value string
}

// RuntimeSpec describes how kagen bootstraps and attaches to an agent runtime.
type RuntimeSpec struct {
	Type            Type
	DisplayName     string
	GitAuthorName   string
	Binary          string
	AttachShell     string
	RequiredEnv     []EnvVar
	legacyBootstrap runtimeBootstrap
}

// DefaultHomeDir returns the shared runtime home directory inside the pod.
func DefaultHomeDir() string {
	return defaultHomeDir
}

// SpecFor returns the runtime specification for the given agent.
func SpecFor(agentType Type) (RuntimeSpec, error) {
	switch agentType {
	case Claude:
		return RuntimeSpec{
			Type:          Claude,
			DisplayName:   "Claude",
			GitAuthorName: "claude",
			Binary:        "claude",
			AttachShell:   `cd /projects/workspace && exec "$KAGEN_AGENT_BIN"`,
			RequiredEnv: []EnvVar{
				{Name: "HOME", Value: defaultHomeDir},
				{Name: "PATH", Value: defaultToolboxPath},
				{Name: "TERM", Value: defaultTerm},
				{Name: "CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC", Value: "1"},
			},
			legacyBootstrap: npmBootstrap("claude", "@anthropic-ai/claude-code"),
		}, nil
	case Codex:
		return RuntimeSpec{
			Type:          Codex,
			DisplayName:   "Codex",
			GitAuthorName: "oai-codex",
			Binary:        "codex",
			AttachShell:   `cd /projects/workspace && exec "$KAGEN_AGENT_BIN" --sandbox danger-full-access -a never`,
			RequiredEnv: []EnvVar{
				{Name: "HOME", Value: defaultHomeDir},
				{Name: "PATH", Value: defaultToolboxPath},
				{Name: "TERM", Value: defaultTerm},
				{Name: "CODEX_HOME", Value: defaultHomeDir + "/.codex"},
			},
			legacyBootstrap: npmBootstrapLatest("codex", "@openai/codex"),
		}, nil
	case OpenCode:
		return RuntimeSpec{
			Type:          OpenCode,
			DisplayName:   "OpenCode",
			GitAuthorName: "opencode",
			Binary:        "opencode",
			AttachShell:   `cd /projects/workspace && exec "$KAGEN_AGENT_BIN"`,
			RequiredEnv: []EnvVar{
				{Name: "HOME", Value: defaultHomeDir},
				{Name: "PATH", Value: defaultToolboxPath},
				{Name: "TERM", Value: defaultTerm},
			},
			legacyBootstrap: npmBootstrap("opencode", "opencode-ai"),
		}, nil
	default:
		return RuntimeSpec{}, fmt.Errorf("runtime spec for %s: %w", agentType, kagerr.ErrAgentUnknown)
	}
}

func (s RuntimeSpec) ContainerName() string {
	return "kagen-agent-" + string(s.Type)
}

func (s RuntimeSpec) LegacyBootstrapCommand() []string {
	return cloneStrings(s.legacyBootstrap.command)
}

func (s RuntimeSpec) LegacyBootstrapArgs() []string {
	return cloneStrings(s.legacyBootstrap.args)
}

// Toolbox bootstrap assumes the runtime binary is already present in the image.
func (s RuntimeSpec) ToolboxBootstrapCommand() []string {
	return []string{"/bin/sh", "-lc"}
}

func (s RuntimeSpec) ToolboxBootstrapArgs() []string {
	return []string{"exec tail -f /dev/null"}
}

func (s RuntimeSpec) ReadyCheck() string {
	return strings.Join([]string{
		`test -d /projects/workspace/.git`,
		s.BinaryReadyCheck(),
	}, " && ")
}

func (s RuntimeSpec) BinaryReadyCheck() string {
	return strings.Join([]string{
		s.resolveBinaryShell("KAGEN_AGENT_BIN"),
		`test -n "$KAGEN_AGENT_BIN"`,
		`test -x "$KAGEN_AGENT_BIN"`,
	}, " && ")
}

func (s RuntimeSpec) BinaryPreflightCheck() string {
	return strings.Join([]string{
		s.BinaryReadyCheck(),
		`printf '%s' "$KAGEN_AGENT_BIN"`,
	}, " && ")
}

func (s RuntimeSpec) StateRoot() string {
	switch s.Type {
	case Codex:
		return defaultHomeDir + "/.codex"
	case Claude:
		return defaultHomeDir + "/.claude"
	case OpenCode:
		return defaultHomeDir + "/.opencode"
	default:
		return defaultHomeDir
	}
}

func (s RuntimeSpec) RequiredEnvMap() map[string]string {
	env := make(map[string]string, len(s.RequiredEnv))
	for _, variable := range s.RequiredEnv {
		env[variable.Name] = variable.Value
	}
	return env
}

// AttachShellForStatePath returns an attach shell that isolates runtime state
// under the provided per-agent-session path where supported.
func (s RuntimeSpec) AttachShellForStatePath(statePath string) string {
	trimmedStatePath := strings.TrimSpace(statePath)
	env := s.RequiredEnvMap()
	exports := []string{}
	if trimmedStatePath == "" {
		if homePath, ok := env["HOME"]; ok {
			exports = append(exports, fmt.Sprintf("mkdir -p %q", homePath))
		}
	} else {
		env["HOME"] = trimmedStatePath
		if s.Type == Codex {
			env["CODEX_HOME"] = trimmedStatePath
		}
		exports = append(exports, fmt.Sprintf("mkdir -p %q", trimmedStatePath))
	}
	for _, variable := range s.RequiredEnv {
		value, ok := env[variable.Name]
		if !ok {
			continue
		}

		exports = append(exports, fmt.Sprintf("export %s=%q", variable.Name, value))
		delete(env, variable.Name)
	}
	if homePath, ok := env["HOME"]; ok {
		exports = append(exports, fmt.Sprintf("export HOME=%q", homePath))
		delete(env, "HOME")
	}
	if codexHome, ok := env["CODEX_HOME"]; ok {
		exports = append(exports, fmt.Sprintf("export CODEX_HOME=%q", codexHome))
		delete(env, "CODEX_HOME")
	}
	for name, value := range env {
		exports = append(exports, fmt.Sprintf("export %s=%q", name, value))
	}

	return strings.Join(append(exports, s.resolveBinaryShell("KAGEN_AGENT_BIN"), s.AttachShell), " && ")
}

// SupportedTypes returns the supported runtime identifiers.
func SupportedTypes() []Type {
	return []Type{Claude, Codex, OpenCode}
}

// SupportedNames returns the display names for interactive prompts.
func SupportedNames() []string {
	names := []string{"Claude", "Codex", "OpenCode"}
	slices.Sort(names)
	return names
}

func npmBootstrap(binary, npmPackage string) runtimeBootstrap {
	return runtimeBootstrap{
		command: []string{"/bin/sh", "-lc"},
		args: []string{fmt.Sprintf(`set -eu
export DEBIAN_FRONTEND=noninteractive
if ! command -v git >/dev/null 2>&1; then
  apt-get update
  apt-get install -y --no-install-recommends git ca-certificates curl ripgrep procps
  rm -rf /var/lib/apt/lists/*
fi
if ! command -v %s >/dev/null 2>&1; then
  npm install -g %s
fi
exec tail -f /dev/null`, binary, npmPackage)},
	}
}

func npmBootstrapLatest(binary, npmPackage string) runtimeBootstrap {
	return runtimeBootstrap{
		command: []string{"/bin/sh", "-lc"},
		args: []string{fmt.Sprintf(`set -eu
export DEBIAN_FRONTEND=noninteractive
if ! command -v git >/dev/null 2>&1; then
  apt-get update
  apt-get install -y --no-install-recommends git ca-certificates curl ripgrep procps
  rm -rf /var/lib/apt/lists/*
fi
npm install -g %s@latest
if ! command -v %s >/dev/null 2>&1; then
  echo "%s was not found after npm bootstrap" >&2
  exit 1
fi
exec tail -f /dev/null`, npmPackage, binary, binary)},
	}
}

func cloneStrings(values []string) []string {
	return append([]string(nil), values...)
}

func (s RuntimeSpec) resolveBinaryShell(variableName string) string {
	lines := []string{
		fmt.Sprintf(`export %s=""`, variableName),
		fmt.Sprintf(`if resolved=$(command -v %s 2>/dev/null); then export %s="$resolved"; fi`, shellQuote(s.Binary), variableName),
	}
	for _, candidate := range s.binaryCandidates() {
		lines = append(lines, fmt.Sprintf(`if [ -z "$%s" ] && [ -x %q ]; then export %s=%q; fi`, variableName, candidate, variableName, candidate))
	}

	return strings.Join(lines, "; ")
}

func (s RuntimeSpec) binaryCandidates() []string {
	return []string{
		"/opt/mise/shims/" + s.Binary,
		"/opt/mise/bin/" + s.Binary,
		"/usr/local/bin/" + s.Binary,
		"/usr/bin/" + s.Binary,
		"/bin/" + s.Binary,
	}
}

func shellQuote(value string) string {
	return fmt.Sprintf("%q", value)
}
