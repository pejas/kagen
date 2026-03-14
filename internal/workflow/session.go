package workflow

import (
	"context"
	"fmt"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/pejas/kagen/internal/agent"
	"github.com/pejas/kagen/internal/config"
	"github.com/pejas/kagen/internal/diagnostics"
	"github.com/pejas/kagen/internal/git"
	"github.com/pejas/kagen/internal/preflight"
	"github.com/pejas/kagen/internal/session"
	"github.com/pejas/kagen/internal/ui"
)

const (
	SessionStatusFailed   = "failed"
	SessionStatusReady    = "ready"
	SessionStatusStarting = "starting"
	RuntimePodName        = "agent"

	stepEnsureRuntime      = "ensure_runtime"
	stepEnsureNamespace    = "ensure_namespace"
	stepEnsureProxy        = "ensure_proxy"
	stepEnsureResources    = "ensure_resources"
	stepPreflightConfig    = "preflight_configuration"
	stepForgejoImport      = "forgejo_import"
	stepLaunchAgentRuntime = "launch_agent_runtime"
	stepPreflightRuntime   = "preflight_runtime"
	stepValidateProxy      = "validate_proxy_policy"
	stepPrepareAgentState  = "prepare_agent_state"
	stepAttachAgent        = "attach_agent"
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
	LoadConfig                func() (*config.Config, error)
	DiscoverRepository        func() (*git.Repository, error)
	EnsureRuntime             func(context.Context, *config.Config) (string, error)
	ResolveRequestedAgent     func(string, *git.Repository, string, *config.Config, bool) (agent.Type, error)
	ShowSelectedAgent         func(*config.Config, agent.Type)
	NewForgejoService         func(string) (ImportService, error)
	EnsureNamespace           func(context.Context, string, *git.Repository, agent.Type) error
	EnsureProxy               func(context.Context, string, *git.Repository, *config.Config, agent.Type) error
	EnsureResources           func(context.Context, string, *git.Repository, *config.Config, agent.Type) error
	EnsureForgejoImport       func(context.Context, ImportService, *git.Repository) error
	RunConfigurationPreflight func(context.Context, *git.Repository, *config.Config, agent.Type) (preflight.Report, error)
	RunRuntimePreflight       func(context.Context, *git.Repository, string, agent.Type) (preflight.Report, error)
	ValidateProxyPolicy       func(context.Context, string, *git.Repository, *config.Config, agent.Type) error
	LaunchAgentRuntime        func(context.Context, *git.Repository, string, agent.Type) error
	PrepareAgentState         func(context.Context, *git.Repository, string, agent.Type, session.AgentSession) error
	AttachAgent               func(context.Context, *git.Repository, string, agent.Type, session.AgentSession) error
	OpenSessionStore          func() (SessionStore, error)
	DiagnosticsReporter       diagnostics.Reporter
	FailureArtefacts          diagnostics.FailureArtefactCollector
	Now                       func() time.Time
}

type StartOptions struct {
	Detach bool
}

type StartWorkflow struct {
	deps SessionDependencies
}

func NewStartWorkflow(deps SessionDependencies) *StartWorkflow {
	return &StartWorkflow{deps: deps}
}

