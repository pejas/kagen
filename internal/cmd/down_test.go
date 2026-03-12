package cmd

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/pejas/kagen/internal/config"
	"github.com/pejas/kagen/internal/session"
)

func TestRunDownStopsRuntimeAndLeavesPersistedSessionsUntouched(t *testing.T) {
	storeHome := t.TempDir()
	t.Setenv("HOME", storeHome)

	store, err := session.OpenDefault()
	if err != nil {
		t.Fatalf("OpenDefault() returned error: %v", err)
	}

	createdAt := time.Date(2026, time.March, 12, 18, 0, 0, 0, time.UTC)
	persisted, err := store.CreateKagenSession(context.Background(), session.CreateKagenSessionParams{
		RepoID:          "repo-1",
		RepoPath:        t.TempDir(),
		BaseBranch:      "main",
		WorkspaceBranch: "kagen/main/s/7",
		HeadSHAAtStart:  "abc123",
		Namespace:       "kagen-repo-1",
		PodName:         "agent",
		Status:          sessionStatusReady,
		CreatedAt:       createdAt,
		LastUsedAt:      createdAt,
	})
	if err != nil {
		t.Fatalf("CreateKagenSession() returned error: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("Close() returned error: %v", err)
	}

	originalLoad := loadRunConfigForDown
	originalManagerFactory := newRuntimeManager
	t.Cleanup(func() {
		loadRunConfigForDown = originalLoad
		newRuntimeManager = originalManagerFactory
	})

	loadRunConfigForDown = func() (*config.Config, error) {
		return config.DefaultConfig(), nil
	}

	manager := &stubRuntimeManager{}
	newRuntimeManager = func(cfg config.RuntimeConfig) runtimeStopper {
		_ = cfg
		return manager
	}

	if err := runDown(context.Background()); err != nil {
		t.Fatalf("runDown() returned error: %v", err)
	}
	if manager.stopCalls != 1 {
		t.Fatalf("stop call count = %d, want 1", manager.stopCalls)
	}

	reopened, err := session.OpenDefault()
	if err != nil {
		t.Fatalf("OpenDefault(reopened) returned error: %v", err)
	}
	defer reopened.Close()

	summary, found, err := reopened.GetSummary(context.Background(), persisted.ID)
	if err != nil {
		t.Fatalf("GetSummary() returned error: %v", err)
	}
	if !found {
		t.Fatal("GetSummary() did not find persisted session")
	}
	if summary.Session.Status != sessionStatusReady {
		t.Fatalf("session status = %q, want %q", summary.Session.Status, sessionStatusReady)
	}
}

func TestRunDownReturnsRuntimeStopError(t *testing.T) {
	originalLoad := loadRunConfigForDown
	originalManagerFactory := newRuntimeManager
	t.Cleanup(func() {
		loadRunConfigForDown = originalLoad
		newRuntimeManager = originalManagerFactory
	})

	loadRunConfigForDown = func() (*config.Config, error) {
		return config.DefaultConfig(), nil
	}

	wantErr := errors.New("runtime stop failed")
	newRuntimeManager = func(cfg config.RuntimeConfig) runtimeStopper {
		_ = cfg
		return &stubRuntimeManager{stopErr: wantErr}
	}

	err := runDown(context.Background())
	if !errors.Is(err, wantErr) {
		t.Fatalf("runDown() error = %v, want %v", err, wantErr)
	}
}

func TestNewDownCommandDocumentsRuntimeShutdown(t *testing.T) {
	cmd := newDownCommand()

	if cmd.Use != "down" {
		t.Fatalf("Use = %q, want %q", cmd.Use, "down")
	}
	if cmd.Short == "" {
		t.Fatal("Short help should not be empty")
	}
	if cmd.Long == "" {
		t.Fatal("Long help should not be empty")
	}
	if !containsAll(cmd.Long, "Colima VM", "K3s cluster", "persisted kagen sessions", "/exit", "kagen down") {
		t.Fatalf("Long help = %q, want runtime/session distinction", cmd.Long)
	}
}

type stubRuntimeManager struct {
	stopCalls int
	stopErr   error
}

func (s *stubRuntimeManager) Stop(_ context.Context) error {
	s.stopCalls++
	return s.stopErr
}

func containsAll(value string, snippets ...string) bool {
	for _, snippet := range snippets {
		if !strings.Contains(value, snippet) {
			return false
		}
	}

	return true
}
