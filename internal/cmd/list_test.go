package cmd

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/pejas/kagen/internal/git"
	"github.com/pejas/kagen/internal/session"
)

func TestListCommandReadsPersistedSessionsForCurrentRepository(t *testing.T) {
	repoDir := t.TempDir()
	initGitRepo(t, repoDir)

	repo, err := git.Discover(repoDir)
	if err != nil {
		t.Fatalf("Discover() returned error: %v", err)
	}

	configHome := t.TempDir()
	t.Setenv("HOME", configHome)

	store, err := session.OpenDefault()
	if err != nil {
		t.Fatalf("OpenDefault() returned error: %v", err)
	}

	persisted, err := store.CreateKagenSession(context.Background(), session.CreateKagenSessionParams{
		RepoID:          repo.ID(),
		RepoPath:        repo.Path,
		BaseBranch:      repo.CurrentBranch,
		WorkspaceBranch: "kagen/main/s/1",
		HeadSHAAtStart:  repo.HeadSHA,
		Namespace:       "kagen-" + repo.ID(),
		PodName:         "agent",
		Status:          "ready",
		CreatedAt:       time.Date(2026, time.March, 12, 9, 0, 0, 0, time.UTC),
		LastUsedAt:      time.Date(2026, time.March, 12, 9, 30, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("CreateKagenSession() returned error: %v", err)
	}
	if _, err := store.CreateAgentSession(context.Background(), session.CreateAgentSessionParams{
		KagenSessionUID: persisted.UID,
		AgentType:       "codex",
		WorkingMode:     "shared_workspace",
		StatePath:       "/home/kagen/.codex/session-1",
		CreatedAt:       persisted.CreatedAt,
		LastUsedAt:      persisted.LastUsedAt,
	}); err != nil {
		t.Fatalf("CreateAgentSession() returned error: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("Close() returned error: %v", err)
	}

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() returned error: %v", err)
	}
	defer func() {
		if err := os.Chdir(cwd); err != nil {
			t.Fatalf("Chdir(%q) returned error: %v", cwd, err)
		}
	}()
	if err := os.Chdir(repoDir); err != nil {
		t.Fatalf("Chdir(%q) returned error: %v", repoDir, err)
	}

	output := captureStdout(t, func() {
		if err := runList(context.Background(), false); err != nil {
			t.Fatalf("runList() returned error: %v", err)
		}
	})

	if !strings.Contains(output, "Sessions") {
		t.Fatalf("output missing header: %q", output)
	}
	rowPattern := regexp.MustCompile(fmt.Sprintf(`(?m)^%d\s+ready\s+kagen/main/s/1\s+`, persisted.ID))
	if !rowPattern.MatchString(output) {
		t.Fatalf("output missing expected session row: %q", output)
	}
	if !strings.Contains(output, "kagen/main/s/1") {
		t.Fatalf("output missing workspace branch: %q", output)
	}
	if !strings.Contains(output, "codex") {
		t.Fatalf("output missing agent type: %q", output)
	}
}

func TestListCommandShowsMultipleAgentSessionsPerType(t *testing.T) {
	repoDir := t.TempDir()
	initGitRepo(t, repoDir)

	repo, err := git.Discover(repoDir)
	if err != nil {
		t.Fatalf("Discover() returned error: %v", err)
	}

	configHome := t.TempDir()
	t.Setenv("HOME", configHome)

	store, err := session.OpenDefault()
	if err != nil {
		t.Fatalf("OpenDefault() returned error: %v", err)
	}

	persisted, err := store.CreateKagenSession(context.Background(), session.CreateKagenSessionParams{
		RepoID:          repo.ID(),
		RepoPath:        repo.Path,
		BaseBranch:      repo.CurrentBranch,
		WorkspaceBranch: "kagen/main/s/2",
		HeadSHAAtStart:  repo.HeadSHA,
		Namespace:       "kagen-" + repo.ID(),
		PodName:         "agent",
		Status:          "ready",
		CreatedAt:       time.Date(2026, time.March, 12, 10, 0, 0, 0, time.UTC),
		LastUsedAt:      time.Date(2026, time.March, 12, 10, 30, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("CreateKagenSession() returned error: %v", err)
	}
	for _, agentSession := range []session.CreateAgentSessionParams{
		{
			ID:              "codex-session-1",
			KagenSessionUID: persisted.UID,
			AgentType:       "codex",
			WorkingMode:     "shared_workspace",
			StatePath:       "/home/kagen/.codex/codex-session-1",
			CreatedAt:       persisted.CreatedAt,
			LastUsedAt:      persisted.LastUsedAt,
		},
		{
			ID:              "codex-session-2",
			KagenSessionUID: persisted.UID,
			AgentType:       "codex",
			WorkingMode:     "shared_workspace",
			StatePath:       "/home/kagen/.codex/codex-session-2",
			CreatedAt:       persisted.CreatedAt.Add(5 * time.Minute),
			LastUsedAt:      persisted.LastUsedAt,
		},
		{
			ID:              "claude-session-1",
			KagenSessionUID: persisted.UID,
			AgentType:       "claude",
			WorkingMode:     "shared_workspace",
			StatePath:       "/home/kagen/.claude/claude-session-1",
			CreatedAt:       persisted.CreatedAt,
			LastUsedAt:      persisted.LastUsedAt,
		},
	} {
		if _, err := store.CreateAgentSession(context.Background(), agentSession); err != nil {
			t.Fatalf("CreateAgentSession() returned error: %v", err)
		}
	}
	if err := store.Close(); err != nil {
		t.Fatalf("Close() returned error: %v", err)
	}

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() returned error: %v", err)
	}
	defer func() {
		if err := os.Chdir(cwd); err != nil {
			t.Fatalf("Chdir(%q) returned error: %v", cwd, err)
		}
	}()
	if err := os.Chdir(repoDir); err != nil {
		t.Fatalf("Chdir(%q) returned error: %v", repoDir, err)
	}

	output := captureStdout(t, func() {
		if err := runList(context.Background(), false); err != nil {
			t.Fatalf("runList() returned error: %v", err)
		}
	})

	if !strings.Contains(output, "AGENT SESSIONS") {
		t.Fatalf("output missing agent sessions header: %q", output)
	}
	if !strings.Contains(output, "claude, codex x2") {
		t.Fatalf("output missing grouped agent session detail: %q", output)
	}
}

func TestListCommandAllShowsSessionsOutsideRepository(t *testing.T) {
	configHome := t.TempDir()
	t.Setenv("HOME", configHome)

	store, err := session.OpenDefault()
	if err != nil {
		t.Fatalf("OpenDefault() returned error: %v", err)
	}
	if _, err := store.CreateKagenSession(context.Background(), session.CreateKagenSessionParams{
		RepoID:          "repo-1",
		RepoPath:        "/tmp/repo-1",
		BaseBranch:      "main",
		WorkspaceBranch: "kagen/main/s/3",
		HeadSHAAtStart:  "abc123",
		Namespace:       "kagen-repo-1",
		PodName:         "agent",
		Status:          "stopped",
	}); err != nil {
		t.Fatalf("CreateKagenSession() returned error: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("Close() returned error: %v", err)
	}

	output := captureStdout(t, func() {
		if err := runList(context.Background(), true); err != nil {
			t.Fatalf("runList(all) returned error: %v", err)
		}
	})

	if !strings.Contains(output, "REPOSITORY") {
		t.Fatalf("output missing repository column: %q", output)
	}
	if !strings.Contains(output, "/tmp/repo-1") {
		t.Fatalf("output missing repo path: %q", output)
	}
}

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()

	original := os.Stdout
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe() returned error: %v", err)
	}
	defer func() {
		if closeErr := reader.Close(); closeErr != nil {
			t.Errorf("reader.Close() returned error: %v", closeErr)
		}
	}()

	os.Stdout = writer
	defer func() {
		os.Stdout = original
	}()

	fn()

	if err := writer.Close(); err != nil {
		t.Fatalf("writer.Close() returned error: %v", err)
	}

	var buf bytes.Buffer
	if _, err := io.Copy(&buf, reader); err != nil {
		t.Fatalf("Copy() returned error: %v", err)
	}

	return buf.String()
}