func (w *StartWorkflow) Run(ctx context.Context, explicitAgent string, options StartOptions) (err error) {
	ctx = contextOrBackground(ctx)
	trace := diagnostics.NewRecorder("start", startTraceSteps(options), w.deps.Now, w.deps.DiagnosticsReporter)
	defer trace.Complete()

	cfg, err := w.deps.LoadConfig()
	if err != nil {
		return err
	}

	repo, err := w.deps.DiscoverRepository()
	if err != nil {
		return err
	}
	trace.AddMetadataMap(map[string]string{
		"namespace":   fmt.Sprintf("kagen-%s", repo.ID()),
		"pod_name":    RuntimePodName,
		"repo_id":     repo.ID(),
		"repo_path":   repo.Path,
		"base_branch": repo.CurrentBranch,
	})
	if options.Detach {
		trace.AddMetadata("start_mode", "detached")
	}

	var kubeCtx string
	if err := trace.RunStep(stepEnsureRuntime, func(step *diagnostics.StepContext) error {
		resolvedKubeCtx, ensureErr := w.deps.EnsureRuntime(ctx, cfg)
		if ensureErr != nil {
			return ensureErr
		}

		kubeCtx = resolvedKubeCtx
		trace.AddMetadata("kube_context", kubeCtx)
		return nil
	}); err != nil {
		return err
	}

	agentType, err := w.deps.ResolveRequestedAgent(explicitAgent, repo, kubeCtx, cfg, true)
	if err != nil {
		return err
	}
	trace.AddMetadataMap(map[string]string{
		"agent_type":      string(agentType),
		"agent_container": agentContainerName(agentType),
		"workspace_image": cfg.Images.Workspace,
		"toolbox_image":   cfg.Images.Toolbox,
		"proxy_image":     cfg.Images.Proxy,
	})
	w.deps.ShowSelectedAgent(cfg, agentType)

	handleEarlyFailure := func(err error) error {
		return w.captureFailure(ctx, trace, err, failureCapture{})
	}
	if err := trace.RunStep(stepPreflightConfig, func(step *diagnostics.StepContext) error {
		if w.deps.RunConfigurationPreflight == nil {
			return nil
		}

		report, preflightErr := w.deps.RunConfigurationPreflight(ctx, repo, cfg, agentType)
		addPreflightMetadata(step, report)
		return preflightErr
	}); err != nil {
		return handleEarlyFailure(err)
	}

	store, err := w.deps.OpenSessionStore()
	if err != nil {
		return fmt.Errorf("opening session store: %w", err)
	}
	defer func() {
		if closeErr := store.Close(); closeErr != nil && err == nil {
			err = fmt.Errorf("closing session store: %w", closeErr)
		}
	}()

	persisted, err := createPersistedKagenSession(ctx, store, repo, w.deps.Now)
	if err != nil {
		return err
	}
	trace.AddMetadata("session_id", strconv.FormatInt(persisted.ID, 10))
	trace.AddMetadata("session_uid", persisted.UID)
	ui.Verbose("Created kagen session id=%d uid=%s", persisted.ID, persisted.UID)

	handleFailure := func(err error) error {
		return w.captureFailure(ctx, trace, err, failureCapture{
			store:     store,
			sessionID: persisted.ID,
		})
	}

	forgejoService, err := w.deps.NewForgejoService(kubeCtx)
	if err != nil {
		return handleFailure(err)
	}
	if err := trace.RunStep(stepEnsureNamespace, func(step *diagnostics.StepContext) error {
		return w.deps.EnsureNamespace(ctx, kubeCtx, repo, agentType)
	}); err != nil {
		return handleFailure(err)
	}
	if err := trace.RunStep(stepEnsureProxy, func(step *diagnostics.StepContext) error {
		return w.deps.EnsureProxy(ctx, kubeCtx, repo, cfg, agentType)
	}); err != nil {
		return handleFailure(err)
	}
	if err := trace.RunStep(stepEnsureResources, func(step *diagnostics.StepContext) error {
		step.AddMetadataMap(map[string]string{
			"workspace_image": cfg.Images.Workspace,
			"toolbox_image":   cfg.Images.Toolbox,
		})
		return w.deps.EnsureResources(ctx, kubeCtx, repo, cfg, agentType)
	}); err != nil {
		return handleFailure(err)
	}
	if err := trace.RunStep(stepForgejoImport, func(step *diagnostics.StepContext) error {
		return w.deps.EnsureForgejoImport(ctx, forgejoService, repo)
	}); err != nil {
		return handleFailure(err)
	}

	sessionReady := false
	if err := trace.RunStep(stepLaunchAgentRuntime, func(step *diagnostics.StepContext) error {
		return w.deps.LaunchAgentRuntime(ctx, repo, kubeCtx, agentType)
	}); err != nil {
		return handleFailure(err)
	}
	if err := trace.RunStep(stepPreflightRuntime, func(step *diagnostics.StepContext) error {
		if w.deps.RunRuntimePreflight == nil {
			return nil
		}

		report, preflightErr := w.deps.RunRuntimePreflight(ctx, repo, kubeCtx, agentType)
		addPreflightMetadata(step, report)
		return preflightErr
	}); err != nil {
		return handleFailure(err)
	}
	if err := trace.RunStep(stepValidateProxy, func(step *diagnostics.StepContext) error {
		return w.deps.ValidateProxyPolicy(ctx, kubeCtx, repo, cfg, agentType)
	}); err != nil {
		return handleFailure(err)
	}

	var attachedAt time.Time
	var agentSession session.AgentSession
	if err := trace.RunStep(stepPrepareAgentState, func(step *diagnostics.StepContext) error {
		attachedAt = w.deps.Now()
		createdAgentSession, createErr := createPersistedAgentSession(ctx, store, persisted.UID, agentType, attachedAt)
		if createErr != nil {
			return createErr
		}
		agentSession = createdAgentSession
		step.AddMetadataMap(map[string]string{
			"agent_session_id": agentSession.ID,
			"state_path":       agentSession.StatePath,
		})
		trace.AddMetadata("agent_session_id", agentSession.ID)
		ui.Verbose("Created agent session %s with state path %s", agentSession.ID, agentSession.StatePath)

		if prepareErr := w.deps.PrepareAgentState(ctx, repo, kubeCtx, agentType, agentSession); prepareErr != nil {
			return prepareErr
		}
		if updateErr := store.UpdateKagenSessionStatus(ctx, persisted.ID, SessionStatusReady); updateErr != nil {
			return fmt.Errorf("marking session %d ready: %w", persisted.ID, updateErr)
		}
		sessionReady = true
		if recordErr := store.RecordAttach(ctx, persisted.ID, agentSession.ID, attachedAt); recordErr != nil {
			return fmt.Errorf("recording attach for session %d: %w", persisted.ID, recordErr)
		}

		return nil
	}); err != nil {
		if !sessionReady {
			return handleFailure(err)
		}
		return w.captureFailure(ctx, trace, err, failureCapture{
			store:     store,
			sessionID: persisted.ID,
		})
	}

	if options.Detach {
		ui.Success(
			"Session %d is ready. Attach later with 'kagen attach %s --session %d'.",
			persisted.ID,
			agentType,
			persisted.ID,
		)
		return nil
	}

	if err := trace.RunStep(stepAttachAgent, func(step *diagnostics.StepContext) error {
		return w.deps.AttachAgent(ctx, repo, kubeCtx, agentType, agentSession)
	}); err != nil {
		return w.captureFailure(ctx, trace, err, failureCapture{
			store:     store,
			sessionID: persisted.ID,
		})
	}

	return nil
}

