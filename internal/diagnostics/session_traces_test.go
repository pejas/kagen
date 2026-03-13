package diagnostics

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLatestOperationStoreSaveAndLoadRoundTrip(t *testing.T) {
	configDir := t.TempDir()
	store := &latestOperationStore{
		userConfigDir: func() (string, error) { return configDir, nil },
	}

	operation := Operation{
		Name:       "attach",
		Status:     StatusSucceeded,
		StartedAt:  time.Date(2026, time.March, 13, 10, 0, 0, 0, time.UTC),
		FinishedAt: time.Date(2026, time.March, 13, 10, 0, 5, 0, time.UTC),
		Duration:   5 * time.Second,
		Metadata: map[string]string{
			"session_id": "42",
			"agent_type": "codex",
		},
		Steps: []StepRecord{
			{
				Name:     "ensure_runtime",
				Status:   StatusSucceeded,
				Duration: 2 * time.Second,
			},
		},
	}

	if err := store.SaveLatestOperation(42, operation); err != nil {
		t.Fatalf("SaveLatestOperation() returned error: %v", err)
	}

	loaded, found, err := store.LoadLatestOperation(42)
	if err != nil {
		t.Fatalf("LoadLatestOperation() returned error: %v", err)
	}
	if !found {
		t.Fatal("LoadLatestOperation() did not find persisted operation")
	}
	if loaded.Name != operation.Name {
		t.Fatalf("loaded operation name = %q, want %q", loaded.Name, operation.Name)
	}
	if loaded.Metadata["agent_type"] != "codex" {
		t.Fatalf("loaded operation agent_type = %q, want codex", loaded.Metadata["agent_type"])
	}
}

func TestLatestOperationStoreReturnsMissingWhenTraceAbsent(t *testing.T) {
	configDir := t.TempDir()
	store := &latestOperationStore{
		userConfigDir: func() (string, error) { return configDir, nil },
	}

	_, found, err := store.LoadLatestOperation(7)
	if err != nil {
		t.Fatalf("LoadLatestOperation() returned error: %v", err)
	}
	if found {
		t.Fatal("LoadLatestOperation() found a trace unexpectedly")
	}
}

func TestLatestOperationStoreFindsFailureArtefactDirectory(t *testing.T) {
	configDir := t.TempDir()
	store := &latestOperationStore{
		userConfigDir: func() (string, error) { return configDir, nil },
	}

	directory := filepath.Join(configDir, "kagen", "failure-artefacts", "session-9")
	if err := os.MkdirAll(directory, 0o755); err != nil {
		t.Fatalf("MkdirAll() returned error: %v", err)
	}

	got, found, err := store.FailureArtefactDirectory(9)
	if err != nil {
		t.Fatalf("FailureArtefactDirectory() returned error: %v", err)
	}
	if !found {
		t.Fatal("FailureArtefactDirectory() did not find created directory")
	}
	if got != directory {
		t.Fatalf("FailureArtefactDirectory() = %q, want %q", got, directory)
	}
}

func TestLatestOperationReporterPersistsSessionTrace(t *testing.T) {
	configDir := t.TempDir()
	store := &latestOperationStore{
		userConfigDir: func() (string, error) { return configDir, nil },
	}

	reporter := NewLatestOperationReporter(store)
	reporter.OperationFinished(Operation{
		Name:   "start",
		Status: StatusFailed,
		Metadata: map[string]string{
			"session_id": "15",
		},
	})

	if _, found, err := store.LoadLatestOperation(15); err != nil {
		t.Fatalf("LoadLatestOperation() returned error: %v", err)
	} else if !found {
		t.Fatal("latest operation reporter did not persist the operation")
	}
}
