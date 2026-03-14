package cluster

import (
	"strings"
	"testing"

	"github.com/pejas/kagen/internal/agent"
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
	if got := env["TERM"]; got != "xterm-256color" {
		t.Fatalf("TERM = %q", got)
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

func TestProxyContainerUsesPinnedBootstrapImage(t *testing.T) {
	t.Parallel()

	container := proxyContainer("ghcr.io/example/proxy:1.2.3")

	if strings.Contains(container.Image, ":latest") {
		t.Fatalf("proxy container image should be pinned, got %q", container.Image)
	}
	if len(container.Command) != 2 || container.Command[0] != "/bin/sh" || container.Command[1] != "-lc" {
		t.Fatalf("proxy container command = %q, want shell bootstrap", container.Command)
	}
	if strings.Contains(strings.Join(container.Args, "\n"), "apk add --no-cache tinyproxy") {
		t.Fatalf("proxy container args should not install tinyproxy during bootstrap: %q", container.Args)
	}
	if !strings.Contains(strings.Join(container.Args, "\n"), "exec tinyproxy -d -c") {
		t.Fatalf("proxy container args should exec a prebuilt tinyproxy binary: %q", container.Args)
	}
}

func TestInjectWorkspaceSyncUsesKagenBranchAsRemoteBase(t *testing.T) {
	t.Parallel()

	pod := &corev1.Pod{}
	pod.Spec.Containers = []corev1.Container{{Name: "workspace", Image: "ghcr.io/example/workspace:1.2.3"}}
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
	if strings.Contains(args[0], `repo_url="http://${FORGEJO_USERNAME}:${FORGEJO_PASSWORD}@forgejo:3000/kagen/workspace.git"`) {
		t.Fatalf("workspace sync script should not embed basic auth in the Forgejo URL: %q", args[0])
	}
	if !strings.Contains(args[0], `export GIT_TERMINAL_PROMPT=0`) {
		t.Fatalf("workspace sync script should disable interactive git prompts: %q", args[0])
	}
	if !strings.Contains(args[0], `auth_header="$(printf '%s' "${FORGEJO_USERNAME}:${FORGEJO_PASSWORD}" | base64 | tr -d '\n')"`) {
		t.Fatalf("workspace sync script missing transient auth header bootstrap: %q", args[0])
	}
	if !strings.Contains(args[0], `git -c "http.extraHeader=Authorization: Basic ${auth_header}" ls-remote "$repo_url"`) {
		t.Fatalf("workspace sync script missing header-auth Forgejo availability check: %q", args[0])
	}
	if !strings.Contains(args[0], `git -c "http.extraHeader=Authorization: Basic ${auth_header}" clone "$repo_url" "$worktree"`) {
		t.Fatalf("workspace sync script missing header-auth clone: %q", args[0])
	}
	if got := pod.Spec.InitContainers[0].Image; got != "ghcr.io/example/workspace:1.2.3" {
		t.Fatalf("workspace sync image = %q, want workspace container image", got)
	}
	if !strings.Contains(args[0], "mkdir -p /home/kagen") {
		t.Fatalf("workspace sync script should prepare the agent home volume: %q", args[0])
	}
	if !strings.Contains(args[0], "chown -R 1000:1000 /projects /home/kagen") {
		t.Fatalf("workspace sync script should hand workspace and home ownership back to the runtime user: %q", args[0])
	}
	securityContext := pod.Spec.InitContainers[0].SecurityContext
	if securityContext == nil || securityContext.RunAsUser == nil || *securityContext.RunAsUser != 0 {
		t.Fatalf("workspace sync should run as root to prepare the shared workspace volume: %#v", securityContext)
	}
	if !hasMount(pod.Spec.InitContainers[0].VolumeMounts, "agent-home", agent.DefaultHomeDir()) {
		t.Fatalf("workspace sync should mount the shared agent-home volume: %#v", pod.Spec.InitContainers[0].VolumeMounts)
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

func hasMount(mounts []corev1.VolumeMount, name, path string) bool {
	for _, mount := range mounts {
		if mount.Name == name && mount.MountPath == path {
			return true
		}
	}

	return false
}