type AttachWorkflow struct {
	deps SessionDependencies
}

func NewAttachWorkflow(deps SessionDependencies) *AttachWorkflow {
	return &AttachWorkflow{deps: deps}
}

func (w *AttachWorkflow) Run(ctx context.Context, explicitAgent string, sessionID int64, sessionSelected bool) (err error) {
	ctx = contextOrBackground(ctx)
	trace := diagnostics.NewRecorder("attach", attachTraceSteps(), w.deps.Now, w.deps.DiagnosticsReporter)
	defer trace.Complete()

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
	defer func() {
		if closeErr := store.Close(); closeErr != nil && err == nil {
			err = fmt.Errorf("closing session store: %w", closeErr)
		}
	}()

	summary, err := resolveAttachSummary(ctx, store, w.deps.DiscoverRepository, sessionID, sessionSelected, agentType)
	if err != nil {
		return err
	}
	if err := validateAttachSummary(summary, agentType); err != nil {
		return err
	}
	trace.AddMetadataMap(map[string]string{
		"agent_type":      string(agentType),
		"agent_container": agentContainerName(agentType),
		"namespace":       summary.Session.Namespace,
		"pod_name":        summary.Session.PodName,
		"repo_id":         summary.Session.RepoID,
		"repo_path":       summary.Session.RepoPath,
		"session_id":      strconv.FormatInt(summary.Session.ID, 10),
		"session_uid":     summary.Session.UID,
		"workspace_image": cfg.Images.Workspace,
		"toolbox_image":   cfg.Images.Toolbox,
		"proxy_image":     cfg.Images.Proxy,
	})
	ui.Verbose("Resolved attach target session id=%d uid=%s", summary.Session.ID, summary.Session.UID)

	var kubeCtx string

	handleFailure := func(err error) error {
		return w.captureFailure(ctx, trace, err, failureCapture{
			store:          store,
			sessionID:      summary.Session.ID,
			sessionSummary: &summary,
		})
	}

	repo := repositoryFromSession(summary.Session)
	w.deps.ShowSelectedAgent(cfg, agentType)

	if err := trace.RunStep(stepEnsureRuntime, func(step *diagnostics.StepContext) error {
		resolvedKubeCtx, ensureErr := w.deps.EnsureRuntime(ctx, cfg)
		if ensureErr != nil {
			return ensureErr
		}

		kubeCtx = resolvedKubeCtx
		trace.AddMetadata("kube_context", kubeCtx)
		return nil
	}); err != nil {
		return handleFailure(err)
	}
	if err := trace.RunStep(stepLaunchAgentRuntime, func(step *diagnostics.StepContext) error {
		return w.deps.LaunchAgentRuntime(ctx, repo, kubeCtx, agentType)
	}); err != nil {
		return handleFailure(err)
	}
	if err := trace.RunStep(stepPreflightRuntime, func(step *diagnostics.StepContext) error {
		if w.deps.RunRuntimePreflight == nil {
			return nil
		}

		report, preflightErr := w.deps.RunRuntimePreflight(ctx, repo, kubeCtx, agentType)
		addPreflightMetadata(step, report)
		return preflightErr
	}); err != nil {
		return handleFailure(err)
	}
	if err := trace.RunStep(stepValidateProxy, func(step *diagnostics.StepContext) error {
		return w.deps.ValidateProxyPolicy(ctx, kubeCtx, repo, cfg, agentType)
	}); err != nil {
		return handleFailure(err)
	}

	var attachedAt time.Time
	var agentSession session.AgentSession
	if err := trace.RunStep(stepPrepareAgentState, func(step *diagnostics.StepContext) error {
		attachedAt = w.deps.Now()
		createdAgentSession, createErr := createPersistedAgentSession(ctx, store, summary.Session.UID, agentType, attachedAt)
		if createErr != nil {
			return createErr
		}
		agentSession = createdAgentSession
		step.AddMetadataMap(map[string]string{
			"agent_session_id": agentSession.ID,
			"state_path":       agentSession.StatePath,
		})
		trace.AddMetadata("agent_session_id", agentSession.ID)
		ui.Verbose("Created agent session %s with state path %s", agentSession.ID, agentSession.StatePath)

		if prepareErr := w.deps.PrepareAgentState(ctx, repo, kubeCtx, agentType, agentSession); prepareErr != nil {
			return prepareErr
		}
		if recordErr := store.RecordAttach(ctx, summary.Session.ID, agentSession.ID, attachedAt); recordErr != nil {
			return fmt.Errorf("recording attach for session %d: %w", summary.Session.ID, recordErr)
		}

		return nil
	}); err != nil {
		return handleFailure(err)
	}

	if err := trace.RunStep(stepAttachAgent, func(step *diagnostics.StepContext) error {
		return w.deps.AttachAgent(ctx, repo, kubeCtx, agentType, agentSession)
	}); err != nil {
		return handleFailure(err)
	}

	return nil
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

func failStartSession(ctx context.Context, store SessionStore, sessionID int64, err error) error {
	if updateErr := store.UpdateKagenSessionStatus(ctx, sessionID, SessionStatusFailed); updateErr != nil {
		return fmt.Errorf("%v (failed to mark session %d failed: %v)", err, sessionID, updateErr)
	}

	return err
}

type failureCapture struct {
	store          SessionStore
	sessionID      int64
	sessionSummary *session.Summary
}

func (w *StartWorkflow) captureFailure(
	ctx context.Context,
	trace *diagnostics.Recorder,
	err error,
	state failureCapture,
) error {
	return captureWorkflowFailure(ctx, w.deps, trace, err, state, true)
}

func (w *AttachWorkflow) captureFailure(
	ctx context.Context,
	trace *diagnostics.Recorder,
	err error,
	state failureCapture,
) error {
	return captureWorkflowFailure(ctx, w.deps, trace, err, state, false)
}

func captureWorkflowFailure(
	ctx context.Context,
	deps SessionDependencies,
	trace *diagnostics.Recorder,
	err error,
	state failureCapture,
	updateSessionStatus bool,
) error {
	if updateSessionStatus && state.sessionID > 0 && state.store != nil {
		err = failStartSession(ctx, state.store, state.sessionID, err)
	}

	if deps.FailureArtefacts == nil {
		return err
	}

	operation := trace.Complete()
	summary := state.sessionSummary
	if summary == nil && state.store != nil && state.sessionID > 0 {
		loadedSummary, found, loadErr := state.store.GetSummary(ctx, state.sessionID)
		if loadErr != nil {
			ui.Warn("Failed to load session summary for failure artefacts: %v", loadErr)
		} else if found {
			summary = &loadedSummary
		}
	}

	result, captureErr := deps.FailureArtefacts.Collect(context.Background(), diagnostics.FailureArtefactRequest{
		Operation:      operation,
		Err:            err,
		KubeContext:    strings.TrimSpace(operation.Metadata["kube_context"]),
		SessionSummary: summary,
	})
	if captureErr != nil {
		ui.Warn("Failed to capture failure artefacts: %v", captureErr)
		return err
	}

	ui.Warn("Failure artefacts saved to %s", result.Directory)
	return err
}

func agentContainerName(agentType agent.Type) string {
	spec, err := agent.SpecFor(agentType)
	if err != nil {
		return ""
	}

	return spec.ContainerName()
}

func startTraceSteps(options StartOptions) []string {
	steps := []string{
		stepEnsureRuntime,
		stepPreflightConfig,
		stepEnsureNamespace,
		stepEnsureProxy,
		stepEnsureResources,
		stepForgejoImport,
		stepLaunchAgentRuntime,
		stepPreflightRuntime,
		stepValidateProxy,
		stepPrepareAgentState,
	}

	if !options.Detach {
		steps = append(steps, stepAttachAgent)
	}

	return steps
}

func attachTraceSteps() []string {
	return []string{
		stepEnsureRuntime,
		stepLaunchAgentRuntime,
		stepPreflightRuntime,
		stepValidateProxy,
		stepPrepareAgentState,
		stepAttachAgent,
	}
}

func addPreflightMetadata(step *diagnostics.StepContext, report preflight.Report) {
	if step == nil {
		return
	}

	step.AddMetadataMap(report.Metadata())
}
