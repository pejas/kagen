package cluster

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/pejas/kagen/internal/ui"
)

// PortForwarder starts kubectl-backed port-forward sessions.
type PortForwarder struct {
	kubeCtx string
}

// ForwardSession owns the lifecycle of a single kubectl port-forward process.
type ForwardSession struct {
	namespace string
	target    string
	cmd       *exec.Cmd

	outputMu sync.Mutex
	output   bytes.Buffer

	readyOnce sync.Once
	readyCh   chan readyResult
	localPort atomic.Int64

	doneOnce    sync.Once
	doneCh      chan struct{}
	waitErr     error
	stopOnce    sync.Once
	stoppedByUs atomic.Bool
}

type readyResult struct {
	localPort int
	err       error
}

var forwardingLinePattern = regexp.MustCompile(`^Forwarding from .+:(\d+) -> (\d+)$`)

// NewPortForwarder returns a new kubectl-backed PortForwarder.
func NewPortForwarder(kubeCtx string) *PortForwarder {
	return &PortForwarder{kubeCtx: kubeCtx}
}

// Start begins the port-forward and returns an explicit session handle.
// namespace: the K8s namespace
// target: the target service or pod name (e.g., "svc/forgejo" or "pod/agent")
// localPort: the requested local port; set to 0 to let kubectl choose an ephemeral port
// remotePort: the target service or pod port to forward
func (p *PortForwarder) Start(ctx context.Context, namespace, target string, localPort, remotePort int) (*ForwardSession, error) {
	portSpec, err := portForwardSpec(localPort, remotePort)
	if err != nil {
		return nil, err
	}
	ui.Verbose("Starting port-forward %s/%s with spec %s", namespace, target, portSpec)

	args := []string{}
	if p.kubeCtx != "" {
		args = append(args, "--context", p.kubeCtx)
	}
	args = append(args, "port-forward", "-n", namespace, target, portSpec)
	cmd := exec.CommandContext(ctx, "kubectl", args...)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("getting stdout pipe: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("getting stderr pipe: %w", err)
	}

	session := &ForwardSession{
		namespace: namespace,
		target:    target,
		cmd:       cmd,
		readyCh:   make(chan readyResult, 1),
		doneCh:    make(chan struct{}),
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("starting port-forward: %w", err)
	}

	go session.run(stdout, stderr, remotePort, localPort)

	for {
		select {
		case <-ctx.Done():
			_ = session.Stop()
			return nil, ctx.Err()
		case ready := <-session.readyCh:
			if ready.err != nil {
				_ = session.Stop()
				return nil, ready.err
			}
			if err := waitForPort(ready.localPort, 5*time.Second); err != nil {
				_ = session.Stop()
				return nil, err
			}

			ui.Verbose("Port-forward %s/%s is ready on local port %d", namespace, target, ready.localPort)
			return session, nil
		case <-session.Done():
			if err := session.Wait(); err != nil {
				ui.Verbose("Port-forward %s/%s exited before readiness: %v", namespace, target, err)
				return nil, err
			}
			ui.Verbose("Port-forward %s/%s exited before reporting readiness", namespace, target)
			return nil, fmt.Errorf(
				"kubectl port-forward %s %s exited before reporting a forwarded port: %s",
				namespace,
				target,
				session.outputString(),
			)
		}
	}
}

// LocalPort returns the local port bound by kubectl after readiness succeeds.
func (s *ForwardSession) LocalPort() int {
	if s == nil {
		return 0
	}

	return int(s.localPort.Load())
}

// Done closes when the underlying kubectl process exits.
func (s *ForwardSession) Done() <-chan struct{} {
	if s == nil {
		ch := make(chan struct{})
		close(ch)
		return ch
	}

	return s.doneCh
}

// Wait blocks until the underlying kubectl process exits.
func (s *ForwardSession) Wait() error {
	if s == nil {
		return nil
	}

	<-s.doneCh
	return s.waitErr
}

// Stop terminates the port-forward process and waits for it to exit.
func (s *ForwardSession) Stop() error {
	if s == nil {
		return nil
	}

	s.stopOnce.Do(func() {
		s.stoppedByUs.Store(true)
		ui.Verbose("Stopping port-forward %s/%s", s.namespace, s.target)
		if s.cmd != nil && s.cmd.Process != nil {
			_ = s.cmd.Process.Kill()
		}
	})

	return s.Wait()
}

