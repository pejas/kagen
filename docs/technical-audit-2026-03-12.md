# Kagen Technical Audit

Date: 2026-03-12

Scope: full repository audit of the current working tree at `/Users/pejas/Projects/kagen`

Validation performed:
- `make test` passed
- `make lint` could not run because `golangci-lint` is not installed in this environment

## Audit Status

Status reviewed on 2026-03-13 against the current repository state.

- Fixed since the original audit:
  - `kagen open` now owns a live review session and browser URL workflow.
  - Forgejo transport no longer persists credentialed remotes or ephemeral localhost ports into host Git config.
  - Git discovery now supports worktrees through `git rev-parse --show-toplevel`.
  - The session store has been split into focused files and list queries no longer use the original N+1 shape.
  - Port-forward lifecycle ownership now lives on an explicit session handle with readiness and terminal error semantics.
  - Checked-in lint configuration and CI now exist.
- Partially fixed since the original audit:
  - Runtime artefacts are now release-tagged and the proxy no longer installs packages at startup, but images are still tag-pinned rather than digest-pinned.
  - `internal/cmd` is materially smaller because start, attach, open, and pull now run through `internal/workflow`, but command composition is still a notable coordination layer.
  - Test coverage now includes `open`, worktree discovery, and port-forward parsing, but runtime-backed E2E coverage remains intentionally narrow.
  - Documentation and tooling drift are reduced, though the rest of this document should still be read as a point-in-time audit from 2026-03-12 rather than a current status report.

Validation status refreshed on 2026-03-13:
- `make test` passed
- `make lint` passed

## Executive Summary

`kagen` is a single-module Go CLI with a clear product idea and a mostly sensible package split: runtime control, Kubernetes reconciliation, Forgejo integration, Git orchestration, session persistence, and agent-specific attach logic. The codebase is readable, package naming is idiomatic, internal-only APIs keep the public surface small, and there are no circular package dependencies.

The main remaining risks are not stylistic. They are mostly about finishing the hardening pass: fully reproducible artefacts, keeping orchestration boundaries narrow, and making sure the audit stays truthful as implementation evolves. The most important current concerns are:

1. Runtime artefacts are not yet fully reproducible or digest-pinned.
2. Some orchestration responsibility still sits in the command composition layer.
3. A few provider-owned interfaces remain broader than current usage justifies.
4. Runtime-backed E2E coverage is still intentionally narrow.
5. The audit itself needs the dated status section above to avoid drifting behind implementation.

Overall assessment: solid early architecture, but not yet hardened for production reliability or security-first claims.

## Phase 1: Repository Discovery

### Module

- Single Go module: `github.com/pejas/kagen` in `go.mod`
- Declared toolchain: `go 1.26.1` in `go.mod:3`

### Package Tree

```text
cmd/
  kagen/                  main entrypoint

internal/
  agent/                  agent registry, runtime specs, attach/launch logic
  cluster/                Kubernetes reconciliation, proxy, port-forward
  cmd/                    Cobra command tree and orchestration flows
  config/                 Viper-backed config load + validation
  e2e/                    end-to-end BDD-style tests
  errors/                 sentinel errors and exit wrapper
  forgejo/                in-cluster Forgejo reconcile + repo sync
  git/                    host-side git discovery and command helpers
  kubeexec/               kubectl exec/attach/wait adapter
  provenance/             import provenance record persistence
  proxy/                  policy model and allowlist catalogue
  runtime/                Colima/K3s lifecycle
  session/                SQLite-backed persisted sessions
  ui/                     terminal output + prompt/table helpers
  workload/               typed runtime pod builder
```

### Entrypoints

- Main package: `cmd/kagen/main.go`
- Main function: `cmd/kagen/main.go:5-7`

### CLI Commands

- Root: `kagen` in `internal/cmd/root.go:29-39`
- `start`: `internal/cmd/session_flow.go:54-68`
- `attach`: `internal/cmd/session_flow.go:70-89`
- `down`: `internal/cmd/down.go:23-34`
- `config`: `internal/cmd/config.go:30-45`
- `config write`: `internal/cmd/config.go:47-61`
- `list`: `internal/cmd/list.go:15-28`
- `open`: `internal/cmd/open.go:22-29`
- `pull`: `internal/cmd/pull.go:15-24`
- `version`: `internal/cmd/version.go`

