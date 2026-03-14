# Runtime Artefacts

This document records the container artefacts currently used on the kagen runtime path.

## Current Artefacts

- Workspace image: `ghcr.io/pejas/kagen-workspace:2026-03-12`
- Toolbox image: `ghcr.io/pejas/kagen-toolbox:2026-03-12`
- Proxy image: `ghcr.io/pejas/kagen-proxy:2026-03-12`

These references are intentionally versioned rather than `latest`, so runtime
behaviour does not drift between runs. The defaults are resolved through
kagen's config stack rather than package-local runtime constants.

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

- `internal/config` is the source of truth for default runtime image references.
- `packaging/runtime-images/` contains the Dockerfiles and `mise` inputs used to build future first-party runtime artefacts.
- Local validation can override the resolved refs with `KAGEN_IMAGES_WORKSPACE`, `KAGEN_IMAGES_TOOLBOX`, and `KAGEN_IMAGES_PROXY`.
- Runtime preflight validates only launch-critical shape and resolution. It does not repair bad image refs, publish artefacts, or replace runtime-backed verification.

## Publish Workflow

`.github/workflows/docker-publish.yml` publishes four multi-architecture images:

- `ghcr.io/pejas/kagen-base`
- `ghcr.io/pejas/kagen-workspace`
- `ghcr.io/pejas/kagen-toolbox`
- `ghcr.io/pejas/kagen-proxy`

The workflow runs on Git tag pushes matching `v*` and on manual dispatch.

The workflow first resolves a canonical image tag:

- `inputs.tag` on manual dispatch, when supplied
- otherwise the pushed Git tag with the leading `v` removed
- otherwise the short Git commit SHA

For a tag push such as `v1.2.3`, `docker/metadata-action` emits these tags for
each image:

- `1.2.3`
- `1.2`
- `1`
- the short Git commit SHA
- `latest`

The base image is built first. The workspace, toolbox, and proxy images are
then built from `packaging/runtime-images/` and receive
`KAGEN_BASE_IMAGE=ghcr.io/pejas/kagen-base:<resolved-tag>` as a build
argument. The verification step checks the same resolved tag, so the workflow
uses one consistent base-image reference across publication and validation.

That means a release tag such as `v1.2.3` publishes runtime images tagged
`1.2.3`, while manual dispatch can target any explicit image tag through
`inputs.tag`.

## How Kagen Selects Runtime Images

The publish workflow does not change the images used by `kagen start` on its
own. Kagen resolves runtime images through the normal config stack:

- defaults in `internal/config`
- global config in `~/.config/kagen/main.yml`
- project config in `.kagen.yaml`
- environment overrides from `KAGEN_IMAGES_WORKSPACE`,
  `KAGEN_IMAGES_TOOLBOX`, and `KAGEN_IMAGES_PROXY`

`kagen config write` documents the optional `images` block in `.kagen.yaml`.

Use overrides for local validation or temporary testing:

```bash
export KAGEN_IMAGES_WORKSPACE=ghcr.io/pejas/kagen-workspace:1.2.3
export KAGEN_IMAGES_TOOLBOX=ghcr.io/pejas/kagen-toolbox:1.2.3
export KAGEN_IMAGES_PROXY=ghcr.io/pejas/kagen-proxy:1.2.3
kagen start codex
```

To change the default image version used by kagen for every run, publish the
replacement images first, then update config to point at them:

```yaml
images:
  workspace: ghcr.io/pejas/kagen-workspace:1.2.3
  toolbox: ghcr.io/pejas/kagen-toolbox:1.2.3
  proxy: ghcr.io/pejas/kagen-proxy:1.2.3
```

That config change is the runtime release point. Publishing a new image tag in
GHCR is not sufficient until resolved config points to it.

## Update Procedure

1. Edit `packaging/runtime-images/toolbox/mise.toml` when the default toolchain changes.
2. Refresh `packaging/runtime-images/toolbox/mise.lock` with `make runtime-images-lock`.
3. Build and publish the replacement image artefacts from `packaging/runtime-images/`.
4. Verify the artefacts are pullable from the Colima/K3s runtime used by `kagen start`.
5. Update the default image refs in `internal/config/config.go`, or point global or project config at the new tags.
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
