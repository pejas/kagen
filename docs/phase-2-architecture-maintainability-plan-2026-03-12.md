# Phase 2 Plan: Architecture and Maintainability

Date: 2026-03-12

Scope: execution plan for Phase 2 of the technical audit, focused on reducing orchestration blast radius, improving package boundaries, and making the codebase easier to evolve safely after Phase 1 correctness/security work lands.

Source inputs:
- `docs/technical-audit-2026-03-12.md`
- `docs/phase-1-correctness-security-plan-2026-03-12.md`
- `docs/ARCHITECTURE.md`
- `docs/CONVENTIONS.md`
- `AGENTS.md`

Architecture alignment:
- Keep `internal/cmd` thin and user-facing.
- Prefer consumer-owned interfaces over provider-owned speculative abstractions.
- Keep shared exec/port-forward behaviour centralised.
- Preserve `internal/workload` as the runtime source of truth.
- Build on Phase 1 transient Forgejo transport rather than reintroducing persistent host Git state.

## Executive Summary

Phase 2 is the structural cleanup phase. Once Phase 1 fixes the most important correctness and security contracts, the next risk is change amplification: too many workflows still terminate inside `internal/cmd`, several infrastructure packages expose broader abstractions than they need, and `internal/session/store.go` and port-forwarding have both grown into high-blast-radius implementation clusters.

The purpose of this phase is to reduce architectural friction without destabilising behaviour. The desired outcome is a codebase where:

- command handlers are mostly argument binding plus error presentation;
- orchestration lives in focused application services/coordinators;
- interfaces are small and defined where substitution is actually needed;
- large persistence and transport components are decomposed into testable units;
- runtime and review flows remain behaviourally identical while becoming easier to extend.

## Goals

- Reduce the fan-out and responsibility concentration in `internal/cmd`.
- Move the codebase toward stable application-service seams.
- Decompose large files and packages that currently combine too many concerns.
- Replace fragile transport lifecycle handling with explicit ownership.
- Make future work on new commands, agents, or review workflows lower-risk.

## Non-Goals

- Reopening Phase 1 correctness/security decisions
- Replacing Cobra, Viper, client-go, or SQLite
- Full domain-driven redesign of the repository
- Large-scale package renames unless they are justified by a boundary change
- UI redesign or CLI semantics changes unrelated to maintainability

## Success Criteria

Phase 2 is complete when all of the following are true:

- `internal/cmd` no longer directly owns most orchestration sequencing for `start`, `attach`, `open`, and `pull`.
- Application-service or coordinator types exist for the main workflows.
- Consumer-owned interfaces replace provider-wide interfaces where abstraction is not justified.
- `internal/session/store.go` is split into focused files with equivalent behaviour.
- Port-forwarding exposes explicit lifecycle ownership and is safe for long-running workflow use.
- `make test` passes.
- `make lint` passes.

## Workstreams

## Workstream 1: Extract Application Services from `internal/cmd`

### Problem

`internal/cmd` still acts as a god package and imports nearly every subsystem. `runStart`, `runAttach`, `runOpen`, and `runPull` combine CLI concerns with orchestration, transport, persistence, and infrastructure sequencing.

### Primary files

- `internal/cmd/session_flow.go`
- `internal/cmd/root_flow.go`
- `internal/cmd/open.go`
- `internal/cmd/pull.go`

### Deliverables

- Introduce focused workflow services/coordinators for:
  - start
  - attach
  - open
  - pull
- Reduce command handlers to:
  - input collection
  - flag interpretation
  - invoking one workflow object
  - user-facing error propagation

### Proposed structure

Possible package shapes:

- `internal/app/start`
- `internal/app/attach`
- `internal/app/open`
- `internal/app/pull`

or

- `internal/workflow/start.go`
- `internal/workflow/attach.go`
- `internal/workflow/open.go`
- `internal/workflow/pull.go`

The exact package name is less important than the responsibility split.

### Design constraints

- Do not move terminal formatting into infrastructure packages.
- Do not push orchestration back into large global helper files.
- Avoid one mega-`service` package that simply recreates `internal/cmd` elsewhere.

### Acceptance criteria

- Each command file is materially smaller and easier to scan.
- Workflow objects can be tested without invoking Cobra directly.
- Behaviour of `start`, `attach`, `open`, and `pull` remains unchanged apart from intended improvements from Phase 1.

## Workstream 2: Move Interface Definitions to Consumer Boundaries

### Problem

Several provider packages define broad interfaces that are not the narrowest useful abstraction. The current repository already demonstrates that tests often prefer tiny consumer-local seams instead.

### Primary files

- `internal/cluster/cluster.go`
- `internal/runtime/runtime.go`
- `internal/forgejo/forgejo.go`
- `internal/agent/agent.go`
- `internal/cmd/*`

### Deliverables

- Replace provider-owned wide interfaces with:
  - concrete exported types where no real substitution exists
  - small consumer-owned interfaces where testing or orchestration needs them
- Remove dead or no-longer-useful abstraction layers.

### Design constraints

- Do not break useful concrete constructors.
- Avoid interface churn that only moves code without reducing conceptual load.
- Prefer deleting abstractions over relocating them if they are not needed.

### Proposed implementation

1. Audit each current provider-owned interface for:
   - multiple real implementations
   - real consumer substitution
   - test utility
2. Keep only abstractions that pass that test.
3. Define narrow interfaces in the consuming workflow package when needed.

### Acceptance criteria

- Fewer provider-level interfaces remain.
- No package exports an interface only to satisfy its own implementation.
- Test seams become more explicit and local.

