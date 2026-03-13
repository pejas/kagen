package diagnostics

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/pejas/kagen/internal/cluster"
	kagerr "github.com/pejas/kagen/internal/errors"
	"github.com/pejas/kagen/internal/session"
)

// FailureArtefactCollector captures deterministic failure diagnostics for runtime operations.
type FailureArtefactCollector interface {
	Collect(context.Context, FailureArtefactRequest) (FailureArtefactResult, error)
}

// FailureArtefactRequest describes the failed operation and any persisted session context.
type FailureArtefactRequest struct {
	Operation      Operation
	Err            error
	KubeContext    string
	SessionSummary *session.Summary
}

// FailureArtefactResult reports the location of the captured artefacts.
type FailureArtefactResult struct {
	Directory string
}

// ClusterSnapshotter captures runtime cluster diagnostics.
type ClusterSnapshotter interface {
	Collect(context.Context, cluster.DiagnosticsRequest) cluster.DiagnosticBundle
}

type failureArtefactCollector struct {
	now             func() time.Time
	userConfigDir   func() (string, error)
	newClusterProbe func(string) (ClusterSnapshotter, error)
}

// NewFailureArtefactCollector returns the default failure artefact collector.
func NewFailureArtefactCollector() FailureArtefactCollector {
	return &failureArtefactCollector{
		now:           func() time.Time { return time.Now().UTC() },
		userConfigDir: os.UserConfigDir,
		newClusterProbe: func(kubeCtx string) (ClusterSnapshotter, error) {
			return cluster.NewDiagnosticsInspector(kubeCtx)
		},
	}
}

func (c *failureArtefactCollector) Collect(ctx context.Context, req FailureArtefactRequest) (FailureArtefactResult, error) {
	baseDir, err := c.baseDir()
	if err != nil {
		return FailureArtefactResult{}, err
	}

	sessionID := sessionIDFromRequest(req)
	directory := artefactDirectory(baseDir, sessionID, req.Operation.Metadata["repo_id"], req.Operation.Name)
	if err := os.MkdirAll(directory, 0o755); err != nil {
		return FailureArtefactResult{}, fmt.Errorf("creating failure artefact directory %s: %w", directory, err)
	}

	selection := selectionSnapshot{
		CapturedAt:   c.now(),
		Error:        errorString(req.Err),
		Operation:    req.Operation.Name,
		Status:       string(req.Operation.Status),
		FailedStep:   failedStepName(req.Operation),
		FailureClass: failureClass(req.Operation, req.Err),
		Metadata:     cloneStringMap(req.Operation.Metadata),
	}
	if err := writeJSONFile(filepath.Join(directory, req.Operation.Name+"-failure.json"), selection); err != nil {
		return FailureArtefactResult{}, err
	}
	if err := writeJSONFile(filepath.Join(directory, req.Operation.Name+"-trace.json"), req.Operation); err != nil {
		return FailureArtefactResult{}, err
	}
	if err := os.WriteFile(
		filepath.Join(directory, req.Operation.Name+"-trace.txt"),
		[]byte(strings.Join(FormatSummary(req.Operation), "\n")+"\n"),
		0o644,
	); err != nil {
		return FailureArtefactResult{}, fmt.Errorf("writing trace summary: %w", err)
	}
	if req.SessionSummary != nil {
		if err := writeJSONFile(filepath.Join(directory, req.Operation.Name+"-session-summary.json"), req.SessionSummary); err != nil {
			return FailureArtefactResult{}, err
		}
	}

	captureErrors := map[string]string{}
	if req.KubeContext != "" && strings.TrimSpace(req.Operation.Metadata["namespace"]) != "" {
		probe, err := c.newClusterProbe(req.KubeContext)
		if err != nil {
			captureErrors["cluster"] = fmt.Sprintf("creating cluster diagnostics inspector: %v", err)
		} else {
			clusterCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			bundle := probe.Collect(clusterCtx, cluster.DiagnosticsRequest{
				Namespace:      req.Operation.Metadata["namespace"],
				PodName:        req.Operation.Metadata["pod_name"],
				AgentContainer: req.Operation.Metadata["agent_container"],
			})
			for key, value := range bundle.CaptureErrors {
				captureErrors[key] = value
			}
			if bundle.PodStatus != nil {
				if err := writeJSONFile(filepath.Join(directory, req.Operation.Name+"-pod-status.json"), bundle.PodStatus); err != nil {
					return FailureArtefactResult{}, err
				}
			}
			if bundle.PodEvents != nil {
				if err := writeJSONFile(filepath.Join(directory, req.Operation.Name+"-pod-events.json"), bundle.PodEvents); err != nil {
					return FailureArtefactResult{}, err
				}
			}
			if bundle.ProxyDeployment != nil {
				if err := writeJSONFile(filepath.Join(directory, req.Operation.Name+"-proxy-deployment.json"), bundle.ProxyDeployment); err != nil {
					return FailureArtefactResult{}, err
				}
			}
			for containerName, content := range bundle.ContainerLogs {
				filename := fmt.Sprintf("%s-%s.log", req.Operation.Name, containerName)
				if err := os.WriteFile(filepath.Join(directory, filename), []byte(content), 0o644); err != nil {
					return FailureArtefactResult{}, fmt.Errorf("writing container log %s: %w", containerName, err)
				}
			}
		}
	}

	if len(captureErrors) > 0 {
		if err := writeJSONFile(filepath.Join(directory, req.Operation.Name+"-capture-errors.json"), captureErrors); err != nil {
			return FailureArtefactResult{}, err
		}
	}

	return FailureArtefactResult{Directory: directory}, nil
}

