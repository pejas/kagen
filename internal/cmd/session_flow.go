package cmd

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/pejas/kagen/internal/agent"
	"github.com/pejas/kagen/internal/config"
	"github.com/pejas/kagen/internal/diagnostics"
	"github.com/pejas/kagen/internal/forgejo"
	"github.com/pejas/kagen/internal/git"
	"github.com/pejas/kagen/internal/preflight"
	"github.com/pejas/kagen/internal/proxy"
	"github.com/pejas/kagen/internal/session"
	"github.com/pejas/kagen/internal/ui"
	"github.com/pejas/kagen/internal/workflow"
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

	discoverRepositoryForSession        = discoverRepository
	loadRunConfigForSession             = loadRunConfig
	ensureRuntimeForSession             = ensureRuntime
	newForgejoServiceForSession         = newForgejoService
	ensureNamespaceForSession           = ensureNamespace
	ensureProxyForSession               = ensureProxy
	ensureResourcesForSession           = ensureResources
	ensureForgejoImportForSession       = ensureForgejoImport
	runConfigurationPreflightForSession = runConfigurationPreflight
	runRuntimePreflightForSession       = runRuntimePreflight
	validateProxyPolicyForSession       = validateProxyPolicy
	launchAgentRuntimeForSession        = launchAgentRuntime
	prepareAgentStateForSession         = prepareAgentState
	attachAgentForSession               = attachAgent
	newDiagnosticsReporterForSession    = func() diagnostics.Reporter {
		return diagnostics.NewCompositeReporter(
			diagnostics.NewUIReporter(),
			diagnostics.NewLatestOperationReporter(diagnostics.NewLatestOperationStore()),
		)
	}
	newFailureArtefactCollectorForSession = func() diagnostics.FailureArtefactCollector {
		return diagnostics.NewFailureArtefactCollector()
	}
	nowForSession = func() time.Time { return time.Now().UTC() }
)

func newStartCommand() *cobra.Command {
	var detach bool

	cmd := &cobra.Command{
		Use:   "start <agent>",
		Short: "Create a new kagen session and attach an agent",
		Long: `Creates a new persisted kagen session for the current repository,
ensures the runtime, Forgejo, and workload are ready, then attaches the
requested agent.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runStart(cmd.Context(), firstArg(args), detach)
		},
	}
	cmd.Flags().BoolVar(&detach, "detach", false, "launch the runtime and leave the session ready without interactive attach")

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

func runStart(ctx context.Context, explicitAgent string, detach bool) error {
	return workflow.NewStartWorkflow(newSessionWorkflowDependencies()).Run(ctx, explicitAgent, workflow.StartOptions{
		Detach: detach,
	})
}

func runAttach(ctx context.Context, explicitAgent string, sessionID int64, sessionSelected bool) error {
	return workflow.NewAttachWorkflow(newSessionWorkflowDependencies()).Run(ctx, explicitAgent, sessionID, sessionSelected)
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

func firstArg(args []string) string {
	if len(args) == 0 {
		return ""
	}

	return args[0]
}

func newSessionWorkflowDependencies() workflow.SessionDependencies {
	return workflow.SessionDependencies{
		LoadConfig:            loadRunConfigForSession,
		DiscoverRepository:    discoverRepositoryForSession,
		EnsureRuntime:         ensureRuntimeForSession,
		ResolveRequestedAgent: resolveRequestedAgent,
		ShowSelectedAgent:     showSelectedAgent,
		NewForgejoService:     func(kubeCtx string) (workflow.ImportService, error) { return newForgejoServiceForSession(kubeCtx) },
		EnsureNamespace:       ensureNamespaceForSession,
		EnsureProxy:           ensureProxyForSession,
		EnsureResources:       ensureResourcesForSession,
		EnsureForgejoImport: func(ctx context.Context, svc workflow.ImportService, repo *git.Repository) error {
			forgejoService, ok := svc.(*forgejo.ForgejoService)
			if !ok {
				return fmt.Errorf("unexpected forgejo service type %T", svc)
			}

			return ensureForgejoImportForSession(ctx, forgejoService, repo)
		},
		RunConfigurationPreflight: func(ctx context.Context, repo *git.Repository, cfg *config.Config, agentType agent.Type) (preflight.Report, error) {
			return runConfigurationPreflightForSession(ctx, repo, cfg, agentType)
		},
		RunRuntimePreflight: func(ctx context.Context, repo *git.Repository, kubeCtx string, agentType agent.Type) (preflight.Report, error) {
			return runRuntimePreflightForSession(ctx, repo, kubeCtx, agentType)
		},
		ValidateProxyPolicy: validateProxyPolicyForSession,
		LaunchAgentRuntime:  launchAgentRuntimeForSession,
		PrepareAgentState:   prepareAgentStateForSession,
		AttachAgent:         attachAgentForSession,
		OpenSessionStore:    func() (workflow.SessionStore, error) { return openSessionStore() },
		DiagnosticsReporter: newDiagnosticsReporterForSession(),
		FailureArtefacts:    newFailureArtefactCollectorForSession(),
		Now:                 nowForSession,
	}
}
