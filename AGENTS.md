# Agent Guide (`AGENTS.md`)

Welcome, Agent. This document provides the essential context and actionable instructions for working on the `kagen` project.

## Project Overview
`kagen` is a security-first CLI for orchestrating isolated development environments. It uses Colima/K3s on macOS ARM to run agents (Claude, Codex, etc.) inside a container, with an in-cluster Forgejo instance acting as the source control boundary.

## Tech Stack
- **Language**: Go 1.23+ (Uber style)
- **CLI**: Cobra + Viper
- **Runtime**: Colima (K3s profile)
- **Orchestration**: Kubernetes (client-go)
- **Git**: Local `git` binary orchestration

## Core Components
- `internal/runtime`: Manages Colima lifecycle and exposes kube context.
- `internal/session`: Persists kagen sessions and agent sessions in the local SQLite-backed store.
- `internal/workload`: Builds the baseline runtime Pod from typed Go configuration.
- `internal/cluster`: Mutates and reconciles the generated Pod; shared adapters for port-forward and (planned) exec.
- `internal/forgejo`: Reconciles Forgejo deployment/service and handles repo sync via shared adapters.
- `internal/git`: Discovery and synchronisation logic (Host <-> Cluster).
- `internal/agent`: Agent lookup plus attach/launch built on shared exec/port-forward adapters.
- `internal/proxy`: Proxy allowlist policy (validate before attach; proxy pod reconciliation to follow).

## Essential Commands
- **Build**: `make build` (outputs to `./bin/kagen`)
- **Test**: `make test` (runs non-e2e package tests with race detector)
- **E2E Test**: `make test-e2e` (runs `internal/e2e`; use only when explicitly requested or when end-to-end validation is needed)
- **Config Write**: `./bin/kagen config write` (writes optional `.kagen.yaml` defaults only)
- **Start**: `./bin/kagen start <agent>` (creates a new kagen session and attaches an agent)
- **Attach**: `./bin/kagen attach <agent> [--session <id>]` (attaches a fresh agent session to a persisted kagen session)
- **List**: `./bin/kagen list` (shows persisted sessions for the current repository)
- **Down**: `./bin/kagen down` (shuts down the whole local runtime without deleting persisted sessions)

## Coding Conventions
1. **Errors**: Use `internal/errors` (e.g., `kagerr.ErrNotGitRepo`). Use `fmt.Errorf("...: %w", err)` for wrapping.
2. **UI**: Use `internal/ui` for all terminal output (Info, Success, Warn, Error).
3. **Stubs**: Maintain the stub implementation pattern in `stub.go` files for infrastructure packages until fully realised.
4. **Generated Runtime First**: `internal/workload` is the source of truth for the baseline runtime Pod; repository `devfile.yaml` files are legacy artefacts and not part of the active runtime flow.
5. **Orchestration Decomposition**: Keep `internal/cmd` thin; add coordinators for runtime, workload generation, session persistence, forgejo sync, and agent launch.
6. **Shared Adapters**: Centralise `kubectl` exec/port-forward in shared adapters; do not shell out elsewhere.
7. **Surface Area**: Export only interfaces and constructors; keep helpers unexported where feasible.
8. **Proxy Validation**: Validate proxy policy before agent attach; fail closed if unenforced.

## Language and Style
- Use precise, normative language in documentation, comments, plans, and review notes.
- Write in British English with Oxford spelling.
- Use `-ize` and `-ization` endings unless `-ise` is intrinsic to the word.
- Prefer direct, active voice.
- Avoid hype adjectives.
- Avoid vague qualifiers such as "as needed" or "etc." in normative statements.
- Keep sentences concise and technical.
- Describe the current contract, not the change history. Do not use release-note phrasing such as "now", "currently now", or similar transition markers unless a time comparison is required.

## Agent Checklist
- [ ] Always run `make test` before proposing a fix.
- [ ] Run `make test-e2e` only when the task explicitly calls for e2e validation or the change needs end-to-end verification.
- [ ] Ensure `internal/` packages do not expose unnecessary public APIs.
- [ ] Follow the documented language and style rules in all written material.
- [ ] Use `client-go` for K8s interactions; avoid shell-ing out to `kubectl` except for `exec` TUIs and port-forwarding.
- [ ] Reuse shared port-forward/exec adapters; do not introduce new direct shell-outs to `kubectl`.

## Documentation Map
- [INTERNALS-BLUEPRINT.md](file:///Users/pejas/Projects/kagen/docs/INTERNALS-BLUEPRINT.md): Quick command-flow mental model with ASCII diagrams.
- [ARCHITECTURE.md](file:///Users/pejas/Projects/kagen/docs/ARCHITECTURE.md): Deep dive into the system design.
- [CONVENTIONS.md](file:///Users/pejas/Projects/kagen/docs/CONVENTIONS.md): Detailed Go coding standards.
- [README.md](file:///Users/pejas/Projects/kagen/README.md): Human-focused overview and installation.
