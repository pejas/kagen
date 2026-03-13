# Runtime Artefacts

This document records the container artefacts currently used on the kagen runtime path.

## Current Artefacts

- Workspace image: `ghcr.io/pejas/kagen-workspace:2026-03-12`
- Toolbox image: `ghcr.io/pejas/kagen-toolbox:2026-03-12`
- Proxy image: `ghcr.io/pejas/kagen-proxy:2026-03-12`

These references are intentionally versioned rather than `latest`, so runtime
behaviour does not drift between runs.

The toolbox baseline is declared in:

- `packaging/runtime-images/toolbox/mise.toml`
- `packaging/runtime-images/toolbox/mise.lock`

Runtime startup assumes those artefacts are prebuilt. The agent pod and proxy
pod no longer install packages during container startup.

## Ownership

- `internal/workload` is the source of truth for baseline agent pod images.
- `internal/cluster` is the source of truth for the proxy image used by the enforced egress path.
- `packaging/runtime-images/` contains the Dockerfiles and `mise` inputs used to build future first-party runtime artefacts.
- Local validation can override the default refs with `KAGEN_WORKSPACE_IMAGE`, `KAGEN_TOOLBOX_IMAGE`, and `KAGEN_PROXY_IMAGE`.

## Update Procedure

1. Edit `packaging/runtime-images/toolbox/mise.toml` when the default toolchain changes.
2. Refresh `packaging/runtime-images/toolbox/mise.lock` with `make runtime-images-lock`.
3. Build and publish the replacement image artefacts from `packaging/runtime-images/`.
4. Verify the artefacts are pullable from the Colima/K3s runtime used by `kagen start`.
5. Update the pinned references in `internal/workload/builder.go` and `internal/cluster/proxy.go`.
6. Run `make test` and `make build`.
7. Run `make test-e2e` when the published artefacts are available and runtime-backed validation is needed.
8. If the runtime contract changes, update `README.md` and any affected architecture documentation in the same change.

## Follow-Up

Phase 1 uses controlled, release-managed image references. The next hardening
step is to move these versioned tags to immutable digests once the published
artefacts and release process are ready for digest pinning. Phase 2 remains
responsible for repository-local `mise.toml` overrides, trusted workspace
config handling, and persistent `mise` cache/data directories on agent state
storage.
