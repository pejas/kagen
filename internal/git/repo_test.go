package git

import (
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