### Services and Background Work

- In-cluster services:
  - Forgejo deployment/service/PVC via `internal/forgejo/reconcile.go`
  - Egress proxy deployment/service/network policy via `internal/cluster/proxy.go`
  - Generated agent pod via `internal/workload/builder.go` and `internal/cluster/kube.go`
- Host background workers:
  - No long-lived daemon or scheduler
  - Short-lived goroutines only in port-forward handling: `internal/cluster/portforward.go:57-65`

### Internal Libraries / Reusable Components

- `internal/git`
- `internal/session`
- `internal/kubeexec`
- `internal/workload`
- `internal/proxy`
- `internal/ui`

### External Dependency Graph

- CLI/config: Cobra, Viper
- Persistence: `modernc.org/sqlite`
- Kubernetes: `k8s.io/api`, `k8s.io/apimachinery`, `k8s.io/client-go`
- IDs: `github.com/google/uuid`
- Test/E2E: Godog/Cucumber indirect dependencies

### Internal Dependency Graph

```mermaid
graph TD
    A["cmd/kagen"] --> B["internal/cmd"]
    B --> C["internal/config"]
    B --> D["internal/runtime"]
    B --> E["internal/git"]
    B --> F["internal/cluster"]
    B --> G["internal/forgejo"]
    B --> H["internal/session"]
    B --> I["internal/agent"]
    B --> J["internal/proxy"]
    B --> K["internal/provenance"]
    B --> L["internal/workload"]
    B --> M["internal/ui"]
    G --> F
    G --> E
    G --> N["internal/kubeexec"]
    I --> E
    I --> N
    F --> I
    F --> E
    F --> J
    D --> C
    J --> C
    L --> I
    K --> E
```

### Major Components

- CLI orchestration: `internal/cmd`
- Local runtime lifecycle: `internal/runtime`
- Repository discovery and Git operations: `internal/git`
- Kubernetes namespace/pod/proxy reconciliation: `internal/cluster`
- Forgejo reconcile and import workflow: `internal/forgejo`
- Session persistence: `internal/session`
- Agent catalogue and attach/launch: `internal/agent`
- Typed workload generation: `internal/workload`

### Architectural Style

The codebase is a monolithic CLI with internal package boundaries and light ports/adapters tendencies. It is not cleanly hexagonal yet, but it is moving in that direction:

- `internal/cmd` is the entry/orchestration layer.
- `internal/runtime`, `internal/git`, `internal/session`, `internal/cluster`, and `internal/forgejo` act as infrastructure/service packages.
- `internal/workload` is the closest thing to a pure builder/domain component.

## Architecture Assessment

### What is working well

- Internal-only packages keep public API exposure low.
- Package names are domain-specific; there is no `util`, `common`, or `helpers` dumping ground.
- Dependency direction is mostly sane: `internal/cmd` depends on lower layers, and lower layers rarely depend on each other except where orchestration genuinely crosses boundaries.
- No circular package dependencies were detected.
- Separation between typed workload generation and Kubernetes mutation is directionally good.

### Architectural weaknesses

- `internal/cmd` still owns too much orchestration and policy sequencing.
- `internal/session/store.go` combines schema management, write paths, read paths, scanning helpers, path normalisation, and timestamp helpers in one 758-line file.
- Several interfaces live in provider packages even though their narrowest consumers are elsewhere.
- The Forgejo transport boundary is not treated as an explicit credential/connection lifecycle; it leaks into host Git state.

### Layering and Dependency Direction

- Good:
  - `internal/workload` depends only on `internal/agent` and Kubernetes API types.
  - `internal/proxy` is a small model package with minimal dependencies.
  - `internal/provenance` is isolated.
- Risky:
  - `internal/cmd` imports nearly every major package and acts as the coordination super-layer.
  - `internal/forgejo` depends on both cluster and Git concerns and is responsible for reconcile, API bootstrap, admin user creation, repo creation, port-forward, and push semantics.

### Circular Dependencies

- None found.

