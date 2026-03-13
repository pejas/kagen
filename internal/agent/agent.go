// Package agent defines the agent type registry and lifecycle interfaces for kagen.
package agent

import "context"

// Type identifies a supported agent runtime.
type Type string

// Supported agent types.
const (
	Claude   Type = "claude"
	Codex    Type = "codex"
	OpenCode Type = "opencode"
)

// Agent defines the lifecycle contract for an agent runtime.
type Agent interface {
	// Name returns a display name for the agent.
	Name() string

	// AgentType returns the Type identifier.
	AgentType() Type

	// Authenticate performs any required authentication flow (e.g., OAuth).
	Authenticate(ctx context.Context) error

	// Launch starts the agent workload inside the cluster.
	Launch(ctx context.Context) error

	// Prepare ensures any runtime-scoped state is ready before attach.
	Prepare(ctx context.Context) error

	// Attach connects the user's terminal to the running agent TUI.
	Attach(ctx context.Context) error
}
