package cmd

import (
	"context"
	"fmt"
	"path"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/spf13/cobra"

	"github.com/pejas/kagen/internal/agent"
	"github.com/pejas/kagen/internal/config"
	"github.com/pejas/kagen/internal/git"
	"github.com/pejas/kagen/internal/proxy"
	"github.com/pejas/kagen/internal/session"
	"github.com/pejas/kagen/internal/ui"
)

const (
	sessionStatusFailed   = "failed"
	sessionStatusReady    = "ready"
	sessionStatusStarting = "starting"
)

type sessionStore interface {
	Close() error
	CreateKagenSession(ctx context.Context, params session.CreateKagenSessionParams) (session.KagenSession, error)
	CreateAgentSession(ctx context.Context, params session.CreateAgentSessionParams) (session.AgentSession, error)
	GetSummary(ctx context.Context, id int64) (session.Summary, bool, error)
	FindMostRecentReady(ctx context.Context, repoPath string) (session.Summary, bool, error)
	UpdateKagenSessionStatus(ctx context.Context, id int64, status string) error
	RecordAttach(ctx context.Context, sessionID int64, agentSessionID string, attachedAt time.Time) error
}

var (
	openSessionStore = func() (sessionStore, error) {
		return session.OpenDefault()
	}

	discoverRepositoryForSession     = discoverRepository
	loadRunConfigForSession          = loadRunConfig
	ensureRuntimeForSession          = ensureRuntime
	newForgejoServiceForSession      = newForgejoService
	ensureClusterResourcesForSession = ensureClusterResources
	ensureForgejoImportForSession    = ensureForgejoImport
	validateProxyPolicyForSession    = validateProxyPolicy
	launchAgentRuntimeForSession     = launchAgentRuntime
	attachAgentForSession            = attachAgent
	nowForSession                    = func() time.Time { return time.Now().UTC() }
)

func newStartCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "start <agent>",
		Short: "Create a new kagen session and attach an agent",
		Long: `Creates a new persisted kagen session for the current repository,
ensures the runtime, Forgejo, and workload are ready, then attaches the
requested agent.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runStart(cmd.Context(), firstArg(args))
		},
	}

	return cmd
}

func newAttachCommand() *cobra.Command {
	var sessionID int64

	cmd := &cobra.Command{
		Use:   "attach <agent>",
		Short: "Attach an agent to a persisted kagen session",
		Long: `Attaches the requested agent to an existing persisted kagen session.

Without --session, the most recent ready session for the current repository
is selected automatically.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runAttach(cmd.Context(), firstArg(args), sessionID, cmd.Flags().Changed("session"))
		},
	}

	cmd.Flags().Int64Var(&sessionID, "session", 0, "persisted session ID to attach to")

	return cmd
}

func runStart(ctx context.Context, explicitAgent string) error {
	ctx = rootContext(ctx)

	cfg, err := loadRunConfigForSession()
	if err != nil {
		return err
	}

	repo, err := discoverRepositoryForSession()
	if err != nil {
		return err
	}

	kubeCtx, err := ensureRuntimeForSession(ctx, cfg)
	if err != nil {
		return err
	}

	agentType, err := resolveRequestedAgent(explicitAgent, repo, kubeCtx, cfg, true)
	if err != nil {
		return err
	}
	showSelectedAgent(cfg, agentType)

	forgejoService, err := newForgejoServiceForSession(kubeCtx)
	if err != nil {
		return err
	}
	if err := ensureClusterResourcesForSession(ctx, kubeCtx, repo, cfg, agentType); err != nil {
		return err
	}
	if err := ensureForgejoImportForSession(ctx, forgejoService, repo); err != nil {
		return err
	}
	if err := validateProxyPolicyForSession(ctx, kubeCtx, repo, cfg, agentType); err != nil {
		return err
	}

	store, err := openSessionStore()
	if err != nil {
		return fmt.Errorf("opening session store: %w", err)
	}
	defer store.Close()

	persisted, err := createPersistedKagenSession(ctx, store, repo)
	if err != nil {
		return err
	}

	if err := launchAgentRuntimeForSession(ctx, repo, kubeCtx, agentType); err != nil {
		if updateErr := store.UpdateKagenSessionStatus(ctx, persisted.ID, sessionStatusFailed); updateErr != nil {
			return fmt.Errorf("launching agent: %w (failed to mark session %d failed: %v)", err, persisted.ID, updateErr)
		}

		return err
	}

	attachedAt := nowForSession()
	agentSession, err := createPersistedAgentSession(ctx, store, persisted.UID, agentType, attachedAt)
	if err != nil {
		return err
	}

	if err := store.UpdateKagenSessionStatus(ctx, persisted.ID, sessionStatusReady); err != nil {
		return fmt.Errorf("marking session %d ready: %w", persisted.ID, err)
	}

	if err := store.RecordAttach(ctx, persisted.ID, agentSession.ID, attachedAt); err != nil {
		return fmt.Errorf("recording attach for session %d: %w", persisted.ID, err)
	}

	return attachAgentForSession(ctx, repo, kubeCtx, agentType, agentSession)
}

