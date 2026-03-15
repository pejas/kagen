package agent

import (
	"context"
	"fmt"
	"path"
	"strings"

	"github.com/pejas/kagen/internal/kubeexec"
)

const defaultHomeDir = "/home/kagen"
const defaultTerm = "xterm-256color"
const defaultToolboxPath = "/opt/mise/shims:/opt/mise/bin:/usr/local/bin:/usr/local/sbin:/usr/sbin:/usr/bin:/sbin:/bin"

// EnvVar defines a required environment variable for a runtime.
type EnvVar struct {
	Name   string
	Value  string
	Source EnvValueSource
}

// EnvValueSource describes how an environment variable value is resolved.
type EnvValueSource int

const (
	EnvValueLiteral EnvValueSource = iota
	EnvValueSessionRoot
	EnvValueSessionPathJoin
)

// RuntimeSpec describes how kagen bootstraps and attaches to an agent runtime.
type RuntimeSpec interface {
	Type() Type
	DisplayName() string
	GitAuthorName() string
	Binary() string
	AttachShell() string
	StateRoot() string
	RequiredEnv() []EnvVar
	Configure(ctx context.Context, namespace, containerName string, exec kubeexec.Runner) error
}

// DefaultHomeDir returns the shared runtime home directory inside the pod.
func DefaultHomeDir() string {
	return defaultHomeDir
}

// ContainerName returns the runtime container name for the agent.
func ContainerName(spec RuntimeSpec) string {
	return "kagen-agent-" + string(spec.Type())
}

// ReadyCheck validates that the workspace and runtime binary are ready.
func ReadyCheck(spec RuntimeSpec) string {
	return strings.Join([]string{
		`test -d /projects/workspace/.git`,
		BinaryReadyCheck(spec),
	}, " && ")
}

// BinaryReadyCheck validates that the runtime binary can be resolved.
func BinaryReadyCheck(spec RuntimeSpec) string {
	return strings.Join([]string{
		resolveBinaryShell(spec, "KAGEN_AGENT_BIN"),
		`test -n "$KAGEN_AGENT_BIN"`,
		`test -x "$KAGEN_AGENT_BIN"`,
	}, " && ")
}

// BinaryPreflightCheck validates the runtime binary and prints the resolved path.
func BinaryPreflightCheck(spec RuntimeSpec) string {
	return strings.Join([]string{
		BinaryReadyCheck(spec),
		`printf '%s' "$KAGEN_AGENT_BIN"`,
	}, " && ")
}

// AttachShellForStatePath returns an attach shell that isolates runtime state
// under the provided per-agent-session path where supported.
func AttachShellForStatePath(spec RuntimeSpec, statePath string) string {
	trimmedStatePath := strings.TrimSpace(statePath)
	exports := []string{}
	if trimmedStatePath != "" {
		exports = append(exports, fmt.Sprintf("mkdir -p %q", trimmedStatePath))
	}

	for _, variable := range resolveEnvVars(spec.RequiredEnv(), trimmedStatePath) {
		exports = append(exports, fmt.Sprintf("export %s=%q", variable.Name, variable.Value))
	}

	return strings.Join(append(exports, resolveBinaryShell(spec, "KAGEN_AGENT_BIN"), spec.AttachShell()), " && ")
}

func resolveEnvVars(variables []EnvVar, statePath string) []EnvVar {
	resolved := make([]EnvVar, 0, len(variables))
	seen := make(map[string]struct{}, len(variables))
	for _, variable := range variables {
		if _, exists := seen[variable.Name]; exists {
			panic(fmt.Sprintf("duplicate environment variable %q", variable.Name))
		}
		seen[variable.Name] = struct{}{}

		resolved = append(resolved, EnvVar{
			Name:  variable.Name,
			Value: resolveEnvValue(variable, statePath),
		})
	}

	return resolved
}

func resolveEnvValue(variable EnvVar, statePath string) string {
	if statePath == "" {
		return variable.Value
	}

	switch variable.Source {
	case EnvValueLiteral:
		return variable.Value
	case EnvValueSessionRoot:
		return statePath
	case EnvValueSessionPathJoin:
		return path.Join(statePath, variable.Value)
	default:
		panic(fmt.Sprintf("unknown environment value source %d for %q", variable.Source, variable.Name))
	}
}

func resolveBinaryShell(spec RuntimeSpec, variableName string) string {
	lines := []string{
		fmt.Sprintf(`export %s=""`, variableName),
		fmt.Sprintf(`if resolved=$(command -v %s 2>/dev/null); then export %s="$resolved"; fi`, shellQuote(spec.Binary()), variableName),
	}
	for _, candidate := range binaryCandidates(spec) {
		lines = append(lines, fmt.Sprintf(`if [ -z "$%s" ] && [ -x %q ]; then export %s=%q; fi`, variableName, candidate, variableName, candidate))
	}

	return strings.Join(lines, "; ")
}

func binaryCandidates(spec RuntimeSpec) []string {
	return []string{
		"/opt/mise/shims/" + spec.Binary(),
		"/opt/mise/bin/" + spec.Binary(),
		"/usr/local/bin/" + spec.Binary(),
		"/usr/bin/" + spec.Binary(),
		"/bin/" + spec.Binary(),
	}
}

func shellQuote(value string) string {
	return fmt.Sprintf("%q", value)
}
