# Kagen Coding Conventions

## Language & Style
- **Go 1.23+**: Use modern idiomatic Go.
- **Uber Style Guide**: Follow the [Uber Go Style Guide](https://github.com/uber-go/guide/blob/master/style.md).
- **British English**: Use Oxford spelling (e.g., 'organize', 'standardize', 'characterization') in all documentation and comments.

## Error Handling
- Use the `internal/errors` package for sentinel errors.
- Wrap errors with context: `fmt.Errorf("failed to [action]: %w", err)`.
- Use `ui.Fatal(err)` only in the `main` or top-level `RunE` if the error is unrecoverable.

## CLI & Configuration
- **Cobra**: Use for command definition.
- **Viper**: Use for configuration loading and environment overrides.
- **Defaults**: Define all default configuration values in `internal/config/config.go`.
- **Validation**: Add configuration validation in `internal/config/validate.go`; run it before orchestration begins.

## Orchestration Responsibilities
- Split host-side orchestration into narrow coordinators (runtime bootstrap, workload generation, forgejo sync, agent launch) and invoke them from `internal/cmd`.
- Keep `internal/cmd` free of shell-outs; coordinators should call injected services instead.

## Interfaces, Implementations, and Surface Area
- Infrastructure-heavy packages (runtime, cluster, forgejo, agent) should define interfaces and keep implementations in `internal` subpackages.
- Provide `stub.go` implementations to aid testing without external dependencies.
- Export only constructors, interfaces, and sentinel errors; unexport helper structs such as BaseAgent when feasible.

## Kubernetes (client-go) and Adapters
- Prefer `client-go` for resource management.
- Centralise `kubectl` usage in shared adapters: port-forwarder and exec wrapper.
- Avoid additional shell-outs from business logic; reuse the adapters.
- Ensure all created resources carry `kagen.io/repo-id` labels.

## Proxy & Egress
- Load proxy policy from configuration and validate it before attaching to the agent; fail closed when unenforced.
- Add proxy reconciliation hooks in the cluster layer when implementing proxy pods.

## Testing
- **Table-Driven Tests**: Use table-driven tests for complex logic.
- **Race Detector**: All tests must pass with `go test -race`.
- **Mocking**: Use interfaces and stubs for unit testing infrastructure packages.
