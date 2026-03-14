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

`kagen start` validates the resolved workspace, toolbox, and proxy image
references before it persists a new session. A malformed override should fail
the preflight path with `image_error` rather than stalling later in runtime
launch.

## Ownership

- `internal/workload` is the source of truth for baseline agent pod images.
- `internal/cluster` is the source of truth for the proxy image used by the enforced egress path.
- `packaging/runtime-images/` contains the Dockerfiles and `mise` inputs used to build future first-party runtime artefacts.
- Local validation can override the default refs with `KAGEN_WORKSPACE_IMAGE`, `KAGEN_TOOLBOX_IMAGE`, and `KAGEN_PROXY_IMAGE`.
- Runtime preflight validates only launch-critical shape and resolution. It does not repair bad image refs, publish artefacts, or replace runtime-backed verification.

## Publish Workflow

`.github/workflows/docker-publish.yml` publishes four multi-architecture images:

- `ghcr.io/pejas/kagen-base`
- `ghcr.io/pejas/kagen-workspace`
- `ghcr.io/pejas/kagen-toolbox`
- `ghcr.io/pejas/kagen-proxy`

The workflow runs on Git tag pushes matching `v*` and on manual dispatch.

For a tag push such as `v1.2.3`, `docker/metadata-action` emits these tags for
each image:

- `1.2.3`
- `1.2`
- `1`
- the short Git commit SHA
- `latest`

The base image is built first. The workspace, toolbox, and proxy images are
then built from `packaging/runtime-images/` and receive
`KAGEN_BASE_IMAGE=ghcr.io/pejas/kagen-base:${{ github.ref_name }}` as a build
argument. On a version-tag push, `github.ref_name` is the pushed tag, so the
dependent images consume the matching base-image tag.

Manual dispatch is not equivalent to a version-tag push. The workflow allows a
manual `inputs.tag`, but the dependent-image build still resolves
`KAGEN_BASE_IMAGE` from `github.ref_name`, and the verification step also checks
`${GITHUB_REF_NAME}`. In practice, manual dispatch is reliable only when the
selected ref name already matches a published base-image tag. The release path
for coherent versioned artefacts is a pushed Git tag such as `v2026-03-14` or
`v1.2.3`.

## How Kagen Selects Runtime Images

The publish workflow does not change the images used by `kagen start` on its
own. Kagen resolves runtime images from code-level defaults first, then applies
environment-variable overrides:

- Workspace and toolbox defaults are pinned in `internal/workload/builder.go`.
- The proxy default is pinned in `internal/cluster/proxy.go`.
- One-off overrides come from `KAGEN_WORKSPACE_IMAGE`,
  `KAGEN_TOOLBOX_IMAGE`, and `KAGEN_PROXY_IMAGE`.

`.kagen.yaml` does not define image references. `kagen config write` does not
persist them.

Use overrides for local validation or temporary testing:

```bash
export KAGEN_WORKSPACE_IMAGE=ghcr.io/pejas/kagen-workspace:1.2.3
export KAGEN_TOOLBOX_IMAGE=ghcr.io/pejas/kagen-toolbox:1.2.3
export KAGEN_PROXY_IMAGE=ghcr.io/pejas/kagen-proxy:1.2.3
kagen start codex
```

To change the default image version used by kagen for every run, publish the
replacement images first, then update the pinned refs in:

- `internal/workload/builder.go`
- `internal/cluster/proxy.go`

That change is the runtime release point. Publishing a new image tag in GHCR is
not sufficient until the pinned refs or the environment overrides point to it.

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
