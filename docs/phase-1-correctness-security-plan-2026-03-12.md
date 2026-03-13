# Phase 1 Plan: Correctness and Security Contracts

Date: 2026-03-12

Scope: execution plan for Phase 1 of the technical audit, focused on correctness, security, and contract hardening.

Source inputs:
- `docs/technical-audit-2026-03-12.md`
- `docs/ARCHITECTURE.md`
- `AGENTS.md`

Architecture alignment:
- No stored ADR exists yet.
- This plan is aligned to the documented project principles instead:
  - keep `internal/cmd` thin
  - treat `internal/workload` as the runtime source of truth
  - reuse shared port-forward and exec adapters
  - keep the host checkout canonical
  - fail closed when proxy enforcement is not active

## Executive Summary

Phase 1 should harden four repository contracts before broader refactoring work begins:

1. `kagen open` must become a real, reliable review workflow.
2. Forgejo transport must stop leaking credentials and transient ports into host Git configuration.
3. Git repository discovery must support real-world repository layouts such as worktrees.
4. Runtime bootstrap artefacts must become reproducible and supply-chain-controlled.

This phase should be treated as a product integrity milestone, not a cleanup pass. The output should be a codebase where the documented workflows are truthful, the security boundary is materially stronger, and the operational contracts are explicit.

## Goals

- Make the review and pull workflows correct for normal users.
- Remove the highest-risk credential handling flaw from the host-side Git path.
- Ensure repository discovery works for standard Git layouts beyond simple `.git` directories.
- Eliminate mutable runtime package installation from the proxy path.
- Preserve the existing architectural direction rather than introducing ad-hoc fixes.

## Non-Goals

- Full orchestration decomposition of `internal/cmd`
- Full session store decomposition
- Replacing all shell-outs to `git`, `kubectl`, or `colima`
- Broad UX redesign of CLI output
- E2E expansion beyond the flows directly touched by this phase

## Success Criteria

Phase 1 is complete when all of the following are true:

- `kagen open` establishes the transport it needs, opens a live review URL, and reports actionable failures.
- No host Git remote is persisted with embedded Forgejo credentials or ephemeral localhost ports.
- `git.Discover` works for normal repositories and worktrees.
- Proxy startup does not install packages from the network at container boot time.
- Image references used in the runtime path are pinned to controlled immutable artefacts.
- `make test` passes.
- `make lint` passes.
- Targeted end-to-end verification is either added or an explicit follow-up is documented if runtime constraints block it.

## Workstreams

## Workstream 1: Make `kagen open` a Real Contract

### Problem

`kagen open` is documented as a working command, but today it depends on an unimplemented `HasNewCommits` path and a hard-coded browser URL that does not own a matching port-forward.

### Primary files

- `internal/cmd/open.go`
- `internal/forgejo/service.go`
- `internal/forgejo/repo.go`
- `README.md`

### Deliverables

- Implement a live `HasNewCommits` strategy.
- Add a Forgejo HTTP transport flow owned by the `open` command or a dedicated coordinator/service.
- Derive the review URL from the actual active transport endpoint.
- Update README/help text only after implementation matches the behaviour.

### Design constraints

- Do not hard-code `localhost:3000` as if transport already exists.
- Do not introduce new direct `kubectl` shell-outs outside shared adapters.
- Keep the host repository canonical; `open` must remain read-oriented.

### Proposed implementation

1. Extend Forgejo service responsibilities just enough to support review navigation.
2. Introduce an explicit HTTP port-forward lifecycle in the Forgejo path.
3. Implement one of these `HasNewCommits` strategies:
   - preferred: compare Forgejo refs through fetched Git refs using transient transport
   - acceptable: query Forgejo API for branch head and compare with local branch SHA
4. Refactor `runOpen` so it:
   - discovers repo
   - ensures runtime context or fails clearly
   - establishes HTTP transport
   - checks reviewability
   - opens the live URL

### Acceptance criteria

- `kagen open` opens a reachable review page when reviewable work exists.
- `kagen open` prints a clear “no reviewable changes” message when appropriate.
- No dead URL is opened.
- A command test covers:
  - reviewable changes
  - no reviewable changes
  - Forgejo transport failure

### Test plan

- Unit tests for URL derivation and reviewability logic.
- Command tests around `runOpen`.
- Manual validation:
  - `kagen start codex`
  - produce reviewed branch state
  - run `kagen open`

