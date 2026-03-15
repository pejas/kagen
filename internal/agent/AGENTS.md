# Adding a New Agent

This guide specifies how to implement and register a new AI agent runtime in `kagen`.

## Architecture Overview

Adding a new agent requires creating exactly **one** new file: `internal/agent/{agent_name}.go`.

The `kagen` CLI uses an interface-driven design for agents. Shared pod lifecycle logic (launching, attaching, state directory creation) is encapsulated within a shared `baseAgent` type. Specific agents only provide their unique configuration data by implementing the `RuntimeSpec` interface and registering that spec during package initialization.

Session-scoped environment behaviour must be declared in `RequiredEnv()`. Do not add agent-specific conditionals to shared infrastructure to rewrite environment variables during attach.

## 1. Create the Agent File

Create `internal/agent/{agent_name}.go` with the following skeleton:

```go
package agent

import "path/filepath"

// 1. Define the unique Type identifier
const MyAgent Type = "myagent"

// 2. Implement the declarative RuntimeSpec interface
type myAgentRuntimeSpec struct{}

func (s myAgentRuntimeSpec) Type() Type            { return MyAgent }
func (s myAgentRuntimeSpec) DisplayName() string   { return "MyAgent" }
func (s myAgentRuntimeSpec) GitAuthorName() string { return "myagent" }
func (s myAgentRuntimeSpec) Binary() string        { return "myagent" }
func (s myAgentRuntimeSpec) AttachShell() string   { return `cd /projects/workspace && exec "$KAGEN_AGENT_BIN"` }
func (s myAgentRuntimeSpec) StateRoot() string     { return DefaultHomeDir() + "/.myagent" }
func (s myAgentRuntimeSpec) RequiredEnv() []EnvVar {
	return []EnvVar{
		{Name: "HOME", Value: DefaultHomeDir(), Source: EnvValueSessionRoot},
		{Name: "PATH", Value: defaultToolboxPath, Source: EnvValueLiteral},
		{Name: "TERM", Value: defaultTerm, Source: EnvValueLiteral},
		// Declare agent-specific environment variables here
	}
}

// 3. Return configuration files to mount as ConfigMaps (return nil if none needed)
func (s myAgentRuntimeSpec) ConfigFiles() []ConfigFile {
	return nil // See opencode.go for an example with config files
}

// 4. Register the agent inside the package init()
func init() {
	registerBaseAgent(myAgentRuntimeSpec{})
}
```

*Note: `registerBaseAgent` wires the shared lifecycle implementation to the agent-specific `RuntimeSpec`. Use `RegisterFactory` directly only when an agent needs custom lifecycle behaviour that `baseAgent` cannot express.*

### Environment Value Sources

Each `EnvVar` declares both a default value and how attach-time session isolation should treat it.

- `EnvValueLiteral`: export `Value` unchanged.
- `EnvValueSessionRoot`: export the isolated session root when `statePath` is set; otherwise export the declared default `Value`.
- `EnvValueSessionPathJoin`: export `path.Join(statePath, Value)` when `statePath` is set; otherwise export the declared default `Value`.

Use `EnvValueSessionRoot` for variables such as `HOME` or `CODEX_HOME` that should point directly at the session root. Use `EnvValueSessionPathJoin` for config directories or files that should live beneath the session root. Keep unrelated variables such as `PATH` and `TERM` as `EnvValueLiteral`.

## 2. Configuration Files (ConfigMap)

If the agent requires configuration files (such as OpenCode's `opencode.json`), return them from `ConfigFiles()`. These are mounted as Kubernetes ConfigMaps before pod startup - no runtime exec calls needed.

```go
func (s myAgentRuntimeSpec) ConfigFiles() []ConfigFile {
	return []ConfigFile{
		{
			Name:      "config.json",                                    // ConfigMap key
			MountPath: filepath.Join(s.StateRoot(), ".config", "config.json"), // Absolute path
			Content: `{
  "setting": "value"
}
`,
		},
	}
}
```

The `MountPath` must be an absolute path. Use `filepath.Join` with `s.StateRoot()` for agent-scoped paths. ConfigMaps are mounted read-only using `subPath` to avoid overwriting parent directories.

See `internal/agent/opencode.go` for a complete example.

## 3. Testing

Create `internal/agent/{agent_name}_test.go` to verify the `RuntimeSpec` logic purely with unit tests. There is no need to write end-to-end integration tests for standard agent configurations.

```go
package agent

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestMyAgentConfigFiles(t *testing.T) {
	spec := myAgentRuntimeSpec{}
	configFiles := spec.ConfigFiles()
	
	// If your agent needs config files:
	if len(configFiles) > 0 {
		cf := configFiles[0]
		if cf.Name != "config.json" {
			t.Errorf("ConfigFile.Name = %q, want config.json", cf.Name)
		}
		if !filepath.IsAbs(cf.MountPath) {
			t.Errorf("ConfigFile.MountPath = %q, want absolute path", cf.MountPath)
		}
	}
}
```

## 4. Run Verification

Always run the full unit test suite to ensure the registration does not panic (e.g., due to duplicate names) and no regressions occurred.

```bash
make test
```

## Reference Checklist

- [ ] File named `{agent_name}.go` created.
- [ ] `Type` constant exported.
- [ ] `RuntimeSpec` fully implemented (including `ConfigFiles()`).
- [ ] Registered via `registerBaseAgent` in `init()`, unless a custom `Agent` implementation is required.
- [ ] Session-scoped env vars use the correct `EnvValueSource`.
- [ ] No agent-specific conditionals were added to shared infrastructure.
- [ ] `ConfigFiles()` returns `nil` if no configuration files are needed.
- [ ] Tested via `{agent_name}_test.go`.
- [ ] Project tests pass (`make test`).
