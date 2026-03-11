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

## Interface vs. Implementation
- Infrastructure-heavy packages (runtime, cluster, forgejo, agent) should define an interface.
- Provide a `stub.go` implementation in each package for local development and testing without dependencies.
- Use `New[Service](...)` constructors to inject dependencies (e.g., K8s clientset, port-forwarder).

## Kubernetes (client-go)
- Prefer `client-go` for resource management.
- Avoid shell-ing out to `kubectl` except for interactive TUI attachment (`exec -it`) or port-forwarding processes.
- Ensure all created resources are labeled with `kagen.io/repo-id`.

## Testing
- **Table-Driven Tests**: Use table-driven tests for complex logic.
- **Race Detector**: All tests must pass with `go test -race`.
- **Mocking**: Use interfaces and stubs for unit testing infrastructure packages.
