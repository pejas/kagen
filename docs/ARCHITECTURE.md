# Kagen Architecture

## System Overview
`kagen` provides a local, security-first agent runtime for a single Git repository at a time. It runs on macOS ARM using Colima and K3s, keeps the host checkout canonical, and uses an in-cluster Forgejo instance as the review and durability boundary for agent work.

## Core Design Principles
1. **Host as Source of Truth**: The local Git repository on the host machine is the ultimate canonical state.
2. **Isolation over Performance**: Every agent session runs in a dedicated K8s namespace with explicit egress policies.
3. **Forgejo as Boundary**: All agent commits are made to an in-cluster Forgejo. Merging back to the host is an explicit, human-initiated `kagen pull`.
4. **Idempotent CLI**: The CLI ensures the runtime, cluster, and services are "up" without requiring complex lifecycle commands.

## Architecture Layers

### 1. Host Layer (CLI)
The `kagen` Go binary orchestrates:
- **Runtime Manager (`internal/runtime`)**: Lifecycle of the Colima `kagen` profile and K3s connectivity.
- **Cluster Manager (`internal/cluster`)**: Translation of `devfile.yaml` into Kubernetes Pods, Namespaces, PVCs, and workspace bootstrap init containers.
- **Git Engine (`internal/git`)**: Local Git operations, repository discovery, and WIP protection commits.
- **Port-Forwarder**: Bridges the host network to in-cluster services (Forgejo, Proxy).

### 2. Cluster Layer (K3s)
K3s provides the workload isolation:
- **Namespace**: Created per repository (`kagen-<repo-id>`).
- **Forgejo**: A lightweight Git service running inside the cluster to host the review boundary.
- **Agent Pod**: A Pod generated from the repository's `devfile.yaml`, with the selected agent runtime provisioned inside the VM and the repository cloned into `/projects/workspace`.
- **Persistence**: Managed through standard Kubernetes PVCs.

## Data Flow
1. **Init**: User runs `kagen init` to bootstrap `devfile.yaml`.
2. **Start**: User runs `kagen`.
   - CLI checks Colima status.
   - CLI ensures K8s resources (Namespace, PVC, Forgejo).
   - CLI imports current Host HEAD into in-cluster Forgejo.
   - CLI creates the Agent Pod after Forgejo import so an init container can clone the in-cluster repository into the workspace volume.
   - CLI launches Agent Pod and attaches the TUI.
3. **Work**: Agent performs changes inside the Pod.
4. **Checkpoint**: On exit, CLI creates a WIP commit in the cluster and pushes to Forgejo.
5. **Review**: User runs `kagen open` to view review URL.
6. **Pull**: User runs `kagen pull` to fetch/merge changes from Forgejo back to the host.

## Security Controls
- **Egress Proxy**: All agent network traffic is routed through a proxy with a project-level allowlist.
- **Credential Isolation**: Agent auth state (for example Codex login state in `.codex`) is stored in a dedicated K8s PVC, never exposed to the host's shell history or host-side config directories.
- **Filesystem Silo**: The agent only has access to the workspace volume and the auth PVC.
