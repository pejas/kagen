package workflow

import (
	"context"
	"fmt"
	"strings"

	"github.com/pejas/kagen/internal/agent"
	"github.com/pejas/kagen/internal/cluster"
	"github.com/pejas/kagen/internal/config"
	"github.com/pejas/kagen/internal/diagnostics"
	"github.com/pejas/kagen/internal/git"
	"github.com/pejas/kagen/internal/proxy"
	"github.com/pejas/kagen/internal/runtime"
	"github.com/pejas/kagen/internal/session"
	"github.com/pejas/kagen/internal/ui"
	corev1 "k8s.io/api/core/v1"
)

type DoctorSessionStore interface {
	Close() error
	GetSummary(context.Context, int64) (session.Summary, bool, error)
	List(context.Context, session.ListOptions) ([]session.Summary, error)
}

type DoctorClusterInspector interface {
	Collect(context.Context, cluster.DiagnosticsRequest) cluster.DiagnosticBundle
}

type DoctorDependencies struct {
	LoadConfig              func() (*config.Config, error)
	DiscoverRepository      func() (*git.Repository, error)
	OpenSessionStore        func() (DoctorSessionStore, error)
	LoadLatestOperation     func(int64) (diagnostics.Operation, bool, error)
	FindFailureArtefactDir  func(int64) (string, bool, error)
	RuntimeStatus           func(context.Context, *config.Config) (runtime.Status, string, error)
	NewDiagnosticsInspector func(string) (DoctorClusterInspector, error)
}

type DoctorWorkflow struct {
	deps DoctorDependencies
}

func NewDoctorWorkflow(deps DoctorDependencies) *DoctorWorkflow {
	return &DoctorWorkflow{deps: deps}
}

func (w *DoctorWorkflow) Run(ctx context.Context, sessionID int64, sessionSelected bool) (err error) {
	ctx = contextOrBackground(ctx)

	cfg, err := w.deps.LoadConfig()
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

	summary, err := resolveDoctorSummary(ctx, store, w.deps.DiscoverRepository, sessionID, sessionSelected)
	if err != nil {
		return err
	}

	latestOperation, operationFound, err := w.deps.LoadLatestOperation(summary.Session.ID)
	if err != nil {
		return fmt.Errorf("loading latest operation trace: %w", err)
	}
	failureDir, failureDirFound, err := w.deps.FindFailureArtefactDir(summary.Session.ID)
	if err != nil {
		return fmt.Errorf("loading failure artefact directory: %w", err)
	}

	runtimeStatus, kubeCtx, runtimeErr := w.deps.RuntimeStatus(ctx, cfg)
	if runtimeErr != nil {
		runtimeStatus = runtime.StatusUnknown
	}

	agentType := resolvedDoctorAgentType(summary, latestOperation, operationFound)
	agentContainer := resolvedAgentContainer(agentType, latestOperation, operationFound)

	var bundle cluster.DiagnosticBundle
	var clusterErr error
	if runtimeErr == nil && runtimeStatus == runtime.StatusRunning && strings.TrimSpace(kubeCtx) != "" {
		inspector, err := w.deps.NewDiagnosticsInspector(kubeCtx)
		if err != nil {
			clusterErr = fmt.Errorf("creating diagnostics inspector: %w", err)
		} else {
			bundle = inspector.Collect(ctx, cluster.DiagnosticsRequest{
				Namespace:      summary.Session.Namespace,
				PodName:        summary.Session.PodName,
				AgentContainer: agentContainer,
			})
		}
	}

	ui.Header("Doctor")
	ui.Info("Session %d: %s (branch %s)", summary.Session.ID, summary.Session.Status, summary.Session.WorkspaceBranch)
	ui.Info("Namespace: %s", summary.Session.Namespace)
	ui.Info("Runtime: %s%s", runtimeStatus.String(), doctorSuffix(runtimeErr))

	for _, line := range podStateLines(summary, bundle, runtimeStatus, runtimeErr, clusterErr) {
		ui.Info("%s", line)
	}
	for _, line := range proxyStateLines(cfg, agentType, bundle, runtimeStatus, runtimeErr, clusterErr) {
		ui.Info("%s", line)
	}

	if failureDirFound {
		ui.Info("Failure artefacts: %s", failureDir)
	} else {
		ui.Info("Failure artefacts: none captured for this session")
	}

	ui.Header("Latest trace")
	if operationFound {
		for _, line := range diagnostics.FormatSummary(latestOperation) {
			ui.Info("%s", line)
		}
	} else {
		ui.Info("No persisted trace captured for this session yet.")
	}

	return nil
}

func resolveDoctorSummary(
	ctx context.Context,
	store DoctorSessionStore,
	discoverRepository func() (*git.Repository, error),
	sessionID int64,
	sessionSelected bool,
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

	summaries, err := store.List(ctx, session.ListOptions{RepoPath: repo.Path})
	if err != nil {
		return session.Summary{}, fmt.Errorf("listing sessions for %s: %w", repo.Path, err)
	}
	if len(summaries) == 0 {
		return session.Summary{}, fmt.Errorf("no persisted sessions found for %s: run 'kagen start <agent>' first", repo.Path)
	}

	return summaries[0], nil
}

