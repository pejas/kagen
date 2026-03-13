package workflow

import (
	"context"
	"fmt"
	"path"
	"time"

	"github.com/google/uuid"

	"github.com/pejas/kagen/internal/agent"
	"github.com/pejas/kagen/internal/config"
	"github.com/pejas/kagen/internal/git"
	"github.com/pejas/kagen/internal/session"
	"github.com/pejas/kagen/internal/ui"
)

const (
	SessionStatusFailed   = "failed"
	SessionStatusReady    = "ready"
	SessionStatusStarting = "starting"
	RuntimePodName        = "agent"
)

type SessionStore interface {
	Close() error
	CreateKagenSession(ctx context.Context, params session.CreateKagenSessionParams) (session.KagenSession, error)
	CreateAgentSession(ctx context.Context, params session.CreateAgentSessionParams) (session.AgentSession, error)
	GetSummary(ctx context.Context, id int64) (session.Summary, bool, error)
	FindMostRecentReady(ctx context.Context, repoPath string) (session.Summary, bool, error)
	UpdateKagenSessionStatus(ctx context.Context, id int64, status string) error
	RecordAttach(ctx context.Context, sessionID int64, agentSessionID string, attachedAt time.Time) error
}

type ImportService interface {
	EnsureRepo(ctx context.Context, repo *git.Repository) error
	ImportRepo(ctx context.Context, repo *git.Repository) error
}

type ReviewSession interface {
	HasNewCommits(ctx context.Context, repo *git.Repository) (bool, error)
	ReviewURL(repo *git.Repository) string
	Stop() error
	Done() <-chan struct{}
	Wait() error
}

type SessionDependencies struct {
	LoadConfig             func() (*config.Config, error)
	DiscoverRepository     func() (*git.Repository, error)
	EnsureRuntime          func(context.Context, *config.Config) (string, error)
	ResolveRequestedAgent  func(string, *git.Repository, string, *config.Config, bool) (agent.Type, error)
	ShowSelectedAgent      func(*config.Config, agent.Type)
	NewForgejoService      func(string) (ImportService, error)
	EnsureClusterResources func(context.Context, string, *git.Repository, *config.Config, agent.Type) error
	EnsureForgejoImport    func(context.Context, ImportService, *git.Repository) error
	ValidateProxyPolicy    func(context.Context, string, *git.Repository, *config.Config, agent.Type) error
	LaunchAgentRuntime     func(context.Context, *git.Repository, string, agent.Type) error
	AttachAgent            func(context.Context, *git.Repository, string, agent.Type, session.AgentSession) error
	OpenSessionStore       func() (SessionStore, error)
	Now                    func() time.Time
}

type StartWorkflow struct {
	deps SessionDependencies
}

func NewStartWorkflow(deps SessionDependencies) *StartWorkflow {
	return &StartWorkflow{deps: deps}
}

