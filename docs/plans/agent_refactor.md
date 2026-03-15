# Agent Package Refactoring Plan

## Objective

Restructure `internal/agent` package to use interface-based polymorphism instead of switch statements on agent types, creating clear boundaries between shared infrastructure and agent-specific implementations, and fully achieving the Open/Closed Principle.

## Current Problems

1. **Scattered Type Switches**: Adding a new agent requires editing 6+ locations across 3 files (`runtime_spec.go`, `real_agent.go`, `registry.go`)
2. **Bloated RuntimeSpec Struct**: Union of all agent data with every field used by only 1-2 agents
3. **Open/Closed Principle Violation**: Core files must be modified to add new agent types
4. **Unclear Boundaries**: Agent-specific configuration logic (e.g., OpenCode's config file writing) mixed with shared orchestration code
5. **Coupled Shell Logic**: Raw shell script construction is heavily mixed with generic agent-specific data.

## Target Architecture

### File Structure

```text
internal/agent/
├── agent.go              # Agent interface and Type constants (shared)
├── runtime_spec.go       # Declarative RuntimeSpec interface definition (shared)
├── base_agent.go         # baseAgent struct with shell construction & lifecycle methods (shared)
├── registry.go           # Dynamic agent registry using init() registration (shared)
├── claude.go             # claudeRuntimeSpec + init() registration
├── codex.go              # codexRuntimeSpec + init() registration
├── opencode.go           # openCodeRuntimeSpec + Configure logic + init() registration
└── gemini.go             # (future) geminiRuntimeSpec + init() registration
```

### Design Boundaries

**Shared Infrastructure (`base_agent.go`, `registry.go`, `runtime_spec.go`):**
- Generic pod lifecycle management (`Launch`, `Attach`, `Prepare`) encapsulated in a default `baseAgent` that accepts a `RuntimeSpec`.
- A mostly declarative `RuntimeSpec` interface (e.g., `RequiredEnv()`, `Binary()`, `AttachShell()`) replacing the struct.
- Translation of declarative info into Kubernetes exec shell structures encapsulated in `baseAgent` or dedicated builders, keeping specific agents ignorant of raw shell manipulation.
- Dynamic agent registry utilizing `init()` registration via a shared registry map (true Open/Closed conformity).

**Agent-Specific Code (`${agent}.go` files):**
- Implementation of the `RuntimeSpec` interface to provide data (environment variables, binary names).
- Optional `Configure(ctx context.Context, namespace, containerName string, exec kubeexec.Runner) error` method for specific file generation (e.g., OpenCode configs) instead of switch statements.
- `init()` function to register a `baseAgent` configured with the specific `RuntimeSpec` into the application runtime.

## Why This Structure

1. **Single File Per Agent**: Each agent's behavior is fully self-contained in one file. A developer can read `opencode.go` and see exact deviations without tracing switch conditionals.
2. **Interface-Driven Design**: The `RuntimeSpec` interface is purely the contract. An agent provides the data it requires, and shared logic converts that data into an executable state.
3. **No Redundant Structs**: Instead of distinct wrappers like `claudeAgent`, instances of `baseAgent` handle identical lifecycle mechanics, only differentiating by the injected `RuntimeSpec`.
4. **True Open/Closed Principle (`init()` Registration)**: Registering agents dynamically ensures that adding or removing an agent requires exactly **0** modifications to shared registry or runtime code.
    - *Safeguard*: The registration function will `panic` if a `Type` is already registered, catching developer errors (like copy-paste collisions) at startup.
5. **Introspection for Visibility**: The `Registry` will include a `RegisteredAgents() []Type` method. This restores the visibility lost by moving to distributed files, allowing the CLI (e.g., `agent --help`) and debug logs to easily list supported runtimes.
6. **Template for Future Agents**: `claude.go` acts as a pure reference point.

## Success Criteria

After refactoring:

- [ ] Adding a new agent requires creating exactly one new file and touching zero shared files
- [ ] Zero switch statements on `Type` in shared infrastructure code (e.g., no `configureAgent` switch map)
- [ ] `registry.go` dynamically registers configurations via `init()`
- [ ] `base_agent.go` knows absolutely nothing about Claude, Codex, or OpenCode constraints
- [ ] Existing tests continue to pass with minimal modification

## Non-Goals

This refactoring does NOT:
- Change the `Agent` interface contract (`agent.go` remains stable)
- Modify external APIs or CLI behavior
- Add new agent functionality (Gemini CLI support is explicitly a follow-up task)
- Move tests out of package (tests stay in `*_test.go` files, though setup functions may adjust)

## Future Work

Once this refactoring is complete, adding Gemini CLI support becomes:
1. Create `gemini.go` implementing `RuntimeSpec` interface
2. Add an `init()` block inside `gemini.go` registering the new agent
3. Create `gemini_test.go` with agent-specific test cases

No other files require modification.
