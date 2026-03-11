package cmd

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	kagerr "github.com/pejas/kagen/internal/errors"
	"github.com/pejas/kagen/internal/git"
)

func TestValidatePullRefsRejectsCanonicalBranchDrift(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	initGitRepo(t, dir)

	repo, err := git.Discover(dir)
	if err != nil {
		t.Fatalf("Discover() returned error: %v", err)
	}

	writeFile(t, filepath.Join(dir, "review.txt"), "review\n")
	runGit(t, dir, "add", "review.txt")
	runGit(t, dir, "commit", "-m", "review")
	runGit(t, dir, "branch", repo.KagenRemoteTrackingBranch("kagen"))
	runGit(t, dir, "reset", "--hard", "HEAD~1")

	writeFile(t, filepath.Join(dir, "base.txt"), "base drift\n")
	runGit(t, dir, "add", "base.txt")
	runGit(t, dir, "commit", "-m", "base drift")
	runGit(t, dir, "branch", repo.RemoteTrackingBranch("kagen"))
	runGit(t, dir, "reset", "--hard", "HEAD~1")

	if err := validatePullRefs(repo, repo.KagenRemoteTrackingBranch("kagen"), repo.RemoteTrackingBranch("kagen")); err == nil {
		t.Fatal("validatePullRefs() expected error for canonical branch drift")
	}
}

func TestValidatePullRefsRejectsMissingReviewBranch(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	initGitRepo(t, dir)

	repo, err := git.Discover(dir)
	if err != nil {
		t.Fatalf("Discover() returned error: %v", err)
	}

	runGit(t, dir, "branch", repo.RemoteTrackingBranch("kagen"))

	err = validatePullRefs(repo, repo.KagenRemoteTrackingBranch("kagen"), repo.RemoteTrackingBranch("kagen"))
	if err == nil {
		t.Fatal("validatePullRefs() expected error for missing review branch")
	}
	if !strings.Contains(err.Error(), kagerr.ErrNoReviewableChanges.Error()) {
		t.Fatalf("validatePullRefs() error = %v, want ErrNoReviewableChanges", err)
	}
}

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
