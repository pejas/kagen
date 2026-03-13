package git

import (
	"context"
	"crypto/sha1"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
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

// BasicAuth carries transient HTTP basic-auth credentials for one Git operation.
type BasicAuth struct {
	Username string
	Password string
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

// KagenRemoteTrackingBranch returns the local remote-tracking ref for the
// in-cluster branch on the given remote.
func (r *Repository) KagenRemoteTrackingBranch(remote string) string {
	return remote + "/" + r.KagenBranch()
}

// RemoteTrackingBranch returns the local remote-tracking ref for the current
// branch on the given remote.
func (r *Repository) RemoteTrackingBranch(remote string) string {
	return remote + "/" + r.CurrentBranch
}

// Discover resolves the top-level directory of the Git repository containing
// startPath. It supports normal repositories and worktrees.
// Returns ErrNotGitRepo if no repository is found.
func Discover(startPath string) (*Repository, error) {
	root, err := gitTopLevel(startPath)
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

func gitTopLevel(startPath string) (string, error) {
	out, err := gitCommand(startPath, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", kagerr.ErrNotGitRepo
	}

	return strings.TrimSpace(out), nil
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

// Push pushes the specified ref to the given remote.
func (r *Repository) Push(ctx context.Context, remote, ref string) error {
	return r.PushRefspecs(ctx, remote, ref)
}

// PushRefspecs pushes one or more refspecs to the given remote.
func (r *Repository) PushRefspecs(ctx context.Context, remote string, refspecs ...string) error {
	if len(refspecs) == 0 {
		return fmt.Errorf("git push %s: no refspecs provided", remote)
	}

	args := []string{"push", "-f", remote}
	args = append(args, refspecs...)

	_, err := gitCommandContext(ctx, r.Path, args...)
	if err != nil {
		return fmt.Errorf("git push %s %s: %w", remote, strings.Join(refspecs, " "), err)
	}

	return nil
}

// PushURL pushes one or more refspecs to the given transient remote URL.
func (r *Repository) PushURL(ctx context.Context, remoteURL string, auth *BasicAuth, refspecs ...string) error {
	if len(refspecs) == 0 {
		return fmt.Errorf("git push %s: no refspecs provided", remoteURL)
	}

	args := []string{"push", "-f", remoteURL}
	args = append(args, refspecs...)

	if _, err := gitCommandContextWithAuth(ctx, r.Path, auth, args...); err != nil {
		return fmt.Errorf("git push %s %s: %w", remoteURL, strings.Join(refspecs, " "), err)
	}

	return nil
}

// ResolveRef resolves the given ref to a commit SHA.
func (r *Repository) ResolveRef(ref string) (string, error) {
	out, err := gitCommand(r.Path, "rev-parse", ref)
	if err != nil {
		return "", fmt.Errorf("git rev-parse %s: %w", ref, err)
	}

	return strings.TrimSpace(out), nil
}

// HasRef reports whether the given ref exists.
func (r *Repository) HasRef(ref string) bool {
	_, err := gitCommand(r.Path, "rev-parse", "--verify", "--quiet", ref)
	return err == nil
}

// Fetch fetches from the specified remote.
func (r *Repository) Fetch(ctx context.Context, remote string) error {
	_, err := gitCommandContext(ctx, r.Path, "fetch", remote)
	if err != nil {
		return fmt.Errorf("git fetch %s: %w", remote, err)
	}
	return nil
}

// FetchURL fetches one or more refspecs from a transient remote URL without
// persisting any remote configuration in .git/config.
func (r *Repository) FetchURL(ctx context.Context, remoteURL string, auth *BasicAuth, refspecs ...string) error {
	if len(refspecs) == 0 {
		return fmt.Errorf("git fetch %s: no refspecs provided", remoteURL)
	}

	args := []string{"fetch", "--prune", remoteURL}
	args = append(args, refspecs...)

	if _, err := gitCommandContextWithAuth(ctx, r.Path, auth, args...); err != nil {
		return fmt.Errorf("git fetch %s %s: %w", remoteURL, strings.Join(refspecs, " "), err)
	}

	return nil
}

// RemoteRefSHA resolves a remote ref on a transient remote URL.
func (r *Repository) RemoteRefSHA(ctx context.Context, remoteURL string, auth *BasicAuth, ref string) (string, bool, error) {
	out, err := gitCommandContextWithAuth(ctx, r.Path, auth, "ls-remote", remoteURL, ref)
	if err != nil {
		return "", false, fmt.Errorf("git ls-remote %s %s: %w", remoteURL, ref, err)
	}

	line := strings.TrimSpace(out)
	if line == "" {
		return "", false, nil
	}

	fields := strings.Fields(line)
	if len(fields) < 2 {
		return "", false, fmt.Errorf("parsing git ls-remote output %q", line)
	}

	return fields[0], true, nil
}

// Merge merges the specified ref into the current branch.
func (r *Repository) Merge(ctx context.Context, ref string) error {
	_, err := gitCommandContext(ctx, r.Path, "merge", "--no-edit", ref)
	if err != nil {
		return fmt.Errorf("git merge %s: %w", ref, err)
	}
	return nil
}

// MergeFFOnly fast-forwards the current branch to the specified ref.
func (r *Repository) MergeFFOnly(ctx context.Context, ref string) error {
	_, err := gitCommandContext(ctx, r.Path, "merge", "--ff-only", ref)
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

func gitCommandContext(ctx context.Context, dir string, args ...string) (string, error) {
	if ctx == nil {
		return gitCommand(dir, args...)
	}

	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir

	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", wrapGitContextError(ctx, args, out, err)
	}

	return string(out), nil
}

func gitCommandContextWithAuth(ctx context.Context, dir string, auth *BasicAuth, args ...string) (string, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	cmd.Env = gitEnvWithAuth(auth)

	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", wrapGitContextError(ctx, args, out, err)
	}

	return string(out), nil
}

func gitEnvWithAuth(auth *BasicAuth) []string {
	env := append([]string{}, os.Environ()...)
	env = append(env, "GIT_TERMINAL_PROMPT=0")
	if auth == nil || auth.Username == "" {
		return env
	}

	header := "Authorization: Basic " + base64.StdEncoding.EncodeToString([]byte(auth.Username+":"+auth.Password))
	env = append(env,
		"GIT_CONFIG_COUNT=1",
		"GIT_CONFIG_KEY_0=http.extraHeader",
		"GIT_CONFIG_VALUE_0="+header,
	)

	return env
}

func wrapGitContextError(ctx context.Context, args []string, out []byte, err error) error {
	if ctx != nil && ctx.Err() != nil {
		return ctx.Err()
	}

	return fmt.Errorf("git %s: %s: %w", strings.Join(args, " "), string(out), err)
}
