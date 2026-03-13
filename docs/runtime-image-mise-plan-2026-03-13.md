# Runtime Image and `mise` Plan

Date: 2026-03-13

Scope: two-phase plan for moving `kagen` to first-party container artefacts with build-time `mise` toolchain management, while preserving room for repository-specific version overrides.

## Executive Summary

`kagen` should ship a first-party toolbox image built from a controlled Debian base and use `mise` as the preferred installation layer for developer-facing tools and language runtimes. The image should be reproducible, pinned, and ready to use without package-manager or language-manager bootstrapping at pod start.

The toolbox should provide a strong default baseline for agents, but it should not assume that every repository wants the newest tool versions. Repository-local `mise.toml` files should remain able to override the image defaults so projects can pin older or vendor-specific versions where needed.

This plan proposes:

1. A curated image-level default toolchain with explicit version policy.
2. A project-level override model built on `mise` configuration precedence.
3. A phased rollout that separates the first-party image from repository-specific overlays.
4. A minimal OS package layer that exists only to support Linux runtime primitives and native compilation needs.

## Version Policy Snapshot

This policy was reviewed on 2026-03-13 against upstream release information.

- Java 25 is an LTS release.
- Node 24 is the current Active LTS line.
- Python should default to 3.13 rather than 3.14 for broader compatibility.
- Go should default to the current stable line, 1.26.

Rationale:

- Node and Java have clear LTS concepts and should follow them.
- Python has no LTS line, so the default should favour ecosystem stability over the newest feature release.
- Go has no LTS line; the current supported stable release is the best default.

## Default Tool Catalogue

The goal is to cover the majority of repositories an agent is likely to inspect, test, build, or patch without turning the image into an uncontrolled kitchen sink.

### Minimal OS Layer

The image should keep OS packages to the minimum needed for:

- Linux runtime primitives
- certificate and SSH handling
- archive extraction
- native compilation and headers
- basic debugging when an agent needs to inspect the environment

Recommended OS-layer packages:

- `bash`, `sh`, `coreutils`, `findutils`, `grep`, `sed`, `gawk`
- `git`, `openssh-client`, `ca-certificates`
- `curl`, `wget`
- `tree`, `less`, `file`
- `tar`, `gzip`, `xz-utils`, `zip`, `unzip`
- `procps`, `psmisc`, `lsof`, `netcat-openbsd`, `dnsutils`, `iproute2`
- `make`, `patch`, `diffutils`, `rsync`
- `openssl`

### Build Primitives

- `build-essential`
- `pkg-config`
- `cmake`
- `ninja-build`
- `sqlite3`
- `libssl-dev`, `zlib1g-dev`, `libbz2-dev`, `libreadline-dev`, `libsqlite3-dev`, `libffi-dev`, `liblzma-dev`

These packages are intentionally biased towards making Python packages, native Node add-ons, Go CGO projects, Rust crates, and common C/C++ dependencies work without another round-trip through `apt`.

### Why Not Use `mise` for Everything

`mise` should be the preferred installation mechanism for toolchains and developer-facing CLIs, but it should not replace the OS package layer entirely.

The OS layer still needs to own:

- base shell and userland tools
- CA certificates and SSH support
- archive tooling used during installs
- compilers, headers, and native libraries
- low-level network and process inspection tools

This keeps the image predictable while still moving most versioned tooling into declarative `mise` configuration instead of custom Dockerfile logic.

### Default `mise`-Managed Toolchains

These should be present in the image by default:

- `node = "lts"`
- `pnpm = "latest"`
- `python = "3.13"`
- `uv = "latest"`
- `go = "prefix:1.26"`
- `java = "temurin-25"`
- `maven = "latest"`
- `gradle = "latest"`
- `rust = "stable"`
- `cargo-binstall = "latest"`
- `ripgrep = "latest"`
- `fd = "latest"`
- `jq = "latest"`
- `yq = "latest"`
- `just = "latest"`

Optional but useful additions:

- `bun = "latest"`
- `deno = "latest"`

Not recommended as phase-1 defaults:

- Ruby
- PHP
- .NET
- Elixir / Erlang
- Android SDK / NDK

Those ecosystems are important, but they materially increase image size and operational complexity. They fit better as phase-2 or repo-local additions.

### Agent CLIs

The toolbox image should bake in the supported agent CLIs directly:

- `codex`
- `claude`
- `opencode`

These should be treated as first-party runtime artefacts, not as ad-hoc installs performed by the pod when it starts.

## `mise` Configuration Model

The image should provide system defaults through `/etc/mise/config.toml` and a matching lockfile-driven install during image build.

Repository-local configuration should then override those defaults naturally:

1. `/etc/mise/config.toml` in the image defines the baseline.
2. A repository `mise.toml` inside the checked-out workspace overrides the baseline.
3. A repository `mise.local.toml` remains available for local-only overrides when needed.

This matches `mise`'s documented precedence model and lets `kagen` stay opinionated without blocking real projects from pinning older versions.

