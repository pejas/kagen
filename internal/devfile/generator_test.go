package devfile

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/pejas/kagen/internal/agent"
)

func TestGenerator_GeneratePod(t *testing.T) {
	// Mock devfile data
	tmpDir := t.TempDir()
	devfilePath := filepath.Join(tmpDir, "devfile.yaml")
	content := `
schemaVersion: 2.2.0
metadata:
  name: test-project
components:
  - name: runtime
    container:
      image: nodejs:16
      env:
        - name: PORT
          value: "3000"
      volumeMounts:
        - name: data
          path: /data
  - name: data
    volume:
      size: 1Gi
`
	if err := os.WriteFile(devfilePath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	devfileData, err := Parse(devfilePath)
	if err != nil {
		t.Fatal(err)
	}

	g := &Generator{Namespace: "test-ns"}
	pod, err := g.GeneratePod("test-pod", devfileData)
	if err != nil {
		t.Fatalf("GeneratePod() error = %v", err)
	}

	if pod.Name != "test-pod" {
		t.Errorf("Pod.Name = %v, want test-pod", pod.Name)
	}

	if len(pod.Spec.Containers) != 1 {
		t.Errorf("len(Containers) = %v, want 1", len(pod.Spec.Containers))
	}

	container := pod.Spec.Containers[0]
	if container.Name != "runtime" {
		t.Errorf("Container.Name = %v, want runtime", container.Name)
	}

	if container.Image != "nodejs:16" {
		t.Errorf("Container.Image = %v, want nodejs:16", container.Image)
	}
	if len(container.Command) != 2 || container.Command[0] != "/bin/sh" {
		t.Errorf("Container.Command = %v, want default shell keepalive", container.Command)
	}

	// check env
	foundPort := false
	for _, e := range container.Env {
		if e.Name == "PORT" && e.Value == "3000" {
			foundPort = true
			break
		}
	}
	if !foundPort {
		t.Error("PORT env var not found in container")
	}

	// check volumes
	if len(pod.Spec.Volumes) != 2 { // git-workspace + data
		t.Errorf("len(Volumes) = %v, want 2", len(pod.Spec.Volumes))
	}
}

func TestGenerator_NoContainers(t *testing.T) {
	tmpDir := t.TempDir()
	devfilePath := filepath.Join(tmpDir, "devfile.yaml")
	content := `
schemaVersion: 2.2.0
metadata:
  name: test-project
components:
  - name: data
    volume:
      size: 1Gi
`
	if err := os.WriteFile(devfilePath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	devfileData, err := Parse(devfilePath)
	if err != nil {
		t.Fatal(err)
	}

	g := &Generator{Namespace: "test-ns"}
	_, err = g.GeneratePod("test-pod", devfileData)
	if err == nil {
		t.Error("GeneratePod() expected error for devfile with no containers")
	}
}

func TestEnsureRuntimeComponentInjectsAgentContainer(t *testing.T) {
	t.Parallel()

	d := &Devfile{
		SchemaVersion: "2.2.0",
		Metadata:      Metadata{Name: "test"},
		Components: []Component{
			{
				Name: "workspace",
				Container: &Container{
					Image: "vxcontrol/codebase:latest",
				},
			},
		},
	}

	spec, err := agent.SpecFor(agent.OpenCode)
	if err != nil {
		t.Fatalf("SpecFor(opencode) error = %v", err)
	}

	containerName, err := EnsureRuntimeComponent(d, spec)
	if err != nil {
		t.Fatalf("EnsureRuntimeComponent(opencode) error = %v", err)
	}
	if containerName != "kagen-agent-opencode" {
		t.Fatalf("EnsureRuntimeComponent(opencode) container = %q, want %q", containerName, "kagen-agent-opencode")
	}

	runtimeComponent := d.FindRuntimeComponent(agent.OpenCode)
	if runtimeComponent == nil {
		t.Fatal("FindRuntimeComponent(opencode) = nil, want injected component")
	}
	if runtimeComponent.Container == nil {
		t.Fatal("runtime component container = nil, want container")
	}
	if runtimeComponent.Container.Image != "vxcontrol/codebase:latest" {
		t.Fatalf("runtime component image = %q, want %q", runtimeComponent.Container.Image, "vxcontrol/codebase:latest")
	}
}
