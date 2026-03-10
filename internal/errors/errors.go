// Package errors defines domain-specific error types and fail-fast helpers
// for the kagen CLI.
package errors

import (
	"errors"
	"fmt"
	"os"
)

// Sentinel errors for well-known failure conditions.
var (
	ErrNotGitRepo          = errors.New("current directory is not a git repository")
	ErrRuntimeUnavailable  = errors.New("colima/k3s runtime is not available")
	ErrClusterUnhealthy    = errors.New("cluster is not in a healthy state")
	ErrProvenanceFailed    = errors.New("failed to record import provenance")
	ErrProxyNotActive      = errors.New("proxy enforcement is not active; failing closed")
	ErrNotImplemented      = errors.New("not implemented")
	ErrAlreadyInitialized  = errors.New("project is already initialized")
	ErrNoReviewableChanges = errors.New("no reviewable changes found")
	ErrAgentUnknown        = errors.New("unknown agent type")
)

// ExitError wraps an error with a specific process exit code.
type ExitError struct {
	Err  error
	Code int
}

// Error implements the error interface.
func (e *ExitError) Error() string {
	return e.Err.Error()
}

// Unwrap supports errors.Is / errors.As chains.
func (e *ExitError) Unwrap() error {
	return e.Err
}

// WithExitCode wraps err with the given exit code.
func WithExitCode(err error, code int) *ExitError {
	return &ExitError{Err: err, Code: code}
}

// FailFast prints the error to stderr and exits with the appropriate code.
// If err is an *ExitError the embedded code is used; otherwise exit code 1.
func FailFast(err error) {
	if err == nil {
		return
	}

	var exitErr *ExitError
	code := 1
	if errors.As(err, &exitErr) {
		code = exitErr.Code
	}

	fmt.Fprintf(os.Stderr, "Error: %v\n", err)
	os.Exit(code)
}
