# Kagen

Kagen is a local, security-first agent runtime. It isolates AI agents from your host and the internet by running them inside a Kubernetes VM, with an in-cluster git forge acting as a review boundary.

**Security properties:**
- Isolation: Agents run in Kubernetes; your host checkout stays canonical
- Egress control: Proxy enforces allowlist before agents reach external networks
- Reviewboundary: Agent commits accumulate in an in-cluster forge; pulling back requires explicit `kagen pull`

## Prerequisites

- Go 1.26+
- Colima
- kubectl

```bash
brew install colima kubernetes-cli
```

## Installation

```bash
git clone https://github.com/pejas/kagen.git
cd kagen
make install
```

## Quick Start

```bash
kagen start codex      # Start a new isolated session
kagen list             # Show persisted sessions
kagen attach codex     # Reattach to last session
kagen down             # Stop the runtime
```

## Workflow

1. `kagen start <agent>` - provisions the runtime, imports your repo, and attaches
2. Work inside the agent; changes push to the in-cluster forge
3. `kagen open` - open the review page
4. `kagen pull` - pull reviewed changes back to your local branch

Exiting the agent TUI only detaches; the session persists.

## Documentation

- [Internals Blueprint](docs/INTERNALS-BLUEPRINT.md) - command flow
- [Architecture](docs/ARCHITECTURE.md) - system design
- [Usage](docs/USAGE.md) - detailed command reference

## Development

```bash
make build
make test
make lint
make test-e2e    # requires full runtime stack
```

See [CONVENTIONS.md](docs/CONVENTIONS.md) for coding standards.
