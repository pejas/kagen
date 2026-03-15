package agent

import (
	"context"
	"fmt"

	"github.com/pejas/kagen/internal/kubeexec"
)

type openCodeRuntimeSpec struct{}

func (openCodeRuntimeSpec) Type() Type {
	return OpenCode
}

func (openCodeRuntimeSpec) DisplayName() string {
	return "OpenCode"
}

func (openCodeRuntimeSpec) GitAuthorName() string {
	return "opencode"
}

func (openCodeRuntimeSpec) Binary() string {
	return "opencode"
}

func (openCodeRuntimeSpec) AttachShell() string {
	return `cd /projects/workspace && exec "$KAGEN_AGENT_BIN"`
}

func (openCodeRuntimeSpec) StateRoot() string {
	return DefaultHomeDir() + "/.opencode"
}

func (openCodeRuntimeSpec) RequiredEnv() []EnvVar {
	return []EnvVar{
		{Name: "HOME", Value: DefaultHomeDir(), Source: EnvValueSessionRoot},
		{Name: "PATH", Value: defaultToolboxPath, Source: EnvValueLiteral},
		{Name: "TERM", Value: defaultTerm, Source: EnvValueLiteral},
	}
}

func (s openCodeRuntimeSpec) Configure(ctx context.Context, namespace, containerName string, exec kubeexec.Runner) error {
	configDir := s.StateRoot() + "/.config"
	configPath := configDir + "/opencode.json"

	checkCmd := []string{"/bin/sh", "-lc", fmt.Sprintf("test -f %s", configPath)}
	if _, err := exec.Run(ctx, namespace, "agent", checkCmd, kubeexec.WithContainer(containerName)); err == nil {
		return nil
	}

	mkdirCmd := []string{"/bin/mkdir", "-p", configDir}
	if _, err := exec.Run(ctx, namespace, "agent", mkdirCmd, kubeexec.WithContainer(containerName)); err != nil {
		return fmt.Errorf("creating opencode config directory: %w", err)
	}

	configContent := `{
  "$schema": "https://opencode.ai/config.json",
  "permission": "allow"
}
`
	writeCmd := []string{"/bin/sh", "-lc", fmt.Sprintf("cat > %s << 'EOF'\n%sEOF", configPath, configContent)}
	if _, err := exec.Run(ctx, namespace, "agent", writeCmd, kubeexec.WithContainer(containerName)); err != nil {
		return fmt.Errorf("writing opencode config: %w", err)
	}

	return nil
}

func init() {
	registerBaseAgent(openCodeRuntimeSpec{})
}
