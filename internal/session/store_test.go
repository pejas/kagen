package session

import (
	"context"
	"database/sql"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func TestOpenInitialisesSchemaAndPersistsSessions(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "sessions.db")

	store, err := Open(path)
	if err != nil {
		t.Fatalf("Open() returned error: %v", err)
	}

	first, err := store.CreateKagenSession(ctx, CreateKagenSessionParams{
		RepoID:          "repo-1",
		RepoPath:        "/tmp/repo-1",
		BaseBranch:      "main",
		WorkspaceBranch: "kagen/main/s/1",
		HeadSHAAtStart:  "abc123",
		Namespace:       "kagen-repo-1",
		PodName:         "agent",
		Status:          "ready",
		CreatedAt:       time.Date(2026, time.March, 12, 10, 0, 0, 0, time.UTC),
		LastUsedAt:      time.Date(2026, time.March, 12, 11, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("CreateKagenSession(first) returned error: %v", err)
	}
	if first.ID != 1 {
		t.Fatalf("first session ID = %d, want 1", first.ID)
	}

	second, err := store.CreateKagenSession(ctx, CreateKagenSessionParams{
		RepoID:          "repo-2",
		RepoPath:        "/tmp/repo-2",
		BaseBranch:      "main",
		WorkspaceBranch: "kagen/main/s/2",
		HeadSHAAtStart:  "def456",
		Namespace:       "kagen-repo-2",
		PodName:         "agent",
		Status:          "starting",
		CreatedAt:       time.Date(2026, time.March, 12, 12, 0, 0, 0, time.UTC),
		LastUsedAt:      time.Date(2026, time.March, 12, 13, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("CreateKagenSession(second) returned error: %v", err)
	}
	if second.ID != 2 {
		t.Fatalf("second session ID = %d, want 2", second.ID)
	}

	if _, err := store.CreateAgentSession(ctx, CreateAgentSessionParams{
		KagenSessionUID: second.UID,
		AgentType:       "codex",
		WorkingMode:     "shared_workspace",
		StatePath:       "/home/kagen/.codex/session-a",
		CreatedAt:       second.CreatedAt,
		LastUsedAt:      second.LastUsedAt,
	}); err != nil {
		t.Fatalf("CreateAgentSession(codex) returned error: %v", err)
	}
	if _, err := store.CreateAgentSession(ctx, CreateAgentSessionParams{
		KagenSessionUID: second.UID,
		AgentType:       "claude",
		WorkingMode:     "shared_workspace",
		StatePath:       "/home/kagen/.claude/session-b",
		CreatedAt:       second.CreatedAt,
		LastUsedAt:      second.LastUsedAt,
	}); err != nil {
		t.Fatalf("CreateAgentSession(claude) returned error: %v", err)
	}

	if err := store.Close(); err != nil {
		t.Fatalf("Close() returned error: %v", err)
	}

	reopened, err := Open(path)
	if err != nil {
		t.Fatalf("Open(reopened) returned error: %v", err)
	}
	defer func() {
		if closeErr := reopened.Close(); closeErr != nil {
			t.Errorf("reopened.Close() returned error: %v", closeErr)
		}
	}()

	got, err := reopened.List(ctx, ListOptions{})
	if err != nil {
		t.Fatalf("List() returned error: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len(List()) = %d, want 2", len(got))
	}

	if got[0].Session.ID != second.ID {
		t.Fatalf("first listed session ID = %d, want %d", got[0].Session.ID, second.ID)
	}
	if !reflect.DeepEqual(got[0].AgentTypes, []string{"claude", "codex"}) {
		t.Fatalf("agent types = %v, want [claude codex]", got[0].AgentTypes)
	}
	if len(got[0].AgentSessions) != 2 {
		t.Fatalf("len(agent sessions) = %d, want 2", len(got[0].AgentSessions))
	}
	if got[1].Session.ID != first.ID {
		t.Fatalf("second listed session ID = %d, want %d", got[1].Session.ID, first.ID)
	}
}

func TestOpenRejectsFutureSchemaVersions(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "sessions.db")

	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("sql.Open() returned error: %v", err)
	}
	if _, err := db.Exec(`PRAGMA user_version = 99`); err != nil {
		t.Fatalf("setting user_version returned error: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("db.Close() returned error: %v", err)
	}

	if _, err := Open(path); err == nil {
		t.Fatal("Open() expected error for future schema version")
	}
}

func TestListFiltersByRepositoryPath(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "sessions.db")

	store, err := Open(path)
	if err != nil {
		t.Fatalf("Open() returned error: %v", err)
	}
	defer func() {
		if closeErr := store.Close(); closeErr != nil {
			t.Errorf("store.Close() returned error: %v", closeErr)
		}
	}()

	if _, err := store.CreateKagenSession(ctx, CreateKagenSessionParams{
		RepoID:          "repo-1",
		RepoPath:        "/tmp/repo-1",
		BaseBranch:      "main",
		WorkspaceBranch: "kagen/main/s/1",
		HeadSHAAtStart:  "abc123",
		Namespace:       "kagen-repo-1",
		PodName:         "agent",
		Status:          "ready",
	}); err != nil {
		t.Fatalf("CreateKagenSession(repo-1) returned error: %v", err)
	}
	target, err := store.CreateKagenSession(ctx, CreateKagenSessionParams{
		RepoID:          "repo-2",
		RepoPath:        "/tmp/repo-2",
		BaseBranch:      "main",
		WorkspaceBranch: "kagen/main/s/2",
		HeadSHAAtStart:  "def456",
		Namespace:       "kagen-repo-2",
		PodName:         "agent",
		Status:          "ready",
	})
	if err != nil {
		t.Fatalf("CreateKagenSession(repo-2) returned error: %v", err)
	}

	got, err := store.List(ctx, ListOptions{RepoPath: "/tmp/repo-2"})
	if err != nil {
		t.Fatalf("List(filtered) returned error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len(List(filtered)) = %d, want 1", len(got))
	}
	if got[0].Session.ID != target.ID {
		t.Fatalf("filtered session ID = %d, want %d", got[0].Session.ID, target.ID)
	}
}

func TestFindMostRecentReadyFiltersByRepository(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "sessions.db")

	store, err := Open(path)
	if err != nil {
		t.Fatalf("Open() returned error: %v", err)
	}
	defer func() {
		if closeErr := store.Close(); closeErr != nil {
			t.Errorf("store.Close() returned error: %v", closeErr)
		}
	}()

	older, err := store.CreateKagenSession(ctx, CreateKagenSessionParams{
		RepoID:          "repo-1",
		RepoPath:        "/tmp/repo-1",
		BaseBranch:      "main",
		WorkspaceBranch: "kagen/main",
		HeadSHAAtStart:  "abc123",
		Namespace:       "kagen-repo-1",
		PodName:         "agent",
		Status:          "ready",
		LastUsedAt:      time.Date(2026, time.March, 12, 9, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("CreateKagenSession(older) returned error: %v", err)
	}
	if _, err := store.CreateAgentSession(ctx, CreateAgentSessionParams{
		KagenSessionUID: older.UID,
		AgentType:       "codex",
		WorkingMode:     "shared_workspace",
		StatePath:       "/home/kagen/.codex/older",
		LastUsedAt:      older.LastUsedAt,
	}); err != nil {
		t.Fatalf("CreateAgentSession(older) returned error: %v", err)
	}

	newer, err := store.CreateKagenSession(ctx, CreateKagenSessionParams{
		RepoID:          "repo-1",
		RepoPath:        "/tmp/repo-1",
		BaseBranch:      "main",
		WorkspaceBranch: "kagen/main",
		HeadSHAAtStart:  "def456",
		Namespace:       "kagen-repo-1",
		PodName:         "agent",
		Status:          "ready",
		LastUsedAt:      time.Date(2026, time.March, 12, 10, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("CreateKagenSession(newer) returned error: %v", err)
	}
	if _, err := store.CreateAgentSession(ctx, CreateAgentSessionParams{
		KagenSessionUID: newer.UID,
		AgentType:       "claude",
		WorkingMode:     "shared_workspace",
		StatePath:       "/home/kagen/.codex/newer",
		LastUsedAt:      newer.LastUsedAt,
	}); err != nil {
		t.Fatalf("CreateAgentSession(newer) returned error: %v", err)
	}

	otherAgent, err := store.CreateKagenSession(ctx, CreateKagenSessionParams{
		RepoID:          "repo-1",
		RepoPath:        "/tmp/repo-1",
		BaseBranch:      "main",
		WorkspaceBranch: "kagen/main",
		HeadSHAAtStart:  "ghi789",
		Namespace:       "kagen-repo-1",
		PodName:         "agent",
		Status:          "ready",
		LastUsedAt:      time.Date(2026, time.March, 12, 11, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("CreateKagenSession(otherAgent) returned error: %v", err)
	}
	if _, err := store.CreateAgentSession(ctx, CreateAgentSessionParams{
		KagenSessionUID: otherAgent.UID,
		AgentType:       "claude",
		WorkingMode:     "shared_workspace",
		StatePath:       "/home/kagen/.claude/other",
		LastUsedAt:      otherAgent.LastUsedAt,
	}); err != nil {
		t.Fatalf("CreateAgentSession(otherAgent) returned error: %v", err)
	}

	notReady, err := store.CreateKagenSession(ctx, CreateKagenSessionParams{
		RepoID:          "repo-1",
		RepoPath:        "/tmp/repo-1",
		BaseBranch:      "main",
		WorkspaceBranch: "kagen/main",
		HeadSHAAtStart:  "jkl012",
		Namespace:       "kagen-repo-1",
		PodName:         "agent",
		Status:          "starting",
		LastUsedAt:      time.Date(2026, time.March, 12, 12, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("CreateKagenSession(notReady) returned error: %v", err)
	}
	if _, err := store.CreateAgentSession(ctx, CreateAgentSessionParams{
		KagenSessionUID: notReady.UID,
		AgentType:       "codex",
		WorkingMode:     "shared_workspace",
		StatePath:       "/home/kagen/.codex/not-ready",
		LastUsedAt:      notReady.LastUsedAt,
	}); err != nil {
		t.Fatalf("CreateAgentSession(notReady) returned error: %v", err)
	}

	summary, found, err := store.FindMostRecentReady(ctx, "/tmp/repo-1")
	if err != nil {
		t.Fatalf("FindMostRecentReady() returned error: %v", err)
	}
	if !found {
		t.Fatal("FindMostRecentReady() did not find a ready session")
	}
	if summary.Session.ID != otherAgent.ID {
		t.Fatalf("FindMostRecentReady() session ID = %d, want %d", summary.Session.ID, otherAgent.ID)
	}
}

func TestRecordAttachUpdatesLastUsedAtForSessionAndAgent(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "sessions.db")

	store, err := Open(path)
	if err != nil {
		t.Fatalf("Open() returned error: %v", err)
	}
	defer func() {
		if closeErr := store.Close(); closeErr != nil {
			t.Errorf("store.Close() returned error: %v", closeErr)
		}
	}()

	persisted, err := store.CreateKagenSession(ctx, CreateKagenSessionParams{
		RepoID:          "repo-1",
		RepoPath:        "/tmp/repo-1",
		BaseBranch:      "main",
		WorkspaceBranch: "kagen/main",
		HeadSHAAtStart:  "abc123",
		Namespace:       "kagen-repo-1",
		PodName:         "agent",
		Status:          "ready",
		CreatedAt:       time.Date(2026, time.March, 12, 9, 0, 0, 0, time.UTC),
		LastUsedAt:      time.Date(2026, time.March, 12, 9, 30, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("CreateKagenSession() returned error: %v", err)
	}
	agentSession, err := store.CreateAgentSession(ctx, CreateAgentSessionParams{
		KagenSessionUID: persisted.UID,
		AgentType:       "codex",
		WorkingMode:     "shared_workspace",
		StatePath:       "/home/kagen/.codex/session-1",
		CreatedAt:       persisted.CreatedAt,
		LastUsedAt:      persisted.LastUsedAt,
	})
	if err != nil {
		t.Fatalf("CreateAgentSession() returned error: %v", err)
	}

	attachedAt := time.Date(2026, time.March, 12, 14, 15, 0, 0, time.UTC)
	if err := store.RecordAttach(ctx, persisted.ID, agentSession.ID, attachedAt); err != nil {
		t.Fatalf("RecordAttach() returned error: %v", err)
	}

	summary, found, err := store.GetSummary(ctx, persisted.ID)
	if err != nil {
		t.Fatalf("GetSummary() returned error: %v", err)
	}
	if !found {
		t.Fatal("GetSummary() did not find persisted session")
	}
	if !summary.Session.LastUsedAt.Equal(attachedAt) {
		t.Fatalf("session last_used_at = %s, want %s", summary.Session.LastUsedAt, attachedAt)
	}

	row := store.db.QueryRowContext(ctx, `SELECT last_used_at FROM agent_sessions WHERE id = ?`, agentSession.ID)

	var got string
	if err := row.Scan(&got); err != nil {
		t.Fatalf("Scan(agent_sessions.last_used_at) returned error: %v", err)
	}

	parsed, err := parseTimestamp(got)
	if err != nil {
		t.Fatalf("parseTimestamp() returned error: %v", err)
	}
	if !parsed.Equal(attachedAt) {
		t.Fatalf("agent session last_used_at = %s, want %s", parsed, attachedAt)
	}
}

func TestRecordAttachUpdatesOnlySelectedAgentSession(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "sessions.db")

	store, err := Open(path)
	if err != nil {
		t.Fatalf("Open() returned error: %v", err)
	}
	defer func() {
		if closeErr := store.Close(); closeErr != nil {
			t.Errorf("store.Close() returned error: %v", closeErr)
		}
	}()

	persisted, err := store.CreateKagenSession(ctx, CreateKagenSessionParams{
		RepoID:          "repo-1",
		RepoPath:        "/tmp/repo-1",
		BaseBranch:      "main",
		WorkspaceBranch: "kagen/main",
		HeadSHAAtStart:  "abc123",
		Namespace:       "kagen-repo-1",
		PodName:         "agent",
		Status:          "ready",
		CreatedAt:       time.Date(2026, time.March, 12, 9, 0, 0, 0, time.UTC),
		LastUsedAt:      time.Date(2026, time.March, 12, 9, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("CreateKagenSession() returned error: %v", err)
	}

	firstAgentSession, err := store.CreateAgentSession(ctx, CreateAgentSessionParams{
		ID:              "agent-session-1",
		KagenSessionUID: persisted.UID,
		AgentType:       "codex",
		WorkingMode:     "shared_workspace",
		StatePath:       "/home/kagen/.codex/agent-session-1",
		CreatedAt:       time.Date(2026, time.March, 12, 9, 0, 0, 0, time.UTC),
		LastUsedAt:      time.Date(2026, time.March, 12, 9, 5, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("CreateAgentSession(first) returned error: %v", err)
	}
	secondAgentSession, err := store.CreateAgentSession(ctx, CreateAgentSessionParams{
		ID:              "agent-session-2",
		KagenSessionUID: persisted.UID,
		AgentType:       "codex",
		WorkingMode:     "shared_workspace",
		StatePath:       "/home/kagen/.codex/agent-session-2",
		CreatedAt:       time.Date(2026, time.March, 12, 9, 10, 0, 0, time.UTC),
		LastUsedAt:      time.Date(2026, time.March, 12, 9, 15, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("CreateAgentSession(second) returned error: %v", err)
	}

	attachedAt := time.Date(2026, time.March, 12, 10, 0, 0, 0, time.UTC)
	if err := store.RecordAttach(ctx, persisted.ID, secondAgentSession.ID, attachedAt); err != nil {
		t.Fatalf("RecordAttach() returned error: %v", err)
	}

	summary, found, err := store.GetSummary(ctx, persisted.ID)
	if err != nil {
		t.Fatalf("GetSummary() returned error: %v", err)
	}
	if !found {
		t.Fatal("GetSummary() did not find persisted session")
	}
	if len(summary.AgentSessions) != 2 {
		t.Fatalf("len(agent sessions) = %d, want 2", len(summary.AgentSessions))
	}

	lastUsedByID := map[string]time.Time{}
	for _, agentSession := range summary.AgentSessions {
		lastUsedByID[agentSession.ID] = agentSession.LastUsedAt
	}
	if !lastUsedByID[secondAgentSession.ID].Equal(attachedAt) {
		t.Fatalf("second agent session last_used_at = %s, want %s", lastUsedByID[secondAgentSession.ID], attachedAt)
	}
	if !lastUsedByID[firstAgentSession.ID].Equal(firstAgentSession.LastUsedAt) {
		t.Fatalf("first agent session last_used_at = %s, want unchanged %s", lastUsedByID[firstAgentSession.ID], firstAgentSession.LastUsedAt)
	}
}
