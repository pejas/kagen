package preflight

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/pejas/kagen/internal/agent"
	"github.com/pejas/kagen/internal/config"
	kagerr "github.com/pejas/kagen/internal/errors"
	"github.com/pejas/kagen/internal/git"
	"github.com/pejas/kagen/internal/kubeexec"
	"github.com/pejas/kagen/internal/proxy"
	"github.com/pejas/kagen/internal/workload"
)

type Scope string

const (
	ScopeConfiguration Scope = "configuration"
	ScopeRuntime       Scope = "runtime"
)

type CheckStatus string

const (
	CheckStatusPassed CheckStatus = "passed"
	CheckStatusFailed CheckStatus = "failed"
)

const (
	CheckWorkspaceImage = "workspace_image"
	CheckToolboxImage   = "toolbox_image"
	CheckProxyImage     = "proxy_image"
	CheckProxyPolicy    = "proxy_policy"
	CheckWorkspaceMount = "workspace_mount"
	CheckAgentBinary    = "agent_binary"
	CheckAgentHome      = "agent_home"
)

// Check reports one preflight check result.
type Check struct {
	Name         string              `json:"name"`
	Status       CheckStatus         `json:"status"`
	Summary      string              `json:"summary,omitempty"`
	FailureClass kagerr.FailureClass `json:"failure_class,omitempty"`
	Metadata     map[string]string   `json:"metadata,omitempty"`
}

// Report groups the results for one preflight scope.
type Report struct {
	Scope  Scope             `json:"scope"`
	Checks []Check           `json:"checks"`
	Meta   map[string]string `json:"metadata,omitempty"`
}

func (r Report) Failed() bool {
	return r.FailedCheck() != nil
}

func (r Report) FailedCheck() *Check {
	for i := range r.Checks {
		if r.Checks[i].Status == CheckStatusFailed {
			return &r.Checks[i]
		}
	}

	return nil
}

// Metadata flattens the report for step-level diagnostics metadata.
func (r Report) Metadata() map[string]string {
	metadata := map[string]string{
		"preflight_scope":       string(r.Scope),
		"preflight_status":      string(CheckStatusPassed),
		"preflight_check_count": fmt.Sprintf("%d", len(r.Checks)),
	}
	for key, value := range r.Meta {
		if strings.TrimSpace(key) == "" || strings.TrimSpace(value) == "" {
			continue
		}
		metadata["preflight_"+metadataKey(key)] = value
	}
	if failed := r.FailedCheck(); failed != nil {
		metadata["preflight_status"] = string(CheckStatusFailed)
		metadata["preflight_failed_check"] = failed.Name
		if failed.FailureClass != "" {
			metadata["preflight_failure_class"] = string(failed.FailureClass)
			metadata["failure_class"] = string(failed.FailureClass)
		}
	}

	for _, check := range r.Checks {
		checkKey := "preflight_" + metadataKey(check.Name)
		metadata[checkKey+"_status"] = string(check.Status)
		if check.FailureClass != "" {
			metadata[checkKey+"_failure_class"] = string(check.FailureClass)
		}
		for key, value := range check.Metadata {
			if strings.TrimSpace(key) == "" || strings.TrimSpace(value) == "" {
				continue
			}
			metadata[checkKey+"_"+metadataKey(key)] = value
		}
	}

	return metadata
}

func ValidateConfiguration(cfg *config.Config, agentType agent.Type) (Report, error) {
	report := Report{
		Scope: ScopeConfiguration,
		Meta: map[string]string{
			"agent_type": string(agentType),
		},
	}

	images := workload.Images{
		Workspace: cfg.Images.Workspace,
		Toolbox:   cfg.Images.Toolbox,
	}
	if failed := validateImageRef(CheckWorkspaceImage, images.Workspace); failed != nil {
		report.Checks = append(report.Checks, *failed)
		return report, kagerr.WithFailureClass(
			kagerr.FailureClassImage,
			fmt.Sprintf("preflight image check %s failed", CheckWorkspaceImage),
			fmt.Errorf("invalid image reference %q", images.Workspace),
		)
	}
	report.Checks = append(report.Checks, Check{
		Name:    CheckWorkspaceImage,
		Status:  CheckStatusPassed,
		Summary: "workspace image reference resolved",
		Metadata: map[string]string{
			"ref": images.Workspace,
		},
	})

	if failed := validateImageRef(CheckToolboxImage, images.Toolbox); failed != nil {
		report.Checks = append(report.Checks, *failed)
		return report, kagerr.WithFailureClass(
			kagerr.FailureClassImage,
			fmt.Sprintf("preflight image check %s failed", CheckToolboxImage),
			fmt.Errorf("invalid image reference %q", images.Toolbox),
		)
	}
	report.Checks = append(report.Checks, Check{
		Name:    CheckToolboxImage,
		Status:  CheckStatusPassed,
		Summary: "toolbox image reference resolved",
		Metadata: map[string]string{
			"ref": images.Toolbox,
		},
	})

	proxyRef := cfg.Images.Proxy
	if failed := validateImageRef(CheckProxyImage, proxyRef); failed != nil {
		report.Checks = append(report.Checks, *failed)
		return report, kagerr.WithFailureClass(
			kagerr.FailureClassImage,
			fmt.Sprintf("preflight image check %s failed", CheckProxyImage),
			fmt.Errorf("invalid image reference %q", proxyRef),
		)
	}
	report.Checks = append(report.Checks, Check{
		Name:    CheckProxyImage,
		Status:  CheckStatusPassed,
		Summary: "proxy image reference resolved",
		Metadata: map[string]string{
			"ref": proxyRef,
		},
	})

	allowedDestinations := proxy.LoadPolicy(cfg, string(agentType)).AllowedDestinations
	report.Checks = append(report.Checks, Check{
		Name:    CheckProxyPolicy,
		Status:  CheckStatusPassed,
		Summary: "proxy policy configuration resolved",
		Metadata: map[string]string{
			"destination_count": fmt.Sprintf("%d", len(allowedDestinations)),
		},
	})

	return report, nil
}