## Workstream 3: Decompose Session Persistence

### Problem

`internal/session/store.go` mixes schema migration, writes, reads, row scanning, path normalisation, timestamp helpers, and query composition. It also carries an N+1 query pattern on list paths.

### Primary files

- `internal/session/store.go`
- `internal/session/store_test.go`

### Deliverables

- Split persistence concerns into focused files, for example:
  - `open.go`
  - `migrations.go`
  - `queries.go`
  - `writes.go`
  - `scan.go`
  - `paths.go`
- Replace N+1 session listing with a prefetched or joined read path.

### Design constraints

- Preserve the external `session` package contract unless there is a strong simplification opportunity.
- Keep SQLite-specific details private to the package.
- Do not regress ordering semantics for agent sessions.

### Acceptance criteria

- No single persistence file dominates the package.
- `List` no longer performs one agent-session query per session record.
- Tests still validate schema initialisation, filtering, and attach bookkeeping.

## Workstream 4: Replace Fragile Port-Forward Lifecycle Handling

### Problem

Current port-forwarding starts goroutines, returns on first readiness, and leaves later output/error lifecycle ownership unclear. It is workable for short-lived operations but brittle as a shared transport primitive.

### Primary files

- `internal/cluster/portforward.go`
- `internal/forgejo/*`
- any Phase 1 review/import transport code

### Deliverables

- Replace the current stateful “start and forget” design with an explicit session model.
- Ensure a forward session owns:
  - readiness
  - lifecycle termination
  - terminal error propagation
  - output draining

### Design options

- preferred: explicit `ForwardSession` handle around current shell-based implementation
- stretch goal: client-go-native port-forward implementation if complexity is acceptable

### Design constraints

- Keep kubectl shell-out centralised if retained.
- Preserve context-aware cancellation.
- Make concurrent use safe by construction.

### Acceptance criteria

- Long-lived consumers can observe late port-forward failure.
- Output draining cannot deadlock on unconsumed channels.
- Forgejo workflows use the new lifecycle cleanly.

## Workstream 5: Clarify Forgejo and Cluster Responsibility Boundaries

### Problem

`internal/forgejo` currently mixes deployment reconcile, admin bootstrap, API polling, repo creation, port-forward ownership, and host Git transport. `internal/cluster` mixes desired-state reconciliation with mutation helpers and some runtime-policy assembly.

### Primary files

- `internal/forgejo/reconcile.go`
- `internal/forgejo/repo.go`
- `internal/forgejo/service.go`
- `internal/cluster/kube.go`
- `internal/cluster/proxy.go`

### Deliverables

- Keep Forgejo deployment/state reconciliation separate from review/import transport logic.
- Reduce cross-file implicit coordination by making service boundaries explicit.
- Keep cluster mutation logic subordinate to the typed workload contract.

### Proposed implementation

1. Keep reconcile and transport/bootstrap in separate files or types.
2. If needed, introduce a small transport-oriented Forgejo helper rather than one broad service type doing everything.
3. Keep agent pod mutation logic in cluster code minimal and intentional; push baseline pod truth back toward `internal/workload`.

### Acceptance criteria

- Forgejo code is easier to navigate by concern.
- Review/import transport changes do not require touching deployment reconciliation code.
- Cluster code no longer feels like a second source of runtime truth.

## Recommended Sequencing

1. Workstream 4: port-forward lifecycle
2. Workstream 1: extract workflow services
3. Workstream 2: consumer-owned interface cleanup
4. Workstream 3: session persistence decomposition
5. Workstream 5: Forgejo/cluster boundary cleanup

Reasoning:

- Port-forward lifecycle is a foundational transport primitive.
- Workflow extraction is easiest once transport ownership is explicit.
- Interface cleanup should follow real workflow seams, not precede them.
- Session decomposition is lower-risk once orchestration call sites settle.

## Testing Strategy

- Add or update unit tests around new workflow services.
- Keep command-level tests for Cobra bindings lightweight.
- Add transport lifecycle tests for the new forward-session model.
- Preserve session store behaviour tests while refactoring internal file layout.
- Use golden behaviour expectations where helpful, but prefer explicit state assertions.

## Risks and Mitigations

### Risk: Service extraction becomes indirection without simplification

- Mitigation: each new workflow type must own a single user workflow and shrink command code noticeably.

### Risk: Interface cleanup causes churn without reducing complexity

- Mitigation: delete abstractions aggressively when they are not earning their keep.

### Risk: Persistence refactor accidentally changes ordering or timestamps

- Mitigation: preserve and extend current behaviour tests before decomposition.

### Risk: Port-forward redesign destabilises Phase 1 review/import flows

- Mitigation: land the transport session model behind tests before broad workflow rewiring.

## Acceptance Checklist

- [x] `start`, `attach`, `open`, and `pull` have dedicated workflow services/coordinators
- [x] `internal/cmd` is materially smaller and thinner
- [x] provider-owned speculative interfaces are removed or narrowed
- [x] `internal/session/store.go` is decomposed
- [x] port-forwarding has explicit lifecycle ownership
- [x] `make test`
- [x] `make lint`

## Suggested Deliverable Breakdown

1. transport lifecycle refactor
2. workflow extraction for `open` and `pull`
3. workflow extraction for `start` and `attach`
4. interface cleanup
5. session package decomposition
6. Forgejo/cluster boundary cleanup

This keeps Phase 2 reviewable and avoids mixing architecture cleanup with correctness fixes already covered by Phase 1.
