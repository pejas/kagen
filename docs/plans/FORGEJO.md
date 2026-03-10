# Stage 4 — Forgejo Integration

Status: draft

## Goal

Replace `forgejo.StubService` with a working Forgejo deployment and Git import pipeline. After this stage, `kagen` deploys a Forgejo instance per repository namespace, imports the host repository into it, and can resolve review URLs.

## Dependencies

- **Stage 3 (Cluster)** — provides the namespace, PVCs, and the K8s client.

## Scope

### Forgejo Deployment

Deploy a single Forgejo instance per repo namespace using the official `codeberg.org/forgejo/forgejo` container image. The deployment uses:

- The `forgejo-data` PVC for `/data` (repository storage, database, config).
- A `Service` exposing HTTP (3000) and SSH (22) ports.
- A `NodePort` or port-forward for host access during `kagen open`.

On first creation, an init container or startup script provisions:

- An admin user (`kagen` / auto-generated password stored in a Secret).
- Forgejo's `app.ini` configured for local-only access (no registration, no public visibility).

### Repository Import

`ImportRepo` performs these steps:

1. Use the Forgejo API (`POST /api/v1/repos/migrate`) to create a mirror of the host repository. The source is a temporary bare clone served over the in-cluster Git protocol or pushed directly via SSH.
2. Alternatively, `git push` from an init container that has the repo content mounted as a ConfigMap or injected via a one-shot Job.
3. Record import provenance (already implemented in `internal/provenance/`).

The import must be idempotent. If the Forgejo repo already exists, the import updates it with a force push of the current branch state.

### Branch Management

After import:

1. If the `kagen/<current-branch>` branch does not exist in Forgejo, create it from the imported HEAD.
2. If it already exists, reuse it. The agent accumulates work on this branch.

### Review URL Resolution

`GetReviewURL` constructs the URL:

```
http://localhost:<forwarded-port>/<owner>/<repo>/compare/<base>...<kagen-branch>
```

Where `<forwarded-port>` is resolved from the Forgejo service's NodePort or an active port-forward.

### New Commits Detection

`HasNewCommits` compares the `kagen/<branch>` HEAD in Forgejo against the last recorded import provenance SHA. If they differ, there are new commits.

## Files

| Action | Path |
|--------|------|
| Modify | `internal/forgejo/forgejo.go` — replace stub with real implementation |
| New    | `internal/forgejo/deploy.go` — Forgejo K8s deployment and Service manifests |
| New    | `internal/forgejo/client.go` — Forgejo API client (repo creation, migration, branch listing) |
| New    | `internal/forgejo/portforward.go` — port-forward management for host access |
| New    | `internal/forgejo/deploy_test.go` |
| New    | `internal/forgejo/client_test.go` |
| Modify | `internal/cmd/open.go` — wire real GetReviewURL and port-forward |
| Modify | `internal/cmd/root.go` — wire real Forgejo service |

## Verification

- Unit tests for URL construction and commit comparison logic.
- Unit tests for deployment manifest generation.
- Integration test: deploy Forgejo into a test namespace → create repo via API → push a branch → verify `HasNewCommits` returns true → resolve review URL.
- Manual: run `kagen` in a real repo → confirm Forgejo is deployed → run `kagen open` → confirm the browser opens the correct compare page.
