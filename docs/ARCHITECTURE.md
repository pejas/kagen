# Kagen Architecture

## System Overview
`kagen` is a monolithic CLI that orchestrates a Colima/K3s runtime to host an isolated agent workspace for a single Git repository. The host checkout remains canonical; the in-cluster Forgejo instance is the review and durability boundary. The active flow is now session-first: persisted kagen sessions own the generated runtime, and nested agent sessions capture runtime-specific attach state. The design now emphasises smaller orchestration coordinators, reuse of shared Kubernetes adapters, and a minimal public surface.

## Core Design Principles
1. **Host as Source of Truth**: The local Git repository on the host machine is the ultimate canonical state.
2. **Isolation over Performance**: Every agent session runs in a dedicated K8s namespace with explicit egress policies.
3. **Forgejo as Boundary**: All agent commits are made to in-cluster Forgejo. Pulling back to the host is an explicit, human-initiated `kagen pull`.
4. **Idempotent CLI**: The CLI should converge the runtime, cluster, and services to a healthy state without multi-step lifecycle commands.
5. **Single Responsibility Orchestration**: Host-side orchestration is split into focused coordinators (runtime bootstrap, workload generation, forgejo sync, agent launch) to reduce coupling inside `internal/cmd`.
6. **Shared Adapters First**: Cluster and Forgejo operations reuse common adapters for port-forwarding and exec; direct `kubectl` shell-outs stay confined to those adapters.

## Architecture Layers

### 1. Host Layer (CLI)
The `kagen` binary drives a set of coordinators:
- **Runtime Coordinator (`internal/runtime`)**: Controls the Colima `kagen` profile and exposes the kube context. Includes dependency checks.
- **Runtime Shutdown (`internal/runtime`)**: `kagen down` also flows through the runtime coordinator so whole-environment shutdown stays separate from persisted session lifecycle.
- **Workload Builder (`internal/workload`)**: Produces the baseline runtime Pod from typed Go configuration.
- **Cluster Coordinator (`internal/cluster`)**: Uses client-go plus shared adapters to reconcile Namespaces, PVCs, and generated Pods; performs workspace bootstrap init injection and agent runtime env injection.
- **Forgejo Coordinator (`internal/forgejo`)**: Ensures the Forgejo deployment/service, handles admin/bootstrap, and synchronises the host repo via port-forwarded HTTP/git.
- **Git Engine (`internal/git`)**: Local Git discovery, branch/HEAD metadata, WIP protection commits, and push/fetch/merge helpers.
- **Session Store (`internal/session`)**: Persists kagen sessions plus per-agent session metadata so `start`, `attach`, and `list` survive CLI restarts.
- **UI and Config (`internal/ui`, `internal/config`)**: Present CLI output and load validated configuration defaults.

### 2. Shared Cluster Adapters
- **Port-Forwarder (`internal/cluster` PortForwarder interface)**: Single implementation point for kubectl port-forward; reused by Forgejo and other services.
- **Exec Adapter (planned)**: Replace ad-hoc kubectl exec calls with a common wrapper to centralise error handling and retries.

### 3. Cluster Layer (K3s)
- **Namespace per Repository**: `kagen-<repo-id>` with labels `kagen.io/scope=repo` and `kagen.io/repo-id=<id>`.
- **Forgejo**: Lightweight Git service (Deployment + PVC + Service) inside the namespace providing the review boundary.
- **Agent Pod**: Generated from the internal workload builder, mounts workspace volume at `/projects/workspace`, and injects agent-specific env/config; bootstrap init container clones Forgejo repo.
- **Persistence**: PVCs for workspace and agent auth state.
- **(Upcoming) Egress Proxy**: Enforced proxy pod with allowlist; policy validated host-side before agent attach.

## Data Flow
1. **Config Write**: `kagen config write` writes an optional `.kagen.yaml` with repository-specific defaults such as the default agent. It does not initialise the runtime, and `kagen start`/`kagen attach` do not require it.
2. **Start**: `kagen start <agent>` runs coordinators sequentially:
   - Runtime coordinator: dependency check + ensure Colima profile running.
   - Config coordinator: load and validate defaults from built-ins, environment, and optional `.kagen.yaml`, including proxy allowlist.
   - Git discovery: identify repo, branch, head SHA, repo ID.
   - Forgejo coordinator: ensure deployment/service/PVC, admin user, and repo stub exist.
   - Workload builder: generate the baseline Pod for the requested runtime.
   - Cluster coordinator: inject workspace sync init container, ensure PVCs, create Pod.
   - Session coordinator: persist the kagen session and nested agent session metadata.
   - Agent coordinator: wait for Pod readiness, wait for runtime-specific bootstrap (e.g., Codex), then attach via shared exec adapter.
3. **Attach/List**: `kagen attach <agent> [--session <id>]` reuses a persisted session, defaults to the most recent ready session for the current repository when `--session` is omitted, and creates a fresh agent session. `kagen list` reads persisted session summaries after CLI restart.
4. **Work**: Agent TUI operates inside the Pod. Exiting the TUI with `/exit` or `/quit` only detaches from the agent process.
5. **Shutdown**: `kagen down` stops the whole Colima/K3s runtime environment without deleting persisted kagen sessions or agent sessions from the local store.
6. **Checkpoint**: Agent-side pushes to Forgejo; host-side provenance recorded.
7. **Review**: `kagen open` uses Forgejo coordinator + port-forward to present review URL.
8. **Pull**: `kagen pull` uses Git engine plus port-forward to merge `kagen/<branch>` back into host branch, safeguarding with optional WIP commit.

## Security Controls
- **Egress Proxy (planned enforcement)**: All agent traffic funnels through an allowlisted proxy pod; host-side proxy policy validation fails closed when unenforced.
- **Credential Isolation**: Agent auth state (for example Codex login state in `.codex`) is stored in a dedicated PVC inside the namespace.
- **Filesystem Silo**: Agent containers see only the workspace and auth PVCs.

## Architecture Improvement Plan (Executable)
1. **Orchestration Decomposition**
   - Extract runtime, workload generation, forgejo sync, and agent launch into discrete coordinator structs invoked from `internal/cmd/root.go`.
   - Add thin service interfaces to enable unit tests without shelling out.
2. **Shared Cluster Adapters**
   - Introduce `internal/cluster/exec.go` wrapping kubectl exec with retries/timeouts.
   - Refactor ForgejoService and Agent Base to consume shared exec/port-forward adapters.
3. **Public Surface Tightening**
   - Limit exported symbols to constructors, interfaces, and sentinel errors; unexport BaseAgent and helper types where possible.
   - Add `internal` subpackages for implementations to keep APIs narrow.
4. **Proxy Enforcement Path**
   - Wire `internal/proxy.Policy` into start flow: validate policy before agent attach; surface actionable errors when unenforced.
   - Add proxy reconciling placeholder in cluster layer for future proxy pod.
5. **Forgejo Responsibility Split**
   - Separate deployment reconciliation from git import/push logic into `forgejo/reconcile.go` and `forgejo/repo.go`.
   - Move port-forward lifecycle to shared adapter; remove duplicated kubectl calls.
6. **Configuration Validation**
   - Centralise config validation (numeric ranges, timeouts, allowlist shape) in `internal/config/validate.go` and call it early in root flow.
7. **Agent Attach Reuse**
   - Replace BaseAgent kubectl exec with shared exec adapter and Pod lookup by label selectors supplied by cluster coordinator.
