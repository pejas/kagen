package errors

import (
	"errors"
	"testing"
)

func TestWithFailureClassWrapsAndClassifies(t *testing.T) {
	t.Parallel()

	base := errors.New("permission denied")
	err := WithFailureClass(FailureClassAgentHome, "agent home is not writable", base)

	if got := Classify(err); got != FailureClassAgentHome {
		t.Fatalf("Classify(err) = %q, want %q", got, FailureClassAgentHome)
	}
	if !errors.Is(err, base) {
		t.Fatalf("errors.Is(err, base) = false, want true")
	}
	if got := err.Error(); got != "agent home is not writable: permission denied" {
		t.Fatalf("err.Error() = %q, want wrapped summary", got)
	}
}
