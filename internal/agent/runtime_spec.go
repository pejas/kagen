package agent

import (
	"fmt"
	"slices"

	kagerr "github.com/pejas/kagen/internal/errors"
)

const defaultHomeDir = "/home/kagen"

// EnvVar defines a required environment variable for a runtime.
type EnvVar struct {
	Name  string
	Value string
}

// RuntimeSpec describes how kagen bootstraps and attaches to an agent runtime.
type RuntimeSpec struct {
	Type          Type
	DisplayName   string
	GitAuthorName string
	Binary        string
	NPMPackage    string
	AttachShell   string
	RequiredEnv   []EnvVar
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
			NPMPackage:    "@anthropic-ai/claude-code",
			AttachShell:   "cd /projects/workspace && exec claude",
			RequiredEnv: []EnvVar{
				{Name: "HOME", Value: defaultHomeDir},
				{Name: "CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC", Value: "1"},
			},
		}, nil
	case Codex:
		return RuntimeSpec{
			Type:          Codex,
			DisplayName:   "Codex",
			GitAuthorName: "oai-codex",
			Binary:        "codex",
			NPMPackage:    "@openai/codex",
			AttachShell:   "cd /projects/workspace && exec codex --sandbox danger-full-access -a never",
			RequiredEnv: []EnvVar{
				{Name: "HOME", Value: defaultHomeDir},
				{Name: "CODEX_HOME", Value: defaultHomeDir + "/.codex"},
			},
		}, nil
	case OpenCode:
		return RuntimeSpec{
			Type:          OpenCode,
			DisplayName:   "OpenCode",
			GitAuthorName: "opencode",
			Binary:        "opencode",
			NPMPackage:    "opencode-ai",
			AttachShell:   "cd /projects/workspace && exec opencode",
			RequiredEnv: []EnvVar{
				{Name: "HOME", Value: defaultHomeDir},
			},
		}, nil
	default:
		return RuntimeSpec{}, fmt.Errorf("runtime spec for %s: %w", agentType, kagerr.ErrAgentUnknown)
	}
}

func (s RuntimeSpec) ContainerName() string {
	return "kagen-agent-" + string(s.Type)
}

func (s RuntimeSpec) BootstrapCommand() []string {
	return []string{"/bin/sh", "-lc"}
}

func (s RuntimeSpec) BootstrapArgs() []string {
	return []string{fmt.Sprintf(`set -eu
export DEBIAN_FRONTEND=noninteractive
if ! command -v git >/dev/null 2>&1; then
  apt-get update
  apt-get install -y --no-install-recommends git ca-certificates curl ripgrep procps
  rm -rf /var/lib/apt/lists/*
fi
if ! command -v %s >/dev/null 2>&1; then
  npm install -g %s
fi
exec tail -f /dev/null`, s.Binary, s.NPMPackage)}
}

func (s RuntimeSpec) ReadyCheck() string {
	return fmt.Sprintf("test -d /projects/workspace/.git && command -v %s >/dev/null 2>&1", s.Binary)
}

func (s RuntimeSpec) RequiredEnvMap() map[string]string {
	env := make(map[string]string, len(s.RequiredEnv))
	for _, variable := range s.RequiredEnv {
		env[variable.Name] = variable.Value
	}
	return env
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
