package workload

import (
	"strings"
	"testing"

	"github.com/pejas/kagen/internal/agent"
	"github.com/pejas/kagen/internal/config"
	corev1 "k8s.io/api/core/v1"
)

func testImages() Images {
	cfg := config.DefaultConfig()
	return Images{
		Workspace: cfg.Images.Workspace,
		Toolbox:   cfg.Images.Toolbox,
	}
}

func TestBuilderBuildPodBuildsBaselinePodForSupportedAgents(t *testing.T) {
	t.Parallel()

	builder := NewBuilder()
	for _, agentType := range agent.SupportedTypes() {
		agentType := agentType
		t.Run(string(agentType), func(t *testing.T) {
			t.Parallel()

			spec, err := agent.SpecFor(agentType)
			if err != nil {
				t.Fatalf("SpecFor(%s) returned error: %v", agentType, err)
			}

			got, err := builder.BuildPod(Request{
				Name:      "agent",
				Namespace: "kagen-test",
				Runtime:   spec,
				Images:    testImages(),
			})
			if err != nil {
				t.Fatalf("BuildPod() returned error: %v", err)
			}

			assertBaselinePod(t, got, spec)
		})
	}
}

func TestBuilderBuildPodUsesInstallFreeToolboxBootstrap(t *testing.T) {
	t.Parallel()

	spec, err := agent.SpecFor(agent.Codex)
	if err != nil {
		t.Fatalf("SpecFor(codex) returned error: %v", err)
	}

	builder := NewBuilder()
	pod, err := builder.BuildPod(Request{
		Name:      "agent",
		Namespace: "kagen-test",
		Runtime:   spec,
		Images:    testImages(),
	})
	if err != nil {
		t.Fatalf("BuildPod() returned error: %v", err)
	}

	runtimeContainer := pod.Spec.Containers[1]
	if runtimeContainer.Image != testImages().Toolbox {
		t.Fatalf("runtime container image = %q, want %q", runtimeContainer.Image, testImages().Toolbox)
	}
	if len(runtimeContainer.Command) != 2 {
		t.Fatalf("runtime container command length = %d, want 2", len(runtimeContainer.Command))
	}
	if len(runtimeContainer.Args) != 1 {
		t.Fatalf("runtime container args length = %d, want 1", len(runtimeContainer.Args))
	}
	if got := runtimeContainer.Args[0]; got != "exec tail -f /dev/null" {
		t.Fatalf("runtime container args[0] = %q, want install-free keepalive", got)
	}
}

func TestConfigDerivedImagesAreReleasePinned(t *testing.T) {
	t.Parallel()

	images := testImages()
	if strings.Contains(images.Workspace, ":latest") {
		t.Fatalf("workspace image should not use a mutable latest tag: %q", images.Workspace)
	}
	if strings.Contains(images.Toolbox, ":latest") {
		t.Fatalf("toolbox image should not use a mutable latest tag: %q", images.Toolbox)
	}
}

func TestBuilderBuildPodRequiresRuntimeType(t *testing.T) {
	t.Parallel()

	builder := NewBuilder()
	if _, err := builder.BuildPod(Request{Name: "agent", Namespace: "kagen-test", Images: testImages()}); err == nil {
		t.Fatal("BuildPod() expected error for missing runtime")
	}
}

