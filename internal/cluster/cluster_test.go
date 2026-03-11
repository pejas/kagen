package cluster

import (
	"context"
	"errors"
	"strings"
	"testing"

	kagerr "github.com/pejas/kagen/internal/errors"
	"github.com/pejas/kagen/internal/git"
	"github.com/pejas/kagen/internal/proxy"
	corev1 "k8s.io/api/core/v1"
)

func TestStubManagerEnsureNamespace(t *testing.T) {
	t.Parallel()

	mgr := NewStubManager()
	repo := &git.Repository{Path: "/fake"}
	err := mgr.EnsureNamespace(context.Background(), repo)
	if !errors.Is(err, kagerr.ErrNotImplemented) {
		t.Errorf("expected ErrNotImplemented, got %v", err)
	}
}

func TestStubManagerEnsureResources(t *testing.T) {
	t.Parallel()
	mgr := NewStubManager()
	err := mgr.EnsureResources(context.Background(), &git.Repository{}, "codex", nil, nil)
	if err == nil {
		t.Error("EnsureResources() expected error, got nil")
	}
}

func TestStubManagerAttachAgent(t *testing.T) {
	t.Parallel()

	mgr := NewStubManager()
	repo := &git.Repository{Path: "/fake"}
	err := mgr.AttachAgent(context.Background(), repo)
	if !errors.Is(err, kagerr.ErrNotImplemented) {
		t.Errorf("expected ErrNotImplemented, got %v", err)
	}
}

func TestStubManagerEnsureProxy(t *testing.T) {
	t.Parallel()

	mgr := NewStubManager()
	repo := &git.Repository{Path: "/fake"}
	err := mgr.EnsureProxy(context.Background(), repo, &proxy.Policy{})
	if !errors.Is(err, kagerr.ErrNotImplemented) {
		t.Errorf("expected ErrNotImplemented, got %v", err)
	}
}

func TestStubManagerProxyReady(t *testing.T) {
	t.Parallel()

	mgr := NewStubManager()
	repo := &git.Repository{Path: "/fake"}
	_, err := mgr.ProxyReady(context.Background(), repo)
	if !errors.Is(err, kagerr.ErrNotImplemented) {
		t.Errorf("expected ErrNotImplemented, got %v", err)
	}
}

func TestInjectAgentRuntimeAddsProxyEnv(t *testing.T) {
	t.Parallel()

	pod := &corev1.Pod{
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{Name: "agent"},
			},
		},
	}

	injectAgentRuntime(pod, "codex", "kagen-12345678", &proxy.Policy{
		AllowedDestinations: []string{"api.openai.com"},
	})

	env := map[string]string{}
	for _, item := range pod.Spec.Containers[0].Env {
		env[item.Name] = item.Value
	}

	if got := env["HTTP_PROXY"]; got != "http://egress-proxy.kagen-12345678.svc.cluster.local:8888" {
		t.Fatalf("HTTP_PROXY = %q", got)
	}
	if got := env["CODEX_HOME"]; got != "/home/kagen/.codex" {
		t.Fatalf("CODEX_HOME = %q", got)
	}
	if !strings.Contains(env["NO_PROXY"], "forgejo.kagen-12345678.svc.cluster.local") {
		t.Fatalf("NO_PROXY missing forgejo service: %q", env["NO_PROXY"])
	}
}

func TestInjectAgentRuntimeSkipsProxyEnvWithoutPolicy(t *testing.T) {
	t.Parallel()

	pod := &corev1.Pod{
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{Name: "agent"},
			},
		},
	}

	injectAgentRuntime(pod, "codex", "kagen-12345678", nil)

	for _, item := range pod.Spec.Containers[0].Env {
		if strings.EqualFold(item.Name, "http_proxy") || strings.EqualFold(item.Name, "https_proxy") {
			t.Fatalf("unexpected proxy env %q=%q", item.Name, item.Value)
		}
	}
}