func (s *ForwardSession) run(stdout, stderr io.Reader, remotePort, requestedLocalPort int) {
	var (
		wg       sync.WaitGroup
		readerMu sync.Mutex
		runErr   error
	)

	recordErr := func(err error) {
		if err == nil {
			return
		}

		readerMu.Lock()
		defer readerMu.Unlock()
		if runErr == nil {
			runErr = err
		}
	}

	wg.Add(2)
	go func() {
		defer wg.Done()
		recordErr(scanPortForwardOutput(stdout, func(line string) {
			s.handleLine(line, remotePort, requestedLocalPort)
		}))
	}()
	go func() {
		defer wg.Done()
		recordErr(scanPortForwardOutput(stderr, func(line string) {
			s.handleLine(line, remotePort, requestedLocalPort)
		}))
	}()

	waitErr := s.cmd.Wait()
	wg.Wait()

	if waitErr != nil && !s.stoppedByUs.Load() {
		recordErr(fmt.Errorf(
			"kubectl port-forward %s %s: %s: %w",
			s.namespace,
			s.target,
			s.outputString(),
			waitErr,
		))
	}
	if waitErr == nil && !s.isReady() {
		recordErr(fmt.Errorf(
			"kubectl port-forward %s %s exited before reporting a forwarded port: %s",
			s.namespace,
			s.target,
			s.outputString(),
		))
	}

	s.readyOnce.Do(func() {
		s.readyCh <- readyResult{err: runErr}
	})

	s.doneOnce.Do(func() {
		s.waitErr = runErr
		close(s.doneCh)
	})
}

func (s *ForwardSession) handleLine(line string, remotePort, requestedLocalPort int) {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return
	}

	s.appendOutput(trimmed)

	if !strings.Contains(trimmed, "Forwarding from") {
		return
	}

	forwardedLocalPort, forwardedRemotePort, err := parseForwardingLine(trimmed)
	if err != nil {
		s.readyOnce.Do(func() {
			s.readyCh <- readyResult{
				err: fmt.Errorf("parsing port-forward output %q: %w", trimmed, err),
			}
		})
		return
	}

	if forwardedRemotePort != remotePort {
		s.readyOnce.Do(func() {
			s.readyCh <- readyResult{
				err: fmt.Errorf("kubectl port-forward forwarded remote port %d, want %d", forwardedRemotePort, remotePort),
			}
		})
		return
	}
	if requestedLocalPort > 0 && forwardedLocalPort != requestedLocalPort {
		s.readyOnce.Do(func() {
			s.readyCh <- readyResult{
				err: fmt.Errorf("kubectl port-forward bound local port %d, want %d", forwardedLocalPort, requestedLocalPort),
			}
		})
		return
	}

	s.readyOnce.Do(func() {
		s.localPort.Store(int64(forwardedLocalPort))
		s.readyCh <- readyResult{localPort: forwardedLocalPort}
	})
}

func (s *ForwardSession) appendOutput(line string) {
	s.outputMu.Lock()
	defer s.outputMu.Unlock()

	appendPortForwardLog(&s.output, line)
}

func (s *ForwardSession) outputString() string {
	s.outputMu.Lock()
	defer s.outputMu.Unlock()

	return strings.TrimSpace(s.output.String())
}

func (s *ForwardSession) isReady() bool {
	return s.LocalPort() > 0
}

func waitForPort(port int, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)), 100*time.Millisecond)
		if err == nil {
			if closeErr := conn.Close(); closeErr != nil {
				return fmt.Errorf("closing readiness probe connection: %w", closeErr)
			}
			return nil
		}
		time.Sleep(200 * time.Millisecond)
	}
	return fmt.Errorf("timeout waiting for local port %d to be ready", port)
}

func portForwardSpec(localPort, remotePort int) (string, error) {
	if remotePort <= 0 {
		return "", fmt.Errorf("remote port must be positive, got %d", remotePort)
	}
	if localPort < 0 {
		return "", fmt.Errorf("local port must be zero or positive, got %d", localPort)
	}
	if localPort == 0 {
		return fmt.Sprintf(":%d", remotePort), nil
	}

	return fmt.Sprintf("%d:%d", localPort, remotePort), nil
}

func parseForwardingLine(line string) (int, int, error) {
	matches := forwardingLinePattern.FindStringSubmatch(strings.TrimSpace(line))
	if len(matches) != 3 {
		return 0, 0, fmt.Errorf("missing forwarding line")
	}

	localPort, err := strconv.Atoi(matches[1])
	if err != nil {
		return 0, 0, fmt.Errorf("parsing local port %q: %w", matches[1], err)
	}
	remotePort, err := strconv.Atoi(matches[2])
	if err != nil {
		return 0, 0, fmt.Errorf("parsing remote port %q: %w", matches[2], err)
	}

	return localPort, remotePort, nil
}

func scanPortForwardOutput(reader io.Reader, handleLine func(string)) error {
	scanner := bufio.NewScanner(reader)
	for scanner.Scan() {
		handleLine(scanner.Text())
	}

	return scanner.Err()
}

func appendPortForwardLog(output *bytes.Buffer, line string) {
	if output.Len() > 0 {
		output.WriteByte('\n')
	}
	output.WriteString(strings.TrimSpace(line))
}
