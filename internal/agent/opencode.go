package agent

import "path/filepath"

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

func (s openCodeRuntimeSpec) ConfigFiles() []ConfigFile {
	return []ConfigFile{
		{
			Name:      "opencode.json",
			MountPath: filepath.Join(s.StateRoot(), ".config", "opencode.json"),
			Content: `{
  "$schema": "https://opencode.ai/config.json",
  "permission": "allow"
}
`,
		},
	}
}

func init() {
	registerBaseAgent(openCodeRuntimeSpec{})
}
