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
- `internal/cluster`: Generates Kubernetes resources from `devfile.yaml`; shared adapters for port-forward and (planned) exec.
- `internal/forgejo`: Reconciles Forgejo deployment/service and handles repo sync via shared adapters.
- `internal/git`: Discovery and synchronisation logic (Host <-> Cluster).
- `internal/agent`: Agent lookup plus attach/launch built on shared exec/port-forward adapters.
- `internal/proxy`: Proxy allowlist policy (validate before attach; proxy pod reconciliation to follow).

## Essential Commands
- **Build**: `make build` (outputs to `./bin/kagen`)
- **Test**: `make test` (runs all package tests with race detector)
- **Init**: `./bin/kagen init` (bootstraps `devfile.yaml`)

## Coding Conventions
1. **Errors**: Use `internal/errors` (e.g., `kagerr.ErrNotGitRepo`). Use `fmt.Errorf("...: %w", err)` for wrapping.
2. **UI**: Use `internal/ui` for all terminal output (Info, Success, Warn, Error).
3. **Stubs**: Maintain the stub implementation pattern in `stub.go` files for infrastructure packages until fully realised.
4. **Devfile-First**: The `devfile.yaml` is the source of truth for the cluster environment.
5. **Orchestration Decomposition**: Keep `internal/cmd` thin; add coordinators for runtime, devfile validation, forgejo sync, and agent launch.
6. **Shared Adapters**: Centralise `kubectl` exec/port-forward in shared adapters; do not shell out elsewhere.
7. **Surface Area**: Export only interfaces and constructors; keep helpers unexported where feasible.
8. **Proxy Validation**: Validate proxy policy before agent attach; fail closed if unenforced.

## Agent Checklist
- [ ] Always run `make test` before proposing a fix.
- [ ] Ensure `internal/` packages do not expose unnecessary public APIs.
- [ ] Follow Oxford spelling (British English) in documentation.
- [ ] Use `client-go` for K8s interactions; avoid shell-ing out to `kubectl` except for `exec` TUIs and port-forwarding.
- [ ] Reuse shared port-forward/exec adapters; do not introduce new direct shell-outs to `kubectl`.

## Documentation Map
- [ARCHITECTURE.md](file:///Users/pejas/Projects/kagen/docs/ARCHITECTURE.md): Deep dive into the system design.
- [CONVENTIONS.md](file:///Users/pejas/Projects/kagen/docs/CONVENTIONS.md): Detailed Go coding standards.
- [README.md](file:///Users/pejas/Projects/kagen/README.md): Human-focused overview and installation.
