// Package cluster manages Kubernetes namespace and resource orchestration for kagen.
package cluster

import (
	"context"
	"fmt"

	"github.com/pejas/kagen/internal/agent"
	kagerr "github.com/pejas/kagen/internal/errors"
	"github.com/pejas/kagen/internal/git"
)

// Manager defines the interface for cluster resource orchestration.
type Manager interface {
	// EnsureNamespace creates or verifies the repo-scoped namespace.
	EnsureNamespace(ctx context.Context, repo *git.Repository) error

	// EnsureResources creates or verifies all required resources
	// (Forgejo, proxy, agent workload, PVCs) in the namespace.
	EnsureResources(ctx context.Context, repo *git.Repository, agentType agent.Type) error

	// AttachAgent connects the terminal to the running agent pod.
	AttachAgent(ctx context.Context, repo *git.Repository) error
}

// StubManager is a placeholder implementation that returns ErrNotImplemented.
type StubManager struct{}

// EnsureNamespace returns ErrNotImplemented.
func (s *StubManager) EnsureNamespace(_ context.Context, _ *git.Repository) error {
	return fmt.Errorf("cluster ensure namespace: %w", kagerr.ErrNotImplemented)
}

// EnsureResources returns ErrNotImplemented.
func (s *StubManager) EnsureResources(_ context.Context, _ *git.Repository, _ agent.Type) error {
	return fmt.Errorf("cluster ensure resources: %w", kagerr.ErrNotImplemented)
}

// AttachAgent returns ErrNotImplemented.
func (s *StubManager) AttachAgent(_ context.Context, _ *git.Repository) error {
	return fmt.Errorf("cluster attach agent: %w", kagerr.ErrNotImplemented)
}

// NewStubManager returns a new StubManager.
func NewStubManager() *StubManager {
	return &StubManager{}
}
