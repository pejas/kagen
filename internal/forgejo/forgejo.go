// Package forgejo manages the in-cluster Forgejo instance used as the review
// and durability boundary for agent work.
package forgejo

import (
	"context"
	"fmt"

	kagerr "github.com/pejas/kagen/internal/errors"
	"github.com/pejas/kagen/internal/git"
)

// Service defines the interface for Forgejo operations.
type Service interface {
	// EnsureRepo ensures a repository exists in Forgejo for the given repo.
	EnsureRepo(ctx context.Context, repo *git.Repository) error

	// ImportRepo pushes the current repository state into Forgejo.
	ImportRepo(ctx context.Context, repo *git.Repository) error

	// GetReviewURL returns the Forgejo web URL for reviewing the kagen branch.
	GetReviewURL(repo *git.Repository) (string, error)

	// HasNewCommits checks if the kagen branch has commits not yet pulled.
	HasNewCommits(ctx context.Context, repo *git.Repository) (bool, error)
}

// StubService is a placeholder implementation that returns ErrNotImplemented.
type StubService struct{}

// EnsureRepo returns ErrNotImplemented.
func (s *StubService) EnsureRepo(_ context.Context, _ *git.Repository) error {
	return fmt.Errorf("forgejo ensure repo: %w", kagerr.ErrNotImplemented)
}

// ImportRepo returns ErrNotImplemented.
func (s *StubService) ImportRepo(_ context.Context, _ *git.Repository) error {
	return fmt.Errorf("forgejo import repo: %w", kagerr.ErrNotImplemented)
}

// GetReviewURL returns ErrNotImplemented.
func (s *StubService) GetReviewURL(_ *git.Repository) (string, error) {
	return "", fmt.Errorf("forgejo get review URL: %w", kagerr.ErrNotImplemented)
}

// HasNewCommits returns ErrNotImplemented.
func (s *StubService) HasNewCommits(_ context.Context, _ *git.Repository) (bool, error) {
	return false, fmt.Errorf("forgejo has new commits: %w", kagerr.ErrNotImplemented)
}

// NewStubService returns a new StubService.
func NewStubService() *StubService {
	return &StubService{}
}
