package forgejo

import (
	"context"
	"errors"
	"testing"

	kagerr "github.com/pejas/kagen/internal/errors"
	"github.com/pejas/kagen/internal/git"
)

func TestStubServiceEnsureRepo(t *testing.T) {
	t.Parallel()

	svc := NewStubService()
	repo := &git.Repository{Path: "/fake"}
	err := svc.EnsureRepo(context.Background(), repo)
	if !errors.Is(err, kagerr.ErrNotImplemented) {
		t.Errorf("expected ErrNotImplemented, got %v", err)
	}
}

func TestStubServiceImportRepo(t *testing.T) {
	t.Parallel()

	svc := NewStubService()
	repo := &git.Repository{Path: "/fake"}
	err := svc.ImportRepo(context.Background(), repo)
	if !errors.Is(err, kagerr.ErrNotImplemented) {
		t.Errorf("expected ErrNotImplemented, got %v", err)
	}
}

func TestStubServiceGetReviewURL(t *testing.T) {
	t.Parallel()

	svc := NewStubService()
	repo := &git.Repository{Path: "/fake"}
	_, err := svc.GetReviewURL(repo)
	if !errors.Is(err, kagerr.ErrNotImplemented) {
		t.Errorf("expected ErrNotImplemented, got %v", err)
	}
}

func TestStubServiceHasNewCommits(t *testing.T) {
	t.Parallel()

	svc := NewStubService()
	repo := &git.Repository{Path: "/fake"}
	_, err := svc.HasNewCommits(context.Background(), repo)
	if !errors.Is(err, kagerr.ErrNotImplemented) {
		t.Errorf("expected ErrNotImplemented, got %v", err)
	}
}
