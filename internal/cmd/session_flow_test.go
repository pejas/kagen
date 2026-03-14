package cmd

import (
	"context"
	"errors"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"

	"github.com/pejas/kagen/internal/agent"
	"github.com/pejas/kagen/internal/config"
	"github.com/pejas/kagen/internal/diagnostics"
	kagerr "github.com/pejas/kagen/internal/errors"
	"github.com/pejas/kagen/internal/forgejo"
	"github.com/pejas/kagen/internal/git"
	"github.com/pejas/kagen/internal/preflight"
	"github.com/pejas/kagen/internal/session"
)

func TestRunStartPersistsReadySessionAndAgent(t *testing.T) {
	storeHome := t.TempDir()
	t.Setenv("HOME", storeHome)

	repo := &git.Repository{
		Path:          t.TempDir(),
		CurrentBranch: "main",
		HeadSHA:       "abc123",
	}

	attachedAt := time.Date(2026, time.March, 12, 15, 0, 0, 0, time.UTC)
	calls := stubSessionFlow(t, sessionFlowStubOptions{
		repo: repo,
		cfg:  config.DefaultConfig(),
		now:  attachedAt,
	})

	if err := runStart(context.Background(), "codex", false); err != nil {
		t.Fatalf("runStart() returned error: %v", err)
	}

	if calls.launches != 1 {
		t.Fatalf("launch count = %d, want 1", calls.launches)
	}
	if calls.attaches != 1 {
		t.Fatalf("attach count = %d, want 1", calls.attaches)
	}

	store, err := session.OpenDefault()
	if err != nil {
		t.Fatalf("OpenDefault() returned error: %v", err)
	}
	defer func() {
		if closeErr := store.Close(); closeErr != nil {
			t.Errorf("store.Close() returned error: %v", closeErr)
		}
	}()

	summaries, err := store.List(context.Background(), session.ListOptions{RepoPath: repo.Path})
	if err != nil {
		t.Fatalf("List() returned error: %v", err)
	}
	if len(summaries) != 1 {
		t.Fatalf("len(List()) = %d, want 1", len(summaries))
	}

	summary := summaries[0]
	if summary.Session.Status != sessionStatusReady {
		t.Fatalf("session status = %q, want %q", summary.Session.Status, sessionStatusReady)
	}
	if summary.Session.WorkspaceBranch != repo.KagenBranch() {
		t.Fatalf("workspace branch = %q, want %q", summary.Session.WorkspaceBranch, repo.KagenBranch())
	}
	if !summary.Session.LastUsedAt.Equal(attachedAt) {
		t.Fatalf("last_used_at = %s, want %s", summary.Session.LastUsedAt, attachedAt)
	}
	if len(summary.AgentTypes) != 1 || summary.AgentTypes[0] != string(agent.Codex) {
		t.Fatalf("agent types = %v, want [codex]", summary.AgentTypes)
	}
	if len(summary.AgentSessions) != 1 {
		t.Fatalf("len(agent sessions) = %d, want 1", len(summary.AgentSessions))
	}
	if got := summary.AgentSessions[0].StatePath; !strings.HasPrefix(got, "/home/kagen/.codex/") {
		t.Fatalf("agent session state path = %q, want codex-scoped session path", got)
	}
}

func TestRunStartRecordsSuccessfulRuntimeTrace(t *testing.T) {
	storeHome := t.TempDir()
	t.Setenv("HOME", storeHome)

	repo := &git.Repository{
		Path:          t.TempDir(),
		CurrentBranch: "main",
		HeadSHA:       "abc123",
	}

	calls := stubSessionFlow(t, sessionFlowStubOptions{
		repo: repo,
		cfg:  config.DefaultConfig(),
		now:  time.Date(2026, time.March, 12, 15, 30, 0, 0, time.UTC),
	})

	if err := runStart(context.Background(), "codex", false); err != nil {
		t.Fatalf("runStart() returned error: %v", err)
	}

	if len(calls.operations) != 1 {
		t.Fatalf("operation count = %d, want 1", len(calls.operations))
	}

	operation := calls.operations[0]
	if operation.Name != "start" {
		t.Fatalf("operation name = %q, want start", operation.Name)
	}
	if operation.Status != diagnostics.StatusSucceeded {
		t.Fatalf("operation status = %q, want succeeded", operation.Status)
	}
	expectedSteps := []string{
		"ensure_runtime",
		"preflight_configuration",
		"ensure_namespace",
		"ensure_proxy",
		"ensure_resources",
		"forgejo_import",
		"launch_agent_runtime",
		"preflight_runtime",
		"validate_proxy_policy",
		"prepare_agent_state",
		"attach_agent",
	}
	if len(operation.Steps) != len(expectedSteps) {
		t.Fatalf("step count = %d, want %d", len(operation.Steps), len(expectedSteps))
	}
	for i, expectedStep := range expectedSteps {
		if operation.Steps[i].Name != expectedStep {
			t.Fatalf("step %d name = %q, want %q", i, operation.Steps[i].Name, expectedStep)
		}
		if operation.Steps[i].Status != diagnostics.StatusSucceeded {
			t.Fatalf("step %s status = %q, want succeeded", operation.Steps[i].Name, operation.Steps[i].Status)
		}
	}
	if operation.Metadata["agent_type"] != string(agent.Codex) {
		t.Fatalf("operation metadata agent_type = %q, want codex", operation.Metadata["agent_type"])
	}
	if operation.Metadata["session_id"] == "" {
		t.Fatal("operation metadata session_id is empty")
	}
	if operation.Metadata["namespace"] != "kagen-"+repo.ID() {
		t.Fatalf("operation metadata namespace = %q, want %q", operation.Metadata["namespace"], "kagen-"+repo.ID())
	}
	prepareStep := operation.Steps[9]
	if prepareStep.Metadata["agent_session_id"] == "" {
		t.Fatal("prepare_agent_state metadata agent_session_id is empty")
	}
	if !strings.HasPrefix(prepareStep.Metadata["state_path"], "/home/kagen/.codex/") {
		t.Fatalf("prepare_agent_state state_path = %q, want codex session path", prepareStep.Metadata["state_path"])
	}
}

