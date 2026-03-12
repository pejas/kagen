# Kagen

![Kagen](docs/kagen.png)

## What
Kagen is a local, security-first agent runtime for Git repositories.

## Why
It isolates AI agents from your host system and the internet. Kagen generates the runtime workload internally and provisions the selected agent inside the Colima VM so the host checkout remains outside the execution boundary.

Changes made by the agent are accumulated in an isolated, in-cluster Forgejo instance. This provides a clear, reviewable boundary before any code is pulled back into your canonical local branch.

## How

Quick docs:
- [Internals Blueprint](docs/INTERNALS-BLUEPRINT.md)
- [Architecture](docs/ARCHITECTURE.md)

### Installation

Requires Go 1.23+.

```bash
git clone https://github.com/pejas/kagen.git
cd kagen
make install
```

### Usage

Write optional repository defaults for Codex (not required before `start` or `attach`):

```bash
kagen config write
```

Rewrite the optional project config with a different default agent:

```bash
kagen config write --agent codex --force
```

Start a new session:

```bash
kagen start codex
```

Attach a new agent session to the most recent ready kagen session for the current repository:

```bash
kagen attach codex
```

List persisted sessions for the current repository:

```bash
kagen list
```

Shut down the whole local Kagen runtime environment:

```bash
kagen down
```

For Codex, Kagen now:
- imports the host repository into the in-cluster Forgejo boundary,
- clones that repository into `/projects/workspace` inside the agent pod,
- persists Codex state in a dedicated PVC mounted at `/home/kagen/.codex`,
- launches Codex with `danger-full-access` and `never` approval mode inside the VM, not on the host.

Existing repository `devfile.yaml` files are treated as legacy repository artefacts: `kagen config write` does not create them, and `kagen start` and `kagen attach` ignore them.

Leaving an agent TUI with `/exit` or `/quit` only detaches from that tool. `kagen config write` only writes optional repo defaults. `kagen down` stops the whole local Colima/K3s runtime environment, while persisted kagen sessions and agent sessions remain in the local store and continue to appear in `kagen list`.

Enable verbose output:

```bash
kagen --verbose
```

Open the review page for the current branch:

```bash
kagen open
```

Pull reviewed changes back into the local branch:

```bash
kagen pull
```

### Development

Build from source:

```bash
make build
```

Run tests:

```bash
make test
```

Run the end-to-end suite explicitly:

```bash
make test-e2e
```

`make test` intentionally excludes `internal/e2e` so the default validation loop stays fast and does not require the full local runtime stack. Use `make test-e2e` when you specifically want end-to-end coverage.