func assertBaselinePod(t *testing.T, pod *corev1.Pod, spec agent.RuntimeSpec) {
	t.Helper()

	if pod.Name != "agent" {
		t.Fatalf("pod name = %q, want %q", pod.Name, "agent")
	}
	if pod.Namespace != "kagen-test" {
		t.Fatalf("pod namespace = %q, want %q", pod.Namespace, "kagen-test")
	}
	if pod.Labels["app.kubernetes.io/name"] != "kagen-agent" {
		t.Fatalf("pod label app.kubernetes.io/name = %q, want %q", pod.Labels["app.kubernetes.io/name"], "kagen-agent")
	}
	if len(pod.Spec.Containers) != 2 {
		t.Fatalf("container count = %d, want 2", len(pod.Spec.Containers))
	}
	if len(pod.Spec.Volumes) != 2 {
		t.Fatalf("volume count = %d, want 2", len(pod.Spec.Volumes))
	}

	workspaceContainer := pod.Spec.Containers[0]
	if workspaceContainer.Name != defaultWorkspaceName {
		t.Fatalf("workspace container name = %q, want %q", workspaceContainer.Name, defaultWorkspaceName)
	}
	if workspaceContainer.Image != testImages().Workspace {
		t.Fatalf("workspace container image = %q, want %q", workspaceContainer.Image, testImages().Workspace)
	}
	assertStringSliceEqual(t, "workspace command", workspaceContainer.Command, []string{"/bin/sh", "-lc"})
	assertStringSliceEqual(t, "workspace args", workspaceContainer.Args, []string{"exec tail -f /dev/null"})
	if !hasMount(workspaceContainer.VolumeMounts, "git-workspace", defaultWorkspaceMount) {
		t.Fatalf("workspace container missing git-workspace mount: %#v", workspaceContainer.VolumeMounts)
	}

	runtimeContainer := pod.Spec.Containers[1]
	if runtimeContainer.Name != agent.ContainerName(spec) {
		t.Fatalf("runtime container name = %q, want %q", runtimeContainer.Name, agent.ContainerName(spec))
	}
	if runtimeContainer.Image != testImages().Toolbox {
		t.Fatalf("runtime container image = %q, want %q", runtimeContainer.Image, testImages().Toolbox)
	}
	assertStringSliceEqual(t, "runtime command", runtimeContainer.Command, []string{"/bin/sh", "-lc"})
	assertStringSliceEqual(t, "runtime args", runtimeContainer.Args, []string{"exec tail -f /dev/null"})
	assertEnvMatches(t, runtimeContainer.Env, envVarMap(spec.RequiredEnv()))
	if !hasMount(runtimeContainer.VolumeMounts, "git-workspace", defaultWorkspaceMount) {
		t.Fatalf("runtime container missing workspace mount: %#v", runtimeContainer.VolumeMounts)
	}
	if !hasMount(runtimeContainer.VolumeMounts, defaultAgentHomeName, agent.DefaultHomeDir()) {
		t.Fatalf("runtime container missing agent-home mount: %#v", runtimeContainer.VolumeMounts)
	}

	workspaceVolume := pod.Spec.Volumes[0]
	if workspaceVolume.Name != "git-workspace" {
		t.Fatalf("workspace volume name = %q, want %q", workspaceVolume.Name, "git-workspace")
	}
	if workspaceVolume.EmptyDir == nil || workspaceVolume.EmptyDir.Medium != corev1.StorageMediumMemory {
		t.Fatalf("workspace volume = %#v, want memory-backed EmptyDir", workspaceVolume.EmptyDir)
	}

	agentHomeVolume := pod.Spec.Volumes[1]
	if agentHomeVolume.Name != defaultAgentHomeName {
		t.Fatalf("agent-home volume name = %q, want %q", agentHomeVolume.Name, defaultAgentHomeName)
	}
	if agentHomeVolume.PersistentVolumeClaim == nil {
		t.Fatal("agent-home volume should be backed by a PVC")
	}
	if agentHomeVolume.PersistentVolumeClaim.ClaimName != "agent-"+defaultAgentHomeName {
		t.Fatalf("agent-home claim name = %q, want %q", agentHomeVolume.PersistentVolumeClaim.ClaimName, "agent-"+defaultAgentHomeName)
	}
}

func assertStringSliceEqual(t *testing.T, field string, got, want []string) {
	t.Helper()

	if len(got) != len(want) {
		t.Fatalf("%s length = %d, want %d", field, len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("%s[%d] = %q, want %q", field, i, got[i], want[i])
		}
	}
}

func assertEnvMatches(t *testing.T, got []corev1.EnvVar, want map[string]string) {
	t.Helper()

	if len(got) != len(want) {
		t.Fatalf("env count = %d, want %d", len(got), len(want))
	}

	gotMap := make(map[string]string, len(got))
	for _, variable := range got {
		gotMap[variable.Name] = variable.Value
	}
	for name, value := range want {
		if gotMap[name] != value {
			t.Fatalf("env %q = %q, want %q", name, gotMap[name], value)
		}
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

func envVarMap(values []agent.EnvVar) map[string]string {
	env := make(map[string]string, len(values))
	for _, variable := range values {
		env[variable.Name] = variable.Value
	}

	return env
}
