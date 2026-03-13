# Kagen Coding Conventions

## Language & Style
- **Go Version**: Match the version declared in `go.mod`; keep docs and tooling in sync with that source of truth.
- **Uber Style Guide**: Follow the [Uber Go Style Guide](https://github.com/uber-go/guide/blob/master/style.md).
- **British English**: Use Oxford spelling (e.g., 'organize', 'standardize', 'characterization') in all documentation and comments.

## Error Handling
- Use the `internal/errors` package for sentinel errors.
- Wrap errors with context: `fmt.Errorf("failed to [action]: %w", err)`.
- Return errors up to the top-level command flow; translate them to user-facing terminal output in the root execute path rather than scattering process exits.

## CLI & Configuration
- **Cobra**: Use for command definition.
- **Viper**: Use for configuration loading and environment overrides.
- **Defaults**: Define all default configuration values in `internal/config/config.go`.
- **Validation**: Add configuration validation in `internal/config/validate.go`; run it before orchestration begins.

## Orchestration Responsibilities
- Split host-side orchestration into narrow coordinators (runtime bootstrap, workload generation, forgejo sync, agent launch) and invoke them from `internal/cmd`.
- Put reusable user workflows such as `start`, `attach`, `open`, and `pull` in dedicated workflow/coordinator types rather than growing command-local helper clusters.
- Keep `internal/cmd` focused on input binding, high-level flow control, and user-facing errors.
- Keep transport, credentials, and infrastructure shell-outs out of command handlers; those belong in shared adapters or infrastructure packages.

## Interfaces, Implementations, and Surface Area
- Prefer small interfaces defined at the consumer boundary.
- Do not introduce provider-owned interfaces unless there is a real substitution need.
- Export constructors and the smallest useful surface area; concrete types are preferable to speculative abstraction.
- Use stubs or fakes where they improve tests, but do not create abstraction solely to preserve a stub pattern.

## Kubernetes (client-go) and Adapters
- Prefer `client-go` for resource management.
- Centralise `kubectl` usage in shared adapters: port-forwarder and exec wrapper.
- Avoid additional shell-outs from business logic; reuse the adapters.
- Ensure all created resources carry `kagen.io/repo-id` labels.

## Proxy & Egress
- Load proxy policy from configuration and validate it before attaching to the agent; fail closed when unenforced.
- Treat proxy enforcement as a current correctness contract, not future work.
- Use reproducible proxy artefacts; avoid package-manager installation in security-sensitive startup paths.

## Git & Forgejo Transport
- Keep the host checkout canonical.
- Do not persist credentialed Forgejo remotes or ephemeral localhost ports into host Git configuration.
- Prefer transient, operation-scoped transport helpers for fetch, push, and review workflows.

## Testing
- **Table-Driven Tests**: Use table-driven tests for complex logic.
- **Race Detector**: All tests must pass with `go test -race`.
- **Mocking**: Use interfaces and stubs for unit testing infrastructure packages.
- **Contract Coverage**: Prioritise tests for user-facing correctness and security contracts, especially repository discovery, review transport, and host Git hygiene.
- **Repository Contract**: Keep `make build`, `make test`, and `make lint` working together locally and in CI.
- **E2E Boundary**: Keep `make test-e2e` explicit, narrow, and documented in `docs/E2E.md`.

## Diagnostics and Documentation
- **Verbose Mode**: Put richer operational detail behind `--verbose`; keep default CLI output concise and actionable.
- **Documentation Maintenance**: Treat README and docs as product contracts. When user-facing behaviour changes, update `README.md`, `docs/ARCHITECTURE.md`, `docs/CONVENTIONS.md`, and `docs/E2E.md` together.
- **Maintainer Checklist**: Follow `docs/MAINTAINER-CHECKLIST.md` whenever changing a documented workflow or repository contract.
