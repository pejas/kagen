package cluster

import (
	"strings"
	"testing"

	"github.com/pejas/kagen/internal/git"
	"github.com/pejas/kagen/internal/proxy"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

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

func TestTinyproxyConfigUsesDedicatedConfigDir(t *testing.T) {
	t.Parallel()

	cfg := tinyproxyConfig()

	if !strings.Contains(cfg, `Filter "`+proxyConfigDir+`/allowlist"`) {
		t.Fatalf("tinyproxyConfig() missing dedicated allowlist path: %q", cfg)
	}
	if strings.Contains(cfg, "/etc/tinyproxy/allowlist") {
		t.Fatalf("tinyproxyConfig() still references package config path: %q", cfg)
	}
	if strings.Contains(cfg, "FilterURLs On") {
		t.Fatalf("tinyproxyConfig() should not enable URL-path filtering: %q", cfg)
	}
}

func TestProxyContainerUsesPinnedImageWithoutRuntimeInstall(t *testing.T) {
	t.Parallel()

	container := proxyContainer()

	if strings.Contains(container.Image, ":latest") {
		t.Fatalf("proxy container image should be pinned, got %q", container.Image)
	}
	if len(container.Command) != 1 || container.Command[0] != "tinyproxy" {
		t.Fatalf("proxy container command = %q, want tinyproxy", container.Command)
	}
	if strings.Contains(strings.Join(container.Args, "\n"), "apk add") {
		t.Fatalf("proxy container args should not install packages at runtime: %q", container.Args)
	}
}

func TestInjectWorkspaceSyncUsesKagenBranchAsRemoteBase(t *testing.T) {
	t.Parallel()

	pod := &corev1.Pod{}
	repo := &git.Repository{CurrentBranch: "main"}

	injectWorkspaceSync(pod, repo)

	if len(pod.Spec.InitContainers) != 1 {
		t.Fatalf("expected 1 init container, got %d", len(pod.Spec.InitContainers))
	}

	args := pod.Spec.InitContainers[0].Args
	if len(args) != 1 {
		t.Fatalf("expected 1 script arg, got %d", len(args))
	}
	if !strings.Contains(args[0], `git checkout --track -b "kagen/main" "origin/kagen/main"`) {
		t.Fatalf("workspace sync script missing review branch tracking checkout: %q", args[0])
	}
	if strings.Contains(args[0], "kagen-internal-secret") {
		t.Fatalf("workspace sync script should not embed forgejo credentials: %q", args[0])
	}
	if !strings.Contains(args[0], `git ls-remote "$repo_url"`) {
		t.Fatalf("workspace sync script missing forgejo availability check: %q", args[0])
	}

	env := pod.Spec.InitContainers[0].Env
	if len(env) != 2 {
		t.Fatalf("expected 2 credential env vars, got %d", len(env))
	}
	if env[0].ValueFrom == nil || env[0].ValueFrom.SecretKeyRef == nil || env[0].ValueFrom.SecretKeyRef.Name != forgejoBootstrapSecretName {
		t.Fatalf("FORGEJO_USERNAME should come from secret %q: %#v", forgejoBootstrapSecretName, env[0].ValueFrom)
	}
	if env[1].ValueFrom == nil || env[1].ValueFrom.SecretKeyRef == nil || env[1].ValueFrom.SecretKeyRef.Name != forgejoBootstrapSecretName {
		t.Fatalf("FORGEJO_PASSWORD should come from secret %q: %#v", forgejoBootstrapSecretName, env[1].ValueFrom)
	}
}

func TestProxyAllowlistUsesEscapedHostPatterns(t *testing.T) {
	t.Parallel()

	got := proxyAllowlist([]string{"registry.npmjs.org", " api.openai.com "})

	if strings.Contains(got, "https?://") || strings.Contains(got, "[0-9]+") {
		t.Fatalf("proxyAllowlist() should use host-only patterns, got %q", got)
	}
	if !strings.Contains(got, "registry\\.npmjs\\.org") {
		t.Fatalf("proxyAllowlist() missing escaped npm host: %q", got)
	}
	if !strings.Contains(got, "api\\.openai\\.com") {
		t.Fatalf("proxyAllowlist() missing trimmed OpenAI host: %q", got)
	}
}

func TestProxyConfigDataChecksumChangesWithAllowlist(t *testing.T) {
	t.Parallel()

	first := proxyConfigDataChecksum(map[string]string{
		"allowlist":      proxyAllowlist([]string{"api.openai.com"}),
		"tinyproxy.conf": tinyproxyConfig(),
	})
	second := proxyConfigDataChecksum(map[string]string{
		"allowlist":      proxyAllowlist([]string{"api.openai.com", "opencode.ai"}),
		"tinyproxy.conf": tinyproxyConfig(),
	})

	if first == "" || second == "" {
		t.Fatal("proxyConfigDataChecksum() returned empty checksum")
	}
	if first == second {
		t.Fatalf("proxyConfigDataChecksum() should change when allowlist changes: %q", first)
	}
}

func TestEnsureProxyDeploymentAnnotatesConfigChecksum(t *testing.T) {
	t.Parallel()

	checksum := proxyConfigDataChecksum(map[string]string{
		"allowlist":      proxyAllowlist([]string{"opencode.ai"}),
		"tinyproxy.conf": tinyproxyConfig(),
	})

	deployment := &corev1.PodTemplateSpec{
		ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{
				proxyConfigChecksum: checksum,
			},
		},
	}

	if got := deployment.Annotations[proxyConfigChecksum]; got != checksum {
		t.Fatalf("proxy config checksum annotation = %q, want %q", got, checksum)
	}
}
