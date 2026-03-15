# Kagen Usage Reference

Detailed command reference for kagen.

## Index

- [kagen start](#kagen-start)
- [kagen attach](#kagen-attach)
- [kagen list](#kagen-list)
- [kagen down](#kagen-down)
- [kagen open](#kagen-open)
- [kagen pull](#kagen-pull)
- [kagen doctor](#kagen-doctor)
- [kagen config write](#kagen-config-write)
- [Image Configuration](#image-configuration)
- [Session Management Patterns](#session-management-patterns)

---

## kagen start

Start a new kagen session and attach an agent.

```
kagen start <agent> [flags]
```

### Arguments

- `<agent>` - Agent to run (e.g., `codex`, `claude`)

### Flags

- `--detach` - Start session without interactive attach. Creates a ready session for later attachment.
- `--session <id>` - Use a specific session ID (rarely needed for start)

### Examples

Start a new Codex session:

```bash
kagen start codex
```

Start a session for automation (non-interactive):

```bash
kagen start --detach codex
```

Later, attach to the prepared session:

```bash
kagen attach codex
```

### VerboseOutput

Enable detailed step tracing:

```bash
kagen --verbose start codex
```

Verbose mode reports:
- Runtime bootstrap phases
- Forgejo import progress
- Pod readiness checks
- Proxy validation
- Agent attach sequence

---

## kagen attach

Attach a new agent session to an existing kagen session.

```
kagen attach <agent> [flags]
```

### Arguments

- `<agent>` - Agent to run (e.g., `codex`, `claude`)

### Flags

- `--session <id>` - Attach to a specific kagen session. Defaults to the most recent ready session for the current repository.

### Examples

Attach to the most recent session:

```bash
kagen attach codex
```

Attach to a specific session:

```bash
kagen attach codex --session abc123
```

### Session Resolution

Without `--session`, attach resolves to:
1. The most recent kagen session with status `ready`
2. Scoped to the current repository

### Verbose Output

```bash
kagen --verbose attach codex
```

---

## kagen list

List persisted sessions for the current repository.

```
kagen list
```

Output includes:
- Session ID
- Status (`starting`, `ready`, `failed`)
- Branch
- Last used timestamp
- Active agent sessions

---

## kagen down

Stop the local Colima/K3s runtime environment.

```
kagen down
```

This stops the entire runtime but preserves session records. Use `kagen list` after `kagen down` to see persisted sessions.

### What Persists

- Kagen session records
- Agent session records
- Local store state

### What Stops

- Colima VM
- K3s cluster
- All pods and services

---

## kagen open

Open the review page for the current branch.

```
kagen open
```

This command:
1. Establishes a transient tunnel to the in-cluster forge
2. Opens the review URL for `kagen/<branch>`
3. Keeps the tunnel open until interrupted

The forge review page shows commits made by the agent inside the isolationboundary.

---

## kagen pull

Pull reviewed changes back into the local branch.

```
kagen pull
```

This command:
1. Fetches the reviewed `kagen/<branch>` from the in-cluster forge
2. Protects local WIP with a temporary commit if needed
3. Fast-forwards or merges into the host branch

### Workflow Integration

After reviewing with `kagen open`:

```bash
kagen pull
```

---

## kagen doctor

Summarise diagnostics for the most recent session.

```
kagen doctor [flags]
```

### Flags

- `--session <id>` - Inspect a specific session

### Output

- Latest operation trace
- Session status
- Runtime pod state (if running)
- Proxy enforcement state
- Failure artefact location (if applicable)

### Use Cases

- Debug a failed `start` or `attach`
- Verify runtime health
- Inspect session state without starting the runtime

---

## kagen config write

Write optional repository defaults to `.kagen.yaml`.

```
kagen config write [flags]
```

### Flags

- `--agent <name>` - Set the default agent
- `--force` - Overwrite existing `.kagen.yaml`

### Examples

Create config with default agent:

```bash
kagen config write --agent codex
```

Overwrite existing config:

```bash
kagen config write --agent codex --force
```

### Notes

- `.kagen.yaml` is optional
- `start` and `attach` work without it
- The file is ignored by Git (add to `.gitignore` if needed)

---

## Image Configuration

Override runtime container images via config file or environment variables.

### Config File (`.kagen.yaml`)

```yaml
images:
  workspace: ghcr.io/pejas/kagen-workspace:0.1.4
  toolbox: ghcr.io/pejas/kagen-toolbox:0.1.4
  proxy: ghcr.io/pejas/kagen-proxy:0.1.4
```

### Environment Variables

```bash
export KAGEN_IMAGES_WORKSPACE=ghcr.io/pejas/kagen-workspace:0.1.4
export KAGEN_IMAGES_TOOLBOX=ghcr.io/pejas/kagen-toolbox:0.1.4
export KAGEN_IMAGES_PROXY=ghcr.io/pejas/kagen-proxy:0.1.4
```

### Precedence

1. Environment variables
2. `.kagen.yaml`
3. Built-in defaults

---

## Session Management Patterns

### Starting a New Project

```bash
cd my-project
kagen start codex
# Work in the agentTUI...
# Exit with /exit
kagen list
```

### Resuming Work After Break

```bash
cd my-project
kagen list              # Find your session
kagen attach codex      # Reattach to most recent
```

### CI/CD Integration

```bash
# Prepare session non-interactively
kagen start --detach codex

# Verify session is ready
kagen list

# Run automated tasks...

# Later, human review
kagen attach codex
```

### Debugging a Failed Start

```bash
kagen start codex       # Fails
kagen doctor            # Show failure artefacts
kagen --verbose start codex  # Verbose retry
```

### Cleaning Up

```bash
kagen down              # Stop runtime
# Sessions persist in local store
kagen list              # Still visible

# To fully reset, remove ~/.config/kagen/sessions/
```

### Multiple Repositories

Each repository has its own session scope:

```bash
cd ~/projects/repo-a
kagen start codex
# Creates session scoped to repo-a

cd~/projects/repo-b
kagen start codex
# Creates session scoped to repo-b

kagen list              # Shows only repo-b sessions
```

---

## See Also

- [INTERNALS-BLUEPRINT.md](INTERNALS-BLUEPRINT.md) - command flow mental model
- [ARCHITECTURE.md](ARCHITECTURE.md) - system design
- [E2E.md](E2E.md) - testing boundaries