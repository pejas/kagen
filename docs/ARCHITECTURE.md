# Kagen Architecture

## System Overview
`kagen` is a monolithic CLI that orchestrates a Colima/K3s runtime to host an isolated agent workspace for a single Git repository. The host checkout remains canonical; the in-cluster Forgejo instance is the review and durability boundary. The active flow is session-first: persisted kagen sessions own the generated runtime, and nested agent sessions capture runtime-specific attach state.

Architecturally, `kagen` should optimise for four properties:
- **truthful user workflows**: documented commands should match shipped behaviour;
- **host correctness**: the host checkout and host Git config stay canonical and minimally mutated;
- **shared transport adapters**: Kubernetes exec and port-forward behaviour is centralised;
- **reproducible runtime state**: security-sensitive runtime components come from controlled artefacts, not ad-hoc package installation at boot.

## Core Design Principles
1. **Host as Source of Truth**: The local Git repository on the host machine is the ultimate canonical state.
2. **Isolation over Performance**: Every agent session runs in a dedicated K8s namespace with explicit egress policies.
3. **Forgejo as Boundary**: All agent commits are made to in-cluster Forgejo. Pulling back to the host is an explicit, human-initiated `kagen pull`.
4. **Idempotent CLI**: The CLI should converge the runtime, cluster, and services to a healthy state without multi-step lifecycle commands.
5. **Single Responsibility Orchestration**: Host-side orchestration is split into focused coordinators (runtime bootstrap, workload generation, forgejo sync, agent launch) to reduce coupling inside `internal/cmd`.
6. **Shared Adapters First**: Cluster and Forgejo operations reuse common adapters for port-forwarding and exec; direct `kubectl` shell-outs stay confined to those adapters.
7. **Transient Forgejo Transport**: Host-side Git operations should use transient, operation-scoped transport rather than persisting credentialed remotes or ephemeral localhost ports into repository config.
8. **Reproducible Artefacts**: Runtime-path images should be pinned and self-contained; security-sensitive startup must not depend on live package installation.

## Architecture Layers

### 1. Host Layer (CLI)
The `kagen` binary drives a set of coordinators:
- **Workflow Coordinators (`internal/workflow`)**: Own the `start`, `attach`, `open`, and `pull` application flows so command handlers stay focused on Cobra binding and top-level error propagation.
- **Runtime Coordinator (`internal/runtime`)**: Controls the Colima `kagen` profile and exposes the kube context. Includes dependency checks.
- **Runtime Shutdown (`internal/runtime`)**: `kagen down` also flows through the runtime coordinator so whole-environment shutdown stays separate from persisted session lifecycle.
- **Workload Builder (`internal/workload`)**: Produces the baseline runtime Pod from typed Go configuration.
- **Cluster Coordinator (`internal/cluster`)**: Uses client-go plus shared adapters to reconcile Namespaces, PVCs, and generated Pods; performs workspace bootstrap init injection and agent runtime env injection.
- **Forgejo Coordinator (`internal/forgejo`)**: Ensures the Forgejo deployment/service, handles bootstrap, and owns review/import transport semantics. The target contract is transient host-side transport with no persistent credential leakage into host Git config.
- **Git Engine (`internal/git`)**: Local Git discovery, branch/HEAD metadata, WIP protection commits, and push/fetch/merge helpers.
- **Session Store (`internal/session`)**: Persists kagen sessions plus per-agent session metadata so `start`, `attach`, and `list` survive CLI restarts.
- **UI and Config (`internal/ui`, `internal/config`)**: Present CLI output and load validated configuration defaults.
- **Verification Contract**: `make build`, `make test`, and `make lint` are the default repository trust boundary; `make test-e2e` is explicit and reserved for runtime-spanning checks.

### 2. Shared Cluster Adapters
- **Port-Forwarder (`internal/cluster`)**: Single implementation point for kubectl port-forward, returning an explicit forward-session handle with readiness, shutdown, and terminal error ownership.
- **Exec Adapter (`internal/kubeexec`)**: Shared wrapper around kubectl exec/attach/wait operations. This is the canonical place for terminal attach, remote readiness checks, and command execution concerns.

