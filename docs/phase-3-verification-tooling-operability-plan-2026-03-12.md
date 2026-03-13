# Phase 3 Plan: Verification, Tooling, and Operability

Date: 2026-03-12

Scope: execution plan for Phase 3 of the technical audit, focused on making the repository trustworthy to change: stronger tests, enforced tooling, truthful documentation, and better operational diagnostics.

Source inputs:
- `docs/technical-audit-2026-03-12.md`
- `docs/phase-1-correctness-security-plan-2026-03-12.md`
- `docs/phase-2-architecture-maintainability-plan-2026-03-12.md`
- `docs/ARCHITECTURE.md`
- `docs/CONVENTIONS.md`
- `AGENTS.md`

Architecture alignment:
- Documented workflows must match shipped behaviour.
- User-facing correctness and security contracts deserve direct test coverage.
- Tooling should enforce the same standards the docs promise.
- Operational diagnostics should make local runtime failures debuggable without reading implementation internals.

## Executive Summary

Phase 3 is the confidence phase. After Phase 1 fixes the most important correctness/security contracts and Phase 2 reduces structural blast radius, the next priority is ensuring those improvements stay true over time. Today the repository has good unit-test habits, but weak enforcement around linting/CI, limited contract-style coverage for key workflows, and documentation/tooling drift that allows reality to diverge from the intended architecture.

The goal of this phase is to make quality self-sustaining. The repository should exit Phase 3 with:

- enforced lint and test expectations;
- stronger contract and integration coverage for the workflows that matter most;
- E2E scope that is intentional rather than incidental;
- documentation that stays aligned with implementation;
- CLI/runtime diagnostics that reduce debugging time.

## Goals

- Turn linting and core test execution into an enforced repository contract.
- Increase coverage for workflow correctness, transport hygiene, and discovery edge cases.
- Make E2E testing explicit, maintainable, and worth running.
- Improve diagnostics for runtime, Forgejo, and transport failures.
- Keep repository docs truthful and useful for contributors.

## Non-Goals

- Broad platform expansion beyond the current local-runtime model
- Full observability stack integration
- Performance benchmarking beyond targeted checks
- Replacing the existing BDD-style E2E approach unless it becomes a blocker

## Success Criteria

Phase 3 is complete when all of the following are true:

- A checked-in lint configuration exists and `make lint` is a documented, working repository contract.
- CI runs at least build, unit/integration test, and lint on every PR or equivalent branch workflow.
- Key correctness/security contracts have direct automated coverage.
- E2E scope is intentionally defined and documented.
- README, architecture docs, and conventions remain aligned with shipped behaviour.
- Verbose or actionable diagnostics exist for the main local-runtime failure modes.

## Workstreams

## Workstream 1: Establish Tooling and CI as Enforced Contracts

### Problem

The repository advertises linting in `Makefile`, but there is no checked-in lint configuration and local success depends on environment setup. That weakens the credibility of the documented development workflow.

### Primary files

- `Makefile`
- repository root lint config to be added
- CI workflow files to be added
- `README.md`

### Deliverables

- Checked-in `golangci-lint` configuration.
- CI workflow for:
  - build
  - unit/integration test
  - lint
- Documentation for required local tooling.

### Acceptance criteria

- `make lint` works with documented setup.
- CI enforces the same contract the README describes.
- Toolchain drift becomes visible early.

## Workstream 2: Expand Contract-Level Test Coverage

### Problem

Unit tests are decent, but several key contracts still lack explicit coverage: review transport, host Git hygiene, worktree discovery, and late transport failure handling.

### Priority contract areas

- worktree-safe repository discovery
- transient Forgejo transport without host Git pollution
- `kagen open` reviewability and live URL semantics
- `kagen pull` safety behaviour
- proxy fail-closed validation
- port-forward lifecycle ownership

### Deliverables

- Behaviour-oriented tests for these contracts.
- Reduced emphasis on tests that only confirm stub behaviour when real behaviour is available.

### Acceptance criteria

- The highest-risk workflows from Phase 1 and Phase 2 each have direct automated coverage.
- Regressions in host Git hygiene or transport correctness are caught by tests.

## Workstream 3: Rationalise E2E Coverage

