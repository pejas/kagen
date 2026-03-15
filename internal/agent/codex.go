package agent

import (
	"context"

	"github.com/pejas/kagen/internal/kubeexec"
)

type codexRuntimeSpec struct{}

func (codexRuntimeSpec) Type() Type {
	return Codex
}

func (codexRuntimeSpec) DisplayName() string {
	return "Codex"
}

func (codexRuntimeSpec) GitAuthorName() string {
	return "oai-codex"
}

func (codexRuntimeSpec) Binary() string {
	return "codex"
}

func (codexRuntimeSpec) AttachShell() string {
	return `cd /projects/workspace && exec "$KAGEN_AGENT_BIN" --sandbox danger-full-access -a never`
}

func (codexRuntimeSpec) StateRoot() string {
	return DefaultHomeDir() + "/.codex"
}

func (codexRuntimeSpec) RequiredEnv() []EnvVar {
	return []EnvVar{
		{Name: "HOME", Value: DefaultHomeDir(), Source: EnvValueSessionRoot},
		{Name: "PATH", Value: defaultToolboxPath, Source: EnvValueLiteral},
		{Name: "TERM", Value: defaultTerm, Source: EnvValueLiteral},
		{Name: "CODEX_HOME", Value: DefaultHomeDir() + "/.codex", Source: EnvValueSessionRoot},
	}
}

func (codexRuntimeSpec) Configure(context.Context, string, string, kubeexec.Runner) error {
	return nil
}

func init() {
	registerBaseAgent(codexRuntimeSpec{})
}
