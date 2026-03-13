## Runtime Images

This directory contains the first-party runtime-image scaffold for Phase 1 of the
Debian toolbox rollout.

Images:

- `base`: minimal Debian runtime with Linux primitives and native build dependencies
- `workspace`: lightweight workspace and init image layered on `base`
- `toolbox`: agent toolbox image layered on `base` and populated via checked-in `mise` config and lock data
- `proxy`: dedicated tinyproxy image layered on `base`

Current code paths resolve image references from:

- `internal/workload` for workspace and toolbox images
- `internal/cluster` for the proxy image

Local image bring-up can override those refs without code changes:

- `KAGEN_WORKSPACE_IMAGE`
- `KAGEN_TOOLBOX_IMAGE`
- `KAGEN_PROXY_IMAGE`

Suggested local build order:

1. Build `base`
2. Generate or refresh `toolbox/mise.lock`
3. Build `workspace`
4. Build `toolbox`
5. Build `proxy`

For local runtime validation, `make runtime-images-build-local` builds:

- `ghcr.io/pejas/kagen-base:local`
- `ghcr.io/pejas/kagen-workspace:local`
- `ghcr.io/pejas/kagen-toolbox:local`
- `ghcr.io/pejas/kagen-proxy:local`

The release flow should publish immutable first-party artefacts and replace the
Phase 1 bootstrap tags in Go code and docs with digests.
