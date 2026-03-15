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

import (
	"context"

	"github.com/pejas/kagen/internal/kubeexec"
)

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
		// Example:
		// {Name: "MYAGENT_CONFIG_DIR", Value: ".myagent", Source: EnvValueSessionPathJoin},
	}
}

// Configure executes early in the lifecycle before attach.
// Ensure this returns nil if no configuration is needed.
func (s myAgentRuntimeSpec) Configure(ctx context.Context, namespace, containerName string, exec kubeexec.Runner) error {
	return nil
}

// 3. Register the agent inside the package init()
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

## 2. Configuration Logic (Optional)

If the agent requires a configuration file generated inside the container before attach (such as OpenCode disabling telemetry prompts), implement the `Configure` method.

Prefer declarative env resolution first. If a tool can be pointed at a session-specific path via `RequiredEnv()`, use that mechanism before adding imperative filesystem setup in `Configure`.

Do not shell out via `os/exec`. You must use the provided `kubeexec.Runner` to run shell commands inside the running pod.

See `internal/agent/opencode.go` for an example of generating JSON configurations natively in bash.

## 3. Testing

Create `internal/agent/{agent_name}_test.go` to verify the `RuntimeSpec` logic purely with unit tests. There is no need to write end-to-end integration tests for standard agent configurations.

```go
package agent

import "testing"

func TestMyAgentStateRoot(t *testing.T) {
	spec := myAgentRuntimeSpec{}
	if got := spec.StateRoot(); got != "/home/kagen/.myagent" {
		t.Errorf("StateRoot() = %q, want /home/kagen/.myagent", got)
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
- [ ] `RuntimeSpec` fully implemented.
- [ ] Registered via `registerBaseAgent` in `init()`, unless a custom `Agent` implementation is required.
- [ ] Session-scoped env vars use the correct `EnvValueSource`.
- [ ] No agent-specific conditionals were added to shared infrastructure.
- [ ] `Configure()` returns `nil` if no filesystem setup is required.
- [ ] Tested via `{agent_name}_test.go`.
- [ ] Project tests pass (`make test`).
