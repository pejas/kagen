package cmd

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"

	"github.com/pejas/kagen/internal/agent"
	"github.com/pejas/kagen/internal/config"
	"github.com/pejas/kagen/internal/diagnostics"
	"github.com/pejas/kagen/internal/forgejo"
	"github.com/pejas/kagen/internal/git"
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

	if err := runStart(context.Background(), "codex"); err != nil {
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
	defer store.Close()

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

	if err := runStart(context.Background(), "codex"); err != nil {
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
		"ensure_namespace",
		"ensure_proxy",
		"ensure_resources",
		"forgejo_import",
		"launch_agent_runtime",
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
	prepareStep := operation.Steps[7]
	if prepareStep.Metadata["agent_session_id"] == "" {
		t.Fatal("prepare_agent_state metadata agent_session_id is empty")
	}
	if !strings.HasPrefix(prepareStep.Metadata["state_path"], "/home/kagen/.codex/") {
		t.Fatalf("prepare_agent_state state_path = %q, want codex session path", prepareStep.Metadata["state_path"])
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
	defer reopened.Close()

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
	defer reopened.Close()

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
	defer reopened.Close()

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
		prepareErr: errors.New("permission denied"),
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
		t.Fatalf("validate_proxy_policy status = %q, want succeeded", operation.Steps[2].Status)
	}
	if operation.Steps[3].Status != diagnostics.StatusFailed {
		t.Fatalf("prepare_agent_state status = %q, want failed", operation.Steps[3].Status)
	}
	if !strings.Contains(operation.Steps[3].ErrorSummary, "permission denied") {
		t.Fatalf("prepare_agent_state error summary = %q, want permission denied", operation.Steps[3].ErrorSummary)
	}
	if operation.Steps[4].Status != diagnostics.StatusPending {
		t.Fatalf("attach_agent status = %q, want pending", operation.Steps[4].Status)
	}
	if calls.attaches != 0 {
		t.Fatalf("attach count = %d, want 0", calls.attaches)
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
	repo               *git.Repository
	cfg                *config.Config
	now                time.Time
	ensureNamespaceErr error
	ensureProxyErr     error
	ensureResourcesErr error
	validateErr        error
	launchErr          error
	prepareErr         error
	attachErr          error
}

type sessionFlowCalls struct {
	launches   int
	prepares   int
	attaches   int
	operations []diagnostics.Operation
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
	originalValidateProxyPolicy := validateProxyPolicyForSession
	originalLaunchAgentRuntime := launchAgentRuntimeForSession
	originalPrepareAgentState := prepareAgentStateForSession
	originalAttachAgent := attachAgentForSession
	originalReporter := newDiagnosticsReporterForSession
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
		validateProxyPolicyForSession = originalValidateProxyPolicy
		launchAgentRuntimeForSession = originalLaunchAgentRuntime
		prepareAgentStateForSession = originalPrepareAgentState
		attachAgentForSession = originalAttachAgent
		newDiagnosticsReporterForSession = originalReporter
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
