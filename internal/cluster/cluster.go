// Package cluster manages Kubernetes namespace and resource orchestration for kagen.
package cluster

import (
	"context"
	"fmt"

	"github.com/pejas/kagen/internal/agent"
	"github.com/pejas/kagen/internal/devfile"
	kagerr "github.com/pejas/kagen/internal/errors"
	"github.com/pejas/kagen/internal/git"
)

// Manager defines the interface for cluster resource orchestration.
type Manager interface {
	// EnsureNamespace creates or verifies the repo-scoped namespace.
	EnsureNamespace(ctx context.Context, repo *git.Repository) error

	// EnsureResources orchestrates the PVCs, Pod, and other resources for the repository.
	EnsureResources(ctx context.Context, repo *git.Repository, agentType agent.Type, d *devfile.Devfile) error

	// AttachAgent connects the current terminal to the agent process inside the Pod.
	AttachAgent(ctx context.Context, repo *git.Repository) error
}

// PortForwarder manages a port-forward session to a service or pod.
type PortForwarder interface {
	// Start begins the port-forward in the background.
	Start(ctx context.Context, namespace, target string, port int) (int, error)
	// Stop terminates the port-forward.
	Stop() error
}

// NewStubManager returns a new StubManager.
func NewStubManager() *StubManager {
	return &StubManager{}
}

// StubManager is a placeholder implementation.
type StubManager struct{}

func (s *StubManager) EnsureNamespace(_ context.Context, _ *git.Repository) error {
	return fmt.Errorf("ensure namespace: %w", kagerr.ErrNotImplemented)
}

func (s *StubManager) EnsureResources(_ context.Context, _ *git.Repository, _ agent.Type, _ *devfile.Devfile) error {
	return fmt.Errorf("ensure resources: %w", kagerr.ErrNotImplemented)
}

func (s *StubManager) AttachAgent(_ context.Context, _ *git.Repository) error {
	return fmt.Errorf("attach agent: %w", kagerr.ErrNotImplemented)
}
