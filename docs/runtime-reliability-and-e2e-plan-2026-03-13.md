# Runtime Reliability and E2E Plan (2026-03-13)

## Purpose

This document breaks runtime reliability and end-to-end coverage improvements into discrete execution phases for `kagen`.

Each phase is intentionally scoped so it can be executed in a clean context by a future agent with a simple instruction such as:

> Read `docs/runtime-reliability-and-e2e-plan-2026-03-13.md` and execute Phase 2 only. Do not overstep into later phases.

The goal is to improve diagnosis, observability, and confidence in the runtime image and agent launch flow without mixing unrelated work into one large change.

## Context

Recent Phase 1 runtime-image work exposed several failure modes that were hard to diagnose quickly:

- image pull denial for first-party images
- Forgejo workspace sync auth mismatch
- workspace init volume ownership issues
- proxy policy timing race during bootstrap
- invalid generated shell in agent readiness probing
- agent home PVC permissions preventing state-path creation
- agent binaries present in the image but not on the effective shell `PATH`

These issues shared two root causes:

1. `kagen` did not expose enough step-level state to show exactly where `start` or `attach` was blocked.
2. E2E coverage proved that the CLI printed expected progress messages, but it did not prove that the runtime became genuinely usable.

## Non-Goals

These phases should not be used to introduce unrelated architecture changes.

Out of scope unless a phase explicitly says otherwise:

- changing the baseline cluster architecture
- replacing `client-go` with other Kubernetes integration approaches
- adding new direct `kubectl` shell-outs outside existing shared adapters
- redesigning the session persistence model
- publishing production image digests or release automation
- broad refactors to agent-specific runtime behaviour beyond reliability work

## Working Rules

Apply these rules in every phase:

- Keep `internal/workload` as the source of truth for baseline pod generation.
- Keep `internal/cmd` thin; place orchestration logic in coordinators or focused packages.
- Reuse shared adapters for `kubectl` exec and port-forward behaviour.
- Prefer structured, machine-readable diagnostics over ad hoc string logging.
- Do not fold later-phase work into the current phase, even if it looks adjacent.
- Every phase must include tests and documentation updates for its own changes.

## Execution Pattern

For every phase:

1. Read this document and the current architecture/runtime docs.
2. Execute only the named phase.
3. If later-phase issues are discovered, record them as follow-up notes only.
4. Validate only the changes required for the current phase.
5. Leave the codebase in a state that the next phase can start from cleanly.

---

## Phase 1: Step-Level Runtime Diagnostics

### Goal

Make `kagen start` and `kagen attach` explain exactly which step is running, which step failed, and how long each step took.

### Why This Phase Exists

The recent failures often surfaced only as a stall at:

- `Launching agent codex...`
- `Launching agent opencode...`
- `Launching agent claude...`

In reality, the fault might have been image pull, init container auth, proxy policy timing, readiness probing, or attach preparation. We need a first-class notion of command phases.

### Scope

Implement a command-scoped runtime operation trace with structured step records.

Include:

- step name
- step status: pending, running, succeeded, failed
- started/finished timestamps
- duration
- concise error summary
- key identifiers such as session ID, namespace, pod name, agent type, image refs where relevant

### Suggested Design

- Add a small internal package for operation tracing or step recording.
- Use it in `start` and `attach` flows first.
- Record steps for at least:
  - `ensure_runtime`
  - `ensure_namespace`
  - `ensure_proxy`
  - `ensure_resources`
  - `forgejo_import`
  - `launch_agent_runtime`
  - `validate_proxy_policy`
  - `prepare_agent_state`
  - `attach_agent`
- Surface a concise human-readable summary in verbose mode.
- Persist the trace summary with the kagen session, or persist a per-session artefact path if full persistence needs a later schema step.

### Out of Scope

- automatic collection of pod logs or describe output
- new CLI commands such as `doctor`
- interactive TUI changes
- E2E coverage beyond what is needed to validate this phase

### Deliverables

- structured step-tracing implementation for `start` and `attach`
- tests for successful and failed step recording
- updated docs describing how to read runtime step output

### Validation

- `make test`
- `make build`
- targeted tests proving that failures point at the precise failed step instead of a generic launch message

### Handoff Instruction

Read `docs/runtime-reliability-and-e2e-plan-2026-03-13.md` and execute Phase 1 only. Implement step-level runtime diagnostics for `start` and `attach`. Do not add artefact collection, `doctor`, or new E2E tiers yet.

---

## Phase 2: Failure Artefact Capture

### Goal

When a runtime command fails, automatically capture the most useful local and cluster diagnostics and store them in a predictable location.

### Why This Phase Exists

The recent debugging loop required manual inspection of:

- pod status
- init container logs
- agent container logs
- pod events
- proxy readiness state
- image refs
- session state

That information should be captured automatically on failure.

### Scope

Capture failure artefacts for `start` and `attach` after a failed step is known.

