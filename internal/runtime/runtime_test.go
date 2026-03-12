package runtime

import (
	"context"
	"errors"
	"testing"

	kagerr "github.com/pejas/kagen/internal/errors"
)

func TestStubManagerEnsureRunning(t *testing.T) {
	t.Parallel()

	mgr := NewStubManager()
	err := mgr.EnsureRunning(context.Background())
	if !errors.Is(err, kagerr.ErrNotImplemented) {
		t.Errorf("expected ErrNotImplemented, got %v", err)
	}
}

func TestStubManagerStatus(t *testing.T) {
	t.Parallel()

	mgr := NewStubManager()
	status, err := mgr.Status(context.Background())
	if !errors.Is(err, kagerr.ErrNotImplemented) {
		t.Errorf("expected ErrNotImplemented, got %v", err)
	}
	if status != StatusUnknown {
		t.Errorf("expected StatusUnknown, got %v", status)
	}
}

func TestStubManagerStop(t *testing.T) {
	t.Parallel()

	mgr := NewStubManager()
	err := mgr.Stop(context.Background())
	if !errors.Is(err, kagerr.ErrNotImplemented) {
		t.Errorf("expected ErrNotImplemented, got %v", err)
	}
}

func TestStatusString(t *testing.T) {
	t.Parallel()

	tests := []struct {
		status Status
		want   string
	}{
		{StatusRunning, "running"},
		{StatusStopped, "stopped"},
		{StatusUnknown, "unknown"},
	}

	for _, tc := range tests {
		if got := tc.status.String(); got != tc.want {
			t.Errorf("Status(%d).String() = %q, want %q", tc.status, got, tc.want)
		}
	}
}
