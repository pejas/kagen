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

## What Stays Below E2E

The following contracts are covered by unit or integration tests instead of the default E2E suite:

- repository discovery, including worktrees;
- transient Forgejo transport and host Git hygiene;
- `kagen open` reviewability and review transport semantics;
- proxy fail-closed validation;
- optional config write behaviour.

This keeps `make test-e2e` focused on the runtime boundary instead of re-testing implementation details that already have faster, more deterministic coverage.

## When Contributors Should Run It

Run `make test-e2e` when:

- changing runtime bootstrap, Forgejo reconciliation, or agent attach behaviour;
- changing the user-visible `start` or `pull` flow;
- validating a release candidate or other cross-package change with high blast radius.

You usually do not need `make test-e2e` for isolated changes in config parsing, Git helper behaviour, session persistence, or command help text when those changes are already covered by `make test`.