## Workstream 2: Remove Persistent Credential Leakage from Host Git Configuration

### Problem

The current import and pull flows write a persistent `kagen` remote using credentials embedded in an ephemeral localhost URL. This weakens the security boundary and produces stale repository state.

### Primary files

- `internal/forgejo/repo.go`
- `internal/cmd/pull.go`
- `internal/git/repo.go`
- `internal/cluster/kube.go`

### Deliverables

- Replace persistent credentialed remotes with transient transport.
- Ensure pull and import use one-shot authenticated operations.
- Remove dependence on a stable host-side remote name for core correctness.

### Design constraints

- Do not write Forgejo credentials into `.git/config`.
- Do not require users to clean up temporary remotes manually.
- Keep `kagen pull` explicit and host-initiated.

### Proposed implementation

1. Extend `internal/git` with transient transport helpers, for example:
   - `FetchURL(ctx, url, refspecs...)`
   - `PushURL(ctx, url, refspecs...)`
2. Refactor Forgejo import to push directly to a transient URL rather than configuring a persistent remote.
3. Refactor pull to fetch from a transient URL into temporary refs or directly into the expected remote-tracking refs without leaving config behind.
4. Replace the static in-cluster password path with one of:
   - short-lived token injection
   - transient basic auth from a Secret-backed value
   - temporary remote plus explicit cleanup only if direct URL invocation proves impractical
5. Audit the workspace sync init container URL because it currently also embeds credentials.

### Acceptance criteria

- Running `kagen start`, `kagen pull`, or import flows does not leave credentials in `.git/config`.
- Running those flows twice does not accumulate stale remotes or stale ports.
- Git operations still succeed through Forgejo.

### Test plan

- Unit tests for new transient Git helper methods.
- Integration-style Git tests using temporary repositories and local HTTP endpoints where feasible.
- Manual validation:
  - inspect `.git/config` before and after `kagen pull`
  - inspect `.git/config` before and after initial import

## Workstream 3: Harden Repository Discovery

### Problem

Repository discovery currently assumes `.git` is a directory. That breaks worktrees and other valid Git layouts.

### Primary files

- `internal/git/repo.go`
- `internal/git/repo_test.go`
- `internal/cmd/*` paths that rely on discovery indirectly

### Deliverables

- Replace or harden repository root detection.
- Add coverage for worktree discovery.
- Preserve clear error semantics for “not a Git repository”.

### Design constraints

- Prefer correctness over manual filesystem cleverness.
- Preserve the existing `Repository` contract where possible.
- Keep error wrapping compatible with `errors.Is(err, kagerr.ErrNotGitRepo)`.

### Proposed implementation

1. Replace the custom `.git` directory walk with `git rev-parse --show-toplevel`, or augment the walker to recognise `.git` files if direct Git invocation proves cleaner.
2. Keep `currentBranch` and `headSHA` discovery in Git rather than reimplementing repository parsing.
3. Add tests for:
   - normal repository
   - nested subdirectory inside repository
   - worktree path
   - non-repository path

### Acceptance criteria

- Discovery succeeds in a Git worktree.
- Existing repository tests still pass.
- Error semantics remain stable for non-repositories.

### Test plan

- New worktree fixture test in `internal/git/repo_test.go`
- Regression tests for current discovery behaviour

## Workstream 4: Make Runtime Bootstrap Reproducible

### Problem

The proxy path installs packages with `apk add` at container startup, and the workspace image still uses a mutable `latest` tag. This undermines the “security-first” positioning and makes failures depend on third-party package availability.

### Primary files

- `internal/cluster/proxy.go`
- `internal/workload/builder.go`
- any build/release definitions added during implementation

### Deliverables

- Replace runtime package installation with a pinned proxy image containing its dependencies.
- Pin runtime-path images to immutable digests or tightly controlled versioned artefacts.
- Document image ownership and update procedure.

### Design constraints

- Preserve `internal/workload` as the source of truth for baseline pod image selection.
- Avoid embedding build logic into runtime reconciliation code.
- Keep local developer workflow reasonable; image management should be explicit, not magical.

### Proposed implementation

1. Create or adopt a `kagen-proxy` image with `tinyproxy` baked in.
2. Replace `proxyImage` in `internal/cluster/proxy.go` with a pinned immutable reference.
3. Replace `defaultWorkspaceImage` and any other mutable runtime-path images with pinned references.
4. Decide whether to store pinned image refs:
   - directly in Go constants for now
   - or in a small release-managed config structure if image churn is expected
