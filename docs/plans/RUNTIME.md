# Stage 2 — Runtime Management

Status: draft

## Goal

Replace the `runtime.StubManager` with a working Colima/K3s lifecycle manager. After this stage, `kagen` can start the container runtime, verify its health, and expose a working kubeconfig for subsequent stages.

## Dependencies

None. This is the foundational infrastructure layer.

## Scope

### Colima Lifecycle

Colima wraps a Linux VM with containerd and optionally K3s. Kagen manages a dedicated Colima profile named `kagen` to avoid interfering with the user's other container workloads.

| Operation | Implementation |
|-----------|---------------|
| **Detect** | Run `colima status --profile kagen`. Parse the output to determine `Running`, `Stopped`, or `Not Found`. |
| **Start** | Run `colima start --profile kagen --kubernetes --cpu 4 --memory 8 --disk 60`. The resource defaults should be configurable via `~/.config/kagen/config.yaml`. |
| **Health check** | After start, verify that `kubectl --context colima-kagen get nodes` returns a `Ready` node within a timeout (default 60s). |
| **Kubeconfig** | Colima writes a context named `colima-kagen` to `~/.kube/config`. The runtime manager exposes the context name so the cluster layer can use it. |

### Dependency Detection

On first run (or when Colima is missing), `kagen` should:

1. Check that `colima` is on PATH. If missing, print install instructions (`brew install colima`) and exit.
2. Check that `kubectl` is on PATH. If missing, print install instructions and exit.
3. Check that `docker` or `nerdctl` is available for image building. If missing, warn but do not block (images can be pulled from registries).

These checks happen inside `EnsureRunning` before attempting to start Colima.

### Manager Interface Refinement

The current `Manager` interface is sufficient. Add one method:

```go
type Manager interface {
    EnsureRunning(ctx context.Context) error
    Status(ctx context.Context) (Status, error)
    KubeContext() string  // returns the kubectl context name
}
```

### Configuration

Add to `config.Config`:

```yaml
runtime:
  cpu: 4
  memory: 8
  disk: 60
  startup_timeout: 60s
```

## Files

| Action | Path |
|--------|------|
| Modify | `internal/runtime/runtime.go` — add `KubeContext()` to interface |
| New    | `internal/runtime/colima.go` — Colima-backed Manager implementation |
| New    | `internal/runtime/deps.go` — dependency detection helpers |
| Modify | `internal/config/config.go` — add `Runtime` config section |
| New    | `internal/runtime/colima_test.go` |
| New    | `internal/runtime/deps_test.go` |
| Modify | `internal/cmd/root.go` — replace `NewStubManager()` with `NewColimaManager(cfg)` |

## Verification

- Unit tests for dependency detection (mock exec.LookPath).
- Unit tests for status parsing from `colima status` output.
- Integration test (requires Colima installed): start → status → kubecontext → stop cycle.
- Manual: run `kagen` without Colima installed — confirm clear install guidance. Run with Colima — confirm it starts and reports a healthy node.
