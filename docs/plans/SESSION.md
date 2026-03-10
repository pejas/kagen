# Stage 7 — Session Lifecycle and Recovery

Status: draft

## Goal

Wire the full end-to-end session lifecycle: first-run bootstrap, daily resume, exit handling, recovery from crashes, and the `kagen open` / `kagen pull` user-facing workflows. After this stage, `kagen` is feature-complete for v1.

## Dependencies

- All previous stages (1–6).

## Scope

### First-Run Flow

Consolidate the steps already partially implemented in the root command into a robust first-run path:

1. Detect missing bootstrap files → direct user to `kagen init`.
2. Verify runtime dependencies (Stage 2).
3. Agent selection (Stage 1).
4. Start Colima/K3s (Stage 2).
5. Create namespace and resources (Stage 3).
6. Deploy Forgejo and import repo (Stage 4).
7. Deploy proxy and validate enforcement (Stage 6).
8. Authenticate agent (Stage 1).
9. Launch agent pod and attach TUI (Stages 1 + 3).

Each step should report progress to the terminal using `internal/ui`.

### Daily Resume Flow

When all resources already exist:

1. Verify runtime health.
2. Verify namespace and resources are intact.
3. Check if the host branch has advanced since the last import — if so, push the new state to Forgejo.
4. Resume or restart the agent pod.
5. Attach TUI.

The difference from first-run is that most steps are fast no-ops. The user should see minimal output.

### Exit Handling

After the TUI detaches:

1. Check for uncommitted workspace changes → WIP commit (Stage 5).
2. Check `HasNewCommits` → print review URL if true.
3. Leave the cluster running. Do not tear down resources on exit.

### Recovery

If the agent pod crashed:

1. On next `kagen` invocation, detect the pod is in `CrashLoopBackOff` or `Error` state.
2. Reclone `kagen/<branch>` from Forgejo into a fresh emptyDir workspace.
3. Restart the agent pod.
4. Report what happened: `"Agent pod was restarted. Workspace recovered from last Forgejo commit."`.

Uncommitted RAM-disk edits are lost. This is stated in the design as accepted behaviour.

### `kagen open` Completion

Wire the full flow:

1. Discover repo.
2. Port-forward to Forgejo.
3. Check `HasNewCommits` — if false, print `"No reviewable changes."` and exit.
4. Resolve review URL.
5. Open browser.

### `kagen pull` Completion

Wire the full flow (Stage 5's pull-back workflow):

1. Discover repo.
2. Port-forward to Forgejo.
3. Add temporary remote.
4. Fetch and merge `kagen/<branch>`.
5. Clean up remote.
6. Report result.

### Idempotency Audit

Review the entire root command flow and verify that every step is safe to run repeatedly. No destructive operations should occur on re-run.

## Files

| Action | Path |
|--------|------|
| Modify | `internal/cmd/root.go` — consolidate first-run vs daily-resume branching, add exit handler |
| Modify | `internal/cmd/open.go` — wire full flow with port-forward and review URL |
| Modify | `internal/cmd/pull.go` — wire full pull-back workflow |
| New    | `internal/session/session.go` — session state tracking (first-run vs resume detection) |
| New    | `internal/session/recovery.go` — pod crash detection and recovery logic |
| New    | `internal/session/session_test.go` |
| New    | `internal/session/recovery_test.go` |

## Verification

- Unit tests for session state detection (first-run vs resume).
- Unit tests for recovery logic (pod state → recovery action).
- Integration test: full lifecycle — init → start → make changes → exit → verify WIP → pull.
- Integration test: crash recovery — kill agent pod → re-run kagen → verify workspace reclone.
- Manual E2E: run through the daily workflow with each agent type. Verify exit messages, review URLs, and pull-back all work correctly.
