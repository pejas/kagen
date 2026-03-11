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
- `internal/runtime`: Manages Colima lifecycle.
- `internal/cluster`: Handles Kubernetes resource generation (Namespaces, Pods, PVCs).
- `internal/forgejo`: Orchestrates in-cluster Forgejo for repository isolation.
- `internal/git`: Discovery and synchronization logic (Host <-> Cluster).
- `internal/agent`: Agent-specific execution and injection logic.

## Essential Commands
- **Build**: `make build` (outputs to `./bin/kagen`)
- **Test**: `make test` (runs all package tests with race detector)
- **Init**: `./bin/kagen init` (bootstraps `devfile.yaml`)

## Coding Conventions
1. **Errors**: Use `internal/errors` (e.g., `kagerr.ErrNotGitRepo`). Use `fmt.Errorf("...: %w", err)` for wrapping.
2. **UI**: Use `internal/ui` for all terminal output (Info, Success, Warn, Error).
3. **Stubs**: Maintain the stub implementation pattern in `stub.go` files for infrastructure packages until fully realized.
4. **Devfile-First**: The `devfile.yaml` is the source of truth for the cluster environment.

## Agent Checklist
- [ ] Always run `make test` before proposing a fix.
- [ ] Ensure `internal/` packages do not expose unnecessary public APIs.
- [ ] Follow Oxford spelling (British English) in documentation.
- [ ] Use `client-go` for K8s interactions; avoid shell-ing out to `kubectl` except for `exec` TUIs and port-forwarding.

## Documentation Map
- [ARCHITECTURE.md](file:///Users/pejas/Projects/kagen/docs/ARCHITECTURE.md): Deep dive into the system design.
- [CONVENTIONS.md](file:///Users/pejas/Projects/kagen/docs/CONVENTIONS.md): Detailed Go coding standards.
- [README.md](file:///Users/pejas/Projects/kagen/README.md): Human-focused overview and installation.