5. Add tests asserting image references are not `latest`.

### Acceptance criteria

- Proxy pod starts without running package manager installation commands.
- Runtime path uses pinned image references.
- Tests fail if mutable `latest` tags reappear in core runtime images.

### Test plan

- Unit tests over rendered deployment/pod specs.
- Manual validation of proxy pod command line and startup behaviour.

## Cross-Cutting Work

## Workstream 5: Secrets, Error Contracts, and Documentation Alignment

### Deliverables

- Audit hard-coded Forgejo credentials and move them to a better secret-management boundary where feasible in this phase.
- Ensure all new errors are wrapped with clear operational context.
- Update README and command help only once behaviour is actually shipped.
- Update the technical audit document if implementation choices materially change the recommendation.

### Acceptance criteria

- No user-facing command advertises behaviour that is still stubbed.
- Failure paths remain actionable and include enough context for debugging.
- Security-sensitive constants are reduced or explicitly justified.

## Sequencing

Recommended execution order:

1. Workstream 3: repository discovery hardening
2. Workstream 2: transient Forgejo transport and credential removal
3. Workstream 1: real `open` workflow built on the hardened transport
4. Workstream 4: reproducible runtime artefacts
5. Workstream 5: documentation and contract reconciliation

Reasoning:

- Discovery correctness is low-risk and unblocks command reliability.
- Transport hardening should come before `open`, so review navigation is built on the final connection model.
- Runtime artefact hardening is operationally important but less entangled with command correctness.

## Package-Level Plan

### `internal/git`

- Add transient fetch/push primitives.
- Improve repository root detection.
- Keep host-side Git orchestration centralised here.

### `internal/forgejo`

- Split review transport concerns from repo bootstrap concerns if needed.
- Add explicit HTTP transport ownership for review flows.
- Stop assuming a persistent host remote exists.

### `internal/cmd`

- Keep command entrypoints thin.
- Prefer one orchestration function per user workflow.
- Avoid leaking transport and credential details into command code.

### `internal/cluster`

- Keep proxy reconciliation focused on desired state, not runtime package installation.
- Reuse shared adapters only.

### `internal/workload`

- Remain the source of truth for baseline runtime image selection.
- Add tests around pinned image references if the image constants continue to live here.

## Risks and Mitigations

### Risk: Git transient transport is awkward to express with the current helper surface

- Mitigation: extend `internal/git` rather than reintroducing direct shell-outs in command or Forgejo code.

### Risk: Removing persistent remotes may affect assumptions in pull/open flows

- Mitigation: land transport helpers first, then refactor command flows in a second commit.

### Risk: Image pinning may require parallel infra/release work

- Mitigation: allow a short-lived intermediate state using versioned non-`latest` tags only if digest pinning is blocked, and document the follow-up explicitly.

### Risk: E2E coverage may be expensive to maintain locally

- Mitigation: prioritise targeted unit and integration tests, and add a single narrow E2E check only for the most user-visible contract if feasible.

## Proposed Milestones

### Milestone A: Correct Host-Side Contracts

- Worktree-aware repository discovery
- Transient Git transport helpers
- No persistent credentialed remotes

### Milestone B: Correct Review Contract

- Functional `kagen open`
- Implemented `HasNewCommits`
- Updated docs/help text

### Milestone C: Reproducible Runtime Path

- Pinned proxy image
- No runtime package installation
- Pinned baseline runtime images

## Acceptance Checklist

- [ ] `make test`
- [ ] `make lint`
- [ ] worktree discovery test added
- [ ] `kagen open` command test added
- [ ] no Forgejo credentials written to `.git/config`
- [ ] no ephemeral localhost remote persisted after pull/import
- [ ] proxy startup no longer runs `apk add`
- [ ] runtime-path images are pinned
- [ ] README and command help reflect the shipped behaviour

## Recommended Deliverable Format

Implement Phase 1 as a small series of reviewable changes rather than one large branch:

1. `git`: discovery + transient transport helpers
2. `forgejo` and `pull`: credential-safe transport refactor
3. `open`: live review workflow
4. `proxy` and `workload`: reproducible artefact hardening
5. docs/tests cleanup

This keeps the host-source-of-truth model intact while reducing rollback risk and making regressions easier to isolate.
