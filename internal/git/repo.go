package git

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	kagerr "github.com/pejas/kagen/internal/errors"
)

// Repository holds the identity of a discovered Git repository.
type Repository struct {
	// Path is the root directory of the repository (containing .git).
	Path string

	// CurrentBranch is the currently checked-out branch name.
	CurrentBranch string

	// HeadSHA is the full SHA of HEAD.
	HeadSHA string
}

// ID returns a unique short identifier for the repository based on its path.
func (r *Repository) ID() string {
	h := sha1.New()
	h.Write([]byte(r.Path))
	return hex.EncodeToString(h.Sum(nil))[:8]
}

// KagenBranch returns the in-cluster branch name for this repository,
// following the pattern kagen/<current-branch>.
func (r *Repository) KagenBranch() string {
	return "kagen/" + r.CurrentBranch
}

// Discover walks up from startPath to find the root of a Git repository.
// Returns ErrNotGitRepo if no repository is found.
func Discover(startPath string) (*Repository, error) {
	root, err := findGitRoot(startPath)
	if err != nil {
		return nil, err
	}

	branch, err := currentBranch(root)
	if err != nil {
		return nil, fmt.Errorf("detecting current branch: %w", err)
	}

	sha, err := headSHA(root)
	if err != nil {
		return nil, fmt.Errorf("detecting HEAD SHA: %w", err)
	}

	return &Repository{
		Path:          root,
		CurrentBranch: branch,
		HeadSHA:       sha,
	}, nil
}

// findGitRoot walks up the directory tree looking for a .git directory.
func findGitRoot(startPath string) (string, error) {
	dir, err := filepath.Abs(startPath)
	if err != nil {
		return "", fmt.Errorf("resolving absolute path: %w", err)
	}

	for {
		gitDir := filepath.Join(dir, ".git")
		if info, err := os.Stat(gitDir); err == nil && info.IsDir() {
			return dir, nil
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			// Reached filesystem root without finding .git.
			return "", kagerr.ErrNotGitRepo
		}
		dir = parent
	}
}

// currentBranch returns the current branch name by running git rev-parse.
func currentBranch(repoRoot string) (string, error) {
	out, err := gitCommand(repoRoot, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

// headSHA returns the full HEAD commit SHA.
func headSHA(repoRoot string) (string, error) {
	out, err := gitCommand(repoRoot, "rev-parse", "HEAD")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

// AddRemote adds a new remote to the repository. If it already exists, it is updated.
func (r *Repository) AddRemote(name, url string) error {
	// Try to add. If it exists, this will fail.
	_, err := gitCommand(r.Path, "remote", "add", name, url)
	if err != nil {
		// Try to set-url instead
		_, err = gitCommand(r.Path, "remote", "set-url", name, url)
		if err != nil {
			return fmt.Errorf("failed to add or update remote: %w", err)
		}
	}
	return nil
}

// Push pushes the specified ref to the given remote.
func (r *Repository) Push(ctx context.Context, remote, ref string) error {
	_, err := gitCommand(r.Path, "push", "-f", remote, ref)
	if err != nil {
		return fmt.Errorf("git push %s %s: %w", remote, ref, err)
	}
	return nil
}

// Fetch fetches from the specified remote.
func (r *Repository) Fetch(ctx context.Context, remote string) error {
	_, err := gitCommand(r.Path, "fetch", remote)
	if err != nil {
		return fmt.Errorf("git fetch %s: %w", remote, err)
	}
	return nil
}

// Merge merges the specified ref into the current branch.
func (r *Repository) Merge(ctx context.Context, ref string) error {
	_, err := gitCommand(r.Path, "merge", "--no-edit", ref)
	if err != nil {
		return fmt.Errorf("git merge %s: %w", ref, err)
	}
	return nil
}

// MergeFFOnly fast-forwards the current branch to the specified ref.
func (r *Repository) MergeFFOnly(ctx context.Context, ref string) error {
	_, err := gitCommand(r.Path, "merge", "--ff-only", ref)
	if err != nil {
		return fmt.Errorf("git merge --ff-only %s: %w", ref, err)
	}

	return nil
}

// HasUncommittedChanges checks if there are uncommitted changes in the worktree.
func (r *Repository) HasUncommittedChanges() bool {
	out, err := gitCommand(r.Path, "status", "--porcelain")
	if err != nil {
		return true // Assume dirty on error.
	}
	return len(strings.TrimSpace(out)) > 0
}

// Commit creates a new commit with all changes (WIP).
func (r *Repository) Commit(message string) error {
	_, err := gitCommand(r.Path, "add", "-A")
	if err != nil {
		return fmt.Errorf("git add: %w", err)
	}
	_, err = gitCommand(r.Path, "commit", "-m", message)
	if err != nil {
		return fmt.Errorf("git commit: %w", err)
	}
	return nil
}

// gitCommand runs a git command in the given directory and returns stdout.
func gitCommand(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir

	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git %s: %s: %w", strings.Join(args, " "), string(out), err)
	}

	return string(out), nil
}
