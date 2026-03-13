# Runtime Artefacts

This document records the release-managed container artefacts used on the kagen runtime path.

## Current Artefacts

- Workspace image: `ghcr.io/pejas/kagen-workspace:2026-03-12`
- Toolbox image: `ghcr.io/pejas/kagen-toolbox:2026-03-12`
- Proxy image: `ghcr.io/pejas/kagen-proxy:2026-03-12`

These references are intentionally versioned rather than `latest`, so runtime behaviour does not drift between runs.

## Ownership

- `internal/workload` is the source of truth for baseline agent pod images.
- `internal/cluster` is the source of truth for the proxy image used by the enforced egress path.
- Updating runtime artefacts is a release concern, not an ad-hoc runtime bootstrap concern.

## Update Procedure

1. Build and publish the replacement image artefacts.
2. Update the pinned references in `internal/workload/builder.go` and `internal/cluster/proxy.go`.
3. Run `make test` and `make lint`.
4. If the runtime contract changes, update `README.md` and any affected architecture documentation in the same change.

## Follow-Up

Phase 1 replaces mutable tags and runtime package installation with controlled, release-managed image references. The next hardening step is to move these versioned tags to immutable digests once the published artefacts and release process are ready for digest pinning.