### Util Package Anti-patterns

- None found.

### God Packages / God Files

- Package-level god package: `internal/cmd`
- File-level god files:
  - `internal/session/store.go` (758 LOC)
  - `internal/cluster/proxy.go` (478 LOC)
  - `internal/cmd/session_flow.go` (360 LOC)
  - `internal/cluster/kube.go` (305 LOC)
  - `internal/git/repo.go` (247 LOC)

## Major Design Problems

### 1. `kagen open` is not production-ready, despite being documented as a working workflow

- Severity: High
- Explanation: `runOpen` calls `HasNewCommits`, but the only implementation currently returns `ErrNotImplemented`. Even if that were implemented, `GetReviewURL` hard-codes `http://localhost:3000` and does not establish or own a matching port-forward. That means the command is either a no-op warning or opens a dead URL. This is a direct mismatch between the documented UX and runtime behaviour.
- Affected files:
  - `internal/cmd/open.go:31-86`
  - `internal/forgejo/service.go:40-50`
  - `README.md:83-87`
- Suggested fix: Make `open` own an HTTP port-forward lifecycle, derive the review URL from the forwarded port or configured HTTP port, and implement `HasNewCommits` against either the Forgejo API or Git refs fetched through the active transport.
- Example improved code:

```go
localPort, err := svc.StartHTTPPortForward(ctx, repo)
if err != nil {
	return fmt.Errorf("starting forgejo port-forward: %w", err)
}
defer svc.StopPortForward()

reviewURL := svc.ReviewURL(repo, localPort)
return openBrowser(reviewURL)
```

### 2. Forgejo credentials and ephemeral localhost ports are persisted into host Git config

- Severity: High
- Explanation: both import and pull write a `kagen` remote using `http://kagen:kagen-internal-secret@127.0.0.1:<ephemeral-port>/...`. This leaves static credentials in `.git/config` on the host and couples a persistent remote to a transient port-forward. That is both a security leak and an operational footgun.
- Affected files:
  - `internal/forgejo/repo.go:32-37`
  - `internal/cmd/pull.go:64-67`
  - `internal/git/repo.go:118-129`
- Suggested fix: avoid persisting the transport endpoint in host config. Use one-shot authenticated fetch/push commands, a temporary remote removed after use, or `git -c credential.helper=...`/`GIT_ASKPASS` style injection for the single operation.
- Example improved code:

```go
func (r *Repository) PushURL(ctx context.Context, url string, refspecs ...string) error {
	args := []string{"push", url}
	args = append(args, refspecs...)
	_, err := gitCommandContext(ctx, r.Path, args...)
	return err
}
```

### 3. Repository discovery breaks on Git worktrees and similar valid layouts

- Severity: High
- Explanation: `findGitRoot` only recognises `.git` when it is a directory. In Git worktrees, submodules, and several tool-managed checkouts, `.git` is a file containing a gitdir pointer. Those repositories will be reported as “not a git repository” even though Git itself handles them correctly.
- Affected files:
  - `internal/git/repo.go:78-98`
  - `internal/git/repo_test.go:14-58`
- Suggested fix: replace the manual filesystem walk with `git rev-parse --show-toplevel`, or at minimum accept `.git` files as well as directories.
- Example improved code:

```go
func Discover(startPath string) (*Repository, error) {
	out, err := gitCommand(startPath, "rev-parse", "--show-toplevel")
	if err != nil {
		return nil, kagerr.ErrNotGitRepo
	}
	root := strings.TrimSpace(out)
	// continue with branch and head discovery
}
```

### 4. The “security-first” runtime is not fully reproducible or supply-chain-hardened

- Severity: High
- Explanation: the proxy deployment installs `tinyproxy` at container startup via `apk add`, which makes startup depend on live package repositories and mutable package indexes. Separately, the workspace image is pinned to `vxcontrol/codebase:latest`, which is also mutable. Both choices weaken provenance, reproducibility, and incident response.
- Affected files:
  - `internal/cluster/proxy.go:27-30`
  - `internal/cluster/proxy.go:198-205`
  - `internal/workload/builder.go:13-15`
