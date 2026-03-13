package e2e

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/creack/pty"
	"github.com/pejas/kagen/internal/agent"
	"github.com/pejas/kagen/internal/session"
)

var (
	ansiEscapePattern = regexp.MustCompile(`\x1b(?:[@-Z\\-_]|\[[0-?]*[ -/]*[@-~]|\][^\x07\x1b]*(?:\x07|\x1b\\))`)
	controlPattern    = regexp.MustCompile(`[\x00-\x08\x0b-\x1f\x7f]`)
)

type interactiveSignal struct {
	name      string
	fragments []string
}

type interactiveExit struct {
	interrupts        int
	betweenInterrupts time.Duration
	afterPrompt       string
	promptTimeout     time.Duration
}

type interactiveCommand struct {
	cancel   context.CancelFunc
	cmd      *exec.Cmd
	ptmx     *os.File
	output   bytes.Buffer
	outputMu sync.Mutex
	readDone chan struct{}
	done     chan commandResult
}

type lockedBuffer struct {
	buffer *bytes.Buffer
	mu     *sync.Mutex
}

func TestInteractiveAttachSmoke(t *testing.T) {
	requireE2EPrerequisites(t)

	tests := []struct {
		name      string
		agentType agent.Type
		signal    interactiveSignal
		exit      interactiveExit
	}{
		{
			name:      "codex",
			agentType: agent.Codex,
			signal: interactiveSignal{
				name:      "Codex sign-in screen",
				fragments: []string{"Welcome to Codex", "Sign in with ChatGPT"},
			},
			exit: interactiveExit{interrupts: 1},
		},
		{
			name:      "claude",
			agentType: agent.Claude,
			signal: interactiveSignal{
				name:      "Claude startup banner",
				fragments: []string{"Welcome to Claude Code"},
			},
			exit: interactiveExit{
				interrupts:        4,
				betweenInterrupts: 500 * time.Millisecond,
				afterPrompt:       "Press Ctrl-C again to exit",
				promptTimeout:     10 * time.Second,
			},
		},
		{
			name:      "opencode",
			agentType: agent.OpenCode,
			signal: interactiveSignal{
				name:      "OpenCode input prompt",
				fragments: []string{"Ask anything...", "/projects/workspace"},
			},
			exit: interactiveExit{interrupts: 1},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h := newReadinessHarness(t, "feature/e2e-interactive-"+tc.name)
			summary := h.startDetachedReadySession(tc.agentType)
			initialSessionCount := len(summary.AgentSessions)
			previousAgentSession := latestAgentSession(summary, tc.agentType)

			attached := h.attachInteractive(
				[]string{"attach", string(tc.agentType), "--session", fmt.Sprintf("%d", summary.Session.ID)},
				nil,
			)
			t.Cleanup(func() {
				attached.close()
			})

			attached.waitForSignal(t, tc.signal, 2*time.Minute)
			attached.exit(t, tc.exit)
			result := attached.wait(t, 45*time.Second)
			if result.exitCode != 0 {
				t.Fatalf(
					"interactive attach for %s exit code = %d, want 0\noutput:\n%s",
					tc.agentType,
					result.exitCode,
					normalizeTerminalOutput(result.output),
				)
			}

			updated := h.mustSummary(summary.Session.ID)
			if updated.Session.Status != workflowStatusReady {
				t.Fatalf("session %d status after interactive attach = %q, want %q", updated.Session.ID, updated.Session.Status, workflowStatusReady)
			}
			if len(updated.AgentSessions) != initialSessionCount+1 {
				t.Fatalf("agent session count after interactive attach = %d, want %d", len(updated.AgentSessions), initialSessionCount+1)
			}

			latest := latestAgentSession(updated, tc.agentType)
			if latest.ID == "" {
				t.Fatalf("missing persisted %s agent session after interactive attach", tc.agentType)
			}
			if previousAgentSession.ID != "" && latest.ID == previousAgentSession.ID {
				t.Fatalf("interactive attach reused agent session %s, want a fresh session", latest.ID)
			}
		})
	}
}

func (h *readinessHarness) startDetachedReadySession(agentType agent.Type) session.Summary {
	h.t.Helper()

	started := h.startAsync([]string{"start", "--detach", string(agentType)}, nil)
	starting := h.waitForSessionStatus(workflowStatusStarting, 2*time.Minute)
	result := started.wait(h.t, 10*time.Minute)
	if result.exitCode != 0 {
		h.t.Fatalf("kagen start --detach %s exit code = %d, want 0\noutput:\n%s", agentType, result.exitCode, result.output)
	}

	summary := h.mustSummary(starting.Session.ID)
	if summary.Session.Status != workflowStatusReady {
		h.t.Fatalf("session %d status = %q, want %q", summary.Session.ID, summary.Session.Status, workflowStatusReady)
	}
	h.assertRuntimeReady(summary, agentType)

	return summary
}

