package git

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	kagerr "github.com/pejas/kagen/internal/errors"
)

func TestDiscoverInGitRepo(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	initGitRepo(t, dir)

	repo, err := Discover(dir)
	if err != nil {
		t.Fatalf("Discover() returned error: %v", err)
	}

	if repo.Path != dir {
		t.Errorf("expected Path=%q, got %q", dir, repo.Path)
	}
	if repo.CurrentBranch == "" {
		t.Error("expected non-empty CurrentBranch")
	}
	if repo.HeadSHA == "" {
		t.Error("expected non-empty HeadSHA")
	}
}

func TestDiscoverFromSubdirectory(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	initGitRepo(t, dir)

	subDir := filepath.Join(dir, "deeply", "nested", "subdir")
	if err := os.MkdirAll(subDir, 0o755); err != nil {
		t.Fatalf("failed to create subdir: %v", err)
	}

	repo, err := Discover(subDir)
	if err != nil {
		t.Fatalf("Discover() from subdir returned error: %v", err)
	}

	if repo.Path != dir {
		t.Errorf("expected Path=%q, got %q", dir, repo.Path)
	}
}

func TestDiscoverNotGitRepo(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	_, err := Discover(dir)
	if !errors.Is(err, kagerr.ErrNotGitRepo) {
		t.Errorf("expected ErrNotGitRepo, got %v", err)
	}
}

func TestKagenBranch(t *testing.T) {
	t.Parallel()

	repo := &Repository{CurrentBranch: "feature/x"}
	if got := repo.KagenBranch(); got != "kagen/feature/x" {
		t.Errorf("expected kagen/feature/x, got %q", got)
	}
}

func TestKagenRemoteTrackingBranch(t *testing.T) {
	t.Parallel()

	repo := &Repository{CurrentBranch: "feature/x"}
	if got := repo.KagenRemoteTrackingBranch("kagen"); got != "kagen/kagen/feature/x" {
		t.Errorf("expected kagen/kagen/feature/x, got %q", got)
	}
}

func TestRemoteTrackingBranch(t *testing.T) {
	t.Parallel()

	repo := &Repository{CurrentBranch: "feature/x"}
	if got := repo.RemoteTrackingBranch("kagen"); got != "kagen/feature/x" {
		t.Errorf("expected kagen/feature/x, got %q", got)
	}
}

func TestMergeFFOnly(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	initGitRepo(t, dir)

	runGit(t, dir, "checkout", "-b", "feature")
	runGit(t, dir, "checkout", "-b", "kagen/feature")
	writeFile(t, filepath.Join(dir, "review.txt"), "reviewed\n")
	runGit(t, dir, "add", "review.txt")
	runGit(t, dir, "commit", "-m", "reviewed change")
	runGit(t, dir, "checkout", "feature")

	repo, err := Discover(dir)
	if err != nil {
		t.Fatalf("Discover() returned error: %v", err)
	}

	if err := repo.MergeFFOnly(t.Context(), repo.KagenBranch()); err != nil {
		t.Fatalf("MergeFFOnly() returned error: %v", err)
	}

	head, err := gitCommand(dir, "rev-parse", "HEAD")
	if err != nil {
		t.Fatalf("git rev-parse HEAD returned error: %v", err)
	}
	branchHead, err := gitCommand(dir, "rev-parse", repo.KagenBranch())
	if err != nil {
		t.Fatalf("git rev-parse %s returned error: %v", repo.KagenBranch(), err)
	}
	if head != branchHead {
		t.Fatalf("HEAD = %q, want %q", head, branchHead)
	}
}

func TestPushRefspecs(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	remote := filepath.Join(t.TempDir(), "remote.git")

	runGit(t, dir, "init")
	runGit(t, dir, "config", "user.email", "test@test.com")
	runGit(t, dir, "config", "user.name", "Test")
	runGit(t, dir, "commit", "--allow-empty", "-m", "init")

	runGit(t, filepath.Dir(remote), "init", "--bare", remote)
	runGit(t, dir, "remote", "add", "kagen", remote)

	repo, err := Discover(dir)
	if err != nil {
		t.Fatalf("Discover() returned error: %v", err)
	}

	if err := repo.PushRefspecs(t.Context(), "kagen", "HEAD:main", "HEAD:"+repo.KagenBranch()); err != nil {
		t.Fatalf("PushRefspecs() returned error: %v", err)
	}

	for _, ref := range []string{"main", repo.KagenBranch()} {
		cmd := exec.Command("git", "--git-dir", remote, "rev-parse", ref)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git rev-parse %s failed: %v\n%s", ref, err, out)
		}
	}
}

func TestGitCommandContextHonoursCancellation(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	initGitRepo(t, dir)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := gitCommandContext(ctx, dir, "status", "--short")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("gitCommandContext() error = %v, want %v", err, context.Canceled)
	}
}

// initGitRepo creates a minimal git repo with one commit in dir.
func initGitRepo(t *testing.T, dir string) {
	t.Helper()

	for _, args := range [][]string{
		{"init"},
		{"config", "user.email", "test@test.com"},
		{"config", "user.name", "Test"},
		{"commit", "--allow-empty", "-m", "init"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v failed: %v\n%s", args, err, out)
		}
	}
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()

	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, out)
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()

	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile(%q) failed: %v", path, err)
	}
}
