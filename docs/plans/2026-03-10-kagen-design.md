# Kagen Design

Date: 2026-03-10
Status: approved for planning

## Overview

`kagen` provides a local, security-first agent runtime for a single Git repository at a time. It runs on macOS ARM using Colima and K3s, keeps the host checkout canonical, and uses an in-cluster Forgejo instance as the review and durability boundary for agent work.

The primary product goal is to feel as close as possible to launching a local agent TUI, while adding isolation, controlled egress, and a reviewable handoff back to the host repository.

## Design Goals

- Keep the host repository as the source of truth.
- Make the default user flow feel like running a local TUI tool.
- Isolate agent credentials, filesystem access, and network access from the host.
- Use Forgejo as the in-cluster review and committed-history boundary.
- Prefer automation over explicit `up` and `down` lifecycle commands.

## Non-Goals

- Preserve uncommitted work across agent pod crashes.
- Auto-merge agent changes back into the host repository.
- Expose host SSH keys, cloud credentials, or direct host workspace mounts to the agent.
- Support multi-repo shared workspaces in v1.

## User Experience

### Commands

- `kagen init`: bootstrap local project configuration only when no `devfile.yaml` exists.
- `kagen`: start or resume work for the current repository and attach directly to the selected agent TUI.
- `kagen --agent claude|codex|opencode`: same as `kagen`, but skip agent selection and launch the requested agent.
- `kagen open`: open the Forgejo review page for the current repository and current branch equivalent.
- `kagen pull`: bring reviewed changes from `kagen/<current-branch>` back into the current local branch using the defined pull/merge workflow.

### First-Run Flow

When the user runs `kagen` in a repository for the first time:

1. Detect the current directory is a Git repository.
2. Detect whether project bootstrap files such as `devfile.yaml` are missing.
3. If bootstrap is missing, direct the user to run `kagen init` and only generate those files there.
4. Verify or install required local runtime dependencies for Colima, K3s access, and local orchestration.
5. Ask which agent to use unless `--agent` was provided.
6. Start the selected agent's authentication flow if required, typically by opening the OAuth URL in the local browser.
7. Reuse or create the repo-scoped cluster resources.
8. Attach the user directly to the working agent TUI.

### Daily Flow

When the environment already exists, `kagen` should behave like opening a local CLI tool:

1. Resolve the current repository identity and current branch.
2. Ensure the local Colima and K3s runtime is healthy.
3. Ensure the repo-scoped namespace, Forgejo instance, proxy, policy, and agent resources exist.
4. Ensure the in-cluster working branch `kagen/<current-branch>` exists or create it from the imported base branch.
5. Start or resume the selected agent session.
6. Attach the terminal to the agent TUI with minimal extra prompts.

### Exit Behavior

When the user exits the agent TUI:

- If there are uncommitted workspace changes, `kagen` creates a WIP commit inside the cluster and pushes it to `kagen/<current-branch>` in Forgejo.
- The local host repository branch is not switched automatically.
- If there is nothing new to commit and nothing new was committed to Forgejo during the session, `kagen` exits quietly.
- If Forgejo contains new commits for `kagen/<current-branch>`, `kagen` prints the local Forgejo review URL so the user can inspect the change before pulling it back.

This gives the convenience of an automatic session checkpoint without keeping a continuous autosave sidecar running in the background.

## Architecture

### Host Layer

A single idempotent `kagen` CLI owns bootstrap and session orchestration. The host is responsible for:

- checking repository state
- ensuring runtime dependencies exist
- starting or reusing Colima
- talking to the K3s API
- opening browser-based auth flows
- attaching the terminal to the running agent TUI

### Cluster Layer

K3s is the isolation boundary. Each repository gets its own namespace and resource set. That namespace contains:

- one Forgejo instance for the repository
- one proxy service plus policy objects
- one agent workload for the selected agent type
- persistent volumes for Forgejo data and agent auth/config

The design remains one-repo-per-environment in v1.

### Workspace Model

- The active working tree lives on a RAM-backed `emptyDir` for fast local iteration.
- Forgejo stores committed history and review state on a PVC.
- Agent auth and tool configuration live on a separate PVC.
- The host checkout is never mounted into the agent workspace.

The local repository remains canonical. The cluster copy is an isolated working mirror with review tooling.

## Source Control Model

### Canonical Source

The host repository is the source of truth. `kagen` imports the repository into Forgejo for isolated execution, review, and branch handoff.

### Branching Model

- The user stays on the local branch, for example `feature/x`.
- The cluster workflow uses `kagen/feature/x` as the branch that accumulates agent work.
- `kagen` never silently switches the local checkout to the `kagen/*` branch.
- `kagen pull` is the explicit bridge from reviewed in-cluster work back into the local branch.

### Import and Pull Back

On startup, `kagen` imports the current repository state into Forgejo and records provenance:

- local repository path
- source branch
- source commit SHA
- import timestamp

When the user is satisfied with the review state, `kagen pull` fetches and merges the reviewed `kagen/<current-branch>` branch back into the current local branch.

## Agent Support

v1 supports at least these agent runtimes:

- Claude
- Codex
- OpenCode

The platform chooses the agent container and auth flow based on either:

- the first-run interactive selection, or
- the explicit `--agent` flag

The rest of the lifecycle should stay uniform across agents so the user experience remains consistent.

## Security Model

### Credential Isolation

- Do not mount host `~/.ssh`.
- Do not mount host cloud credentials.
- Do not expose the host workspace directly to the agent.
- Keep agent auth state inside the cluster on PVC-backed storage.

### Network Control

The proxy is nearly deny-all by default. It permits:

- required LLM/API calls for the selected agent
- explicitly whitelisted destinations from a user-editable config file, such as `~/.config/kagen/proxy.yaml`

If proxy or policy enforcement is not active, secure mode must fail closed instead of allowing direct outbound access.

### Review Boundary

Forgejo is the operational safety layer. The agent can produce commits inside the cluster, but host adoption remains explicit and reviewable.

## Failure Handling

- If Colima or K3s is unavailable, `kagen` fails fast with repair guidance.
- If the current directory is not a Git repo, `kagen` does not attempt bootstrap.
- If import provenance cannot be recorded, startup fails.
- If the agent pod crashes, uncommitted RAM-disk edits are lost.
- If the agent had already committed to Forgejo, recovery happens by recloning `kagen/<current-branch>`.
- If no reviewable changes exist, `kagen open` should make that clear instead of opening a dead page.

## Testing Scope

The implementation plan should cover tests for:

- idempotent `kagen init`
- first-run `kagen` bootstrap and agent selection
- agent-specific auth handoff behavior
- repo import correctness and provenance recording
- `kagen/<branch>` branch creation and reuse
- TUI attach and exit handling
- automatic WIP commit on dirty exit
- `kagen open` review URL resolution
- `kagen pull` merge behavior into the current branch
- proxy allowlist enforcement and fail-closed behavior
- pod recovery from committed Forgejo state

## Recommended V1 Shape

Keep v1 opinionated and small:

- one repo at a time
- one selected agent per session
- one internal Forgejo per repo namespace
- one explicit review path back to the host repo
- automatic session checkpoint on exit, but no continuous autosave sidecar

This keeps the experience close to a native local agent CLI while preserving the extra isolation and review layer that justify the system.
