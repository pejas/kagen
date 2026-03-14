# Kagen Runtime Images

Multi-platform Docker images for the kagen isolated development environment runtime.

## Images

| Image | Purpose | Base | Size |
|-------|---------|------|------|
| `ghcr.io/pejas/kagen-base` | Base image with development tools | `debian:bookworm-slim` | ~300MB |
| `ghcr.io/pejas/kagen-workspace` | Lightweight workspace container | `kagen-base` | ~300MB |
| `ghcr.io/pejas/kagen-toolbox` | Full toolchain with mise-managed languages | `kagen-base` | ~2GB |
| `ghcr.io/pejas/kagen-proxy` | Tinyproxy for network proxying | `kagen-base` | ~320MB |

*Sizes are approximate uncompressed. Compressed sizes in registry are typically 30-50% smaller.

## Supported Platforms

- `linux/amd64`
- `linux/arm64`

## Usage

### Local Development

Build images locally for testing:

```bash
# Build all images for local platform
make runtime-images-build-local

# Build multi-platform images (requires Docker buildx)
make runtime-images-build VERSION=local
```

### CI/CD Publishing

Images are automatically published to GHCR when a version tag is pushed:

```bash
git tag v1.2.3
git push origin v1.2.3
```

This triggers the Docker Publish workflow which:
1. Builds all images for `linux/amd64` and `linux/arm64`
2. Tags images with: `1.2.3`, `1.2`, `1`, `latest`, and commit SHA
3. Pushes images to `ghcr.io/pejas/kagen-*`
4. Generates artifact attestations for provenance

### Manual Publishing

Maintainers can trigger a manual publish via GitHub Actions:

1. Go to Actions → Docker Publish
2. Click "Run workflow"
3. Optionally specify a custom tag

## Image Details

### kagen-base

Base Debian Bookworm image with essential development tools:
- Build tools (build-essential, cmake, ninja-build)
- Version control (git)
- System utilities (curl, wget, jq, yq)
- Python build dependencies
- Non-root user `kagen` (UID 1000)

### kagen-workspace

Minimal workspace container extending `kagen-base`:
- Suitable for project-specific workspaces
- Runs `tail -f /dev/null` to keep container alive

### kagen-toolbox

Full development toolbox with mise-managed languages:
- Node.js 24, pnpm, Bun, Deno
- Python 3.13, uv
- Go 1.26
- Java (Temurin 25), Maven, Gradle
- Rust, cargo-binstall
- CLI tools: ripgrep, fd, jq, yq, just
- AI coding assistants: Codex, Claude Code, OpenCode

### kagen-proxy

Tinyproxy for network traffic management:
- Tinyproxy configured for kagen
- Runs as non-root user

## Image Overrides

For local development, you can override which images are used without code changes:

| Environment Variable | Default | Purpose |
|---------------------|---------|---------|
| `KAGEN_WORKSPACE_IMAGE` | `ghcr.io/pejas/kagen-workspace:latest` | Workspace container image |
| `KAGEN_TOOLBOX_IMAGE` | `ghcr.io/pejas/kagen-toolbox:latest` | Toolbox container image |
| `KAGEN_PROXY_IMAGE` | `ghcr.io/pejas/kagen-proxy:latest` | Proxy container image |

These are read by `internal/workload` and `internal/cluster` at runtime.

## Production Recommendations

For production stability, pin to specific image digests rather than tags:

```bash
# Instead of:
ghcr.io/pejas/kagen-base:latest

# Use:
ghcr.io/pejas/kagen-base@sha256:abc123...
```

Digests provide immutable references that never change, ensuring reproducible deployments.

## Makefile Targets

| Target | Description |
|--------|-------------|
| `runtime-images-build-local` | Build single-platform images for local use |
| `runtime-images-build` | Build multi-platform images locally |
| `runtime-images-push` | Build and push multi-platform images to GHCR |
| `runtime-images-lock` | Refresh mise.lock for toolbox dependencies (reproducible language versions) |

## Registry Authentication

To pull images from GHCR:

```bash
echo $GITHUB_TOKEN | docker login ghcr.io -u $GITHUB_USERNAME --password-stdin
```

Images are public and can be pulled without authentication.

## Security

- Images are built with Docker BuildKit for improved security
- Artifact attestations are generated for provenance verification
- All images run as non-root user (`kagen`)
- Base image uses Debian Bookworm (stable) with security updates
