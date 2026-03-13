package workflow

import (
	"bytes"
	"context"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/pejas/kagen/internal/cluster"
	"github.com/pejas/kagen/internal/config"
	"github.com/pejas/kagen/internal/diagnostics"
	"github.com/pejas/kagen/internal/git"
	"github.com/pejas/kagen/internal/runtime"
	"github.com/pejas/kagen/internal/session"
	corev1 "k8s.io/api/core/v1"
)

func TestDoctorWorkflowReportsStoppedRuntimeAndLatestTrace(t *testing.T) {
	store := &doctorSessionStoreStub{
		summary: session.Summary{
			Session: session.KagenSession{
				ID:              7,
				Status:          SessionStatusFailed,
				WorkspaceBranch: "kagen/main/s/7",
				Namespace:       "kagen-repo-7",
				PodName:         "agent",
			},
		},
	}

	workflow := NewDoctorWorkflow(DoctorDependencies{
		LoadConfig: func() (*config.Config, error) {
			return config.DefaultConfig(), nil
		},
		DiscoverRepository: func() (*git.Repository, error) {
			t.Fatal("DiscoverRepository should not be called when --session is provided")
			return nil, nil
		},
		OpenSessionStore: func() (DoctorSessionStore, error) {
			return store, nil
		},
		LoadLatestOperation: func(sessionID int64) (diagnostics.Operation, bool, error) {
			if sessionID != 7 {
				t.Fatalf("LoadLatestOperation() session ID = %d, want 7", sessionID)
			}
			return diagnostics.Operation{
				Name:     "attach",
				Status:   diagnostics.StatusFailed,
				Duration: 3 * time.Second,
				Metadata: map[string]string{
					"session_id": "7",
					"agent_type": "codex",
				},
				Steps: []diagnostics.StepRecord{
					{Name: "ensure_runtime", Status: diagnostics.StatusSucceeded, Duration: time.Second},
					{Name: "preflight_runtime", Status: diagnostics.StatusFailed, Duration: 2 * time.Second},
				},
			}, true, nil
		},
		FindFailureArtefactDir: func(sessionID int64) (string, bool, error) {
			return "/tmp/kagen/failure-artefacts/session-7", true, nil
		},
		RuntimeStatus: func(context.Context, *config.Config) (runtime.Status, string, error) {
			return runtime.StatusStopped, "colima-kagen", nil
		},
		NewDiagnosticsInspector: func(string) (DoctorClusterInspector, error) {
			t.Fatal("NewDiagnosticsInspector should not be called when runtime is stopped")
			return nil, nil
		},
	})

	output := captureWorkflowStdout(t, func() {
		if err := workflow.Run(context.Background(), 7, true); err != nil {
			t.Fatalf("Run() returned error: %v", err)
		}
	})

	if !strings.Contains(output, "Session 7: failed") {
		t.Fatalf("output missing session summary: %q", output)
	}
	if !strings.Contains(output, "Runtime: stopped") {
		t.Fatalf("output missing runtime status: %q", output)
	}
	if !strings.Contains(output, "Pod: cluster inspection skipped because the runtime is not running") {
		t.Fatalf("output missing stopped-runtime pod guidance: %q", output)
	}
	if !strings.Contains(output, "/tmp/kagen/failure-artefacts/session-7") {
		t.Fatalf("output missing failure artefact directory: %q", output)
	}
	if !strings.Contains(output, "Runtime trace for attach: failed") {
		t.Fatalf("output missing latest trace summary: %q", output)
	}
}

func TestDoctorWorkflowSelectsMostRecentRepositorySession(t *testing.T) {
	store := &doctorSessionStoreStub{
		listSummaries: []session.Summary{
			{
				Session: session.KagenSession{
					ID:              12,
					Status:          SessionStatusReady,
					RepoPath:        "/tmp/repo",
					WorkspaceBranch: "kagen/main/s/12",
					Namespace:       "kagen-repo",
					PodName:         "agent",
				},
			},
			{
				Session: session.KagenSession{
					ID:              11,
					Status:          SessionStatusFailed,
					RepoPath:        "/tmp/repo",
					WorkspaceBranch: "kagen/main/s/11",
					Namespace:       "kagen-repo",
					PodName:         "agent",
				},
			},
		},
	}

	workflow := NewDoctorWorkflow(DoctorDependencies{
		LoadConfig: func() (*config.Config, error) {
			return config.DefaultConfig(), nil
		},
		DiscoverRepository: func() (*git.Repository, error) {
			return &git.Repository{Path: "/tmp/repo"}, nil
		},
		OpenSessionStore: func() (DoctorSessionStore, error) {
			return store, nil
		},
		LoadLatestOperation: func(sessionID int64) (diagnostics.Operation, bool, error) {
			return diagnostics.Operation{}, false, nil
		},
		FindFailureArtefactDir: func(sessionID int64) (string, bool, error) {
			return "", false, nil
		},
		RuntimeStatus: func(context.Context, *config.Config) (runtime.Status, string, error) {
			return runtime.StatusStopped, "colima-kagen", nil
		},
		NewDiagnosticsInspector: func(string) (DoctorClusterInspector, error) {
			return nil, nil
		},
	})

	output := captureWorkflowStdout(t, func() {
		if err := workflow.Run(context.Background(), 0, false); err != nil {
			t.Fatalf("Run() returned error: %v", err)
		}
	})

	if store.listOpts.RepoPath != "/tmp/repo" {
		t.Fatalf("List() repo path = %q, want /tmp/repo", store.listOpts.RepoPath)
	}
	if !strings.Contains(output, "Session 12: ready") {
		t.Fatalf("output missing most recent session: %q", output)
	}
}