func TestStartCommandDetachPersistsReadySessionWithoutAttach(t *testing.T) {
	storeHome := t.TempDir()
	t.Setenv("HOME", storeHome)

	repo := &git.Repository{
		Path:          t.TempDir(),
		CurrentBranch: "main",
		HeadSHA:       "abc123",
	}

	readyAt := time.Date(2026, time.March, 12, 15, 45, 0, 0, time.UTC)
	calls := stubSessionFlow(t, sessionFlowStubOptions{
		repo: repo,
		cfg:  config.DefaultConfig(),
		now:  readyAt,
	})

	cmd := newStartCommand()
	cmd.SetArgs([]string{"--detach", "codex"})
	if err := cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("start --detach returned error: %v", err)
	}

	if calls.launches != 1 {
		t.Fatalf("launch count = %d, want 1", calls.launches)
	}
	if calls.prepares != 1 {
		t.Fatalf("prepare count = %d, want 1", calls.prepares)
	}
	if calls.attaches != 0 {
		t.Fatalf("attach count = %d, want 0", calls.attaches)
	}
	if len(calls.operations) != 1 {
		t.Fatalf("operation count = %d, want 1", len(calls.operations))
	}

	operation := calls.operations[0]
	if operation.Metadata["start_mode"] != "detached" {
		t.Fatalf("operation start_mode = %q, want detached", operation.Metadata["start_mode"])
	}
	for _, step := range operation.Steps {
		if step.Name == "attach_agent" {
			t.Fatal("detached start recorded attach_agent step, want no attach boundary step")
		}
	}

	store, err := session.OpenDefault()
	if err != nil {
		t.Fatalf("OpenDefault() returned error: %v", err)
	}
	defer func() {
		if closeErr := store.Close(); closeErr != nil {
			t.Errorf("store.Close() returned error: %v", closeErr)
		}
	}()

	summaries, err := store.List(context.Background(), session.ListOptions{RepoPath: repo.Path})
	if err != nil {
		t.Fatalf("List() returned error: %v", err)
	}
	if len(summaries) != 1 {
		t.Fatalf("len(List()) = %d, want 1", len(summaries))
	}

	summary := summaries[0]
	if summary.Session.Status != sessionStatusReady {
		t.Fatalf("session status = %q, want %q", summary.Session.Status, sessionStatusReady)
	}
	if len(summary.AgentSessions) != 1 {
		t.Fatalf("len(agent sessions) = %d, want 1", len(summary.AgentSessions))
	}
	if !summary.Session.LastUsedAt.Equal(readyAt) {
		t.Fatalf("last_used_at = %s, want %s", summary.Session.LastUsedAt, readyAt)
	}
	if got := summary.AgentSessions[0].StatePath; !strings.HasPrefix(got, "/home/kagen/.codex/") {
		t.Fatalf("agent session state path = %q, want codex session path", got)
	}
}