type selectionSnapshot struct {
	CapturedAt   time.Time         `json:"captured_at"`
	Error        string            `json:"error"`
	Operation    string            `json:"operation"`
	Status       string            `json:"status"`
	FailedStep   string            `json:"failed_step,omitempty"`
	FailureClass string            `json:"failure_class,omitempty"`
	Metadata     map[string]string `json:"metadata,omitempty"`
}

func (c *failureArtefactCollector) baseDir() (string, error) {
	configDir, err := c.userConfigDir()
	if err != nil {
		return "", fmt.Errorf("finding user config directory: %w", err)
	}

	return filepath.Join(configDir, "kagen", "failure-artefacts"), nil
}

func writeJSONFile(path string, value any) error {
	content, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Errorf("marshalling %s: %w", filepath.Base(path), err)
	}
	content = append(content, '\n')
	if err := os.WriteFile(path, content, 0o644); err != nil {
		return fmt.Errorf("writing %s: %w", filepath.Base(path), err)
	}

	return nil
}

func artefactDirectory(baseDir string, sessionID int64, repoID, operation string) string {
	if sessionID > 0 {
		return filepath.Join(baseDir, fmt.Sprintf("session-%d", sessionID))
	}

	repoKey := strings.TrimSpace(repoID)
	if repoKey == "" {
		repoKey = "unknown"
	}

	return filepath.Join(baseDir, "pending", repoKey, operation)
}

func sessionIDFromRequest(req FailureArtefactRequest) int64 {
	if req.SessionSummary != nil && req.SessionSummary.Session.ID > 0 {
		return req.SessionSummary.Session.ID
	}

	raw := strings.TrimSpace(req.Operation.Metadata["session_id"])
	if raw == "" {
		return 0
	}

	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return 0
	}

	return value
}

func failedStepName(operation Operation) string {
	for _, step := range operation.Steps {
		if step.Status == StatusFailed {
			return step.Name
		}
	}

	return ""
}

func failureClass(operation Operation, err error) string {
	for _, step := range operation.Steps {
		if step.Status == StatusFailed && step.FailureClass != "" {
			return string(step.FailureClass)
		}
	}

	return string(kagerr.Classify(err))
}

func errorString(err error) string {
	if err == nil {
		return ""
	}

	return err.Error()
}

func cloneStringMap(input map[string]string) map[string]string {
	if len(input) == 0 {
		return nil
	}

	cloned := make(map[string]string, len(input))
	for key, value := range input {
		cloned[key] = value
	}

	return cloned
}
