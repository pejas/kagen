package cluster

import (
	"context"
	"errors"
	"testing"

	kagerr "github.com/pejas/kagen/internal/errors"
	"github.com/pejas/kagen/internal/git"
	"github.com/pejas/kagen/internal/proxy"
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
	err := mgr.EnsureResources(context.Background(), &git.Repository{}, "codex", nil)
	if err == nil {
		t.Error("EnsureResources() expected error, got nil")
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

func TestStubManagerEnsureProxy(t *testing.T) {
	t.Parallel()

	mgr := NewStubManager()
	repo := &git.Repository{Path: "/fake"}
	err := mgr.EnsureProxy(context.Background(), repo, &proxy.Policy{})
	if !errors.Is(err, kagerr.ErrNotImplemented) {
		t.Errorf("expected ErrNotImplemented, got %v", err)
	}
}

func TestStubManagerProxyReady(t *testing.T) {
	t.Parallel()

	mgr := NewStubManager()
	repo := &git.Repository{Path: "/fake"}
	_, err := mgr.ProxyReady(context.Background(), repo)
	if !errors.Is(err, kagerr.ErrNotImplemented) {
		t.Errorf("expected ErrNotImplemented, got %v", err)
	}
}
