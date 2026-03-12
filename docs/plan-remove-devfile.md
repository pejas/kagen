# Plan: Remove Devfile and Move to Session-First Runtime

## 1. Intent

This plan replaces repository-level `devfile.yaml` with an internal, opinionated workload builder and a session-first model designed for:

1. One container image with all supported agent CLIs available.
2. Many agents working in the same repository context.
3. Persistent sessions with clear visibility and safe merge flow via Forgejo.

This plan incorporates your clarifications:

- `repo + session` is the core kagen identity.
- Agent runtime launch details still live in code (env, proxy, auth setup), not user prompts/skills.
- There are two session concepts:
  - **Kagen session**: container + workspace state.
  - **Agent session**: runtime-specific conversation/session state inside a kagen session.

---

## 2. Decisions Locked In

1. **No user devfile**
   - `devfile.yaml` is removed from the happy path.
   - Pod/workload spec is generated internally from typed Go config.

2. **Single toolbox image**
   - One pinned image contains `codex`, `claude`, `opencode`, `git`, `ripgrep`, and `kagen`.
   - Runtime installation at pod start is removed.

3. **Session identity**
   - Kagen session identity is `repo_id + session_id`.
   - Agent is not part of kagen session identity.

4. **Multiple agent sessions per kagen session**
   - A kagen session can host any number of agent sessions (including many Codex sessions).
   - Agent session state is persisted per agent/runtime where supported.

5. **CLI direction**
   - `kagen start codex` creates a new kagen session and attaches Codex.
   - `kagen attach codex --session 8` attaches Codex in kagen session `8`.
   - `kagen attach claude` attaches Claude to the most recent session.
   - `kagen list` shows sessions and status.

---

## 3. Target Architecture

## 3.1 New Core Model

### KagenSession (control plane object)

- `id` (numeric short id for CLI UX, e.g. `8`)
- `uid` (internal UUID)
- `repo_id`
- `repo_path`
- `base_branch`
- `workspace_branch` (Forgejo branch for this session)
- `head_sha_at_start`
- `namespace`
- `pod_name`
- `status` (`starting`, `ready`, `stopped`, `failed`)
- `created_at`, `last_used_at`

### AgentSession (nested runtime object)

- `id` (UUID)
- `kagen_session_uid`
- `agent_type` (`codex|claude|opencode`)
- `name` (optional human alias)
- `working_mode` (`shared_workspace|isolated_worktree`)
- `branch` (if isolated mode)
- `state_path` (runtime session path)
- `created_at`, `last_used_at`

## 3.2 Internal Packages

1. `internal/session`
   - Persistence and query for kagen sessions + agent sessions.

2. `internal/workload`
   - Replaces `internal/devfile` in runtime orchestration.
   - Builds baseline Pod/PVC specs from typed structs.
   - Intentional boundary: returns a baseline `*corev1.Pod`; `internal/cluster` injects orchestration concerns and reconciles it.

3. `internal/agentprofile`
   - Canonical runtime launch metadata (env, command, readiness, auth directories).

4. `internal/forgejobranch`
   - Session branch naming, merge policy checks, and PR visibility helpers.

---

## 4. Git / Forgejo Strategy

## 4.1 Branch Strategy (recommended)

For each kagen session:

- Workspace branch: `kagen/<base_branch>/s/<session_id>`

For parallel delegated work (optional isolated mode):

- Agent branch: `kagen/<base_branch>/s/<session_id>/a/<agent_session_id>`

Rationale:

- Keeps one clear branch per session by default.
- Supports high parallelism when needed without forcing every workflow into per-agent branches.
- Works with Forgejo review and merge policy cleanly.

## 4.2 Merge Policy

1. Default: PR from `kagen/<base>/s/<id>` to `<base_branch>`.
2. If isolated agent branches are used:
   - Merge agent branches into session branch first.
   - Then open one session PR to base branch.
3. Pull safeguards stay strict (`ff-only` where applicable, explicit conflict messaging).

---

## 5. CLI Specification (v2)

## 5.1 Commands

1. `kagen start <agent>`
   - Creates new kagen session.
   - Ensures runtime + Forgejo + workload.
   - Attaches requested agent.

2. `kagen attach <agent> [--session <id>]`
   - Attaches to existing session.
   - Without `--session`, uses most recent ready session for current repo.

3. `kagen list`
   - Shows sessions for current repo (id, status, branch, last used, active agents).

4. `kagen list --all`
   - Shows sessions across repos.

5. `kagen session stop --session <id>`
   - Stops pod, keeps metadata.

6. `kagen session resume --session <id> [--agent <agent>]`
   - Restarts and optionally attaches immediately.

## 5.2 Command Cleanup

1. Use `kagen start <agent>` as the primary entrypoint for creating a session and attaching an agent.
2. Remove `kagen init` entirely in favour of `kagen config write`, which writes an optional `.kagen.yaml` only.

---

## 6. Refactor Phases

## Phase 0: Reliability Baseline (must complete first)