### Problem

The repository has E2E tests, but the boundary between what should be covered by E2E versus integration tests is not explicit enough.

### Deliverables

- A documented E2E scope that explains:
  - what must be E2E
  - what should remain integration-tested
  - when contributors are expected to run `make test-e2e`
- A lean E2E set focused on user-visible end-to-end guarantees rather than every internal path.

### Suggested E2E contract set

- start a session
- attach to an existing session
- open/pull reviewed changes if feasible in the local runtime
- proxy enforcement path if practical

### Acceptance criteria

- E2E tests are fewer, sharper, and easier to trust.
- Default contributor workflows stay fast.

## Workstream 4: Improve Diagnostics and Failure Reporting

### Problem

The code generally wraps errors well, but local-runtime failures still require too much internal knowledge to diagnose quickly. Port-forward, runtime readiness, and Forgejo bootstrap failures would benefit from more structured operational context.

### Primary areas

- `internal/runtime`
- `internal/forgejo`
- `internal/cluster/portforward.go`
- command workflow services from Phase 2
- `internal/ui`

### Deliverables

- Better verbose-mode reporting for:
  - runtime bootstrap steps
  - Forgejo readiness and bootstrap retries
  - transport/port-forward lifecycle
  - proxy readiness
- Consistent user-facing summaries with enough context to act.

### Design constraints

- Do not replace wrapped errors with noisy logs.
- Keep default output concise; richer diagnostics should be gated behind `--verbose` or equivalent debug paths.

### Acceptance criteria

- Common local failures are diagnosable from CLI output alone.
- Verbose mode adds useful context without overwhelming normal command output.

## Workstream 5: Keep Documentation Truthful and Maintained

### Problem

Documentation drift was a material audit finding. Once implementation stabilises, docs must be treated as enforceable product contracts rather than passive notes.

### Deliverables

- README aligned with actual workflow behaviour and toolchain requirements.
- Architecture and conventions docs updated alongside structural changes.
- A lightweight maintainer checklist for updating docs when user-facing contracts change.

### Acceptance criteria

- No documented command is materially ahead of or behind implementation.
- Developer docs describe the actual supported workflow.

## Recommended Sequencing

1. Workstream 1: tooling and CI
2. Workstream 2: contract-level tests
3. Workstream 4: diagnostics
4. Workstream 3: E2E rationalisation
5. Workstream 5: documentation maintenance contract

Reasoning:

- CI and linting should exist before expanding quality scope.
- Contract tests provide the biggest confidence gain after infrastructure cleanup.
- Diagnostics become easier to shape once workflow seams from Phase 2 exist.

## Metrics to Track

Suggested success indicators:

- `make lint` and `make test` green in CI
- count of user-facing workflows with dedicated contract tests
- count of stale doc corrections required after feature work
- time-to-diagnose for common runtime failures during manual validation

These do not need to become formal dashboards, but they should guide whether Phase 3 actually improved repository confidence.

## Risks and Mitigations

### Risk: CI becomes flaky because runtime-dependent checks are too broad

- Mitigation: keep default CI to build, lint, and non-E2E tests; isolate heavier runtime-dependent checks.

### Risk: Additional tests overfit current implementation details

- Mitigation: prefer contract assertions over internal call-order assertions.

### Risk: Diagnostics become noisy

- Mitigation: keep default CLI output minimal and gate extra detail behind verbose mode.

### Risk: Documentation work is deferred again

- Mitigation: treat doc alignment as part of completion criteria for user-facing workflow changes.

## Acceptance Checklist

- [ ] checked-in lint configuration
- [ ] CI for build, test, lint
- [ ] contract tests for discovery, transport hygiene, review, and pull
- [ ] explicit E2E scope documentation
- [ ] improved verbose diagnostics for key runtime flows
- [ ] README/architecture/conventions aligned with shipped behaviour
- [ ] `make test`
- [ ] `make lint`

## Suggested Deliverable Breakdown

1. lint config + CI
2. contract test expansion
3. diagnostics improvements
4. E2E scope reduction and documentation
5. documentation maintenance checklist and final alignment pass

Phase 3 should leave the repository in a state where future changes are easier to validate than to accidentally break.
