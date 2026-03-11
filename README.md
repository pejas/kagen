# Kagen

![Kagen](docs/kagen.png)

## What
Kagen is a local, security-first agent runtime for Git repositories.

## Why
It isolates AI agents from your host system and the internet. The agent's execution environment is defined by a repository `devfile.yaml`, and Kagen provisions the selected agent runtime inside the Colima VM so the host checkout remains outside the execution boundary.

Changes made by the agent are accumulated in an isolated, in-cluster Forgejo instance. This provides a clear, reviewable boundary before any code is pulled back into your canonical local branch.

## How

### Installation

Requires Go 1.23+.

```bash
git clone https://github.com/pejas/kagen.git
cd kagen
make install
```

### Usage

Initialize the environment for Codex (writes a Codex-ready `devfile.yaml` and `.kagen.yaml`):

```bash
kagen init
```

Rewrite an older placeholder project template with the real Codex runtime:

```bash
kagen init --agent codex --force
```

Start or resume a session:

```bash
kagen
```

Launch a specific agent directly:

```bash
kagen --agent claude
kagen --agent codex
kagen --agent opencode
```

For Codex, Kagen now:
- imports the host repository into the in-cluster Forgejo boundary,
- clones that repository into `/projects/workspace` inside the agent pod,
- persists Codex state in a dedicated PVC mounted at `/home/kagen/.codex`,
- launches Codex with `danger-full-access` and `never` approval mode inside the VM, not on the host.

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