Address known correctness issues before structural refactor:

1. Fix swallowed errors in cluster/forgejo paths.
2. Implement context-aware git command execution.
3. Add HTTP client timeouts and context propagation in Forgejo HTTP calls.
4. Replace placeholder `HasNewCommits` with real implementation or explicit `ErrNotImplemented`.
5. Tighten brittle port-forward parsing and local port handling.

Exit criteria:

- Existing behaviour remains green under `make test`.
- Failure paths are explicit and observable.

## Phase 1: Session Domain + Persistence

1. Add `internal/session` with persistent store (SQLite recommended in `~/.config/kagen/sessions.db`).
2. Add migration-safe schema for `kagen_sessions` and `agent_sessions`.
3. Introduce numeric session IDs for UX, UUID internally.

Exit criteria:

- `kagen list` can show persisted sessions even after CLI restart.

## Phase 2: Internal Workload Builder (Devfile shadow mode)

1. Add `internal/workload` builder that produces current-equivalent pod spec.
2. Wire `runRoot` to workload builder under feature flag.
3. Keep devfile parser only as temporary fallback.
4. Treat expected behaviour as the default/generated pod shape, not arbitrary user-customised devfile behaviour.

Exit criteria:

- New builder can run full flow without `devfile.yaml` when flag enabled.

## Phase 3: Toolbox Image

1. Create pinned image with all agent CLIs and `kagen`.
2. Remove runtime npm installs from bootstrap path.
3. Keep per-agent env injection and proxy enforcement in code.
4. Persist per-agent runtime homes under dedicated directories/volumes.
5. Split runtime metadata from legacy install-at-runtime bootstrap metadata where needed so the toolbox image path can stay clean.

Exit criteria:

- Cold start latency reduced.
- No network package installation at pod start.

## Phase 4: New CLI Session Flow

1. Implement `kagen start`, `kagen attach`, `kagen list`.
2. Make attach default to last active session when `--session` is omitted.
3. Record `last_used_at` on attach.

Exit criteria:

- Requested command UX works end-to-end.

## Phase 5: Agent Sessions and Parallel Delegation

1. Track agent sessions separately from kagen sessions.
2. Support multiple sessions per agent type in one kagen session.
3. Add optional isolated worktree mode for delegated parallel tasks.
4. Expose active agent sessions in `kagen list`.

Exit criteria:

- One kagen session can host several Codex/Claude/OpenCode sessions safely.

## Phase 6: Remove Devfile Fully

1. Remove `internal/devfile` from runtime path.
2. Update docs, e2e features, and the optional config-writing workflow.
3. Keep a short note for users with existing `devfile.yaml`.

Exit criteria:

- `devfile.yaml` no longer required by any command.

Phase 6 status note:

- `devfile.yaml` is no longer required by runtime commands.
- `internal/workload` is the only runtime workload source.
- Session-first CLI orchestration now targets generated workloads only.
- Compatibility continues to target the current generated/runtime flow rather than arbitrary historical devfile customisation.

---

## 7. Data Persistence and Isolation Rules

1. Workspace state persists per kagen session.
2. Agent runtime state persists per agent session path, not globally shared.
3. If "all agents in one container" is enabled, enforce path-level separation:
   - `/home/kagen/.codex/<agent_session_id>`
   - `/home/kagen/.claude/<agent_session_id>`
   - `/home/kagen/.opencode/<agent_session_id>`
4. Proxy and env policy remain enforced at container level.

---

## 8. Testing Strategy

1. Unit:
   - session store CRUD and migration tests.
   - branch naming and merge policy tests.
   - agent session path and command composition tests.

2. Integration:
   - start/attach/list lifecycle with persisted sessions.
   - recover attach after CLI restart.

3. E2E:
   - no-devfile startup.
   - create 1 kagen session + 2+ agent sessions.
   - delegated parallel mode with isolated worktrees.
   - pull/merge flow from session branch to base branch.

---

## 9. Risks and Mitigations

1. **Risk**: Shared workspace conflicts with many concurrent agents.
   - **Mitigation**: Isolated worktree mode for delegated parallel tasks.

2. **Risk**: One container with all agent binaries increases credential blast radius.
   - **Mitigation**: strict per-agent session directories, explicit env scoping, secret hygiene.

3. **Risk**: Session metadata drift from cluster reality.
   - **Mitigation**: reconciliation on `kagen list` and `kagen attach` (live pod status check).

4. **Risk**: Migration churn for existing users.
   - **Mitigation**: keep the command surface explicit and document the final command model clearly.

---

## 10. Suggested PR Slices

1. PR1: Reliability baseline fixes (Phase 0 only).
2. PR2: `internal/session` + persistence + `kagen list` read path.
3. PR3: `internal/workload` builder and shadow-mode orchestration.
4. PR4: toolbox image wiring and bootstrap removal.
5. PR5: `kagen start` + `kagen attach` + last-session logic.
6. PR6: agent sessions + isolated worktree mode.
7. PR7: full devfile removal, docs and e2e updates.