func (w *StartWorkflow) Run(ctx context.Context, explicitAgent string) error {
	ctx = contextOrBackground(ctx)

	cfg, err := w.deps.LoadConfig()
	if err != nil {
		return err
	}

	repo, err := w.deps.DiscoverRepository()
	if err != nil {
		return err
	}

	kubeCtx, err := w.deps.EnsureRuntime(ctx, cfg)
	if err != nil {
		return err
	}

	agentType, err := w.deps.ResolveRequestedAgent(explicitAgent, repo, kubeCtx, cfg, true)
	if err != nil {
		return err
	}
	w.deps.ShowSelectedAgent(cfg, agentType)

	forgejoService, err := w.deps.NewForgejoService(kubeCtx)
	if err != nil {
		return err
	}
	if err := w.deps.EnsureClusterResources(ctx, kubeCtx, repo, cfg, agentType); err != nil {
		return err
	}
	if err := w.deps.EnsureForgejoImport(ctx, forgejoService, repo); err != nil {
		return err
	}
	store, err := w.deps.OpenSessionStore()
	if err != nil {
		return fmt.Errorf("opening session store: %w", err)
	}
	defer store.Close()

	persisted, err := createPersistedKagenSession(ctx, store, repo, w.deps.Now)
	if err != nil {
		return err
	}
	ui.Verbose("Created kagen session id=%d uid=%s", persisted.ID, persisted.UID)

	if err := w.deps.LaunchAgentRuntime(ctx, repo, kubeCtx, agentType); err != nil {
		if updateErr := store.UpdateKagenSessionStatus(ctx, persisted.ID, SessionStatusFailed); updateErr != nil {
			return fmt.Errorf("launching agent: %w (failed to mark session %d failed: %v)", err, persisted.ID, updateErr)
		}

		return err
	}
	if err := w.deps.ValidateProxyPolicy(ctx, kubeCtx, repo, cfg, agentType); err != nil {
		if updateErr := store.UpdateKagenSessionStatus(ctx, persisted.ID, SessionStatusFailed); updateErr != nil {
			return fmt.Errorf("validating proxy policy: %w (failed to mark session %d failed: %v)", err, persisted.ID, updateErr)
		}

		return err
	}

	attachedAt := w.deps.Now()
	agentSession, err := createPersistedAgentSession(ctx, store, persisted.UID, agentType, attachedAt)
	if err != nil {
		return err
	}
	ui.Verbose("Created agent session %s with state path %s", agentSession.ID, agentSession.StatePath)

	if err := store.UpdateKagenSessionStatus(ctx, persisted.ID, SessionStatusReady); err != nil {
		return fmt.Errorf("marking session %d ready: %w", persisted.ID, err)
	}
	if err := store.RecordAttach(ctx, persisted.ID, agentSession.ID, attachedAt); err != nil {
		return fmt.Errorf("recording attach for session %d: %w", persisted.ID, err)
	}

	return w.deps.AttachAgent(ctx, repo, kubeCtx, agentType, agentSession)
}

type AttachWorkflow struct {
	deps SessionDependencies
}

func NewAttachWorkflow(deps SessionDependencies) *AttachWorkflow {
	return &AttachWorkflow{deps: deps}
}

func (w *AttachWorkflow) Run(ctx context.Context, explicitAgent string, sessionID int64, sessionSelected bool) error {
	ctx = contextOrBackground(ctx)

	cfg, err := w.deps.LoadConfig()
	if err != nil {
		return err
	}

	agentType, err := w.deps.ResolveRequestedAgent(explicitAgent, nil, "", cfg, false)
	if err != nil {
		return err
	}

	store, err := w.deps.OpenSessionStore()
	if err != nil {
		return fmt.Errorf("opening session store: %w", err)
	}
	defer store.Close()

	summary, err := resolveAttachSummary(ctx, store, w.deps.DiscoverRepository, sessionID, sessionSelected, agentType)
	if err != nil {
		return err
	}
	if err := validateAttachSummary(summary, agentType); err != nil {
		return err
	}
	ui.Verbose("Resolved attach target session id=%d uid=%s", summary.Session.ID, summary.Session.UID)

	repo := repositoryFromSession(summary.Session)
	w.deps.ShowSelectedAgent(cfg, agentType)

	kubeCtx, err := w.deps.EnsureRuntime(ctx, cfg)
	if err != nil {
		return err
	}
	if err := w.deps.LaunchAgentRuntime(ctx, repo, kubeCtx, agentType); err != nil {
		return err
	}
	if err := w.deps.ValidateProxyPolicy(ctx, kubeCtx, repo, cfg, agentType); err != nil {
		return err
	}

	attachedAt := w.deps.Now()
	agentSession, err := createPersistedAgentSession(ctx, store, summary.Session.UID, agentType, attachedAt)
	if err != nil {
		return err
	}
	ui.Verbose("Created agent session %s with state path %s", agentSession.ID, agentSession.StatePath)
	if err := store.RecordAttach(ctx, summary.Session.ID, agentSession.ID, attachedAt); err != nil {
		return fmt.Errorf("recording attach for session %d: %w", summary.Session.ID, err)
	}

	return w.deps.AttachAgent(ctx, repo, kubeCtx, agentType, agentSession)
}

func contextOrBackground(ctx context.Context) context.Context {
	if ctx != nil {
		return ctx
	}

	return context.Background()
}