func runAttach(ctx context.Context, explicitAgent string, sessionID int64, sessionSelected bool) error {
	ctx = rootContext(ctx)

	cfg, err := loadRunConfigForSession()
	if err != nil {
		return err
	}

	agentType, err := resolveRequestedAgent(explicitAgent, nil, "", cfg, false)
	if err != nil {
		return err
	}

	store, err := openSessionStore()
	if err != nil {
		return fmt.Errorf("opening session store: %w", err)
	}
	defer store.Close()

	summary, err := resolveAttachSummary(ctx, store, sessionID, sessionSelected, agentType)
	if err != nil {
		return err
	}

	if err := validateAttachSummary(summary, agentType); err != nil {
		return err
	}

	repo := repositoryFromSession(summary.Session)
	showSelectedAgent(cfg, agentType)

	kubeCtx, err := ensureRuntimeForSession(ctx, cfg)
	if err != nil {
		return err
	}
	if err := validateProxyPolicyForSession(ctx, kubeCtx, repo, cfg, agentType); err != nil {
		return err
	}
	if err := launchAgentRuntimeForSession(ctx, repo, kubeCtx, agentType); err != nil {
		return err
	}

	attachedAt := nowForSession()
	agentSession, err := createPersistedAgentSession(ctx, store, summary.Session.UID, agentType, attachedAt)
	if err != nil {
		return err
	}

	if err := store.RecordAttach(ctx, summary.Session.ID, agentSession.ID, attachedAt); err != nil {
		return fmt.Errorf("recording attach for session %d: %w", summary.Session.ID, err)
	}

	return attachAgentForSession(ctx, repo, kubeCtx, agentType, agentSession)
}

func resolveRequestedAgent(explicitAgent string, repo *git.Repository, kubeCtx string, cfg *config.Config, interactive bool) (agent.Type, error) {
	source := strings.TrimSpace(explicitAgent)
	if source == "" && cfg != nil {
		source = strings.TrimSpace(cfg.Agent)
	}

	if source != "" {
		return agent.TypeFromString(source)
	}
	if !interactive {
		return "", fmt.Errorf("agent type is required")
	}

	return resolveAgent(repo, kubeCtx, cfg)
}

func showSelectedAgent(cfg *config.Config, agentType agent.Type) {
	ui.Info("Agent: %s", agentType)

	policy := proxy.LoadPolicy(cfg, string(agentType))
	if verboseFlag && len(policy.AllowedDestinations) > 0 {
		ui.Info("Required egress hosts: %s", strings.Join(policy.AllowedDestinations, ", "))
	}
}

func createPersistedKagenSession(ctx context.Context, store sessionStore, repo *git.Repository) (session.KagenSession, error) {
	createdAt := nowForSession()

	persisted, err := store.CreateKagenSession(ctx, session.CreateKagenSessionParams{
		RepoID:          repo.ID(),
		RepoPath:        repo.Path,
		BaseBranch:      repo.CurrentBranch,
		WorkspaceBranch: repo.KagenBranch(),
		HeadSHAAtStart:  repo.HeadSHA,
		Namespace:       fmt.Sprintf("kagen-%s", repo.ID()),
		PodName:         runtimePodName,
		Status:          sessionStatusStarting,
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
	store sessionStore,
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

func resolveAttachSummary(ctx context.Context, store sessionStore, sessionID int64, sessionSelected bool, agentType agent.Type) (session.Summary, error) {
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

	repo, err := discoverRepositoryForSession()
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
	if summary.Session.Status != sessionStatusReady {
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

func firstArg(args []string) string {
	if len(args) == 0 {
		return ""
	}

	return args[0]
}
