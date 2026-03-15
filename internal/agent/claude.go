package agent

type claudeRuntimeSpec struct{}

func (claudeRuntimeSpec) Type() Type {
	return Claude
}

func (claudeRuntimeSpec) DisplayName() string {
	return "Claude"
}

func (claudeRuntimeSpec) GitAuthorName() string {
	return "claude"
}

func (claudeRuntimeSpec) Binary() string {
	return "claude"
}

func (claudeRuntimeSpec) AttachShell() string {
	return `cd /projects/workspace && exec "$KAGEN_AGENT_BIN"`
}

func (claudeRuntimeSpec) StateRoot() string {
	return DefaultHomeDir() + "/.claude"
}

func (claudeRuntimeSpec) RequiredEnv() []EnvVar {
	return []EnvVar{
		{Name: "HOME", Value: DefaultHomeDir(), Source: EnvValueSessionRoot},
		{Name: "PATH", Value: defaultToolboxPath, Source: EnvValueLiteral},
		{Name: "TERM", Value: defaultTerm, Source: EnvValueLiteral},
		{Name: "CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC", Value: "1", Source: EnvValueLiteral},
	}
}

func (claudeRuntimeSpec) ConfigFiles() []ConfigFile {
	return nil
}

func init() {
	registerBaseAgent(claudeRuntimeSpec{})
}
