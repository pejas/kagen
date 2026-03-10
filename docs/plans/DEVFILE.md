# Stage 1.5 — Devfile Orchestration

Status: draft

## Goal

Implement the ability to parse a `devfile.yaml` (v2.2.0) and translate it into a Kubernetes Pod definition that can run on the local K3s cluster.

## Dependencies

- **Stage 2 (Runtime)** — Requires a working K3s cluster to eventually run the generated manifests.

## Scope

### Devfile Parsing

Use a Go-based Devfile parser (e.g., `devfile/library-go`) to read the project's `devfile.yaml`. Kagen will support:
- `components`: Specifically `container` types.
- `volumes`: To be mapped to K8s PVCs.
- `env`: Environment variables for specific components.
- `commands`: (Optional) To define how the agent should trigger specific builds or tests.

### K8s Manifest Generation

Create a generator that maps Devfile components to a single Kubernetes Pod:
- **Main Container:** The primary container where the agent will work.
- **Sidecars:** Supporting containers defined in the Devfile (e.g., a database).
- **Injected Sidecars:** Kagen-specific sidecars (Proxy).
- **Volumes:** Devfile volume components mapped to the PVCs created in Stage 3.
- **Git Workspace:** A RAM-backed `emptyDir` (as per the design) where the repo is cloned.

### Project Scoping

The generator must ensure that the generated Pod:
1. Runs in the repo-scoped namespace.
2. Has the correct labels for selective NetworkPolicies.
3. Has the `agent-auth` and `git-workspace` volumes correctly mounted.

## Files

| Action | Path |
|--------|------|
| New    | `internal/devfile/parser.go` — Devfile v2 parsing logic |
| New    | `internal/devfile/generator.go` — Mapping Devfile to K8s Pod spec |
| New    | `internal/devfile/parser_test.go` |
| New    | `internal/devfile/generator_test.go` |
| Modify | `internal/cluster/resources.go` — Use the generator to define the Pod workload |