func createPersistedKagenSession(
	ctx context.Context,
	store SessionStore,
	repo *git.Repository,
	now func() time.Time,
) (session.KagenSession, error) {
	createdAt := now()

	persisted, err := store.CreateKagenSession(ctx, session.CreateKagenSessionParams{
		RepoID:          repo.ID(),
		RepoPath:        repo.Path,
		BaseBranch:      repo.CurrentBranch,
		WorkspaceBranch: repo.KagenBranch(),
		HeadSHAAtStart:  repo.HeadSHA,
		Namespace:       fmt.Sprintf("kagen-%s", repo.ID()),
		PodName:         RuntimePodName,
		Status:          SessionStatusStarting,
		CreatedAt:       createdAt,
		LastUsedAt:      createdAt,
	})
	if err != nil {
		return session.KagenSession{}, fmt.Errorf("creating persisted session: %w", err)
	}

	return persisted, nil
}

func createPersistedAgentSession(
	ctx context.Context,
	store SessionStore,
	kagenSessionUID string,
	agentType agent.Type,
	attachedAt time.Time,
) (session.AgentSession, error) {
	agentSessionID := uuid.NewString()
	agentSession, err := store.CreateAgentSession(ctx, session.CreateAgentSessionParams{
		ID:              agentSessionID,
		KagenSessionUID: kagenSessionUID,
		AgentType:       string(agentType),
		WorkingMode:     "shared_workspace",
		StatePath:       agentSessionStatePath(agentType, agentSessionID),
		CreatedAt:       attachedAt,
		LastUsedAt:      attachedAt,
	})
	if err != nil {
		return session.AgentSession{}, fmt.Errorf("creating %s agent session: %w", agentType, err)
	}

	return agentSession, nil
}

func resolveAttachSummary(
	ctx context.Context,
	store SessionStore,
	discoverRepository func() (*git.Repository, error),
	sessionID int64,
	sessionSelected bool,
	agentType agent.Type,
) (session.Summary, error) {
	if sessionSelected {
		summary, found, err := store.GetSummary(ctx, sessionID)
		if err != nil {
			return session.Summary{}, fmt.Errorf("loading session %d: %w", sessionID, err)
		}
		if !found {
			return session.Summary{}, fmt.Errorf("session %d not found: run 'kagen list --all' to inspect persisted sessions", sessionID)
		}

		return summary, nil
	}

	repo, err := discoverRepository()
	if err != nil {
		return session.Summary{}, err
	}

	summary, found, err := store.FindMostRecentReady(ctx, repo.Path)
	if err != nil {
		return session.Summary{}, fmt.Errorf("resolving ready session for %s: %w", repo.Path, err)
	}
	if !found {
		return session.Summary{}, fmt.Errorf("no ready %s session found for %s: run 'kagen start %s' to create one", agentType, repo.Path, agentType)
	}

	return summary, nil
}

func validateAttachSummary(summary session.Summary, agentType agent.Type) error {
	if summary.Session.Status != SessionStatusReady {
		return fmt.Errorf(
			"session %d is %s, not ready: choose a ready session with 'kagen list' or start a new one with 'kagen start %s'",
			summary.Session.ID,
			summary.Session.Status,
			agentType,
		)
	}

	return nil
}

func repositoryFromSession(persisted session.KagenSession) *git.Repository {
	return &git.Repository{
		Path:          persisted.RepoPath,
		CurrentBranch: persisted.BaseBranch,
		HeadSHA:       persisted.HeadSHAAtStart,
	}
}

func agentSessionStatePath(agentType agent.Type, agentSessionID string) string {
	switch agentType {
	case agent.Codex:
		return path.Join(agent.DefaultHomeDir(), ".codex", agentSessionID)
	case agent.Claude:
		return path.Join(agent.DefaultHomeDir(), ".claude", agentSessionID)
	case agent.OpenCode:
		return path.Join(agent.DefaultHomeDir(), ".opencode", agentSessionID)
	default:
		return path.Join(agent.DefaultHomeDir(), agentSessionID)
	}
}