- Suggested fix: publish a pinned proxy image that already contains tinyproxy, pin all images by immutable digest, and treat the image set as release-managed artefacts rather than runtime bootstrap dependencies.
- Example improved code:

```go
const (
	proxyImage            = "ghcr.io/pejas/kagen-proxy@sha256:..."
	defaultWorkspaceImage = "ghcr.io/pejas/kagen-workspace@sha256:..."
)
```

### 5. The session store mixes concerns and uses an N+1 query pattern on list paths

- Severity: Medium
- Explanation: `Store.List` loads all sessions and then calls `agentSessions` once per session. That scales poorly as history grows, and the file has become a catch-all for migrations, persistence, query composition, scanning, and path/time utilities. It is maintainable today, but it is on the wrong trajectory.
- Affected files:
  - `internal/session/store.go:38-57`
  - `internal/session/store.go:200-260`
  - `internal/session/store.go:540-620`
- Suggested fix: split the file into `migrations.go`, `writes.go`, `queries.go`, and `scan.go`, and change `List` to prefetch agent sessions in one ordered query keyed by session UID.
- Example improved code:

```sql
SELECT
  ks.id, ks.uid, ks.repo_path, ks.status, ks.last_used_at,
  as.id, as.agent_type, as.state_path, as.last_used_at
FROM kagen_sessions ks
LEFT JOIN agent_sessions as
  ON as.kagen_session_uid = ks.uid
WHERE (? = '' OR ks.repo_path = ?)
ORDER BY ks.last_used_at DESC, ks.id DESC, as.agent_type ASC, as.last_used_at DESC;
```

### 6. Port-forward lifecycle management can leak goroutines and obscure late failures

- Severity: Medium
- Explanation: `Start` spawns scanner goroutines and a `Wait` goroutine, then returns as soon as the port becomes ready. After that point there is no consumer draining `lineCh` or `readErrCh`. If `kubectl` emits further output later, those goroutines can block on channel sends, which can backpressure the subprocess pipes and hide failure signals. The type is also stateful and not safe for concurrent reuse.
- Affected files:
  - `internal/cluster/portforward.go:35-113`
  - `internal/cluster/portforward.go:169-176`
- Suggested fix: return a session handle that owns the goroutines, drains output for the full lifetime, and exposes readiness and terminal error channels; alternatively use the client-go port-forward package instead of shelling out.
- Example improved code:

```go
type ForwardSession struct {
	LocalPort int
	ErrCh     <-chan error
	stop      func() error
}
```

### 7. Interface placement is provider-centric and more abstract than current usage justifies

- Severity: Medium
- Explanation: `cluster.Manager`, `runtime.Manager`, `forgejo.Service`, and `agent.Agent` are defined in provider packages, but most tests do not consume these interfaces directly. Instead, the command layer introduces its own function-variable seams and tiny local interfaces. That is a sign the abstractions are not sitting at the narrowest consumer boundary.
- Affected files:
  - `internal/cluster/cluster.go:14-38`
  - `internal/runtime/runtime.go:35-48`
  - `internal/forgejo/forgejo.go:13-26`
  - `internal/agent/agent.go:21-37`
  - `internal/cmd/down.go:13-19`
  - `internal/cmd/session_flow.go:27-35`
- Suggested fix: move narrow interfaces to the command/service consumers that actually need substitution, and let concrete infrastructure packages export constructors plus concrete types where no alternate implementation exists.
- Example improved code:

```go
type forgejoImporter interface {
	EnsureRepo(context.Context, *git.Repository) error
	ImportRepo(context.Context, *git.Repository) error
}
```

### 8. `internal/cmd` still functions as a god package and orchestration hub

- Severity: Medium
- Explanation: the package imports almost every subsystem and carries both command definitions and orchestration policy. `runStart` and `runAttach` sequence runtime boot, cluster reconcile, Forgejo import, proxy validation, persistence, and agent attach themselves. The code is readable, but the blast radius of changes is too high.
- Affected files:
  - `internal/cmd/session_flow.go:91-217`
  - `internal/cmd/root_flow.go:37-217`
  - `internal/cmd/open.go:31-86`
  - `internal/cmd/pull.go:26-132`
