# End-to-End Scope

`make test-e2e` is intentionally narrower than `make test`.

The default contributor loop is:

- `make build`
- `make test`
- `make lint`

Run `make test-e2e` when you need confidence in behaviour that only becomes truthful once the full CLI, Colima/K3s runtime, Kubernetes resources, and in-cluster Forgejo boundary are exercised together.

## What Must Stay E2E

The checked-in E2E suite is reserved for user-visible contracts that are difficult to prove below the full runtime boundary:

- starting a session with the generated runtime and attached agent path;
- pulling reviewed changes back through the Forgejo boundary, including local WIP protection.

These are exercised through:

- `features/workflow.feature`
- `features/pull.feature`
- `internal/e2e/runtime_readiness_test.go`
- `internal/e2e/runtime_interactive_test.go`

## What Stays Below E2E

The following contracts are covered by unit or integration tests instead of the default E2E suite:

- repository discovery, including worktrees;
- transient Forgejo transport and host Git hygiene;
- `kagen open` reviewability and review transport semantics;
- proxy fail-closed validation;
- optional config write behaviour.

This keeps `make test-e2e` focused on the runtime boundary instead of re-testing implementation details that already have faster, more deterministic coverage.

## Detached Readiness Coverage

The real-cluster Phase 5 coverage keeps the runtime boundary non-interactive.

`internal/e2e/runtime_readiness_test.go` exercises:

- `kagen start --detach` for Codex, Claude, and OpenCode;
- persisted session transitions from `starting` to `ready` and a representative `starting` to `failed` path;
- attach into an existing ready session without PTY assumptions;
- `kagen down` followed by `kagen attach` against a persisted session;
- runtime readiness assertions through the session store, Kubernetes API objects, and shared `kubectl exec` adapter.

These checks assert real readiness rather than CLI strings alone:

- persisted session status and agent-session state path;
- running pod phase, ready containers, and completed `workspace-sync` init;
- workspace checkout on `kagen/<branch>` with the expected commit SHA;
- proxy deployment, config, and network policy for allowlisted agents.

## Interactive Smoke Coverage

Phase 6 adds a small PTY-backed smoke layer on top of the detached readiness path.

`internal/e2e/runtime_interactive_test.go`:

- creates a ready session through `kagen start --detach`;
- attaches through the real `kagen attach` CLI path for Codex, Claude, and OpenCode;
- waits for an agent-specific startup signal instead of matching generic CLI output;
- terminates the interactive session with Ctrl+C and requires a clean CLI exit.

The interactive smoke suite stays intentionally narrow. It proves the TTY hand-off without trying to script full agent conversations.

## When Contributors Should Run It

Run `make test-e2e` when:

- changing runtime bootstrap, Forgejo reconciliation, or agent attach behaviour;
- changing the user-visible `start` or `pull` flow;
- validating a release candidate or other cross-package change with high blast radius.

Before running the detached readiness suite locally:

- build `bin/kagen` through `make build` or a superset target;
- ensure `git`, `kubectl`, and `colima` are installed and available on `PATH`;
- ensure the local machine can access the `colima-kagen` kube context and profile state.

In environments where the pinned published runtime tags are not pullable from the cluster, run `make test-e2e` with the documented local-image overrides instead of changing runtime contracts:

- `KAGEN_WORKSPACE_IMAGE=ghcr.io/pejas/kagen-workspace:local`
- `KAGEN_TOOLBOX_IMAGE=ghcr.io/pejas/kagen-toolbox:local`
- `KAGEN_PROXY_IMAGE=ghcr.io/pejas/kagen-proxy:local`

You usually do not need `make test-e2e` for isolated changes in config parsing, Git helper behaviour, session persistence, or command help text when those changes are already covered by `make test`.
