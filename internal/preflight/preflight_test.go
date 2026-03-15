package preflight

import (
	"context"
	"errors"
	"testing"

	"github.com/pejas/kagen/internal/agent"
	"github.com/pejas/kagen/internal/config"
	kagerr "github.com/pejas/kagen/internal/errors"
	"github.com/pejas/kagen/internal/git"
	"github.com/pejas/kagen/internal/kubeexec"
)

func TestValidateConfigurationClassifiesInvalidImageReference(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Images.Workspace = "bad image"

	report, err := ValidateConfiguration(cfg, agent.Codex)
	if err == nil {
		t.Fatal("ValidateConfiguration() error = nil, want image validation failure")
	}
	if got := kagerr.Classify(err); got != kagerr.FailureClassImage {
		t.Fatalf("Classify(err) = %q, want %q", got, kagerr.FailureClassImage)
	}
	if !report.Failed() {
		t.Fatal("report.Failed() = false, want true")
	}
	if failed := report.FailedCheck(); failed == nil || failed.Name != CheckWorkspaceImage {
		t.Fatalf("failed check = %#v, want %s", failed, CheckWorkspaceImage)
	}
}

func TestRuntimeValidatorClassifiesWorkspaceFailure(t *testing.T) {
	t.Parallel()

	validator := NewRuntimeValidatorWithRunner(stubRunner{
		run: func(_ context.Context, _ string, _ string, command []string, _ ...kubeexec.Option) (string, error) {
			if command[2] == workspaceCheckScript() {
				return "", errors.New("missing mount")
			}
			return "", nil
		},
	})

	report, err := validator.Validate(context.Background(), &git.Repository{Path: "/tmp/repo", CurrentBranch: "main"}, agent.Codex)
	if err == nil {
		t.Fatal("Validate() error = nil, want workspace failure")
	}
	if got := kagerr.Classify(err); got != kagerr.FailureClassWorkspaceBootstrap {
		t.Fatalf("Classify(err) = %q, want %q", got, kagerr.FailureClassWorkspaceBootstrap)
	}
	if failed := report.FailedCheck(); failed == nil || failed.Name != CheckWorkspaceMount {
		t.Fatalf("failed check = %#v, want %s", failed, CheckWorkspaceMount)
	}
}

func TestRuntimeValidatorClassifiesBinaryFailure(t *testing.T) {
	t.Parallel()

	spec, err := agent.SpecFor(agent.Codex)
	if err != nil {
		t.Fatalf("SpecFor(codex) returned error: %v", err)
	}
	validator := NewRuntimeValidatorWithRunner(stubRunner{
		run: func(_ context.Context, _ string, _ string, command []string, _ ...kubeexec.Option) (string, error) {
			if command[2] == agent.BinaryPreflightCheck(spec) {
				return "", errors.New("command not found")
			}
			return "/opt/mise/shims/codex", nil
		},
	})

	report, err := validator.Validate(context.Background(), &git.Repository{Path: "/tmp/repo", CurrentBranch: "main"}, agent.Codex)
	if err == nil {
		t.Fatal("Validate() error = nil, want binary failure")
	}
	if got := kagerr.Classify(err); got != kagerr.FailureClassAgentBinary {
		t.Fatalf("Classify(err) = %q, want %q", got, kagerr.FailureClassAgentBinary)
	}
	if failed := report.FailedCheck(); failed == nil || failed.Name != CheckAgentBinary {
		t.Fatalf("failed check = %#v, want %s", failed, CheckAgentBinary)
	}
}

func TestRuntimeValidatorClassifiesHomeFailure(t *testing.T) {
	t.Parallel()

	spec, err := agent.SpecFor(agent.Codex)
	if err != nil {
		t.Fatalf("SpecFor(codex) returned error: %v", err)
	}
	validator := NewRuntimeValidatorWithRunner(stubRunner{
		run: func(_ context.Context, _ string, _ string, command []string, _ ...kubeexec.Option) (string, error) {
			if command[2] == homeCheckScript(spec.StateRoot()) {
				return "", errors.New("permission denied")
			}
			return "/opt/mise/shims/codex", nil
		},
	})

	report, err := validator.Validate(context.Background(), &git.Repository{Path: "/tmp/repo", CurrentBranch: "main"}, agent.Codex)
	if err == nil {
		t.Fatal("Validate() error = nil, want home failure")
	}
	if got := kagerr.Classify(err); got != kagerr.FailureClassAgentHome {
		t.Fatalf("Classify(err) = %q, want %q", got, kagerr.FailureClassAgentHome)
	}
	if failed := report.FailedCheck(); failed == nil || failed.Name != CheckAgentHome {
		t.Fatalf("failed check = %#v, want %s", failed, CheckAgentHome)
	}
}

type stubRunner struct {
	run func(context.Context, string, string, []string, ...kubeexec.Option) (string, error)
}

func (s stubRunner) Run(ctx context.Context, namespace, pod string, command []string, opts ...kubeexec.Option) (string, error) {
	return s.run(ctx, namespace, pod, command, opts...)
}

func (s stubRunner) Attach(context.Context, string, string, []string, ...kubeexec.Option) error {
	return nil
}

func (s stubRunner) WaitForPodReady(context.Context, string, string, string) error {
	return nil
}
