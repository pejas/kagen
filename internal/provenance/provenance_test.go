package provenance

import (
	"path/filepath"
	"testing"

	"github.com/pejas/kagen/internal/git"
)

func TestRecordImportCapturesFields(t *testing.T) {
	t.Parallel()

	repo := &git.Repository{
		Path:          "/fake/repo",
		CurrentBranch: "main",
		HeadSHA:       "abc123def456",
	}

	rec := RecordImport(repo)

	if rec.RepoPath != repo.Path {
		t.Errorf("expected RepoPath=%q, got %q", repo.Path, rec.RepoPath)
	}
	if rec.SourceBranch != repo.CurrentBranch {
		t.Errorf("expected SourceBranch=%q, got %q", repo.CurrentBranch, rec.SourceBranch)
	}
	if rec.SourceCommitSHA != repo.HeadSHA {
		t.Errorf("expected SourceCommitSHA=%q, got %q", repo.HeadSHA, rec.SourceCommitSHA)
	}
	if rec.ImportedAt.IsZero() {
		t.Error("expected non-zero ImportedAt")
	}
}

func TestSaveAndLoadRoundTrip(t *testing.T) {
	t.Parallel()

	repo := &git.Repository{
		Path:          "/fake/repo",
		CurrentBranch: "feature/test",
		HeadSHA:       "deadbeef",
	}

	rec := RecordImport(repo)
	path := filepath.Join(t.TempDir(), "provenance.json")

	if err := Save(rec, path); err != nil {
		t.Fatalf("Save() returned error: %v", err)
	}

	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("Load() returned error: %v", err)
	}

	if loaded.RepoPath != rec.RepoPath {
		t.Errorf("round-trip RepoPath: want %q, got %q", rec.RepoPath, loaded.RepoPath)
	}
	if loaded.SourceBranch != rec.SourceBranch {
		t.Errorf("round-trip SourceBranch: want %q, got %q", rec.SourceBranch, loaded.SourceBranch)
	}
	if loaded.SourceCommitSHA != rec.SourceCommitSHA {
		t.Errorf("round-trip SourceCommitSHA: want %q, got %q", rec.SourceCommitSHA, loaded.SourceCommitSHA)
	}
	if !loaded.ImportedAt.Equal(rec.ImportedAt) {
		t.Errorf("round-trip ImportedAt: want %v, got %v", rec.ImportedAt, loaded.ImportedAt)
	}
}

func TestLoadNonExistentFile(t *testing.T) {
	t.Parallel()

	_, err := Load("/nonexistent/path/provenance.json")
	if err == nil {
		t.Error("expected error for nonexistent file, got nil")
	}
}
