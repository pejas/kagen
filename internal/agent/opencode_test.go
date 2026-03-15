package agent

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/pejas/kagen/internal/kubeexec"
)

func TestOpenCodeConfigureWritesConfigWhenMissing(t *testing.T) {
	t.Parallel()

	spec := openCodeRuntimeSpec{}
	commands := [][]string{}
	runner := agentStubRunner{
		run: func(_ context.Context, namespace, pod string, command []string, _ ...kubeexec.Option) (string, error) {
			commands = append(commands, append([]string(nil), command...))
			if command[2] == fmt.Sprintf("test -f %s/.config/opencode.json", spec.StateRoot()) {
				return "", errors.New("missing config")
			}

			return "", nil
		},
	}

	if err := spec.Configure(context.Background(), "kagen-test", "kagen-agent-opencode", runner); err != nil {
		t.Fatalf("Configure() returned error: %v", err)
	}
	if len(commands) != 3 {
		t.Fatalf("Configure() ran %d commands, want 3", len(commands))
	}
	if got := commands[1]; len(got) != 3 || got[0] != "/bin/mkdir" || got[1] != "-p" || got[2] != spec.StateRoot()+"/.config" {
		t.Fatalf("mkdir command = %q, want config directory creation", got)
	}
	if got := commands[2][2]; got != fmt.Sprintf("cat > %s/.config/opencode.json << 'EOF'\n{\n  \"$schema\": \"https://opencode.ai/config.json\",\n  \"permission\": \"allow\"\n}\nEOF", spec.StateRoot()) {
		t.Fatalf("write command = %q, want opencode permission config", got)
	}
}

func TestOpenCodeConfigureSkipsWriteWhenConfigExists(t *testing.T) {
	t.Parallel()

	spec := openCodeRuntimeSpec{}
	runs := 0
	runner := agentStubRunner{
		run: func(_ context.Context, _ string, _ string, _ []string, _ ...kubeexec.Option) (string, error) {
			runs++
			return "", nil
		},
	}

	if err := spec.Configure(context.Background(), "kagen-test", "kagen-agent-opencode", runner); err != nil {
		t.Fatalf("Configure() returned error: %v", err)
	}
	if runs != 1 {
		t.Fatalf("Configure() ran %d commands, want 1", runs)
	}
}

type agentStubRunner struct {
	run func(context.Context, string, string, []string, ...kubeexec.Option) (string, error)
}

func (s agentStubRunner) Run(ctx context.Context, namespace, pod string, command []string, opts ...kubeexec.Option) (string, error) {
	return s.run(ctx, namespace, pod, command, opts...)
}

func (agentStubRunner) Attach(context.Context, string, string, []string, ...kubeexec.Option) error {
	return nil
}

func (agentStubRunner) WaitForPodReady(context.Context, string, string, string) error {
	return nil
}
