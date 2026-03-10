// Package agent defines the agent type registry and lifecycle interfaces for kagen.
package agent

import (
	"context"
	"fmt"

	kagerr "github.com/pejas/kagen/internal/errors"
)

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

	// Attach connects the user's terminal to the running agent TUI.
	Attach(ctx context.Context) error
}

// stubAgent is a placeholder implementation that returns ErrNotImplemented
// for all lifecycle operations.
type stubAgent struct {
	name      string
	agentType Type
}

func (s *stubAgent) Name() string    { return s.name }
func (s *stubAgent) AgentType() Type { return s.agentType }

func (s *stubAgent) Authenticate(_ context.Context) error {
	return fmt.Errorf("authenticate %s: %w", s.name, kagerr.ErrNotImplemented)
}

func (s *stubAgent) Launch(_ context.Context) error {
	return fmt.Errorf("launch %s: %w", s.name, kagerr.ErrNotImplemented)
}

func (s *stubAgent) Attach(_ context.Context) error {
	return fmt.Errorf("attach %s: %w", s.name, kagerr.ErrNotImplemented)
}