func TestDoctorWorkflowReportsPodAndProxyState(t *testing.T) {
	store := &doctorSessionStoreStub{
		summary: session.Summary{
			Session: session.KagenSession{
				ID:              21,
				Status:          SessionStatusReady,
				WorkspaceBranch: "kagen/main/s/21",
				Namespace:       "kagen-repo-21",
				PodName:         "agent",
			},
			AgentSessions: []session.AgentSession{
				{
					ID:         "agent-session-21",
					AgentType:  "codex",
					LastUsedAt: time.Date(2026, time.March, 13, 12, 0, 0, 0, time.UTC),
				},
			},
		},
	}

	workflow := NewDoctorWorkflow(DoctorDependencies{
		LoadConfig: func() (*config.Config, error) {
			cfg := config.DefaultConfig()
			cfg.ProxyAllowlist = []string{"api.openai.com"}
			return cfg, nil
		},
		DiscoverRepository: func() (*git.Repository, error) {
			t.Fatal("DiscoverRepository should not be called when --session is provided")
			return nil, nil
		},
		OpenSessionStore: func() (DoctorSessionStore, error) {
			return store, nil
		},
		LoadLatestOperation: func(sessionID int64) (diagnostics.Operation, bool, error) {
			return diagnostics.Operation{
				Name:   "start",
				Status: diagnostics.StatusSucceeded,
				Metadata: map[string]string{
					"session_id":      "21",
					"agent_type":      "codex",
					"agent_container": "codex",
				},
			}, true, nil
		},
		FindFailureArtefactDir: func(sessionID int64) (string, bool, error) {
			return "", false, nil
		},
		RuntimeStatus: func(context.Context, *config.Config) (runtime.Status, string, error) {
			return runtime.StatusRunning, "colima-kagen", nil
		},
		NewDiagnosticsInspector: func(string) (DoctorClusterInspector, error) {
			return doctorInspectorStub{
				bundle: cluster.DiagnosticBundle{
					PodStatus: &cluster.PodSnapshot{
						Name:      "agent",
						Namespace: "kagen-repo-21",
						Phase:     corev1.PodRunning,
						InitContainerStatuses: []corev1.ContainerStatus{
							{
								Name: "workspace-sync",
								State: corev1.ContainerState{
									Terminated: &corev1.ContainerStateTerminated{Reason: "Completed"},
								},
							},
						},
						ContainerStatuses: []corev1.ContainerStatus{
							{
								Name:  "codex",
								Ready: true,
								State: corev1.ContainerState{
									Running: &corev1.ContainerStateRunning{},
								},
							},
						},
					},
					ProxyDeployment: &cluster.DeploymentSnapshot{
						Replicas:      1,
						ReadyReplicas: 1,
					},
				},
			}, nil
		},
	})

	output := captureWorkflowStdout(t, func() {
		if err := workflow.Run(context.Background(), 21, true); err != nil {
			t.Fatalf("Run() returned error: %v", err)
		}
	})

	if !strings.Contains(output, "Pod: running (kagen-repo-21/agent)") {
		t.Fatalf("output missing pod state: %q", output)
	}
	if !strings.Contains(output, "Init containers: workspace-sync=terminated:completed") {
		t.Fatalf("output missing init container state: %q", output)
	}
	if !strings.Contains(output, "Containers: codex=ready") {
		t.Fatalf("output missing container state: %q", output)
	}
	if !strings.Contains(output, "Proxy: enforced (1/1 ready, ") {
		t.Fatalf("output missing proxy state: %q", output)
	}
}

type doctorSessionStoreStub struct {
	summary       session.Summary
	listSummaries []session.Summary
	listOpts      session.ListOptions
}

func (s *doctorSessionStoreStub) Close() error {
	return nil
}

func (s *doctorSessionStoreStub) GetSummary(_ context.Context, _ int64) (session.Summary, bool, error) {
	if s.summary.Session.ID == 0 {
		return session.Summary{}, false, nil
	}

	return s.summary, true, nil
}

func (s *doctorSessionStoreStub) List(_ context.Context, opts session.ListOptions) ([]session.Summary, error) {
	s.listOpts = opts
	return append([]session.Summary(nil), s.listSummaries...), nil
}

type doctorInspectorStub struct {
	bundle cluster.DiagnosticBundle
}

func (s doctorInspectorStub) Collect(context.Context, cluster.DiagnosticsRequest) cluster.DiagnosticBundle {
	return s.bundle
}

func captureWorkflowStdout(t *testing.T, fn func()) string {
	t.Helper()

	original := os.Stdout
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe() returned error: %v", err)
	}
	defer reader.Close()

	os.Stdout = writer
	defer func() {
		os.Stdout = original
	}()

	fn()

	if err := writer.Close(); err != nil {
		t.Fatalf("writer.Close() returned error: %v", err)
	}

	var buf bytes.Buffer
	if _, err := io.Copy(&buf, reader); err != nil {
		t.Fatalf("Copy() returned error: %v", err)
	}

	return buf.String()
}
