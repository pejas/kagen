package agent

import (
	"fmt"
	"slices"
	"sync"

	kagerr "github.com/pejas/kagen/internal/errors"
	"github.com/pejas/kagen/internal/git"
)

// AgentDependencies captures the runtime context injected by the CLI.
type AgentDependencies struct {
	Repo          *git.Repository
	KubeCtx       string
	ContainerName string
	StatePath     string
}

// Factory constructs an agent for the provided runtime dependencies.
type Factory func(AgentDependencies) Agent

type registration struct {
	spec    RuntimeSpec
	factory Factory
}

var (
	registryMu      sync.RWMutex
	registryEntries = map[Type]registration{}
)

// Registry provides lookup and enumeration of supported agents.
type Registry struct {
	repo    *git.Repository
	kubeCtx string

	containerName string
	statePath     string
}

// NewRegistry creates a Registry for the given context.
func NewRegistry(repo *git.Repository, kubeCtx string) *Registry {
	return &Registry{
		repo:    repo,
		kubeCtx: kubeCtx,
	}
}

// WithContainer returns a copy of the registry that targets the given
// container for attach and readiness checks.
func (r *Registry) WithContainer(name string) *Registry {
	clone := *r
	clone.containerName = name
	return &clone
}

// WithStatePath returns a copy of the registry that targets the provided
// per-agent-session runtime state path during attach.
func (r *Registry) WithStatePath(path string) *Registry {
	clone := *r
	clone.statePath = path
	return &clone
}

// RegisterFactory registers a runtime specification and agent factory.
func RegisterFactory(spec RuntimeSpec, factory Factory) {
	if spec == nil {
		panic("agent runtime spec must not be nil")
	}
	if factory == nil {
		panic(fmt.Sprintf("agent factory for %q must not be nil", spec.Type()))
	}
	if spec.Type() == "" {
		panic("agent runtime spec type must not be empty")
	}

	registryMu.Lock()
	defer registryMu.Unlock()

	if _, exists := registryEntries[spec.Type()]; exists {
		panic(fmt.Sprintf("agent %q already registered", spec.Type()))
	}

	registryEntries[spec.Type()] = registration{
		spec:    spec,
		factory: factory,
	}
}

// SpecFor returns the runtime specification for the given agent.
func SpecFor(agentType Type) (RuntimeSpec, error) {
	registration, ok := lookupRegistration(agentType)
	if !ok {
		return nil, fmt.Errorf("runtime spec for %s: %w", agentType, kagerr.ErrAgentUnknown)
	}

	return registration.spec, nil
}

// Get returns the agent for the given type, or ErrAgentUnknown if not found.
func (r *Registry) Get(agentType Type) (Agent, error) {
	registration, ok := lookupRegistration(agentType)
	if !ok {
		return nil, fmt.Errorf("%w: %q", kagerr.ErrAgentUnknown, agentType)
	}

	return registration.factory(AgentDependencies{
		Repo:          r.repo,
		KubeCtx:       r.kubeCtx,
		ContainerName: r.containerName,
		StatePath:     r.statePath,
	}), nil
}

// Available returns a sorted list of all registered agent types.
func (r *Registry) Available() []Type {
	return SupportedTypes()
}

// RegisteredAgents returns the registered agent types.
func (r *Registry) RegisteredAgents() []Type {
	return SupportedTypes()
}

// AvailableNames returns a list of display names for prompt selection.
func (r *Registry) AvailableNames() []string {
	return SupportedNames()
}

// SupportedTypes returns the supported runtime identifiers.
func SupportedTypes() []Type {
	registryMu.RLock()
	defer registryMu.RUnlock()

	types := make([]Type, 0, len(registryEntries))
	for agentType := range registryEntries {
		types = append(types, agentType)
	}
	slices.Sort(types)

	return types
}

// SupportedNames returns the display names for interactive prompts.
func SupportedNames() []string {
	registryMu.RLock()
	defer registryMu.RUnlock()

	names := make([]string, 0, len(registryEntries))
	for _, registration := range registryEntries {
		names = append(names, registration.spec.DisplayName())
	}
	slices.Sort(names)

	return names
}

// TypeFromString converts a string to a Type, returning ErrAgentUnknown if invalid.
func TypeFromString(s string) (Type, error) {
	t := Type(s)
	if _, ok := lookupRegistration(t); ok {
		return t, nil
	}

	return "", fmt.Errorf("%w: %q", kagerr.ErrAgentUnknown, s)
}

func lookupRegistration(agentType Type) (registration, bool) {
	registryMu.RLock()
	defer registryMu.RUnlock()

	registration, ok := registryEntries[agentType]
	return registration, ok
}
