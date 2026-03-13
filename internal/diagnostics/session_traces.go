package diagnostics

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/pejas/kagen/internal/ui"
)

const latestOperationFilename = "latest-operation.json"

type compositeReporter struct {
	reporters []Reporter
}

type latestOperationStore struct {
	userConfigDir func() (string, error)
}

type latestOperationReporter struct {
	store *latestOperationStore
}

// NewCompositeReporter fans out diagnostics events to each reporter.
func NewCompositeReporter(reporters ...Reporter) Reporter {
	filtered := make([]Reporter, 0, len(reporters))
	for _, reporter := range reporters {
		if reporter == nil {
			continue
		}
		filtered = append(filtered, reporter)
	}
	if len(filtered) == 0 {
		return nil
	}
	if len(filtered) == 1 {
		return filtered[0]
	}

	return compositeReporter{reporters: filtered}
}

func (r compositeReporter) StepStarted(operation Operation, step StepRecord) {
	for _, reporter := range r.reporters {
		reporter.StepStarted(operation, step)
	}
}

func (r compositeReporter) OperationFinished(operation Operation) {
	for _, reporter := range r.reporters {
		reporter.OperationFinished(operation)
	}
}

// NewLatestOperationStore returns the default store for persisted session traces.
func NewLatestOperationStore() *latestOperationStore {
	return &latestOperationStore{
		userConfigDir: os.UserConfigDir,
	}
}

// NewLatestOperationReporter persists the latest completed operation for each session.
func NewLatestOperationReporter(store *latestOperationStore) Reporter {
	if store == nil {
		store = NewLatestOperationStore()
	}

	return latestOperationReporter{store: store}
}

func (r latestOperationReporter) StepStarted(Operation, StepRecord) {}

func (r latestOperationReporter) OperationFinished(operation Operation) {
	if r.store == nil {
		return
	}

	sessionID, ok := operationSessionID(operation)
	if !ok {
		return
	}
	if err := r.store.SaveLatestOperation(sessionID, operation); err != nil {
		ui.Verbose("Failed to persist latest operation trace for session %d: %v", sessionID, err)
	}
}

// SaveLatestOperation writes the latest completed operation trace for the session.
func (s *latestOperationStore) SaveLatestOperation(sessionID int64, operation Operation) error {
	if sessionID <= 0 {
		return fmt.Errorf("session ID must be greater than zero")
	}

	path, err := s.latestOperationPath(sessionID)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("creating latest operation directory: %w", err)
	}

	content, err := json.MarshalIndent(operation, "", "  ")
	if err != nil {
		return fmt.Errorf("marshalling latest operation: %w", err)
	}
	content = append(content, '\n')
	if err := os.WriteFile(path, content, 0o644); err != nil {
		return fmt.Errorf("writing latest operation trace: %w", err)
	}

	return nil
}

// LoadLatestOperation returns the latest completed operation trace for the session.
func (s *latestOperationStore) LoadLatestOperation(sessionID int64) (Operation, bool, error) {
	if sessionID <= 0 {
		return Operation{}, false, fmt.Errorf("session ID must be greater than zero")
	}

	path, err := s.latestOperationPath(sessionID)
	if err != nil {
		return Operation{}, false, err
	}

	content, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return Operation{}, false, nil
		}
		return Operation{}, false, fmt.Errorf("reading latest operation trace: %w", err)
	}

	var operation Operation
	if err := json.Unmarshal(content, &operation); err != nil {
		return Operation{}, false, fmt.Errorf("unmarshalling latest operation trace: %w", err)
	}

	return operation, true, nil
}

// FailureArtefactDirectory returns the deterministic failure artefact directory when it exists.
func (s *latestOperationStore) FailureArtefactDirectory(sessionID int64) (string, bool, error) {
	if sessionID <= 0 {
		return "", false, fmt.Errorf("session ID must be greater than zero")
	}

	baseDir, err := s.failureArtefactBaseDir()
	if err != nil {
		return "", false, err
	}

	directory := filepath.Join(baseDir, fmt.Sprintf("session-%d", sessionID))
	info, err := os.Stat(directory)
	if err != nil {
		if os.IsNotExist(err) {
			return "", false, nil
		}
		return "", false, fmt.Errorf("stating failure artefact directory: %w", err)
	}
	if !info.IsDir() {
		return "", false, nil
	}

	return directory, true, nil
}

func (s *latestOperationStore) latestOperationPath(sessionID int64) (string, error) {
	baseDir, err := s.baseDir()
	if err != nil {
		return "", err
	}

	return filepath.Join(baseDir, fmt.Sprintf("session-%d", sessionID), latestOperationFilename), nil
}

func (s *latestOperationStore) baseDir() (string, error) {
	configDir, err := s.userConfigDir()
	if err != nil {
		return "", fmt.Errorf("finding user config directory: %w", err)
	}

	return filepath.Join(configDir, "kagen", "session-diagnostics"), nil
}

func (s *latestOperationStore) failureArtefactBaseDir() (string, error) {
	configDir, err := s.userConfigDir()
	if err != nil {
		return "", fmt.Errorf("finding user config directory: %w", err)
	}

	return filepath.Join(configDir, "kagen", "failure-artefacts"), nil
}

func operationSessionID(operation Operation) (int64, bool) {
	raw := strings.TrimSpace(operation.Metadata["session_id"])
	if raw == "" {
		return 0, false
	}

	sessionID, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || sessionID <= 0 {
		return 0, false
	}

	return sessionID, true
}
