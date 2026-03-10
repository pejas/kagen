# Kagen

## What
Kagen is a local, security-first agent runtime for Git repositories.

## Why
It isolates AI agents from your host system and the internet. The agent's execution environment is strictly defined by a standard [Devfile](https://devfile.io/), ensuring reproducible and contained workspaces. 

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

Initialize the environment (creates a minimal `devfile.yaml` and `.kagen.yaml`):

```bash
kagen init
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
