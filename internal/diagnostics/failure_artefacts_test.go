package diagnostics

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/pejas/kagen/internal/cluster"
	kagerr "github.com/pejas/kagen/internal/errors"
	"github.com/pejas/kagen/internal/session"
)

func TestFailureArtefactCollectorWritesDeterministicSessionFiles(t *testing.T) {
	t.Parallel()

	configDir := t.TempDir()
	collector := &failureArtefactCollector{
		now:           func() time.Time { return time.Date(2026, time.March, 13, 12, 0, 0, 0, time.UTC) },
		userConfigDir: func() (string, error) { return configDir, nil },
		newClusterProbe: func(string) (ClusterSnapshotter, error) {
			return stubClusterSnapshotter{
				bundle: cluster.DiagnosticBundle{
					PodStatus: &cluster.PodSnapshot{
						Name:      "agent",
						Namespace: "kagen-repo-1",
						Phase:     "Pending",
					},
					PodEvents: []cluster.EventSnapshot{{
						Reason:  "Failed",
						Message: "Image pull back-off",
					}},
					ContainerLogs: map[string]string{
						"workspace-sync":    "clone failed\n",
						"kagen-agent-codex": "runtime failed\n",
					},
					ProxyDeployment: &cluster.DeploymentSnapshot{
						Name:          "egress-proxy",
						Namespace:     "kagen-repo-1",
						ReadyReplicas: 0,
					},
					CaptureErrors: map[string]string{
						"logs.proxy": "not collected",
					},
				},
			}, nil
		},
	}

	operation := Operation{
		Name:   "attach",
		Status: StatusFailed,
		Metadata: map[string]string{
			"session_id":      "42",
			"repo_id":         "repo-1",
			"namespace":       "kagen-repo-1",
			"pod_name":        "agent",
			"agent_container": "kagen-agent-codex",
			"agent_type":      "codex",
		},
		Steps: []StepRecord{
			{Name: "ensure_runtime", Status: StatusSucceeded},
			{Name: "prepare_agent_state", Status: StatusFailed, ErrorSummary: "permission denied", FailureClass: kagerr.FailureClassAgentHome},
		},
	}
	summary := &session.Summary{
		Session: session.KagenSession{
			ID:        42,
			UID:       "session-uid",
			RepoID:    "repo-1",
			Namespace: "kagen-repo-1",
			PodName:   "agent",
			Status:    "failed",
		},
		AgentTypes: []string{"codex"},
	}

	result, err := collector.Collect(context.Background(), FailureArtefactRequest{
		Operation:      operation,
		Err:            errors.New("attach failed at step prepare_agent_state: permission denied"),
		KubeContext:    "kagen-test",
		SessionSummary: summary,
	})
	if err != nil {
		t.Fatalf("Collect() returned error: %v", err)
	}

	expectedDir := filepath.Join(configDir, "kagen", "failure-artefacts", "session-42")
	if result.Directory != expectedDir {
		t.Fatalf("directory = %q, want %q", result.Directory, expectedDir)
	}

	expectedFiles := []string{
		"attach-capture-errors.json",
		"attach-failure.json",
		"attach-kagen-agent-codex.log",
		"attach-pod-events.json",
		"attach-pod-status.json",
		"attach-proxy-deployment.json",
		"attach-session-summary.json",
		"attach-trace.json",
		"attach-trace.txt",
		"attach-workspace-sync.log",
	}
	for _, name := range expectedFiles {
		if _, err := os.Stat(filepath.Join(result.Directory, name)); err != nil {
			t.Fatalf("expected file %s: %v", name, err)
		}
	}

	traceSummary, err := os.ReadFile(filepath.Join(result.Directory, "attach-trace.txt"))
	if err != nil {
		t.Fatalf("ReadFile(trace.txt) returned error: %v", err)
	}
	if !strings.Contains(string(traceSummary), "step prepare_agent_state: failed") {
		t.Fatalf("trace summary = %q, want failed step", string(traceSummary))
	}

	var captureErrors map[string]string
	if err := readJSONFile(filepath.Join(result.Directory, "attach-capture-errors.json"), &captureErrors); err != nil {
		t.Fatalf("readJSONFile(capture-errors) returned error: %v", err)
	}
	if captureErrors["logs.proxy"] != "not collected" {
		t.Fatalf("capture error logs.proxy = %q, want not collected", captureErrors["logs.proxy"])
	}

	var failure selectionSnapshot
	if err := readJSONFile(filepath.Join(result.Directory, "attach-failure.json"), &failure); err != nil {
		t.Fatalf("readJSONFile(failure) returned error: %v", err)
	}
	if failure.FailureClass != string(kagerr.FailureClassAgentHome) {
		t.Fatalf("failure class = %q, want %q", failure.FailureClass, kagerr.FailureClassAgentHome)
	}
}

func TestFailureArtefactCollectorFallsBackToPendingDirectoryWithoutSession(t *testing.T) {
	t.Parallel()

	configDir := t.TempDir()
	collector := &failureArtefactCollector{
		now:           func() time.Time { return time.Date(2026, time.March, 13, 13, 0, 0, 0, time.UTC) },
		userConfigDir: func() (string, error) { return configDir, nil },
		newClusterProbe: func(string) (ClusterSnapshotter, error) {
			return stubClusterSnapshotter{}, nil
		},
	}

	result, err := collector.Collect(context.Background(), FailureArtefactRequest{
		Operation: Operation{
			Name:   "start",
			Status: StatusFailed,
			Metadata: map[string]string{
				"repo_id":    "repo-2",
				"namespace":  "kagen-repo-2",
				"pod_name":   "agent",
				"agent_type": "codex",
			},
			Steps: []StepRecord{{Name: "ensure_runtime", Status: StatusFailed}},
		},
		Err: errors.New("start failed at step ensure_runtime: runtime not available"),
	})
	if err != nil {
		t.Fatalf("Collect() returned error: %v", err)
	}

	expectedDir := filepath.Join(configDir, "kagen", "failure-artefacts", "pending", "repo-2", "start")
	if result.Directory != expectedDir {
		t.Fatalf("directory = %q, want %q", result.Directory, expectedDir)
	}
}

type stubClusterSnapshotter struct {
	bundle cluster.DiagnosticBundle
}

func (s stubClusterSnapshotter) Collect(context.Context, cluster.DiagnosticsRequest) cluster.DiagnosticBundle {
	return s.bundle
}

func readJSONFile(path string, target any) error {
	content, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	return json.Unmarshal(content, target)
}
