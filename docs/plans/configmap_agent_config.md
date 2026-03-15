# Agent ConfigMap Configuration

## Objective

Replace runtime `exec`-based config file creation with Kubernetes ConfigMap volumes, eliminating shell commands from `Configure` method and making configuration declarative.

## Current Problems

1. **Three sequential exec calls** over network (test, mkdir, write)
2. **Shell-heavy logic** in Go code (`/bin/sh -lc`, heredocs)
3. **No path.Join** - manual string concatenation
4. **Test-then-act race condition** - config could appear between test and write
5. **Configure method is anti-pattern** - Kubernetes-native approach should use ConfigMaps

## Target Architecture

### RuntimeSpec Interface Extension

Add `ConfigFiles()` method returning files to mount as ConfigMaps:

```go
type ConfigFile struct {
    Name string    // e.g., "opencode.json"
    MountPath string// e.g., "/home/kagen/.config/opencode.json"
    Content string// file content
}

type RuntimeSpec interface {
    // ... existing methods
    ConfigFiles() []ConfigFile
}
```

### Implementation Flow

1. **OpenCode declares config**: `ConfigFiles()` returns `opencode.json` content
2. **Other agents**: Return `nil` or empty slice (no config files)
3. **Cluster creates ConfigMaps**: Before pod creation, in `EnsureResources`
4. **Pod spec mounts volumes**: Added during pod injection in `cluster/kube.go`

### File Changes

**internal/agent/runtime_spec.go**
- Add `ConfigFile` struct
- Add `ConfigFiles() []ConfigFile` method to `RuntimeSpec` interface

**internal/agent/opencode.go**
- Implement `ConfigFiles()` returning the JSON config
- Remove `Configure()` method (ConfigMap supersedes it)

**internal/agent/claude.go, codex.go**
- Implement `ConfigFiles()` returning `nil` (no config files)

**internal/cluster/kube.go**
- Add `ensureAgentConfigMaps()` method (follow `ensureProxyConfig` pattern)
- Call it in `EnsureResources()` before pod creation
- Add ConfigMap volume injection with `subPath` in `injectAgentRuntime()`

**internal/agent/base_agent.go**
- Remove `Configure()` call from `Prepare()` method

## Why This Structure

1. **Declarative**: Config is defined in code, mounted as K8s resource
2. **Zero runtime exec**: No shell commands, no network latency
3. **Kubernetes-native**: Uses ConfigMap pattern already established for proxy
4. **GitOps-ready**: Config content is version-controlled in agent code
5. **Atomic**: Config exists before container starts (no race conditions)
6. **Extensible**: Any agent can declare config files via `ConfigFiles()`

## ConfigMap Naming Convention

```
kagen-agent-config-{agent-type}
```

Example: `kagen-agent-config-opencode`

**Mount strategy:** Use `subPath` to mount single file without overwriting parent directory.

```yaml
volumeMounts:
  - name: opencode-config
    mountPath: /home/kagen/.config/opencode.json
    subPath: opencode.json
```

**Mount paths:** Use absolute paths for clarity (e.g., `/home/kagen/.config/opencode.json`).

## Success Criteria

After refactoring:

- [ ] `opencode.go` has no `Configure()` method
- [ ] `opencode.go` implements `ConfigFiles()` returning JSON config
- [ ] ConfigMap created before pod starts
- [ ] ConfigMap mounted as volume with subPath in agent container
- [ ] Zero exec calls for config setup
- [ ] All agent tests pass
- [ ] E2E tests pass with ConfigMap-based config

## Non-Goals

This refactoring does NOT:
- Support dynamic per-session config values (static config only)
- Modify external APIs or CLI behavior

## Implementation Order

1. Add `ConfigFile` struct to runtime_spec.go
2. Add `ConfigFiles() []ConfigFile` method to `RuntimeSpec` interface
3. Implement `ConfigFiles()` in opencode.go returning JSON config
4. Implement `ConfigFiles()` in claude.go and codex.go returning `nil`
5. Remove `Configure()` method from `RuntimeSpec` interface
6. Remove `Configure()` implementation from opencode.go
7. Remove `Configure()` call from base_agent.go `Prepare()` method
8. Add `ensureAgentConfigMaps()` in cluster/kube.go
9. Add volume mount injection with `subPath` in `injectAgentRuntime()`
10. Update tests
11. Run `make test` and `make test-e2e`
