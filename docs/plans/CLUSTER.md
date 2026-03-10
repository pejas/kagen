# Stage 3 — Cluster Orchestration

Status: draft

## Goal

Replace `cluster.StubManager` with a working Kubernetes resource manager that creates and maintains per-repository namespaces, persistent volumes, and workload objects. After this stage, `kagen` can deploy all required resources into K3s and connect to the agent pod.

## Dependencies

- **Stage 2 (Runtime)** — provides a healthy K3s cluster and kubeconfig context.
- **Stage 1 (Agent)** — provides the `AgentSpec` that defines what to deploy.

## Scope

### Namespace Strategy

Each repository gets a deterministic namespace derived from the repository path:

```
kagen-<sanitised-repo-name>
```

Where `sanitised-repo-name` is the basename of the repo directory, lowercased, with non-alphanumeric characters replaced by hyphens. Collisions are handled by appending a short hash if needed.

### Resource Set

For each namespace, `EnsureResources` creates or updates the following Kubernetes objects:

| Resource | Purpose |
|----------|---------|
| `Namespace` | Isolation boundary for the repo |
| `PersistentVolumeClaim` — `forgejo-data` | Forgejo committed history and config |
| `PersistentVolumeClaim` — `agent-auth` | Agent authentication state and tool config |
| `Deployment` — `forgejo` | Forgejo instance (created in Stage 4) |
| `Service` — `forgejo` | In-cluster access to Forgejo HTTP and SSH |
| `Deployment` — `proxy` | Egress proxy (created in Stage 6) |
| `ConfigMap` — `proxy-policy` | Allowlist configuration (created in Stage 6) |
| `Pod` — `agent-<type>` | The agent workload (from AgentSpec) |
| `Secret` — `agent-credentials` | API keys or tokens for the selected agent |

Not all resources are created in this stage. This stage creates the namespace, PVCs, agent pod, and credentials secret. The Forgejo and proxy resources have placeholder manifests that Stage 4 and Stage 6 fill in.

### Idempotency

All operations must be idempotent. The manager uses server-side apply (`kubectl apply`) semantics:

- If a resource exists and matches the desired state, do nothing.
- If a resource exists but differs, update it.
- If a resource does not exist, create it.

### Client Approach

Use the `k8s.io/client-go` library directly rather than shelling out to `kubectl` for resource management. The `kubectl exec` path for TUI attach (Stage 1) is the one exception where exec is acceptable.

The manager takes a kubeconfig context name from the runtime manager and builds a client from it.

### Attach

`AttachAgent` locates the agent pod in the namespace and delegates to the shared `agent.Attach` function from Stage 1.

## Files

| Action | Path |
|--------|------|
| Modify | `internal/cluster/cluster.go` — Replace stub with K8s client-go implementation |
| New    | `internal/cluster/namespace.go` — namespace naming and creation |
| New    | `internal/cluster/resources.go` — PVC, Pod, Secret, Service manifests |
| New    | `internal/cluster/client.go` — kubeconfig client construction |
| New    | `internal/cluster/namespace_test.go` |
| New    | `internal/cluster/resources_test.go` |
| Modify | `internal/cmd/root.go` — wire real cluster manager with runtime kubecontext |
| Modify | `go.mod` — add `k8s.io/client-go` dependency |

## Verification

- Unit tests for namespace name generation (sanitisation, collision handling).
- Unit tests for manifest generation (given an AgentSpec, produce correct Pod spec).
- Integration test with a real K3s cluster: create namespace → create PVCs → create agent pod → verify pod is running → delete namespace.
- Manual: run `kagen --agent claude` and confirm namespace and PVCs are created in the cluster (`kubectl get ns`, `kubectl get pvc -n kagen-<repo>`).
