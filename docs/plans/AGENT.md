# Stage 1 — Agent Support

Status: draft

## Goal

Replace the stub `Agent` implementations with concrete agent definitions for Claude, Codex, and OpenCode. Each agent must define its container image, authentication mechanism, and TUI attach strategy.

## Dependencies

This stage has no dependencies on other stages. It defines the agent contracts that later stages (Cluster, Session) consume.

## Scope

### Agent Specification Model

Introduce an `AgentSpec` struct that captures everything the cluster layer needs to deploy and connect to an agent:

```go
type AgentSpec struct {
    Image       string            // container image reference
    Command     []string          // entrypoint override, if any
    Env         []EnvVar          // environment variables (auth tokens, config)
    Ports       []PortSpec        // ports the agent TUI listens on
    AuthFlow    AuthFlow          // how the agent authenticates
    VolumeMounts []VolumeMount    // persistent paths (auth state, config)
}
```

### Per-Agent Definitions

#### Claude

- **Image:** `ghcr.io/anthropics/claude-code:latest` (or a pinned version).
- **Auth flow:** OAuth via browser. The user is directed to an Anthropic URL; on success, tokens are written to `~/.claude/`. Alternatively, an `ANTHROPIC_API_KEY` environment variable bypasses OAuth entirely.
- **Persisted state:** `~/.claude/` mounted from a PVC so authentication survives pod restarts.
- **TUI attach:** `kubectl exec -it` into the running pod, connecting stdin/stdout to the Claude Code process.

#### Codex

- **Image:** Built from `openai/codex-universal` base image with the Codex CLI installed (`npm install -g @openai/codex`).
- **Auth flow:** API key only. The user sets `OPENAI_API_KEY` via `kagen` config or environment variable. No browser-based flow.
- **Persisted state:** Minimal. API key injected via environment variable from a Kubernetes Secret sourced from the auth PVC.
- **TUI attach:** Same `kubectl exec -it` strategy.

#### OpenCode

- **Image:** Custom Dockerfile based on a lightweight Go base. OpenCode is a single Go binary; the image pulls a release binary or builds from source.
- **Auth flow:** Provider-dependent. OpenCode supports multiple backends (OpenAI, Anthropic, Gemini, local). The user configures the provider and API key in `~/.config/opencode/config.json`. No browser-based flow for API-key providers.
- **Persisted state:** `~/.config/opencode/` mounted from the auth PVC.
- **TUI attach:** Same `kubectl exec -it` strategy.

### Authentication Orchestration

Expand the `Agent.Authenticate` method to handle two paths:

1. **API key path** — Check if the required key is available in config or environment. If present, write it into the cluster Secret and return.
2. **OAuth path** (Claude only) — Open the browser to the provider's OAuth URL. Poll or wait for the token file to appear on the auth PVC. Fail with a clear message if the flow times out.

The root command already calls `Authenticate` before `Launch`. No changes to the orchestration flow are needed.

### TUI Attach Strategy

All three agents use the same attach mechanism:

1. Identify the agent pod in the repo-scoped namespace.
2. Execute `kubectl exec -it <pod> -- <shell-or-entrypoint>`.
3. Forward the local terminal's stdin/stdout/stderr to the pod process.
4. On detach (user exits the TUI), return control to `kagen` for exit handling.

This logic should live in a shared `attach.go` inside `internal/agent/`, not duplicated per agent.

## Files

| Action | Path |
|--------|------|
| Modify | `internal/agent/agent.go` — add `AgentSpec`, `AuthFlow`, `EnvVar`, `PortSpec`, `VolumeMount` types |
| New    | `internal/agent/claude.go` — Claude agent implementation |
| New    | `internal/agent/codex.go` — Codex agent implementation |
| New    | `internal/agent/opencode.go` — OpenCode agent implementation |
| New    | `internal/agent/attach.go` — shared TUI attach logic |
| Modify | `internal/agent/registry.go` — replace stubs with real agent constructors |
| Modify | `internal/config/config.go` — add agent-specific config fields (API keys, OAuth settings) |
| New    | `internal/agent/claude_test.go` |
| New    | `internal/agent/codex_test.go` |
| New    | `internal/agent/opencode_test.go` |
| New    | `internal/agent/attach_test.go` |

## Verification

- Unit tests for each agent's `AgentSpec` generation (image, env, volumes).
- Unit tests for `AuthFlow` resolution (API key present → skip OAuth, API key missing + Claude → OAuth).
- Integration test: given a mock kubeconfig, verify `attach.go` constructs the correct `kubectl exec` command.
- Manual: run `kagen --agent claude` and confirm it prints the expected auth flow steps before failing at the cluster layer (which is not yet implemented).
