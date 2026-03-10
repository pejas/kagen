# Stage 1 — Agent Integration

Status: draft

## Goal

Replace the stub `Agent` implementations with logic that prepares the selected agent (Claude, Codex, or OpenCode) to run **within** the environment defined by the project's Devfile.

## Dependencies

- **Stage 3 (Devfile Orchestration)** — The agent execution relies on a Pod created from the Devfile.

## Scope

### Agent as a Tool

Instead of a standalone Pod, the Agent is now treated as a tool executed inside a container defined by the Devfile. `kagen` ensures:
1. The necessary binaries are available (either pre-installed in the image or injected via a shared volume).
2. The agent's persistent state (auth, config) is mounted from a dedicated PVC (`agent-auth`).

### Per-Agent Logic

#### Claude
- **Binary:** `claude-code`.
- **Injection:** If not in the Devfile image, `kagen` can inject it via an `initContainer` that copies the binary to a shared `/kagen/tools` volume.
- **Auth Flow:** OAuth via browser. Tokens are persisted in `/home/user/.claude/` (mapped to the `agent-auth` PVC).
- **Execution:** Runs as a process in the main Devfile container.

#### Codex
- **Binary:** `codex`.
- **Injection:** Similar to Claude.
- **Auth Flow:** API Key injected via `OPEN_AI_KEY` environment variable.
- **Execution:** Process in the main Devfile container.

#### OpenCode
- **Binary:** `opencode`.
- **Injection:** Similar to Claude.
- **Auth Flow:** Config file in `/home/user/.config/opencode/` (mapped to `agent-auth` PVC).
- **Execution:** Process in the main Devfile container.

### Shared Attach Logic

The `Attach` strategy remains a `kubectl exec -it` into the "main" container of the Devfile-defined Pod, specifically starting the agent's TUI process.

## Files

| Action | Path |
|--------|------|
| Modify | `internal/agent/agent.go` — Update interface to support Devfile-based injection |
| New    | `internal/agent/injection.go` — Logic to prepare agent binaries/tools for the cluster |
| New    | `internal/agent/claude.go` |
| New    | `internal/agent/codex.go` |
| New    | `internal/agent/opencode.go` |
| Modify | `internal/cmd/root.go` — Orchestrate agent injection before pod launch |
