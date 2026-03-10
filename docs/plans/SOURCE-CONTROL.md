# Stage 5 — Source Control Operations

Status: draft

## Goal

Implement the Git-level operations that bridge the host repository and the in-cluster Forgejo instance: import, branch creation, WIP commit on exit, and pull-back.

## Dependencies

- **Stage 4 (Forgejo)** — provides the deployed Forgejo instance and API client.
- **Stage 3 (Cluster)** — provides namespace and agent pod access.

## Scope

### Import Flow

On session start:

1. Resolve the host repository identity (path, branch, HEAD SHA) — already implemented in `internal/git/`.
2. Record import provenance — already implemented in `internal/provenance/`.
3. Push the current branch state to the Forgejo repository (handled by Stage 4's `ImportRepo`).
4. Persist the provenance record alongside the Forgejo data so recovery is possible.

### Branch Creation and Reuse

After import, ensure the working branch exists:

1. Query Forgejo API for `kagen/<current-branch>`.
2. If it does not exist, create it from the imported HEAD.
3. If it exists, verify it descends from the imported base. If not (e.g., the user rebased locally), warn and ask whether to force-reset.

This logic lives in `internal/forgejo/client.go` (branch operations) and is orchestrated from `internal/cmd/root.go`.

### WIP Commit on Exit

When the user exits the agent TUI and the agent workspace has uncommitted changes:

1. The exit handler (triggered by TUI detach) runs inside the agent pod.
2. Execute `git add -A && git commit -m "wip: kagen session checkpoint"` in the workspace.
3. Push the WIP commit to `kagen/<branch>` in Forgejo.
4. Print a summary: `"Session checkpoint pushed to kagen/<branch>."`.

If there are no changes, skip silently.

This requires a post-detach hook in the agent attach logic (Stage 1's `attach.go`). After `kubectl exec` returns, `kagen` runs the WIP commit sequence inside the pod before it terminates.

### Pull-Back Workflow

`kagen pull` performs:

1. Discover the host repo and current branch.
2. Resolve the Forgejo remote URL (via port-forward).
3. Add or update a Git remote named `kagen-forgejo` pointing at the Forgejo instance.
4. Fetch `kagen/<branch>` from the remote.
5. Merge `kagen-forgejo/kagen/<branch>` into the current local branch.
6. Clean up the temporary remote.

The merge strategy should default to a standard merge commit. A `--rebase` flag can be added later.

### Exit Without Changes

If the agent session produced no new commits in Forgejo (i.e., `HasNewCommits` returns false after detach), `kagen` exits quietly without printing the review URL.

If there are new commits, print:

```
Review changes: http://localhost:<port>/<owner>/<repo>/compare/...
Run `kagen pull` to merge them into your local branch.
```

## Files

| Action | Path |
|--------|------|
| Modify | `internal/agent/attach.go` — add post-detach WIP commit hook |
| New    | `internal/git/remote.go` — add/update/remove temporary Git remotes |
| New    | `internal/git/merge.go` — fetch and merge from Forgejo remote |
| Modify | `internal/cmd/pull.go` — replace stub with real pull workflow |
| Modify | `internal/cmd/root.go` — add exit handling after TUI detach |
| New    | `internal/git/remote_test.go` |
| New    | `internal/git/merge_test.go` |

## Verification

- Unit tests for remote management (add, update, remove).
- Unit tests for WIP commit detection (dirty workspace vs clean).
- Integration test: create a test repo → push to Forgejo → run WIP commit → pull back → verify commit history.
- Manual: run a full `kagen` session → make changes via the agent → exit → confirm WIP commit appears in Forgejo → run `kagen pull` → confirm changes are in local branch.
