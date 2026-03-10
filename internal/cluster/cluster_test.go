package cluster

import (
	"context"
	"errors"
	"testing"

	"github.com/pejas/kagen/internal/agent"
	kagerr "github.com/pejas/kagen/internal/errors"
	"github.com/pejas/kagen/internal/git"
)

func TestStubManagerEnsureNamespace(t *testing.T) {
	t.Parallel()

	mgr := NewStubManager()
	repo := &git.Repository{Path: "/fake"}
	err := mgr.EnsureNamespace(context.Background(), repo)
	if !errors.Is(err, kagerr.ErrNotImplemented) {
		t.Errorf("expected ErrNotImplemented, got %v", err)
	}
}

func TestStubManagerEnsureResources(t *testing.T) {
	t.Parallel()

	mgr := NewStubManager()
	repo := &git.Repository{Path: "/fake"}
	err := mgr.EnsureResources(context.Background(), repo, agent.Claude)
	if !errors.Is(err, kagerr.ErrNotImplemented) {
		t.Errorf("expected ErrNotImplemented, got %v", err)
	}
}

func TestStubManagerAttachAgent(t *testing.T) {
	t.Parallel()

	mgr := NewStubManager()
	repo := &git.Repository{Path: "/fake"}
	err := mgr.AttachAgent(context.Background(), repo)
	if !errors.Is(err, kagerr.ErrNotImplemented) {
		t.Errorf("expected ErrNotImplemented, got %v", err)
	}
}