Create a per-session debug directory that stores, where available:

- session summary JSON or text
- step trace summary
- pod status snapshot
- pod events snapshot
- `workspace-sync` logs
- agent container logs
- proxy deployment readiness snapshot
- selected image refs and agent type

### Suggested Design

- Add a dedicated diagnostic collector package or coordinator helper.
- Reuse `client-go` for pod, event, deployment, and object inspection where practical.
- Reuse existing shared adapters only for log/exec behaviour that already belongs in shared adapters.
- Print the final artefact directory on failure.

### Out of Scope

- adding OpenTelemetry
- creating a rich `doctor` command
- changing the session schema unless strictly needed
- new E2E orchestration modes

### Deliverables

- failure artefact collector
- deterministic artefact directory layout
- tests proving artefacts are collected for representative failures
- docs describing where failure artefacts live and how to inspect them

### Validation

- `make test`
- `make build`
- targeted failure-path verification showing a printed artefact path

### Handoff Instruction

Read `docs/runtime-reliability-and-e2e-plan-2026-03-13.md` and execute Phase 2 only. Build automatic failure artefact capture on top of the Phase 1 step trace. Do not add `doctor`, OpenTelemetry, or new attach modes.

---

## Phase 3: Preflight Validation and Failure Classification

### Goal

Fail fast when the runtime can detect a known bad state before creating or launching a session.

### Why This Phase Exists

Several recent failures were deterministic before attach:

- invalid or unavailable image refs
- missing agent binary in the toolbox image
- bad home-volume permissions
- proxy not enforceable for the selected policy

These should become explicit validation failures rather than delayed runtime stalls.

### Scope

Add preflight checks and typed failure classification for known launch prerequisites.

Potential checks:

- selected image refs resolved from config and environment
- required runtime binary expected for the chosen agent
- agent home path writable after pod bootstrap
- workspace mount present
- proxy policy enabled and enforceable when configured

### Suggested Design

- Represent failures with typed categories such as:
  - `image_error`
  - `workspace_bootstrap_error`
  - `proxy_error`
  - `agent_binary_error`
  - `agent_home_error`
  - `attach_error`
- Use those categories in step traces and failure artefacts.
- Add a small preflight report object that can be logged and later reused by a future `doctor` command.

### Out of Scope

- new user-facing remediation workflows
- automatic repair attempts
- release automation for publishing images

### Deliverables

- preflight validation layer for launch-critical checks
- typed failure classification used by step tracing and artefact capture
- tests for representative validation failures
- doc updates for preflight semantics

### Validation

- `make test`
- `make build`
- targeted tests for each failure class added in this phase

### Handoff Instruction

Read `docs/runtime-reliability-and-e2e-plan-2026-03-13.md` and execute Phase 3 only. Add preflight validation and typed failure classes. Do not add self-healing, release automation, or a full `doctor` command.

---

## Phase 4: Detachable Runtime Launch Mode

### Goal

Introduce a non-interactive success contract for runtime startup so E2E tests can verify readiness without depending on a live interactive terminal.

### Why This Phase Exists

Right now, `start` combines:

- runtime creation
- repo import
- proxy enforcement
- agent launch
- interactive attach

That makes E2E brittle and makes it harder to distinguish launch success from TTY behaviour.

### Scope

Add a `start --detach` mode, or equivalent, that:

- creates the session
- fully launches and validates runtime readiness
- persists the session as ready
- does not attach interactively

Optionally consider a matching `attach --check` or `attach --detach` only if needed for symmetry, but do not broaden scope unless necessary.

### Suggested Design

- Keep the existing `start` interactive behaviour as default.
- Route detached and interactive variants through the same workflow up to the attach boundary.
- Ensure session status transitions remain correct.

### Out of Scope

- TUI redesign
- background agent supervisors
- persistent daemon mode

### Deliverables

- detached start mode
- session-flow tests for detached success/failure
- docs describing when to use detached mode

### Validation

- `make test`
- `make build`
- targeted verification that `start --detach` exits zero for a healthy runtime and leaves a ready session

### Handoff Instruction

Read `docs/runtime-reliability-and-e2e-plan-2026-03-13.md` and execute Phase 4 only. Add a detached runtime launch mode suitable for automation. Do not redesign interactive attach behaviour beyond what is necessary.

---

## Phase 5: Real Cluster Non-Interactive E2E Coverage

### Goal

Upgrade E2E so it proves the runtime is actually ready, not just that the CLI printed expected messages.

### Why This Phase Exists

The current E2E suite mostly validates output strings. It would not have caught most of the runtime-image regressions we just fixed.

### Scope

Expand E2E coverage around `start --detach` and cluster state assertions.

Core scenarios:

- `start --detach codex`
- `start --detach claude`
- `start --detach opencode`
- `attach` into an existing ready session
- `down` then `attach` to recreate runtime from persisted session state
- workspace branch sync correctness
- proxy enforcement for an allowlisted agent
- session transitions: starting to ready, starting to failed