### 3. Cluster Layer (K3s)
- **Namespace per Repository**: `kagen-<repo-id>` with labels `kagen.io/scope=repo` and `kagen.io/repo-id=<id>`.
- **Forgejo**: Lightweight Git service (Deployment + PVC + Service) inside the namespace providing the review boundary.
- **Agent Pod**: Generated from the internal workload builder, mounts workspace volume at `/projects/workspace`, and injects agent-specific env/config; bootstrap init container clones Forgejo repo, while the runtime toolbox container is expected to arrive prebuilt with its default toolchain and agent CLIs.
- **Persistence**: PVCs for workspace and agent auth state.
- **Egress Proxy**: Namespace-scoped proxy workload and network policy used to constrain outbound access. Proxy policy is validated host-side before agent attach and should fail closed when enforcement is not active.

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
   - Agent coordinator: wait for Pod readiness, verify the prebuilt runtime is ready (for example Codex is already on `PATH`), then attach via shared exec adapter.
3. **Attach/List**: `kagen attach <agent> [--session <id>]` reuses a persisted session, defaults to the most recent ready session for the current repository when `--session` is omitted, and creates a fresh agent session. `kagen list` reads persisted session summaries after CLI restart.
4. **Work**: Agent TUI operates inside the Pod. Exiting the TUI with `/exit` or `/quit` only detaches from the agent process.
5. **Shutdown**: `kagen down` stops the whole Colima/K3s runtime environment without deleting persisted kagen sessions or agent sessions from the local store.
6. **Checkpoint**: Agent-side pushes to Forgejo; host-side provenance recorded.
7. **Review**: `kagen open` should establish the transport it needs, determine whether reviewable work exists, and open a live Forgejo review URL. This is a correctness contract, not a best-effort convenience feature.
8. **Pull**: `kagen pull` should use transient Forgejo transport to fetch and merge `kagen/<branch>` back into the host branch, safeguarding with optional WIP commit while leaving host Git config clean.

## Security Controls
- **Egress Proxy Enforcement**: Agent traffic should funnel through an allowlisted proxy pod; host-side proxy policy validation must fail closed when enforcement is not active.
- **Credential Isolation**: Agent auth state (for example Codex login state in `.codex`) is stored in a dedicated PVC inside the namespace.
- **Filesystem Silo**: Agent containers see only the workspace and auth PVCs.
- **Transient Host Transport**: Forgejo credentials should be operation-scoped and must not be persisted into host repository remotes or other long-lived local config.
- **Controlled Runtime Artefacts**: Runtime-path images should be pinned and self-contained; avoid package-manager installation in security-sensitive startup paths.

## Verification and Diagnostics
- **Default Verification Loop**: Contributors should be able to trust `make build`, `make test`, and `make lint` as the fast default validation contract. CI should enforce the same contract.
- **Intentional E2E Scope**: Runtime-backed E2E coverage stays narrow and explicit. Use `docs/E2E.md` to define what belongs in `make test-e2e` and what should remain integration-tested.
- **Verbose Diagnostics**: Runtime bootstrap, Forgejo readiness, proxy readiness, and review transport lifecycle should expose richer detail behind `--verbose` without making normal command output noisy.

## Architecture Improvement Plan (Executable)
1. **Orchestration Decomposition**
   - Extract runtime, workload generation, forgejo sync, and agent launch into discrete coordinator structs invoked from `internal/cmd/root.go`.
   - Add thin service interfaces to enable unit tests without shelling out.
2. **Shared Cluster Adapters**
   - Keep kubectl exec, attach, wait, and port-forward behaviour centralised in shared adapters.
   - Refactor remaining orchestration code to consume shared exec/port-forward adapters rather than managing transport directly.
3. **Public Surface Tightening**
   - Limit exported symbols to constructors, concrete types where appropriate, and narrow consumer-facing interfaces where substitution is required.
   - Avoid interface proliferation in provider packages when a concrete type is sufficient.
4. **Proxy Enforcement Path**
   - Keep proxy validation in the start/attach flow and surface actionable failures when enforcement is inactive.
   - Move towards reproducible proxy artefacts with pinned images and no runtime package installation.
5. **Forgejo Responsibility Split**
   - Keep deployment reconciliation separate from review/import transport semantics.
   - Use transient transport for host-side Git and review workflows; avoid persistent credentialed remotes.
6. **Configuration Validation**
   - Centralise config validation (numeric ranges, timeouts, allowlist shape) in `internal/config/validate.go` and call it early in root flow.
7. **Agent Attach Reuse**
   - Keep agent attach and readiness logic built on the shared exec adapter and container-aware runtime specs.
