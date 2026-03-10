package agent

import (
	"fmt"

	kagerr "github.com/pejas/kagen/internal/errors"
)

// Registry provides lookup and enumeration of supported agents.
type Registry struct {
	agents map[Type]Agent
}

// NewRegistry creates a Registry pre-populated with all supported agent stubs.
func NewRegistry() *Registry {
	return &Registry{
		agents: map[Type]Agent{
			Claude:   &stubAgent{name: "Claude", agentType: Claude},
			Codex:    &stubAgent{name: "Codex", agentType: Codex},
			OpenCode: &stubAgent{name: "OpenCode", agentType: OpenCode},
		},
	}
}

// Get returns the agent for the given type, or ErrAgentUnknown if not found.
func (r *Registry) Get(agentType Type) (Agent, error) {
	a, ok := r.agents[agentType]
	if !ok {
		return nil, fmt.Errorf("%w: %q", kagerr.ErrAgentUnknown, agentType)
	}
	return a, nil
}

// Available returns a sorted list of all registered agent types.
func (r *Registry) Available() []Type {
	return []Type{Claude, Codex, OpenCode}
}

// AvailableNames returns a list of display names for prompt selection.
func (r *Registry) AvailableNames() []string {
	types := r.Available()
	names := make([]string, len(types))
	for i, t := range types {
		a := r.agents[t]
		names[i] = a.Name()
	}
	return names
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
