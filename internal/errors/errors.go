// Package errors defines domain-specific error types and fail-fast helpers
// for the kagen CLI.
package errors

import (
	"errors"
	"fmt"
	"os"
	"strings"
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

// FailureClass describes a machine-readable runtime failure category.
type FailureClass string

const (
	FailureClassImage              FailureClass = "image_error"
	FailureClassWorkspaceBootstrap FailureClass = "workspace_bootstrap_error"
	FailureClassProxy              FailureClass = "proxy_error"
	FailureClassAgentBinary        FailureClass = "agent_binary_error"
	FailureClassAgentHome          FailureClass = "agent_home_error"
	FailureClassAttach             FailureClass = "attach_error"
)

// ClassifiedError wraps an error with a stable failure class.
type ClassifiedError struct {
	Class   FailureClass
	Summary string
	Err     error
}

// Error implements the error interface.
func (e *ClassifiedError) Error() string {
	if e == nil {
		return ""
	}

	summary := strings.TrimSpace(e.Summary)
	switch {
	case summary != "" && e.Err != nil:
		return fmt.Sprintf("%s: %v", summary, e.Err)
	case summary != "":
		return summary
	case e.Err != nil:
		return e.Err.Error()
	default:
		return ""
	}
}

// Unwrap supports errors.Is / errors.As chains.
func (e *ClassifiedError) Unwrap() error {
	if e == nil {
		return nil
	}

	return e.Err
}

// WithFailureClass wraps err with a stable failure class and summary.
func WithFailureClass(class FailureClass, summary string, err error) error {
	if class == "" {
		return err
	}

	return &ClassifiedError{
		Class:   class,
		Summary: summary,
		Err:     err,
	}
}

// Classify extracts the stable failure class from err when present.
func Classify(err error) FailureClass {
	var classified *ClassifiedError
	if errors.As(err, &classified) {
		return classified.Class
	}

	return ""
}

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