- Suggested fix: extract start/attach/open/pull application services or coordinators so Cobra commands only bind CLI input to a single orchestration object.
- Example improved code:

```go
type StartService struct {
	Runtime runtimeEnsurer
	Cluster clusterEnsurer
	Forgejo forgejoImporter
	Store   sessionStore
}
```

### 9. The test suite proves many placeholders, but misses critical end-to-end behaviour

- Severity: Medium
- Explanation: the tests are plentiful and mostly table-driven or parallelised, which is good. However, several packages are primarily tested for `ErrNotImplemented` placeholder behaviour rather than real workflows. There is no command-level test for `open`, no real validation of port-forward lifecycle ownership, and no test coverage for Git worktree discovery.
- Affected files:
  - `internal/forgejo/forgejo_test.go:12-81`
  - `internal/runtime/runtime_test.go:11-56`
  - `internal/cluster/cluster_test.go:16-65`
  - `internal/cmd/open.go:31-86`
  - `internal/git/repo_test.go:14-161`
- Suggested fix: add focused tests for real command behaviour around `open`, repository discovery edge cases, and one integration test around a temporary Forgejo remote workflow. Prefer behaviour tests over placeholder tests as implementation matures.
- Example improved code:

```go
func TestDiscoverSupportsGitWorktree(t *testing.T) {
	// create repo + worktree, then assert Discover succeeds from worktree path
}
```

### 10. Documentation and tooling drift from the actual build/runtime requirements

- Severity: Medium
- Explanation: the repository declares `go 1.26.1`, while the README still says “Requires Go 1.23+”. The `Makefile` advertises `golangci-lint`, but there is no repository lint config and the tool is not provisioned here. The result is avoidable onboarding friction and potentially failed local builds.
- Affected files:
  - `go.mod:3`
  - `README.md:19-27`
  - `Makefile:18-39`
- Suggested fix: document the real minimum supported Go version, add a checked-in `golangci` configuration if linting is part of the contract, and wire lint/test into CI so the documented workflow is the enforced workflow.
- Example improved code:

```yaml
version: "2"
linters:
  enable:
    - errcheck
    - govet
    - staticcheck
    - ineffassign
```

## Code Quality Findings

### Naming and idiomatic Go

- Package names are short and domain-focused.
- Exported symbol names are generally idiomatic and readable.
- Error wrapping is usually good, especially in command flows and persistence code.
- Early returns are used consistently.

### Naming / style issues

- `remoteUrl` in `internal/cmd/pull.go:64` should be `remoteURL` to follow Go initialism conventions.
- Comment quality is generally fine, but a few comments are stale relative to implementation status, especially around “planned” versus shipped behaviour.

### Large functions and files

Largest files by line count:

- `internal/session/store.go` — 758 LOC
- `internal/cluster/proxy.go` — 478 LOC
- `internal/cmd/session_flow_test.go` — 521 LOC
- `internal/cmd/session_flow.go` — 360 LOC
- `internal/cluster/kube.go` — 305 LOC

The code is not deeply nested overall, but the file sizes point to missing decomposition.

## Concurrency Risks

### Observed concurrency primitives

- Goroutines: `internal/cluster/portforward.go:61-65`
- Channels: `internal/cluster/portforward.go:57-59`
- Context cancellation: used across runtime, Forgejo, Git, session, and attach flows
- No mutexes, atomics, or wait groups found in production code

### Main risks

- Port-forward goroutines can outlive their active consumer.
- Poll loops in `internal/runtime/colima.go:110-128` and `internal/cluster/portforward.go:124-133` use `time.Sleep`, so cancellation responsiveness is coarser than it should be.
- No unbounded fan-out or obvious deadlocks were found elsewhere.

## Performance Issues

### Medium priority

- Issue 5: session listing uses N+1 queries.

### Low priority

- `runtime.waitReady` polls with `kubectl get nodes` every 5 seconds and sleeps directly: `internal/runtime/colima.go:110-128`.
- `waitForProxyReady` and similar loops use API polling rather than watches: `internal/cluster/proxy.go:312-331`.
- `waitForPort` performs repeated short-lived TCP dials: `internal/cluster/portforward.go:124-135`.