Each scenario should assert more than CLI output:

- session status in the store
- pod running and containers ready
- init container completed
- workspace checked out to `kagen/<branch>`
- agent state directory created in the correct home path
- proxy deployment and policy present when required

### Suggested Design

- Keep these tests non-interactive.
- Use helper utilities to inspect session state and Kubernetes objects.
- Avoid requiring manual terminal interaction.

### Out of Scope

- PTY-driven agent UI tests
- chaos or fault-injection testing
- broad fixture frameworks beyond what is needed here

### Deliverables

- stronger real-cluster e2e suite
- helpers for asserting pod/session/proxy/workspace state
- docs for local execution expectations and prerequisites

### Validation

- `make test-e2e`
- verify the suite fails when core readiness expectations are intentionally broken

### Handoff Instruction

Read `docs/runtime-reliability-and-e2e-plan-2026-03-13.md` and execute Phase 5 only. Expand real-cluster non-interactive E2E around detached start and readiness assertions. Do not add PTY/TUI automation yet.

---

## Phase 6: Interactive Agent Smoke Tests

### Goal

Add minimal but real interactive validation that each supported agent can attach and reach its expected startup UI.

### Why This Phase Exists

A runtime can be fully ready and still fail at the final interactive hand-off. We need a small number of high-value smoke tests for the TTY path.

### Scope

Add PTY-based smoke tests for:

- Codex
- Claude
- OpenCode

Each test should:

- launch from a ready session
- attach through the real CLI path
- detect a credible startup signal
- exit cleanly with Ctrl+C or equivalent

### Suggested Design

- Keep these tests sparse and stable.
- Focus on startup signal detection, not deep interaction.
- Gate them behind an explicit E2E subset or marker if needed to manage runtime and flakiness.

### Out of Scope

- full conversational scripting of agents
- screenshot diffing or UI regression testing
- product-level behavioural tests inside each agent

### Deliverables

- PTY-capable smoke tests
- helper layer for interactive attach assertions
- docs on how to run and interpret interactive smoke results

### Validation

- targeted PTY smoke runs in CI or a controlled local environment
- manual confirmation that tests can recover cleanly from Ctrl+C termination

### Handoff Instruction

Read `docs/runtime-reliability-and-e2e-plan-2026-03-13.md` and execute Phase 6 only. Add minimal PTY smoke coverage for supported agents. Do not expand into full agent interaction test suites.

---

## Phase 7: Optional `doctor` Command

### Goal

Provide a user-facing diagnostic command that summarises session state, runtime steps, and captured artefacts.

### Why This Phase Exists

Once Phases 1 through 6 are in place, the system will already have rich diagnostics. A `doctor` command becomes a thin presentation layer over information we already capture.

### Scope

Add a `kagen doctor` command, ideally session-scoped, that reports:

- latest operation trace
- current session status
- runtime pod and container state
- proxy enforcement state
- diagnostic artefact directory if present

### Suggested Design

- Build on top of persisted or reconstructable diagnostics from earlier phases.
- Prefer read-only inspection.
- Keep output concise and actionable.

### Out of Scope

- automatic remediation
- advanced telemetry backends
- support bundles uploaded to remote services

### Deliverables

- `doctor` command
- tests for rendering and basic session inspection
- docs for troubleshooting workflows

### Validation

- `make test`
- `make build`
- targeted command verification against healthy and failed sessions

### Handoff Instruction

Read `docs/runtime-reliability-and-e2e-plan-2026-03-13.md` and execute Phase 7 only. Implement a user-facing `doctor` command using diagnostics created in earlier phases. Do not add self-healing or remote telemetry export.

---

## Recommended Order

Execute phases strictly in this order:

1. Phase 1: Step-Level Runtime Diagnostics
2. Phase 2: Failure Artefact Capture
3. Phase 3: Preflight Validation and Failure Classification
4. Phase 4: Detachable Runtime Launch Mode
5. Phase 5: Real Cluster Non-Interactive E2E Coverage
6. Phase 6: Interactive Agent Smoke Tests
7. Phase 7: Optional `doctor` Command

## Why This Order

- Phases 1 to 3 make failures visible and diagnosable.
- Phase 4 creates the clean automation boundary E2E needs.
- Phases 5 and 6 then improve confidence with better test coverage.
- Phase 7 becomes easy because the diagnostic substrate already exists.

## Success Criteria Across All Phases

This plan is complete when:

- `start` and `attach` failures identify the precise failing phase
- failures automatically leave useful local diagnostic artefacts
- known-bad states fail fast before misleading stalls
- detached runtime startup is available for automation
- E2E proves real runtime readiness across supported agents
- interactive attach is smoke-tested in a controlled way
- troubleshooting a failed session no longer requires ad hoc manual cluster inspection
