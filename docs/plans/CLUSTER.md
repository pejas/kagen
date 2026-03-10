# Stage 3 — Cluster Orchestration

Status: draft

## Goal

Replace `cluster.StubManager` with a working Kubernetes resource manager that creates and maintains per-repository namespaces, persistent volumes, and dynamic Devfile-based workloads.

## Dependencies

- **Stage 2 (Runtime)** — provides a healthy K3s cluster.
- **Stage 1.5 (Devfile Orchestration)** — provides the logic to generate Pod specs from a Devfile.
- **Stage 1 (Agent Integration)** — provides the agent tool injection logic.

## Scope

### Namespace and Scope

Each repository continues to have its own namespace (`kagen-<repo>`). This stage focuses on orchestrating the lifecycle of everything within that namespace.

### Resource Set

For each repository, `kagen` ensures:
1. **Namespace:** Created and labeled for NetworkPolicies.
2. **PVCs:**
   - `forgejo-data`: Persistent storage for the in-cluster Git server.
   - `agent-auth`: Persistent storage for agent-specific credentials/config.
3. **The Workload Pod:**
   - Generated from `devfile.yaml`.
   - Injected with the selected **Agent** as a tool.
   - Injected with the **Egress Proxy** sidecar (Stage 6).
   - Mounted with `git-workspace` (RAM-backed `emptyDir`).
4. **Secrets:** API keys or tokens for the agent are synced into the namespace.

### Kubernetes Client-go

Implement the cluster manager using `k8s.io/client-go`. The manager will:
- Connect to K3s using the kubeconfig context from Stage 2.
- Use **Server-Side Apply** to reconcile the desired state (Namespace, PVCs, Pod, Service).
- Provide a `WaitUntilReady` method that blocks until the Pod and Forgejo service are healthy.

## Files

| Action | Path |
|--------|------|
| Modify | `internal/cluster/cluster.go` — Update interface to handle Devfile inputs |
| New    | `internal/cluster/reconciler.go` — Generic K8s resource reconciliation logic |
| Modify | `internal/cluster/resources.go` — Orchestration logic combining Devfile and security sidecars |
| Modify | `internal/cmd/root.go` — Update orchestration flow to pass Devfile to the cluster manager |