These are acceptable at current scale, but they are still worth tightening for CLI responsiveness.

## API Design Issues

- Consumer-defined interfaces are used effectively in some command paths:
  - `internal/cmd/down.go:13-19`
  - `internal/cmd/session_flow.go:27-35`
- That makes the provider-defined wide interfaces stand out as unnecessary abstraction.
- `internal/cluster/Manager` is already broad for the number of call sites and responsibilities it covers.
- `internal/agent/Agent` includes `Authenticate`, `Launch`, and `Attach`, but authentication is not materially integrated in the main flow today.

Recommendation: define interfaces where substitution is actually required, keep them small, and let concrete types carry the rest.

## Testing Gaps

### Strengths

- 19 `_test.go` files across the repository.
- Good use of `t.Parallel()` in unit tests.
- Several tests are table-driven.
- Session store, config validation, workload building, and Git fast-forward logic are covered reasonably well.

### Gaps

- No command test for `kagen open`.
- No worktree/submodule discovery tests.
- No real lint/static analysis enforcement in CI.
- A noticeable share of tests prove stub behaviour instead of integration behaviour.
- `internal/kubeexec` and `internal/ui` have no direct test coverage.

## Documentation Gaps

- README runtime/toolchain requirement is stale versus `go.mod`.
- README presents `kagen open` as a ready workflow, but the implementation is still stubbed behind `ErrNotImplemented`.
- There is no checked-in CI or lint configuration explaining the expected local toolchain.

## Security Risks

### High priority

- Issue 2: static Forgejo credentials and ephemeral transport details written into host Git config.
- Issue 4: mutable and runtime-installed artefacts in a security-sensitive path.

### Medium priority

- Static in-cluster credentials are hard-coded in multiple places:
  - `internal/forgejo/repo.go:32`
  - `internal/forgejo/repo.go:43-45`
  - `internal/forgejo/repo.go:184`
  - `internal/cmd/pull.go:64`
- The workspace sync init container also embeds the same static secret in the clone URL:
  - `internal/cluster/kube.go:96-107`

Recommendation: move these to Kubernetes Secrets scoped per repo/session and rotate them, or replace password auth with short-lived tokens.

## Refactoring Plan

### Phase 1: Fix correctness and security contracts

1. Finish the `open` workflow end to end.
2. Remove persistent localhost remotes with embedded credentials.
3. Support Git worktrees and `.git` file layouts.
4. Replace runtime package installation with pinned images.

### Phase 2: Reduce orchestration blast radius

1. Extract `StartService`, `AttachService`, `OpenService`, and `PullService` from `internal/cmd`.
2. Move provider interfaces to consumers where appropriate.
3. Split `internal/session/store.go` into focused files.

### Phase 3: Improve runtime resilience

1. Replace shell-based port-forward lifecycle with a managed handle or client-go implementation.
2. Make long poll loops fully cancellable.
3. Add integration tests for Forgejo transport, worktree discovery, and `open`.

## Quick Wins

- Rename `remoteUrl` to `remoteURL` in `internal/cmd/pull.go:64`.
- Update README to reflect the real Go version and current `open` status.
- Add a worktree discovery test before changing implementation.
- Add a checked-in linter config and CI job.
- Replace `time.Sleep` in readiness loops with cancellable timers.

## Long-term Improvements

- Treat Forgejo connectivity as a dedicated transport abstraction rather than ad-hoc port-forward + URL formatting.
- Introduce release-managed container images for workspace, toolbox, and proxy, all pinned by digest.
- Promote `internal/workload` into a stronger source of truth for the entire runtime pod, leaving `internal/cluster` to reconcile rather than mutate.
- Consider watch-based Kubernetes readiness checks instead of polling for better responsiveness.

## Overall Verdict

The repository is promising and already shows disciplined package naming, good internal API hygiene, and a healthy test baseline. The next step is not a wholesale rewrite. It is a hardening pass: finish the user-facing Forgejo workflow, stop leaking transport credentials into host Git state, support real-world repository layouts, and reduce orchestration concentration in `internal/cmd`.

Once those are addressed, the existing structure is good enough to scale into a genuinely production-ready CLI.