func (h *readinessHarness) attachInteractive(args []string, extraEnv map[string]string) *interactiveCommand {
	h.t.Helper()

	ctx, cancel := context.WithCancel(context.Background())
	cmd := h.command(ctx, args, extraEnv)
	cmd.Env = removeEnv(cmd.Env, "KAGEN_NON_INTERACTIVE")
	cmd.Env = removeEnv(cmd.Env, "TERM")
	cmd.Env = append(cmd.Env, "TERM=xterm-256color")

	ptmx, err := pty.StartWithSize(cmd, &pty.Winsize{Rows: 24, Cols: 80})
	if err != nil {
		cancel()
		h.t.Fatalf("starting interactive %q: %v", strings.Join(args, " "), err)
	}

	running := &interactiveCommand{
		cancel:   cancel,
		cmd:      cmd,
		ptmx:     ptmx,
		readDone: make(chan struct{}),
		done:     make(chan commandResult, 1),
	}

	go func() {
		defer close(running.readDone)
		_, _ = io.Copy(&lockedBuffer{buffer: &running.output, mu: &running.outputMu}, ptmx)
	}()

	go func() {
		err := cmd.Wait()
		_ = ptmx.Close()
		<-running.readDone
		cancel()

		result := commandResult{
			output: running.snapshot(),
			err:    err,
		}
		if err != nil {
			var exitErr *exec.ExitError
			if errors.As(err, &exitErr) {
				result.exitCode = exitErr.ExitCode()
			}
		}
		running.done <- result
	}()

	return running
}

func (c *interactiveCommand) waitForSignal(t *testing.T, signal interactiveSignal, timeout time.Duration) string {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		output := normalizeTerminalOutput(c.snapshot())
		if output != "" && signal.matches(output) {
			return output
		}

		select {
		case result := <-c.done:
			t.Fatalf(
				"interactive attach exited before %s appeared (exit=%d)\noutput:\n%s",
				signal.name,
				result.exitCode,
				normalizeTerminalOutput(result.output),
			)
		default:
			time.Sleep(100 * time.Millisecond)
		}
	}

	t.Fatalf("timed out waiting for %s\noutput:\n%s", signal.name, normalizeTerminalOutput(c.snapshot()))
	return ""
}

func (c *interactiveCommand) exit(t *testing.T, exit interactiveExit) {
	t.Helper()

	interrupts := exit.interrupts
	if interrupts < 1 {
		interrupts = 1
	}
	betweenInterrupts := exit.betweenInterrupts
	if betweenInterrupts <= 0 {
		betweenInterrupts = 250 * time.Millisecond
	}

	for i := 0; i < interrupts; i++ {
		if _, err := c.ptmx.Write([]byte{3}); err != nil {
			if errors.Is(err, os.ErrClosed) {
				return
			}
			t.Fatalf("sending Ctrl+C %d/%d to interactive attach: %v", i+1, interrupts, err)
		}
		if i == 0 && exit.afterPrompt != "" {
			c.waitForFragment(t, exit.afterPrompt, exit.promptTimeout)
		}
		if i+1 < interrupts {
			time.Sleep(betweenInterrupts)
		}
	}
}

func (c *interactiveCommand) wait(t *testing.T, timeout time.Duration) commandResult {
	t.Helper()

	select {
	case result := <-c.done:
		return result
	case <-time.After(timeout):
		_ = c.cmd.Process.Kill()
		t.Fatalf(
			"timed out waiting for interactive command %q\noutput:\n%s",
			strings.Join(c.cmd.Args, " "),
			normalizeTerminalOutput(c.snapshot()),
		)
		return commandResult{}
	}
}

func (c *interactiveCommand) close() {
	if c == nil {
		return
	}

	if c.cancel != nil {
		c.cancel()
	}
	if c.ptmx != nil {
		_ = c.ptmx.Close()
	}
	if c.cmd != nil && c.cmd.Process != nil {
		_ = c.cmd.Process.Kill()
	}
}

func (c *interactiveCommand) snapshot() string {
	c.outputMu.Lock()
	defer c.outputMu.Unlock()

	return c.output.String()
}

func (w *lockedBuffer) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	return w.buffer.Write(p)
}

func (s interactiveSignal) matches(output string) bool {
	for _, fragment := range s.fragments {
		if !strings.Contains(output, fragment) {
			return false
		}
	}

	return true
}

func (c *interactiveCommand) waitForFragment(t *testing.T, fragment string, timeout time.Duration) {
	t.Helper()

	if strings.TrimSpace(fragment) == "" {
		return
	}
	if timeout <= 0 {
		timeout = 5 * time.Second
	}

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if strings.Contains(normalizeTerminalOutput(c.snapshot()), fragment) {
			return
		}

		select {
		case result := <-c.done:
			t.Fatalf(
				"interactive attach exited before %q appeared (exit=%d)\noutput:\n%s",
				fragment,
				result.exitCode,
				normalizeTerminalOutput(result.output),
			)
		default:
			time.Sleep(100 * time.Millisecond)
		}
	}

	t.Fatalf("timed out waiting for %q after interrupt\noutput:\n%s", fragment, normalizeTerminalOutput(c.snapshot()))
}

func normalizeTerminalOutput(output string) string {
	withoutANSI := ansiEscapePattern.ReplaceAllString(output, " ")
	withoutControl := controlPattern.ReplaceAllString(withoutANSI, " ")
	collapsed := strings.Join(strings.Fields(withoutControl), " ")

	return strings.TrimSpace(collapsed)
}

func removeEnv(env []string, key string) []string {
	prefix := key + "="
	filtered := make([]string, 0, len(env))
	for _, item := range env {
		if strings.HasPrefix(item, prefix) {
			continue
		}
		filtered = append(filtered, item)
	}

	return filtered
}
