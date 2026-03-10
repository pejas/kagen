# Stage 6 — Network Policy and Proxy

Status: draft

## Goal

Deploy an egress proxy and network policies that enforce a deny-all-by-default posture for agent pods. Only explicitly whitelisted destinations are reachable. If the proxy is not active, the system fails closed.

## Dependencies

- **Stage 3 (Cluster)** — provides the namespace and K8s client.

## Scope

### Proxy Deployment

Deploy a lightweight forward proxy (e.g., Squid or mitmproxy in forward mode) as a Deployment in the repo namespace. The proxy:

- Listens on a known in-cluster address (e.g., `proxy.kagen-<repo>.svc.cluster.local:3128`).
- Reads its allowlist from the `proxy-policy` ConfigMap.
- Denies all traffic not matching the allowlist.

The agent pod's `HTTP_PROXY` and `HTTPS_PROXY` environment variables point to the proxy service.

### Network Policy

A Kubernetes `NetworkPolicy` attached to the repo namespace:

- Denies all egress from agent pods by default.
- Allows egress only to the proxy service's ClusterIP.
- Allows egress to the Forgejo service (for `git push`).
- Allows DNS resolution (UDP 53 to kube-dns).

This ensures that even if the agent ignores `HTTP_PROXY`, it cannot reach the internet directly.

### Allowlist Management

The allowlist is sourced from two places, merged at deploy time:

1. **Agent defaults** — each `AgentSpec` (from Stage 1) declares its required API endpoints (e.g., `api.anthropic.com` for Claude, `api.openai.com` for Codex).
2. **User overrides** — from `proxy_allowlist` in `~/.config/kagen/config.yaml` or `.kagen.yaml`.

The merged list is written into the `proxy-policy` ConfigMap.

### Fail-Closed Enforcement

The existing `proxy.Validate()` method already returns `ErrProxyNotActive` when enforcement is off. In this stage:

1. After deploying the proxy, the cluster manager marks the policy as `Enforced = true`.
2. If the proxy pod is not `Running` or the NetworkPolicy is missing, `Validate()` fails and `kagen` refuses to launch the agent.
3. This check runs as part of the root command flow, after `EnsureResources` and before `Launch`.

### Proxy Configuration

Add to `config.Config`:

```yaml
proxy:
  image: "docker.io/ubuntu/squid:latest"
  port: 3128
```

## Files

| Action | Path |
|--------|------|
| Modify | `internal/proxy/proxy.go` — add deployment and enforcement validation |
| New    | `internal/proxy/deploy.go` — proxy Deployment, Service, ConfigMap manifests |
| New    | `internal/proxy/netpol.go` — NetworkPolicy manifest generation |
| New    | `internal/proxy/allowlist.go` — merge agent defaults with user overrides |
| Modify | `internal/cluster/resources.go` — include proxy and netpol in EnsureResources |
| Modify | `internal/config/config.go` — add Proxy config section |
| Modify | `internal/cmd/root.go` — add proxy validation before agent launch |
| New    | `internal/proxy/deploy_test.go` |
| New    | `internal/proxy/netpol_test.go` |
| New    | `internal/proxy/allowlist_test.go` |

## Verification

- Unit tests for allowlist merging (agent defaults + user overrides, deduplication).
- Unit tests for NetworkPolicy manifest correctness (egress rules, pod selectors).
- Unit tests for proxy ConfigMap generation from merged allowlist.
- Integration test: deploy proxy → deploy agent pod → verify agent can reach an allowed host → verify agent cannot reach a denied host.
- Manual: run `kagen` → make an agent request to an allowed API → confirm it succeeds. Add a non-whitelisted URL → confirm it is blocked. Stop the proxy pod → confirm `kagen` refuses to continue.
