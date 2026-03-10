// Package provenance records and persists import provenance metadata
// for repository imports into the cluster Forgejo instance.
package provenance

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/pejas/kagen/internal/git"
)

// Record captures the provenance of a repository import.
type Record struct {
	// RepoPath is the absolute path to the source repository on the host.
	RepoPath string `json:"repo_path"`

	// SourceBranch is the branch that was active at import time.
	SourceBranch string `json:"source_branch"`

	// SourceCommitSHA is the HEAD SHA at import time.
	SourceCommitSHA string `json:"source_commit_sha"`

	// ImportedAt is the UTC timestamp of the import.
	ImportedAt time.Time `json:"imported_at"`
}

// RecordImport creates a new provenance Record from the given Repository.
func RecordImport(repo *git.Repository) *Record {
	return &Record{
		RepoPath:        repo.Path,
		SourceBranch:    repo.CurrentBranch,
		SourceCommitSHA: repo.HeadSHA,
		ImportedAt:      time.Now().UTC(),
	}
}

// Save serializes the Record to a JSON file at the given path.
func Save(record *Record, path string) error {
	data, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return fmt.Errorf("marshalling provenance: %w", err)
	}

	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("writing provenance file: %w", err)
	}

	return nil
}

// Load deserializes a Record from a JSON file at the given path.
func Load(path string) (*Record, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading provenance file: %w", err)
	}

	var record Record
	if err := json.Unmarshal(data, &record); err != nil {
		return nil, fmt.Errorf("unmarshalling provenance: %w", err)
	}

	return &record, nil
}
