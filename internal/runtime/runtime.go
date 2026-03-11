// Package runtime manages the Colima/K3s lifecycle for kagen.
package runtime

import (
	"context"
	"fmt"

	kagerr "github.com/pejas/kagen/internal/errors"
)

// Status represents the state of the local runtime.
type Status int

const (
	// StatusUnknown indicates the runtime state could not be determined.
	StatusUnknown Status = iota
	// StatusRunning indicates the runtime is healthy.
	StatusRunning
	// StatusStopped indicates the runtime is not running.
	StatusStopped
)

// String returns a human-readable representation of the status.
func (s Status) String() string {
	switch s {
	case StatusRunning:
		return "running"
	case StatusStopped:
		return "stopped"
	default:
		return "unknown"
	}
}

// Manager defines the interface for controlling the local container runtime.
type Manager interface {
	// EnsureRunning starts the runtime if it is not already running.
	EnsureRunning(ctx context.Context) error

	// Status returns the current runtime status.
	Status(ctx context.Context) (Status, error)

	// KubeContext returns the kubectl context name for this runtime.
	KubeContext() string
}

// StubManager is a placeholder implementation that returns ErrNotImplemented.
type StubManager struct{}

// EnsureRunning returns ErrNotImplemented.
func (s *StubManager) EnsureRunning(_ context.Context) error {
	return fmt.Errorf("runtime ensure running: %w", kagerr.ErrNotImplemented)
}

// Status returns ErrNotImplemented.
func (s *StubManager) Status(_ context.Context) (Status, error) {
	return StatusUnknown, fmt.Errorf("runtime status: %w", kagerr.ErrNotImplemented)
}

// KubeContext returns a dummy context for the stub.
func (s *StubManager) KubeContext() string {
	return "stub-context"
}

// NewStubManager returns a new StubManager.
func NewStubManager() *StubManager {
	return &StubManager{}
}