func TestRunAttachWithSessionIDUpdatesLastUsedAt(t *testing.T) {
	storeHome := t.TempDir()
	t.Setenv("HOME", storeHome)

	store, err := session.OpenDefault()
	if err != nil {
		t.Fatalf("OpenDefault() returned error: %v", err)
	}

	persisted, err := store.CreateKagenSession(context.Background(), session.CreateKagenSessionParams{
		RepoID:          "repo-1",
		RepoPath:        t.TempDir(),
		BaseBranch:      "main",
		WorkspaceBranch: "kagen/main",
		HeadSHAAtStart:  "abc123",
		Namespace:       "kagen-repo-1",
		PodName:         "agent",
		Status:          sessionStatusReady,
		CreatedAt:       time.Date(2026, time.March, 12, 10, 0, 0, 0, time.UTC),
		LastUsedAt:      time.Date(2026, time.March, 12, 10, 30, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("CreateKagenSession() returned error: %v", err)
	}
	if _, err := store.CreateAgentSession(context.Background(), session.CreateAgentSessionParams{
		KagenSessionUID: persisted.UID,
		AgentType:       string(agent.Codex),
		WorkingMode:     "shared_workspace",
		StatePath:       "/home/kagen/.codex",
		CreatedAt:       persisted.CreatedAt,
		LastUsedAt:      persisted.LastUsedAt,
	}); err != nil {
		t.Fatalf("CreateAgentSession() returned error: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("Close() returned error: %v", err)
	}

	attachedAt := time.Date(2026, time.March, 12, 16, 0, 0, 0, time.UTC)
	calls := stubSessionFlow(t, sessionFlowStubOptions{
		cfg: config.DefaultConfig(),
		now: attachedAt,
	})

	if err := runAttach(context.Background(), "codex", persisted.ID, true); err != nil {
		t.Fatalf("runAttach() returned error: %v", err)
	}

	if calls.launches != 1 {
		t.Fatalf("launch count = %d, want 1", calls.launches)
	}
	if calls.attaches != 1 {
		t.Fatalf("attach count = %d, want 1", calls.attaches)
	}

	reopened, err := session.OpenDefault()
	if err != nil {
		t.Fatalf("OpenDefault(reopened) returned error: %v", err)
	}
	defer func() {
		if closeErr := reopened.Close(); closeErr != nil {
			t.Errorf("reopened.Close() returned error: %v", closeErr)
		}
	}()

	summary, found, err := reopened.GetSummary(context.Background(), persisted.ID)
	if err != nil {
		t.Fatalf("GetSummary() returned error: %v", err)
	}
	if !found {
		t.Fatal("GetSummary() did not find persisted session")
	}
	if !summary.Session.LastUsedAt.Equal(attachedAt) {
		t.Fatalf("last_used_at = %s, want %s", summary.Session.LastUsedAt, attachedAt)
	}
	if len(summary.AgentSessions) != 2 {
		t.Fatalf("len(agent sessions) = %d, want 2", len(summary.AgentSessions))
	}
	isolatedCodexSessions := 0
	for _, agentSession := range summary.AgentSessions {
		if agentSession.AgentType != string(agent.Codex) {
			continue
		}
		if strings.HasPrefix(agentSession.StatePath, "/home/kagen/.codex/") {
			isolatedCodexSessions++
		}
	}
	if isolatedCodexSessions != 1 {
		t.Fatalf("isolated codex session count = %d, want 1 newly created isolated session", isolatedCodexSessions)
	}
}

func TestRunAttachSelectsMostRecentReadySessionForCurrentRepository(t *testing.T) {
	storeHome := t.TempDir()
	t.Setenv("HOME", storeHome)

	repo := &git.Repository{
		Path:          t.TempDir(),
		CurrentBranch: "main",
		HeadSHA:       "abc123",
	}

	store, err := session.OpenDefault()
	if err != nil {
		t.Fatalf("OpenDefault() returned error: %v", err)
	}

	older, err := store.CreateKagenSession(context.Background(), session.CreateKagenSessionParams{
		RepoID:          repo.ID(),
		RepoPath:        repo.Path,
		BaseBranch:      repo.CurrentBranch,
		WorkspaceBranch: repo.KagenBranch(),
		HeadSHAAtStart:  repo.HeadSHA,
		Namespace:       "kagen-" + repo.ID(),
		PodName:         "agent",
		Status:          sessionStatusReady,
		LastUsedAt:      time.Date(2026, time.March, 12, 11, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("CreateKagenSession(older) returned error: %v", err)
	}
	if _, err := store.CreateAgentSession(context.Background(), session.CreateAgentSessionParams{
		KagenSessionUID: older.UID,
		AgentType:       string(agent.Codex),
		WorkingMode:     "shared_workspace",
		StatePath:       "/home/kagen/.codex",
		LastUsedAt:      older.LastUsedAt,
	}); err != nil {
		t.Fatalf("CreateAgentSession(older) returned error: %v", err)
	}

	newer, err := store.CreateKagenSession(context.Background(), session.CreateKagenSessionParams{
		RepoID:          repo.ID(),
		RepoPath:        repo.Path,
		BaseBranch:      repo.CurrentBranch,
		WorkspaceBranch: repo.KagenBranch(),
		HeadSHAAtStart:  repo.HeadSHA,
		Namespace:       "kagen-" + repo.ID(),
		PodName:         "agent",
		Status:          sessionStatusReady,
		LastUsedAt:      time.Date(2026, time.March, 12, 12, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("CreateKagenSession(newer) returned error: %v", err)
	}
	if _, err := store.CreateAgentSession(context.Background(), session.CreateAgentSessionParams{
		KagenSessionUID: newer.UID,
		AgentType:       string(agent.Claude),
		WorkingMode:     "shared_workspace",
		StatePath:       "/home/kagen/.claude",
		LastUsedAt:      newer.LastUsedAt,
	}); err != nil {
		t.Fatalf("CreateAgentSession(newer) returned error: %v", err)
	}

	notReady, err := store.CreateKagenSession(context.Background(), session.CreateKagenSessionParams{
		RepoID:          repo.ID(),
		RepoPath:        repo.Path,
		BaseBranch:      repo.CurrentBranch,
		WorkspaceBranch: repo.KagenBranch(),
		HeadSHAAtStart:  repo.HeadSHA,
		Namespace:       "kagen-" + repo.ID(),
		PodName:         "agent",
		Status:          sessionStatusStarting,
		LastUsedAt:      time.Date(2026, time.March, 12, 13, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("CreateKagenSession(notReady) returned error: %v", err)
	}
	if _, err := store.CreateAgentSession(context.Background(), session.CreateAgentSessionParams{
		KagenSessionUID: notReady.UID,
		AgentType:       string(agent.Codex),
		WorkingMode:     "shared_workspace",
		StatePath:       "/home/kagen/.codex",
		LastUsedAt:      notReady.LastUsedAt,
	}); err != nil {
		t.Fatalf("CreateAgentSession(notReady) returned error: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("Close() returned error: %v", err)
	}

	attachedAt := time.Date(2026, time.March, 12, 17, 0, 0, 0, time.UTC)
	stubSessionFlow(t, sessionFlowStubOptions{
		repo: repo,
		cfg:  config.DefaultConfig(),
		now:  attachedAt,
	})

	if err := runAttach(context.Background(), "codex", 0, false); err != nil {
		t.Fatalf("runAttach() returned error: %v", err)
	}

	reopened, err := session.OpenDefault()
	if err != nil {
		t.Fatalf("OpenDefault(reopened) returned error: %v", err)
	}
	defer func() {
		if closeErr := reopened.Close(); closeErr != nil {
			t.Errorf("reopened.Close() returned error: %v", closeErr)
		}
	}()

	olderSummary, found, err := reopened.GetSummary(context.Background(), older.ID)
	if err != nil {
		t.Fatalf("GetSummary(older) returned error: %v", err)
	}
	if !found {
		t.Fatal("GetSummary(older) did not find persisted session")
	}
	if !olderSummary.Session.LastUsedAt.Equal(older.LastUsedAt) {
		t.Fatalf("older last_used_at = %s, want unchanged %s", olderSummary.Session.LastUsedAt, older.LastUsedAt)
	}

	newerSummary, found, err := reopened.GetSummary(context.Background(), newer.ID)
	if err != nil {
		t.Fatalf("GetSummary(newer) returned error: %v", err)
	}
	if !found {
		t.Fatal("GetSummary(newer) did not find persisted session")
	}
	if !newerSummary.Session.LastUsedAt.Equal(attachedAt) {
		t.Fatalf("newer last_used_at = %s, want %s", newerSummary.Session.LastUsedAt, attachedAt)
	}
	if len(newerSummary.AgentSessions) != 2 {
		t.Fatalf("len(newerSummary.AgentSessions) = %d, want 2", len(newerSummary.AgentSessions))
	}
	codexSessions := 0
	for _, agentSession := range newerSummary.AgentSessions {
		if agentSession.AgentType != string(agent.Codex) {
			continue
		}
		codexSessions++
		if !strings.HasPrefix(agentSession.StatePath, "/home/kagen/.codex/") {
			t.Fatalf("codex agent session state path = %q, want per-session codex path", agentSession.StatePath)
		}
	}
	if codexSessions != 1 {
		t.Fatalf("codex session count = %d, want 1 newly created session", codexSessions)
	}
}

func TestRunAttachCreatesDistinctAgentSessionsForSameAgentType(t *testing.T) {
	storeHome := t.TempDir()
	t.Setenv("HOME", storeHome)

	store, err := session.OpenDefault()
	if err != nil {
		t.Fatalf("OpenDefault() returned error: %v", err)
	}

	persisted, err := store.CreateKagenSession(context.Background(), session.CreateKagenSessionParams{
		RepoID:          "repo-1",
		RepoPath:        t.TempDir(),
		BaseBranch:      "main",
		WorkspaceBranch: "kagen/main/s/5",
		HeadSHAAtStart:  "abc123",
		Namespace:       "kagen-repo-1",
		PodName:         "agent",
		Status:          sessionStatusReady,
		CreatedAt:       time.Date(2026, time.March, 12, 10, 0, 0, 0, time.UTC),
		LastUsedAt:      time.Date(2026, time.March, 12, 10, 5, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("CreateKagenSession() returned error: %v", err)
	}
	if _, err := store.CreateAgentSession(context.Background(), session.CreateAgentSessionParams{
		ID:              "existing-codex-session",
		KagenSessionUID: persisted.UID,
		AgentType:       string(agent.Codex),
		WorkingMode:     "shared_workspace",
		StatePath:       "/home/kagen/.codex/existing-codex-session",
		CreatedAt:       persisted.CreatedAt,
		LastUsedAt:      persisted.LastUsedAt,
	}); err != nil {
		t.Fatalf("CreateAgentSession() returned error: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("Close() returned error: %v", err)
	}

	stubSessionFlow(t, sessionFlowStubOptions{
		cfg: config.DefaultConfig(),
		now: time.Date(2026, time.March, 12, 18, 30, 0, 0, time.UTC),
	})

	if err := runAttach(context.Background(), "codex", persisted.ID, true); err != nil {
		t.Fatalf("runAttach() returned error: %v", err)
	}

	reopened, err := session.OpenDefault()
	if err != nil {
		t.Fatalf("OpenDefault(reopened) returned error: %v", err)
	}
	defer func() {
		if closeErr := reopened.Close(); closeErr != nil {
			t.Errorf("reopened.Close() returned error: %v", closeErr)
		}
	}()

	summary, found, err := reopened.GetSummary(context.Background(), persisted.ID)
	if err != nil {
		t.Fatalf("GetSummary() returned error: %v", err)
	}
	if !found {
		t.Fatal("GetSummary() did not find persisted session")
	}
	if len(summary.AgentSessions) != 2 {
		t.Fatalf("len(agent sessions) = %d, want 2", len(summary.AgentSessions))
	}

	statePaths := map[string]bool{}
	for _, agentSession := range summary.AgentSessions {
		if agentSession.AgentType != string(agent.Codex) {
			t.Fatalf("agent type = %q, want codex", agentSession.AgentType)
		}
		if statePaths[agentSession.StatePath] {
			t.Fatalf("duplicate state path found: %q", agentSession.StatePath)
		}
		statePaths[agentSession.StatePath] = true
	}
}

func TestRunAttachFailureNamesFailedStepAndRecordsPendingTail(t *testing.T) {
	storeHome := t.TempDir()
	t.Setenv("HOME", storeHome)

	store, err := session.OpenDefault()
	if err != nil {
		t.Fatalf("OpenDefault() returned error: %v", err)
	}

	persisted, err := store.CreateKagenSession(context.Background(), session.CreateKagenSessionParams{
		RepoID:          "repo-1",
		RepoPath:        t.TempDir(),
		BaseBranch:      "main",
		WorkspaceBranch: "kagen/main",
		HeadSHAAtStart:  "abc123",
		Namespace:       "kagen-repo-1",
		PodName:         "agent",
		Status:          sessionStatusReady,
		CreatedAt:       time.Date(2026, time.March, 12, 10, 0, 0, 0, time.UTC),
		LastUsedAt:      time.Date(2026, time.March, 12, 10, 30, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("CreateKagenSession() returned error: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("Close() returned error: %v", err)
	}

	calls := stubSessionFlow(t, sessionFlowStubOptions{
		cfg:        config.DefaultConfig(),
		now:        time.Date(2026, time.March, 12, 16, 30, 0, 0, time.UTC),
		prepareErr: kagerr.WithFailureClass(kagerr.FailureClassAgentHome, "preparing agent state path", errors.New("permission denied")),
	})

	err = runAttach(context.Background(), "codex", persisted.ID, true)
	if err == nil {
		t.Fatal("runAttach() error = nil, want prepare_agent_state failure")
	}
	if !strings.Contains(err.Error(), "attach failed at step prepare_agent_state") {
		t.Fatalf("runAttach() error = %v, want failed step name", err)
	}

	if len(calls.operations) != 1 {
		t.Fatalf("operation count = %d, want 1", len(calls.operations))
	}
	operation := calls.operations[0]
	if operation.Status != diagnostics.StatusFailed {
		t.Fatalf("operation status = %q, want failed", operation.Status)
	}
	if operation.Steps[0].Status != diagnostics.StatusSucceeded {
		t.Fatalf("ensure_runtime status = %q, want succeeded", operation.Steps[0].Status)
	}
	if operation.Steps[1].Status != diagnostics.StatusSucceeded {
		t.Fatalf("launch_agent_runtime status = %q, want succeeded", operation.Steps[1].Status)
	}
	if operation.Steps[2].Status != diagnostics.StatusSucceeded {
		t.Fatalf("preflight_runtime status = %q, want succeeded", operation.Steps[2].Status)
	}
	if operation.Steps[3].Status != diagnostics.StatusSucceeded {
		t.Fatalf("validate_proxy_policy status = %q, want succeeded", operation.Steps[2].Status)
	}
	if operation.Steps[4].Status != diagnostics.StatusFailed {
		t.Fatalf("prepare_agent_state status = %q, want failed", operation.Steps[4].Status)
	}
	if operation.Steps[4].FailureClass != kagerr.FailureClassAgentHome {
		t.Fatalf("prepare_agent_state failure class = %q, want %q", operation.Steps[4].FailureClass, kagerr.FailureClassAgentHome)
	}
	if !strings.Contains(operation.Steps[4].ErrorSummary, "permission denied") {
		t.Fatalf("prepare_agent_state error summary = %q, want permission denied", operation.Steps[4].ErrorSummary)
	}
	if operation.Steps[5].Status != diagnostics.StatusPending {
		t.Fatalf("attach_agent status = %q, want pending", operation.Steps[5].Status)
	}
	if calls.attaches != 0 {
		t.Fatalf("attach count = %d, want 0", calls.attaches)
	}
}

func TestRunStartPreflightConfigurationFailsBeforePersistingSession(t *testing.T) {
	storeHome := t.TempDir()
	t.Setenv("HOME", storeHome)

	repo := &git.Repository{
		Path:          t.TempDir(),
		CurrentBranch: "main",
		HeadSHA:       "abc123",
	}

	calls := stubSessionFlow(t, sessionFlowStubOptions{
		repo:               repo,
		cfg:                config.DefaultConfig(),
		now:                time.Date(2026, time.March, 12, 20, 30, 0, 0, time.UTC),
		configPreflightErr: kagerr.WithFailureClass(kagerr.FailureClassImage, "preflight image check workspace_image failed", errors.New("invalid image reference")),
	})

	err := runStart(context.Background(), "codex", false)
	if err == nil {
		t.Fatal("runStart() error = nil, want preflight configuration failure")
	}
	if !strings.Contains(err.Error(), "start failed at step preflight_configuration") {
		t.Fatalf("runStart() error = %v, want preflight step name", err)
	}
	if len(calls.operations) != 1 {
		t.Fatalf("operation count = %d, want 1", len(calls.operations))
	}
	if got := calls.operations[0].Steps[1].FailureClass; got != kagerr.FailureClassImage {
		t.Fatalf("preflight_configuration failure class = %q, want %q", got, kagerr.FailureClassImage)
	}

	store, err := session.OpenDefault()
	if err != nil {
		t.Fatalf("OpenDefault() returned error: %v", err)
	}
	defer func() {
		if closeErr := store.Close(); closeErr != nil {
			t.Errorf("store.Close() returned error: %v", closeErr)
		}
	}()

	summaries, err := store.List(context.Background(), session.ListOptions{RepoPath: repo.Path})
	if err != nil {
		t.Fatalf("List() returned error: %v", err)
	}
	if len(summaries) != 0 {
		t.Fatalf("len(List()) = %d, want 0 sessions after preflight failure", len(summaries))
	}
}

func TestRunAttachPreflightRuntimeRecordsFailureClass(t *testing.T) {
	storeHome := t.TempDir()
	t.Setenv("HOME", storeHome)

	store, err := session.OpenDefault()
	if err != nil {
		t.Fatalf("OpenDefault() returned error: %v", err)
	}

	persisted, err := store.CreateKagenSession(context.Background(), session.CreateKagenSessionParams{
		RepoID:          "repo-1",
		RepoPath:        t.TempDir(),
		BaseBranch:      "main",
		WorkspaceBranch: "kagen/main",
		HeadSHAAtStart:  "abc123",
		Namespace:       "kagen-repo-1",
		PodName:         "agent",
		Status:          sessionStatusReady,
		CreatedAt:       time.Date(2026, time.March, 12, 10, 0, 0, 0, time.UTC),
		LastUsedAt:      time.Date(2026, time.March, 12, 10, 30, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("CreateKagenSession() returned error: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("Close() returned error: %v", err)
	}

	calls := stubSessionFlow(t, sessionFlowStubOptions{
		cfg:                 config.DefaultConfig(),
		now:                 time.Date(2026, time.March, 12, 20, 45, 0, 0, time.UTC),
		runtimePreflightErr: kagerr.WithFailureClass(kagerr.FailureClassAgentBinary, "runtime binary \"codex\" is not available in the toolbox image", errors.New("command not found")),
	})

	err = runAttach(context.Background(), "codex", persisted.ID, true)
	if err == nil {
		t.Fatal("runAttach() error = nil, want runtime preflight failure")
	}
	if !strings.Contains(err.Error(), "attach failed at step preflight_runtime") {
		t.Fatalf("runAttach() error = %v, want preflight_runtime failure", err)
	}
	if len(calls.operations) != 1 {
		t.Fatalf("operation count = %d, want 1", len(calls.operations))
	}
	if got := calls.operations[0].Steps[2].FailureClass; got != kagerr.FailureClassAgentBinary {
		t.Fatalf("preflight_runtime failure class = %q, want %q", got, kagerr.FailureClassAgentBinary)
	}
	if got := calls.operations[0].Steps[2].Metadata["failure_class"]; got != string(kagerr.FailureClassAgentBinary) {
		t.Fatalf("preflight_runtime metadata failure_class = %q, want %q", got, kagerr.FailureClassAgentBinary)
	}
}

func TestResolveRequestedAgentUsesConfigDefault(t *testing.T) {
	got, err := resolveRequestedAgent("", nil, "", &config.Config{Agent: "codex"}, false)
	if err != nil {
		t.Fatalf("resolveRequestedAgent() returned error: %v", err)
	}
	if got != agent.Codex {
		t.Fatalf("agent type = %q, want %q", got, agent.Codex)
	}
}

func TestResolveRequestedAgentRequiresAttachAgentWhenUnset(t *testing.T) {
	_, err := resolveRequestedAgent("", nil, "", config.DefaultConfig(), false)
	if err == nil {
		t.Fatal("resolveRequestedAgent() error = nil, want missing agent error")
	}
	if !strings.Contains(err.Error(), "agent type is required") {
		t.Fatalf("resolveRequestedAgent() error = %v, want missing agent message", err)
	}
}

func TestRootCommandShowsHelpInsteadOfCompatibilityStart(t *testing.T) {
	if rootCmd.RunE == nil {
		t.Fatal("root command should provide help when invoked without a subcommand")
	}

	calls := stubSessionFlow(t, sessionFlowStubOptions{
		cfg: config.DefaultConfig(),
		now: time.Date(2026, time.March, 12, 18, 0, 0, 0, time.UTC),
	})

	cmd := &cobra.Command{}
	if err := rootCmd.RunE(cmd, nil); err != nil {
		t.Fatalf("root help handler returned error: %v", err)
	}

	if calls.launches != 0 {
		t.Fatalf("launch count = %d, want 0", calls.launches)
	}
	if calls.attaches != 0 {
		t.Fatalf("attach count = %d, want 0", calls.attaches)
	}
}

type sessionFlowStubOptions struct {
	repo                *git.Repository
	cfg                 *config.Config
	now                 time.Time
	ensureNamespaceErr  error
	ensureProxyErr      error
	ensureResourcesErr  error
	configPreflightErr  error
	runtimePreflightErr error
	validateErr         error
	launchErr           error
	prepareErr          error
	attachErr           error
	artefactCollector   diagnostics.FailureArtefactCollector
}

type sessionFlowCalls struct {
	launches         int
	prepares         int
	attaches         int
	operations       []diagnostics.Operation
	artefactRequests []diagnostics.FailureArtefactRequest
}

func stubSessionFlow(t *testing.T, opts sessionFlowStubOptions) *sessionFlowCalls {
	t.Helper()

	calls := &sessionFlowCalls{}
	reporter := &captureDiagnosticsReporter{calls: calls}

	originalDiscover := discoverRepositoryForSession
	originalLoadConfig := loadRunConfigForSession
	originalEnsureRuntime := ensureRuntimeForSession
	originalNewForgejoService := newForgejoServiceForSession
	originalEnsureNamespace := ensureNamespaceForSession
	originalEnsureProxy := ensureProxyForSession
	originalEnsureResources := ensureResourcesForSession
	originalEnsureForgejoImport := ensureForgejoImportForSession
	originalConfigPreflight := runConfigurationPreflightForSession
	originalRuntimePreflight := runRuntimePreflightForSession
	originalValidateProxyPolicy := validateProxyPolicyForSession
	originalLaunchAgentRuntime := launchAgentRuntimeForSession
	originalPrepareAgentState := prepareAgentStateForSession
	originalAttachAgent := attachAgentForSession
	originalReporter := newDiagnosticsReporterForSession
	originalArtefactCollector := newFailureArtefactCollectorForSession
	originalNow := nowForSession

	t.Cleanup(func() {
		discoverRepositoryForSession = originalDiscover
		loadRunConfigForSession = originalLoadConfig
		ensureRuntimeForSession = originalEnsureRuntime
		newForgejoServiceForSession = originalNewForgejoService
		ensureNamespaceForSession = originalEnsureNamespace
		ensureProxyForSession = originalEnsureProxy
		ensureResourcesForSession = originalEnsureResources
		ensureForgejoImportForSession = originalEnsureForgejoImport
		runConfigurationPreflightForSession = originalConfigPreflight
		runRuntimePreflightForSession = originalRuntimePreflight
		validateProxyPolicyForSession = originalValidateProxyPolicy
		launchAgentRuntimeForSession = originalLaunchAgentRuntime
		prepareAgentStateForSession = originalPrepareAgentState
		attachAgentForSession = originalAttachAgent
		newDiagnosticsReporterForSession = originalReporter
		newFailureArtefactCollectorForSession = originalArtefactCollector
		nowForSession = originalNow
	})

	discoverRepositoryForSession = func() (*git.Repository, error) {
		if opts.repo == nil {
			t.Fatal("discoverRepositoryForSession unexpectedly called without a stub repository")
		}

		return opts.repo, nil
	}
	loadRunConfigForSession = func() (*config.Config, error) {
		if opts.cfg != nil {
			return opts.cfg, nil
		}

		return config.DefaultConfig(), nil
	}
	ensureRuntimeForSession = func(_ context.Context, _ *config.Config) (string, error) {
		return "kagen-test", nil
	}
	newForgejoServiceForSession = func(string) (*forgejo.ForgejoService, error) {
		return nil, nil
	}
	ensureNamespaceForSession = func(_ context.Context, _ string, _ *git.Repository, _ agent.Type) error {
		return opts.ensureNamespaceErr
	}
	ensureProxyForSession = func(_ context.Context, _ string, _ *git.Repository, _ *config.Config, _ agent.Type) error {
		return opts.ensureProxyErr
	}
	ensureResourcesForSession = func(_ context.Context, _ string, _ *git.Repository, _ *config.Config, _ agent.Type) error {
		return opts.ensureResourcesErr
	}
	ensureForgejoImportForSession = func(_ context.Context, _ *forgejo.ForgejoService, _ *git.Repository) error {
		return nil
	}
	runConfigurationPreflightForSession = func(_ context.Context, _ *git.Repository, _ *config.Config, _ agent.Type) (preflight.Report, error) {
		return preflight.Report{}, opts.configPreflightErr
	}
	runRuntimePreflightForSession = func(_ context.Context, _ *git.Repository, _ string, _ agent.Type) (preflight.Report, error) {
		return preflight.Report{}, opts.runtimePreflightErr
	}
	validateProxyPolicyForSession = func(_ context.Context, _ string, _ *git.Repository, _ *config.Config, _ agent.Type) error {
		return opts.validateErr
	}
	launchAgentRuntimeForSession = func(_ context.Context, _ *git.Repository, _ string, _ agent.Type) error {
		calls.launches++
		return opts.launchErr
	}
	prepareAgentStateForSession = func(_ context.Context, _ *git.Repository, _ string, _ agent.Type, _ session.AgentSession) error {
		calls.prepares++
		return opts.prepareErr
	}
	attachAgentForSession = func(_ context.Context, _ *git.Repository, _ string, _ agent.Type, _ session.AgentSession) error {
		calls.attaches++
		return opts.attachErr
	}
	newDiagnosticsReporterForSession = func() diagnostics.Reporter { return reporter }
	newFailureArtefactCollectorForSession = func() diagnostics.FailureArtefactCollector {
		if opts.artefactCollector != nil {
			return opts.artefactCollector
		}

		return &captureFailureArtefactCollector{calls: calls, directory: "/tmp/kagen-failure-artefacts"}
	}
	nowForSession = func() time.Time {
		if opts.now.IsZero() {
			return time.Date(2026, time.March, 12, 19, 0, 0, 0, time.UTC)
		}

		return opts.now
	}

	return calls
}

type captureDiagnosticsReporter struct {
	calls *sessionFlowCalls
}

func (r *captureDiagnosticsReporter) StepStarted(diagnostics.Operation, diagnostics.StepRecord) {}

func (r *captureDiagnosticsReporter) OperationFinished(operation diagnostics.Operation) {
	r.calls.operations = append(r.calls.operations, operation)
}

type captureFailureArtefactCollector struct {
	calls     *sessionFlowCalls
	directory string
}

func (c *captureFailureArtefactCollector) Collect(_ context.Context, req diagnostics.FailureArtefactRequest) (diagnostics.FailureArtefactResult, error) {
	if c.calls != nil {
		c.calls.artefactRequests = append(c.calls.artefactRequests, req)
	}
	return diagnostics.FailureArtefactResult{Directory: c.directory}, nil
}

func TestRunStartFailureCapturesFailureArtefactsForPersistedSession(t *testing.T) {
	storeHome := t.TempDir()
	t.Setenv("HOME", storeHome)

	repo := &git.Repository{
		Path:          t.TempDir(),
		CurrentBranch: "main",
		HeadSHA:       "abc123",
	}

	collector := &captureFailureArtefactCollector{directory: "/tmp/kagen-start-failure"}
	calls := stubSessionFlow(t, sessionFlowStubOptions{
		repo:               repo,
		cfg:                config.DefaultConfig(),
		now:                time.Date(2026, time.March, 12, 20, 0, 0, 0, time.UTC),
		ensureResourcesErr: errors.New("image pull denied"),
		artefactCollector:  collector,
	})
	collector.calls = calls

	err := runStart(context.Background(), "codex", false)
	if err == nil {
		t.Fatal("runStart() error = nil, want ensure_resources failure")
	}
	if !strings.Contains(err.Error(), "start failed at step ensure_resources") {
		t.Fatalf("runStart() error = %v, want failed step", err)
	}
	if len(calls.artefactRequests) != 1 {
		t.Fatalf("artefact request count = %d, want 1", len(calls.artefactRequests))
	}
	request := calls.artefactRequests[0]
	if request.Operation.Name != "start" {
		t.Fatalf("artefact operation = %q, want start", request.Operation.Name)
	}
	if request.SessionSummary == nil {
		t.Fatal("artefact request session summary is nil")
	}
	if request.SessionSummary.Session.Status != sessionStatusFailed {
		t.Fatalf("artefact session status = %q, want failed", request.SessionSummary.Session.Status)
	}
	if request.SessionSummary.Session.ID == 0 {
		t.Fatal("artefact session ID is empty")
	}
}

func TestRunStartDetachFailureCapturesFailureClassAndMarksSessionFailed(t *testing.T) {
	storeHome := t.TempDir()
	t.Setenv("HOME", storeHome)

	repo := &git.Repository{
		Path:          t.TempDir(),
		CurrentBranch: "main",
		HeadSHA:       "abc123",
	}

	collector := &captureFailureArtefactCollector{directory: "/tmp/kagen-start-detach-failure"}
	calls := stubSessionFlow(t, sessionFlowStubOptions{
		repo:              repo,
		cfg:               config.DefaultConfig(),
		now:               time.Date(2026, time.March, 12, 20, 15, 0, 0, time.UTC),
		prepareErr:        kagerr.WithFailureClass(kagerr.FailureClassAgentHome, "preparing agent state path", errors.New("permission denied")),
		artefactCollector: collector,
	})
	collector.calls = calls

	err := runStart(context.Background(), "codex", true)
	if err == nil {
		t.Fatal("runStart() error = nil, want prepare_agent_state failure")
	}
	if !strings.Contains(err.Error(), "start failed at step prepare_agent_state") {
		t.Fatalf("runStart() error = %v, want failed step", err)
	}
	if calls.attaches != 0 {
		t.Fatalf("attach count = %d, want 0", calls.attaches)
	}
	if len(calls.operations) != 1 {
		t.Fatalf("operation count = %d, want 1", len(calls.operations))
	}

	operation := calls.operations[0]
	if got := operation.Steps[9].FailureClass; got != kagerr.FailureClassAgentHome {
		t.Fatalf("prepare_agent_state failure class = %q, want %q", got, kagerr.FailureClassAgentHome)
	}
	if len(calls.artefactRequests) != 1 {
		t.Fatalf("artefact request count = %d, want 1", len(calls.artefactRequests))
	}
	request := calls.artefactRequests[0]
	if request.SessionSummary == nil {
		t.Fatal("artefact request session summary is nil")
	}
	if request.SessionSummary.Session.Status != sessionStatusFailed {
		t.Fatalf("artefact session status = %q, want failed", request.SessionSummary.Session.Status)
	}
	if got := request.Operation.Steps[9].FailureClass; got != kagerr.FailureClassAgentHome {
		t.Fatalf("artefact failure class = %q, want %q", got, kagerr.FailureClassAgentHome)
	}
	if request.Operation.Metadata["start_mode"] != "detached" {
		t.Fatalf("artefact start_mode = %q, want detached", request.Operation.Metadata["start_mode"])
	}

	store, openErr := session.OpenDefault()
	if openErr != nil {
		t.Fatalf("OpenDefault() returned error: %v", openErr)
	}
	defer func() {
		if closeErr := store.Close(); closeErr != nil {
			t.Errorf("store.Close() returned error: %v", closeErr)
		}
	}()

	summaries, listErr := store.List(context.Background(), session.ListOptions{RepoPath: repo.Path})
	if listErr != nil {
		t.Fatalf("List() returned error: %v", listErr)
	}
	if len(summaries) != 1 {
		t.Fatalf("len(List()) = %d, want 1", len(summaries))
	}
	if summaries[0].Session.Status != sessionStatusFailed {
		t.Fatalf("persisted session status = %q, want failed", summaries[0].Session.Status)
	}
}

func TestRunAttachFailurePrintsFailureArtefactPath(t *testing.T) {
	storeHome := t.TempDir()
	t.Setenv("HOME", storeHome)

	store, err := session.OpenDefault()
	if err != nil {
		t.Fatalf("OpenDefault() returned error: %v", err)
	}

	persisted, err := store.CreateKagenSession(context.Background(), session.CreateKagenSessionParams{
		RepoID:          "repo-1",
		RepoPath:        t.TempDir(),
		BaseBranch:      "main",
		WorkspaceBranch: "kagen/main",
		HeadSHAAtStart:  "abc123",
		Namespace:       "kagen-repo-1",
		PodName:         "agent",
		Status:          sessionStatusReady,
		CreatedAt:       time.Date(2026, time.March, 12, 10, 0, 0, 0, time.UTC),
		LastUsedAt:      time.Date(2026, time.March, 12, 10, 30, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("CreateKagenSession() returned error: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("Close() returned error: %v", err)
	}

	collector := &captureFailureArtefactCollector{directory: "/tmp/kagen-attach-failure"}
	calls := stubSessionFlow(t, sessionFlowStubOptions{
		cfg:               config.DefaultConfig(),
		now:               time.Date(2026, time.March, 12, 21, 0, 0, 0, time.UTC),
		prepareErr:        errors.New("permission denied"),
		artefactCollector: collector,
	})
	collector.calls = calls

	stderr, restore := captureStderr(t)
	defer restore()

	err = runAttach(context.Background(), "codex", persisted.ID, true)
	if err == nil {
		t.Fatal("runAttach() error = nil, want prepare_agent_state failure")
	}
	output := stderr()
	if !strings.Contains(output, "/tmp/kagen-attach-failure") {
		t.Fatalf("stderr = %q, want failure artefact path", output)
	}
	if len(calls.artefactRequests) != 1 {
		t.Fatalf("artefact request count = %d, want 1", len(calls.artefactRequests))
	}
}

func captureStderr(t *testing.T) (func() string, func()) {
	t.Helper()

	original := os.Stderr
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe() returned error: %v", err)
	}
	os.Stderr = writer

	done := make(chan string, 1)
	go func() {
		content, _ := io.ReadAll(reader)
		done <- string(content)
	}()

	return func() string {
			_ = writer.Close()
			return <-done
		}, func() {
			os.Stderr = original
			_ = reader.Close()
		}
}
