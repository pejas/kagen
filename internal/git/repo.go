// Package git provides Git repository detection and branch operations for kagen.
package git

import (
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

// gitCommand runs a git command in the given directory and returns stdout.
func gitCommand(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir

	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
	}

	return string(out), nil
}