func resolvedDoctorAgentType(summary session.Summary, operation diagnostics.Operation, operationFound bool) string {
	if operationFound {
		if agentType := strings.TrimSpace(operation.Metadata["agent_type"]); agentType != "" {
			return agentType
		}
	}

	latest := latestAgentSession(summary)
	return latest.AgentType
}

func resolvedAgentContainer(agentType string, operation diagnostics.Operation, operationFound bool) string {
	if operationFound {
		if container := strings.TrimSpace(operation.Metadata["agent_container"]); container != "" {
			return container
		}
	}
	if strings.TrimSpace(agentType) == "" {
		return ""
	}

	spec, err := agent.SpecFor(agent.Type(agentType))
	if err != nil {
		return ""
	}

	return spec.ContainerName()
}

func latestAgentSession(summary session.Summary) session.AgentSession {
	var latest session.AgentSession
	for _, agentSession := range summary.AgentSessions {
		if latest.ID == "" || agentSession.LastUsedAt.After(latest.LastUsedAt) {
			latest = agentSession
		}
	}

	return latest
}

func podStateLines(
	summary session.Summary,
	bundle cluster.DiagnosticBundle,
	runtimeStatus runtime.Status,
	runtimeErr error,
	clusterErr error,
) []string {
	if runtimeErr != nil {
		return []string{fmt.Sprintf("Pod: inspection unavailable (%v)", runtimeErr)}
	}
	if runtimeStatus != runtime.StatusRunning {
		return []string{"Pod: cluster inspection skipped because the runtime is not running"}
	}
	if clusterErr != nil {
		return []string{fmt.Sprintf("Pod: inspection unavailable (%v)", clusterErr)}
	}
	if bundle.PodStatus == nil {
		if message, ok := bundle.CaptureErrors["pod_status"]; ok && strings.TrimSpace(message) != "" {
			return []string{fmt.Sprintf("Pod: unavailable (%s)", message)}
		}
		return []string{fmt.Sprintf("Pod: unavailable in %s/%s", summary.Session.Namespace, summary.Session.PodName)}
	}

	lines := []string{
		fmt.Sprintf("Pod: %s (%s/%s)", strings.ToLower(string(bundle.PodStatus.Phase)), bundle.PodStatus.Namespace, bundle.PodStatus.Name),
	}

	initState := formatContainerStatuses("Init containers", bundle.PodStatus.InitContainerStatuses)
	if initState != "" {
		lines = append(lines, initState)
	}
	containerState := formatContainerStatuses("Containers", bundle.PodStatus.ContainerStatuses)
	if containerState != "" {
		lines = append(lines, containerState)
	}

	return lines
}

func proxyStateLines(
	cfg *config.Config,
	agentType string,
	bundle cluster.DiagnosticBundle,
	runtimeStatus runtime.Status,
	runtimeErr error,
	clusterErr error,
) []string {
	policy := proxy.LoadPolicy(cfg, agentType)
	if len(policy.AllowedDestinations) == 0 {
		return []string{"Proxy: disabled for this session"}
	}

	if runtimeErr != nil {
		return []string{fmt.Sprintf("Proxy: inspection unavailable (%v)", runtimeErr)}
	}
	if runtimeStatus != runtime.StatusRunning {
		return []string{
			fmt.Sprintf(
				"Proxy: configured for %d destination(s); enforcement cannot be checked while the runtime is stopped",
				len(policy.AllowedDestinations),
			),
		}
	}
	if clusterErr != nil {
		return []string{fmt.Sprintf("Proxy: inspection unavailable (%v)", clusterErr)}
	}
	if bundle.ProxyDeployment == nil {
		if message, ok := bundle.CaptureErrors["proxy_deployment"]; ok && strings.TrimSpace(message) != "" {
			return []string{fmt.Sprintf("Proxy: unavailable (%s)", message)}
		}
		return []string{"Proxy: unavailable"}
	}

	enforced := bundle.ProxyDeployment.ReadyReplicas > 0
	state := "not enforced"
	if enforced {
		state = "enforced"
	}

	return []string{
		fmt.Sprintf(
			"Proxy: %s (%d/%d ready, %d destination(s))",
			state,
			bundle.ProxyDeployment.ReadyReplicas,
			bundle.ProxyDeployment.Replicas,
			len(policy.AllowedDestinations),
		),
	}
}

func formatContainerStatuses(prefix string, statuses []corev1.ContainerStatus) string {
	if len(statuses) == 0 {
		return ""
	}

	parts := make([]string, 0, len(statuses))
	for _, status := range statuses {
		parts = append(parts, fmt.Sprintf("%s=%s", status.Name, containerState(status)))
	}

	return fmt.Sprintf("%s: %s", prefix, strings.Join(parts, ", "))
}

func containerState(status corev1.ContainerStatus) string {
	if status.Ready {
		return "ready"
	}
	if status.State.Running != nil {
		return "running"
	}
	if status.State.Waiting != nil {
		return "waiting:" + strings.ToLower(status.State.Waiting.Reason)
	}
	if status.State.Terminated != nil {
		reason := strings.TrimSpace(status.State.Terminated.Reason)
		if reason == "" {
			reason = fmt.Sprintf("exit-%d", status.State.Terminated.ExitCode)
		}
		return "terminated:" + strings.ToLower(reason)
	}

	return "unknown"
}

func doctorSuffix(err error) string {
	if err == nil {
		return ""
	}

	return fmt.Sprintf(" (%v)", err)
}