In practical terms:

- prefer `mise` whenever a tool is meaningfully versioned and user-facing
- keep Dockerfile logic focused on OS primitives and image assembly
- treat `mise.toml` and `mise.lock` as the main definition of the toolbox contents

## Recommended Default Config Shape

The image-level config should look broadly like this:

```toml
min_version = "2024.11.1"

[tools]
node = "lts"
pnpm = "latest"
python = "3.13"
uv = "latest"
go = "prefix:1.26"
java = "temurin-25"
maven = "latest"
gradle = "latest"
rust = "stable"
cargo-binstall = "latest"
ripgrep = "latest"
fd = "latest"
jq = "latest"
yq = "latest"
just = "latest"
bun = "latest"
deno = "latest"

[settings]
lockfile = true
idiomatic_version_file_enable_tools = ["node"]
```

Important constraint:

- `mise.toml` may express moving tracks such as `lts`, `stable`, or `latest`.
- The image build must resolve and freeze them into `mise.lock`.
- Runtime startup must not perform fresh tool resolution.
- The toolbox definition should live primarily in `mise` config, not in long lists of imperative install commands in the Dockerfile.

## Project Override Model

`kagen` should explicitly support repository-level `mise.toml` files because agents often work in repositories that need older or vendor-specific toolchains.

Implications:

- A project can pin `python = "3.11"` or `java = "temurin-21"` even if the image default is newer.
- A project can add extra tools not present in the image baseline.
- `kagen` should preserve the principle that the repository remains the source of truth for repository-specific build requirements.

Operationally, this suggests:

- `MISE_DATA_DIR` and `MISE_CACHE_DIR` should live on persistent agent state storage rather than the ephemeral workspace volume.
- Project-triggered tool installs should survive agent restarts when practical.
- The repo workspace path should be trusted automatically or explicitly during attach so `mise` does not block agent flows with trust prompts.

## Two-Phase Rollout

## Phase 1: First-Party Toolbox Baseline

### Goal

Replace runtime bootstrap drift with a reproducible, pinned toolbox image built from Debian, with a minimal OS package layer and a `mise`-managed default toolchain prepared at image build time.

### Deliverables

- `kagen-base` image from `debian:bookworm-slim`
- `kagen-toolbox` image with the curated default tool catalogue
- build-time `mise` install driven by checked-in config and lock data
- pinned agent CLIs in the image
- removal of runtime `apt-get`, `npm install`, and similar bootstrap steps from the pod startup path

### Design Notes

- Keep the initial rollout conservative: solve reproducibility first.
- Keep the Dockerfile intentionally small by moving versioned tooling definitions into `mise`.
- It is acceptable in this phase to keep the current pod shape while swapping the runtime bootstrap logic to an install-free keepalive path.
- Image references should remain release-managed and pinned by digest.

### Acceptance Criteria

- `kagen start` no longer depends on live package installation inside the pod.
- The image builds deterministically from pinned artefacts.
- The default tool catalogue covers common Go, Python, Node, Java, and Rust repositories out of the box.
- Most developer-facing tools are defined in `mise.toml` and frozen in `mise.lock` rather than installed imperatively with OS package commands.

## Phase 2: Repository-Aware Overlay and Persistence

### Goal

Make project-local `mise.toml` an explicit part of the agent runtime model without compromising reproducibility or user control.

### Deliverables

- repo-local `mise.toml` overrides documented and supported
- persistent `mise` cache/data directories on agent state storage
- safe trust flow for workspace configs
- diagnostics showing which config files and tool versions `mise` resolved
- optional support for project-specific extra tools beyond the baseline image

### Design Notes

- The image remains the default baseline, not the final authority for every project.
- Repository config should override image defaults, but only inside the workspace context.
- This phase is the right time to simplify the runtime shape towards a single stable toolbox container that can host multiple agent types cleanly.

### Acceptance Criteria

- A repository can pin older tool versions than the image defaults and agents will honour them.
- Additional tools requested by the repo can be installed into persistent state rather than disappearing with the workspace.
- Users can understand which tool versions came from the image and which came from the repository override.

## Recommendation

Start with Phase 1 and keep the phase boundary strict.

Phase 1 delivers the biggest security and operability gain: first-party artefacts, no runtime package installation, and a useful default toolbox.

Phase 2 is where the repository-level override model becomes a first-class feature. That is the right place to add persistence, trust behaviour, and richer `mise` diagnostics rather than trying to solve every edge case in the first image PR.

## Future Considerations

Today `kagen` runs on a Linux container runtime inside Colima/K3s, so the toolbox design should assume Linux userspace and Linux containers. That makes a Linux base image the practical choice for the current architecture.

Specialised operating-system runtimes could be revisited later if the lower runtime architecture changes materially, but they are out of scope for this plan:

- macOS does not map cleanly onto the current container model
- Windows-specific container support would require a different runtime contract than the current Colima/K3s stack
