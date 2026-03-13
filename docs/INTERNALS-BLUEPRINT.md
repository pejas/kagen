# Kagen Internals Blueprint

This is the quick mental model for how Kagen behaves when you run a command.

It is intentionally not a deep architecture document. Think of it as the
"what just happened?" guide.

## One-Screen View

```text
you in terminal
      |
      v
+-------------------+
|   kagen CLI       |
|   internal/cmd    |
+-------------------+
      |
      +------------------------------+
      |                              |
      v                              v
+-------------------+       +-------------------+
| project config    |       | session store     |
| .kagen.yaml       |       | internal/session  |
+-------------------+       +-------------------+
      |
      v
+-------------------+
| runtime manager   |
| Colima + K3s      |
| internal/runtime  |
+-------------------+
      |
      v
+-------------------+       +-------------------+
| Forgejo boundary  | <---- | cluster reconcile |
| repo import/review|       | internal/cluster  |
+-------------------+       +-------------------+
                                  |
                                  v
                          +-------------------+
                          | generated pod     |
                          | internal/workload |
                          +-------------------+
                                  |
                                  v
                          +-------------------+
                          | agent process     |
                          | codex/claude/etc. |
                          +-------------------+
```

## What Lives Where

```text
host repo on your machine
  - your canonical checkout
  - discovered by internal/git

.kagen.yaml
  - optional project defaults
  - written by kagen config write when you want repo-specific overrides

session store
  - persisted kagen sessions
  - persisted agent sessions
  - survives CLI restart

Forgejo in cluster
  - review boundary
  - imported copy of the repo

agent pod in cluster
  - workspace clone at /projects/workspace
  - runtime state in agent-specific home paths
```

## `kagen config write`

```text
terminal
  |
  v
kagen config write
  |
  +--> choose default agent
  |
  +--> write .kagen.yaml
  |
  `--> stop
```

Plain English:
- This only writes optional local project config.
- `kagen start` and `kagen attach` do not require `.kagen.yaml`.
- It does not start Colima.
- It does not create a pod.
- It does not create `devfile.yaml`.

## `kagen start <agent>`

```text
terminal
  |
  v
kagen start codex
  |
  +--> load config
  +--> discover git repo
  +--> ensure Colima + K3s are running
  +--> ensure Forgejo is ready
  +--> build baseline pod from internal/workload
  +--> let internal/cluster inject:
  |      - workspace sync
  |      - agent env
  |      - proxy env/policy
  |      - git authorship
  +--> reconcile pod + PVCs
  +--> persist new kagen session
  +--> persist new agent session
  `--> attach to the requested agent
```

With `--verbose`, the runtime-facing portion of this flow is reported as named steps such as `ensure_runtime`, `ensure_namespace`, `ensure_proxy`, `ensure_resources`, `forgejo_import`, `launch_agent_runtime`, `validate_proxy_policy`, `prepare_agent_state`, and `attach_agent`.

If a runtime step fails after session persistence, `kagen` also writes failure artefacts to a deterministic directory under the user config directory:

- `kagen/failure-artefacts/session-<id>/`
- files include `<operation>-failure.json`, `<operation>-trace.json`, `<operation>-trace.txt`, `<operation>-session-summary.json`, pod snapshots, pod events, `workspace-sync` logs, agent container logs, and proxy deployment state when available
- a start failure before session persistence falls back to `kagen/failure-artefacts/pending/<repo-id>/start/`

Quick picture:

```text
start
  |
  v
[config] -> [repo] -> [runtime up] -> [Forgejo ready] -> [pod generated]
                                                     -> [pod reconciled]
                                                     -> [sessions saved]
                                                     -> [agent attached]
```

## `kagen attach <agent> [--session <id>]`

```text
terminal
  |
  v
kagen attach codex
  |
  +--> open session store
  +--> resolve target kagen session
  |      - explicit --session
  |      - or most recent ready session for this repo
  +--> ensure runtime is available
  +--> validate proxy policy
  +--> persist a fresh agent session
  `--> attach that agent to the selected kagen session
```

Failures in `start` and `attach` report the exact failed runtime step in the command error. This distinguishes attach preparation failures from workload or proxy failures.

Attach failures also print the captured artefact directory so you can inspect the latest machine-readable session diagnostics without manual cluster inspection.

Important distinction:
- A kagen session is the persisted workspace/runtime identity.
- An agent session is one attach/run inside that kagen session.
- Re-attaching creates a fresh agent session instead of reusing an old one.

## `kagen list`

```text
terminal
  |
  v
kagen list
  |
  +--> read persisted sessions from SQLite
  +--> filter to current repo
  `--> print status, branch, last used, active agent-session summary
```

This is mostly a read from the session store, not a full runtime bootstrap.

## `kagen down`

```text
terminal
  |
  v
kagen down
  |
  +--> stop Colima
  +--> stop K3s
  `--> keep persisted kagen sessions and agent sessions in the local store
```

Plain English:
- This is the whole-runtime shutdown command.
- It does not delete persisted session records.
- It is different from leaving an agent TUI with `/exit` or `/quit`.

## `kagen open`

```text
terminal
  |
  v
kagen open
  |
  +--> inspect repo + branch state
  +--> talk to Forgejo
  +--> build review URL for the kagen branch
  `--> open/show the review target
```

Think of this as "show me the review boundary", not "change the runtime".

## `kagen pull`

```text
terminal
  |
  v
kagen pull
  |
  +--> inspect local repo state
  +--> protect local WIP if needed
  +--> fetch reviewed changes from Forgejo
  `--> fast-forward or merge back into the host branch
```

Quick picture:

```text
Forgejo review branch
        |
        v
   kagen pull
        |
        +--> protect host WIP
        +--> fetch reviewed refs
        `--> update local canonical branch
```

## When You Exit the Agent

```text
inside pod
  |
  v
/exit or /quit
  |
  `--> leave the agent TUI/process
```

Usually this does not mean:
- deleting the kagen session
- deleting the agent session records
- shutting down Colima
- shutting down K3s

It just ends that interactive agent process. If you want to stop the runtime
itself, run `kagen down`.

## Session-First Mental Model

```text
repo
  |
  `--> kagen session
         |
         +--> agent session 1 (codex)
         +--> agent session 2 (codex)
         `--> agent session 3 (claude)
```

The key idea is:
- one persisted kagen session can host many agent sessions
- the runtime pod shape comes from `internal/workload`
- orchestration mutations happen in `internal/cluster`
- review and sync flow through Forgejo, not straight back to the host repo

## If You Only Remember Four Things

```text
1. config write = write optional repo defaults only
2. start = create a new persisted kagen session and attach
3. attach = reuse a persisted kagen session and create a fresh agent session
4. down = stop the whole local runtime
```
