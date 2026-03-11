package agent

import (
	"fmt"

	kagerr "github.com/pejas/kagen/internal/errors"
	"github.com/pejas/kagen/internal/git"
)

// Registry provides lookup and enumeration of supported agents.
type Registry struct {
	repo    *git.Repository
	kubeCtx string
}

// NewRegistry creates a Registry for the given context.
func NewRegistry(repo *git.Repository, kubeCtx string) *Registry {
	return &Registry{
		repo:    repo,
		kubeCtx: kubeCtx,
	}
}

// Get returns the agent for the given type, or ErrAgentUnknown if not found.
func (r *Registry) Get(agentType Type) (Agent, error) {
	switch agentType {
	case Claude:
		return NewClaudeAgent(r.repo, r.kubeCtx), nil
	case Codex:
		return NewCodexAgent(r.repo, r.kubeCtx), nil
	case OpenCode:
		return NewOpenCodeAgent(r.repo, r.kubeCtx), nil
	default:
		return nil, fmt.Errorf("%w: %q", kagerr.ErrAgentUnknown, agentType)
	}
}

// Available returns a sorted list of all registered agent types.
func (r *Registry) Available() []Type {
	return []Type{Claude, Codex, OpenCode}
}

// AvailableNames returns a list of display names for prompt selection.
func (r *Registry) AvailableNames() []string {
	return []string{"Claude", "Codex", "OpenCode"}
}

// TypeFromString converts a string to a Type, returning ErrAgentUnknown if invalid.
func TypeFromString(s string) (Type, error) {
	t := Type(s)
	switch t {
	case Claude, Codex, OpenCode:
		return t, nil
	default:
		return "", fmt.Errorf("%w: %q", kagerr.ErrAgentUnknown, s)
	}
}
