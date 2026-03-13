package diagnostics

import (
	"errors"
	"testing"
	"time"

	kagerr "github.com/pejas/kagen/internal/errors"
)

func TestRecorderTracksSuccessfulAndFailedSteps(t *testing.T) {
	nowValues := []time.Time{
		time.Date(2026, time.March, 13, 10, 0, 0, 0, time.UTC),
		time.Date(2026, time.March, 13, 10, 0, 1, 0, time.UTC),
		time.Date(2026, time.March, 13, 10, 0, 3, 0, time.UTC),
		time.Date(2026, time.March, 13, 10, 0, 4, 0, time.UTC),
		time.Date(2026, time.March, 13, 10, 0, 6, 0, time.UTC),
		time.Date(2026, time.March, 13, 10, 0, 7, 0, time.UTC),
	}
	nextNow := func() func() time.Time {
		index := 0
		return func() time.Time {
			value := nowValues[index]
			index++
			return value
		}
	}()

	recorder := NewRecorder("start", []string{"ensure_runtime", "ensure_resources", "attach_agent"}, nextNow, nil)
	recorder.AddMetadata("agent_type", "codex")

	if err := recorder.RunStep("ensure_runtime", func(step *StepContext) error {
		step.AddMetadata("kube_context", "kagen-test")
		return nil
	}); err != nil {
		t.Fatalf("ensure_runtime returned error: %v", err)
	}

	expectedErr := kagerr.WithFailureClass(kagerr.FailureClassImage, "image pull denied", errors.New("pod image pull failed"))
	err := recorder.RunStep("ensure_resources", func(step *StepContext) error {
		step.AddMetadata("pod_name", "agent")
		return expectedErr
	})
	if err == nil {
		t.Fatal("ensure_resources error = nil, want step failure")
	}

	var stepErr *StepError
	if !errors.As(err, &stepErr) {
		t.Fatalf("error type = %T, want *StepError", err)
	}
	if stepErr.Step != "ensure_resources" {
		t.Fatalf("step name = %q, want ensure_resources", stepErr.Step)
	}
	if stepErr.FailureClass != kagerr.FailureClassImage {
		t.Fatalf("failure class = %q, want %q", stepErr.FailureClass, kagerr.FailureClassImage)
	}

	operation := recorder.Complete()
	if operation.Status != StatusFailed {
		t.Fatalf("operation status = %q, want failed", operation.Status)
	}
	if len(operation.Steps) != 3 {
		t.Fatalf("step count = %d, want 3", len(operation.Steps))
	}
	if operation.Steps[0].Status != StatusSucceeded {
		t.Fatalf("ensure_runtime status = %q, want succeeded", operation.Steps[0].Status)
	}
	if operation.Steps[0].Duration != 2*time.Second {
		t.Fatalf("ensure_runtime duration = %s, want 2s", operation.Steps[0].Duration)
	}
	if operation.Steps[1].Status != StatusFailed {
		t.Fatalf("ensure_resources status = %q, want failed", operation.Steps[1].Status)
	}
	if operation.Steps[1].ErrorSummary != expectedErr.Error() {
		t.Fatalf("error summary = %q, want %q", operation.Steps[1].ErrorSummary, expectedErr.Error())
	}
	if operation.Steps[1].FailureClass != kagerr.FailureClassImage {
		t.Fatalf("failure class = %q, want %q", operation.Steps[1].FailureClass, kagerr.FailureClassImage)
	}
	if operation.Steps[1].Metadata["failure_class"] != string(kagerr.FailureClassImage) {
		t.Fatalf("failure metadata = %q, want %q", operation.Steps[1].Metadata["failure_class"], kagerr.FailureClassImage)
	}
	if operation.Steps[2].Status != StatusPending {
		t.Fatalf("attach_agent status = %q, want pending", operation.Steps[2].Status)
	}
	if operation.Metadata["agent_type"] != "codex" {
		t.Fatalf("operation metadata agent_type = %q, want codex", operation.Metadata["agent_type"])
	}
}