type RuntimeValidator struct {
	runner kubeexec.Runner
}

func NewRuntimeValidator(kubeCtx string) *RuntimeValidator {
	return &RuntimeValidator{runner: kubeexec.NewRunner(kubeCtx)}
}

func NewRuntimeValidatorWithRunner(runner kubeexec.Runner) *RuntimeValidator {
	return &RuntimeValidator{runner: runner}
}

func (v *RuntimeValidator) Validate(ctx context.Context, repo *git.Repository, agentType agent.Type) (Report, error) {
	report := Report{
		Scope: ScopeRuntime,
		Meta: map[string]string{
			"agent_type": string(agentType),
		},
	}

	spec, err := agent.SpecFor(agentType)
	if err != nil {
		return report, err
	}
	namespace := fmt.Sprintf("kagen-%s", repo.ID())
	opts := []kubeexec.Option{kubeexec.WithContainer(agent.ContainerName(spec))}

	if err := v.runCheck(ctx, namespace, spec, CheckWorkspaceMount, workspaceCheckScript(), opts, &report); err != nil {
		return report, err
	}
	if err := v.runCheck(ctx, namespace, spec, CheckAgentBinary, agent.BinaryPreflightCheck(spec), opts, &report); err != nil {
		return report, err
	}
	if err := v.runCheck(ctx, namespace, spec, CheckAgentHome, homeCheckScript(spec.StateRoot()), opts, &report); err != nil {
		return report, err
	}

	return report, nil
}

func (v *RuntimeValidator) runCheck(
	ctx context.Context,
	namespace string,
	spec agent.RuntimeSpec,
	name string,
	script string,
	opts []kubeexec.Option,
	report *Report,
) error {
	output, err := v.runner.Run(ctx, namespace, "agent", []string{"/bin/sh", "-lc", script}, opts...)
	metadata := checkMetadata(name, spec, strings.TrimSpace(output))
	if err == nil {
		report.Checks = append(report.Checks, Check{
			Name:     name,
			Status:   CheckStatusPassed,
			Summary:  passedSummary(name, spec),
			Metadata: metadata,
		})
		return nil
	}

	failed := Check{
		Name:         name,
		Status:       CheckStatusFailed,
		Summary:      failedSummary(name, spec),
		FailureClass: failureClass(name),
		Metadata:     metadata,
	}
	report.Checks = append(report.Checks, failed)

	return kagerr.WithFailureClass(failed.FailureClass, failed.Summary, err)
}

func validateImageRef(name, ref string) *Check {
	trimmed := strings.TrimSpace(ref)
	if trimmed == "" || strings.Contains(trimmed, "://") || strings.ContainsAny(trimmed, " \t\r\n") || strings.HasSuffix(trimmed, ":") || strings.HasSuffix(trimmed, "@") {
		return &Check{
			Name:         name,
			Status:       CheckStatusFailed,
			Summary:      "image reference is invalid",
			FailureClass: kagerr.FailureClassImage,
			Metadata: map[string]string{
				"ref": ref,
			},
		}
	}

	return nil
}

func workspaceCheckScript() string {
	return `test -d /projects/workspace && test -d /projects/workspace/.git && test -w /projects/workspace`
}

func homeCheckScript(stateRoot string) string {
	return fmt.Sprintf(`mkdir -p %q && test -w %q && printf '%%s' %q`, stateRoot, stateRoot, stateRoot)
}

func checkMetadata(name string, spec agent.RuntimeSpec, output string) map[string]string {
	switch name {
	case CheckWorkspaceMount:
		return map[string]string{"path": "/projects/workspace"}
	case CheckAgentBinary:
		metadata := map[string]string{"binary": spec.Binary()}
		if output != "" {
			metadata["path"] = output
		}
		return metadata
	case CheckAgentHome:
		return map[string]string{"path": spec.StateRoot()}
	default:
		return nil
	}
}

func passedSummary(name string, spec agent.RuntimeSpec) string {
	switch name {
	case CheckWorkspaceMount:
		return "workspace mount is present and writable"
	case CheckAgentBinary:
		return "agent runtime binary is present"
	case CheckAgentHome:
		return "agent home root is writable"
	default:
		return "preflight check passed"
	}
}

func failedSummary(name string, spec agent.RuntimeSpec) string {
	switch name {
	case CheckWorkspaceMount:
		return "workspace bootstrap did not produce a writable /projects/workspace checkout"
	case CheckAgentBinary:
		return fmt.Sprintf("runtime binary %q is not available in the toolbox image", spec.Binary())
	case CheckAgentHome:
		return fmt.Sprintf("agent home root %q is not writable", spec.StateRoot())
	default:
		return "preflight check failed"
	}
}

func failureClass(name string) kagerr.FailureClass {
	switch name {
	case CheckWorkspaceMount:
		return kagerr.FailureClassWorkspaceBootstrap
	case CheckAgentBinary:
		return kagerr.FailureClassAgentBinary
	case CheckAgentHome:
		return kagerr.FailureClassAgentHome
	default:
		return ""
	}
}

var metadataKeyPattern = regexp.MustCompile(`[^a-z0-9]+`)

func metadataKey(value string) string {
	key := strings.ToLower(strings.TrimSpace(value))
	key = metadataKeyPattern.ReplaceAllString(key, "_")
	key = strings.Trim(key, "_")
	if key == "" {
		return "value"
	}

	return key
}
