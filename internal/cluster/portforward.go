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
	"time"
)

// KubectlPortForwarder implements PortForwarder using os/exec and kubectl.
type KubectlPortForwarder struct {
	cmd *exec.Cmd
}

var forwardingLinePattern = regexp.MustCompile(`^Forwarding from .+:(\d+) -> (\d+)$`)

// NewPortForwarder returns a new KubectlPortForwarder.
func NewPortForwarder() *KubectlPortForwarder {
	return &KubectlPortForwarder{}
}

// Start begins the port-forward in the background.
// namespace: the K8s namespace
// target: the target service or pod name (e.g., "svc/forgejo" or "pod/agent")
// localPort: the requested local port; set to 0 to let kubectl choose an ephemeral port
// remotePort: the target service or pod port to forward
// Returns the actual local port assigned by kubectl, or an error.
func (p *KubectlPortForwarder) Start(ctx context.Context, namespace, target string, localPort, remotePort int) (int, error) {
	portSpec, err := portForwardSpec(localPort, remotePort)
	if err != nil {
		return 0, err
	}

	args := []string{"port-forward", "-n", namespace, target, portSpec}
	p.cmd = exec.CommandContext(ctx, "kubectl", args...)

	stdout, err := p.cmd.StdoutPipe()
	if err != nil {
		return 0, fmt.Errorf("getting stdout pipe: %w", err)
	}
	stderr, err := p.cmd.StderrPipe()
	if err != nil {
		return 0, fmt.Errorf("getting stderr pipe: %w", err)
	}

	if err := p.cmd.Start(); err != nil {
		return 0, fmt.Errorf("starting port-forward: %w", err)
	}

	lineCh := make(chan string, 16)
	readErrCh := make(chan error, 2)
	waitCh := make(chan error, 1)

	go scanPortForwardOutput(stdout, lineCh, readErrCh)
	go scanPortForwardOutput(stderr, lineCh, readErrCh)
	go func() {
		waitCh <- p.cmd.Wait()
	}()

	var output bytes.Buffer
	for {
		select {
		case <-ctx.Done():
			_ = p.Stop()
			return 0, ctx.Err()
		case line := <-lineCh:
			if strings.TrimSpace(line) == "" {
				continue
			}
			appendPortForwardLog(&output, line)

			forwardedLocalPort, forwardedRemotePort, err := parseForwardingLine(line)
			if err != nil {
				if !strings.Contains(line, "Forwarding from") {
					continue
				}
				_ = p.Stop()
				return 0, fmt.Errorf("parsing port-forward output %q: %w", line, err)
			}

			if forwardedRemotePort != remotePort {
				_ = p.Stop()
				return 0, fmt.Errorf("kubectl port-forward forwarded remote port %d, want %d", forwardedRemotePort, remotePort)
			}
			if localPort > 0 && forwardedLocalPort != localPort {
				_ = p.Stop()
				return 0, fmt.Errorf("kubectl port-forward bound local port %d, want %d", forwardedLocalPort, localPort)
			}
			if err := waitForPort(forwardedLocalPort, 5*time.Second); err != nil {
				_ = p.Stop()
				return 0, err
			}

			return forwardedLocalPort, nil
		case err := <-readErrCh:
			if err != nil {
				_ = p.Stop()
				return 0, fmt.Errorf("reading kubectl port-forward output: %w", err)
			}
		case err := <-waitCh:
			if err != nil {
				return 0, fmt.Errorf("kubectl port-forward %s %s: %s: %w", namespace, target, strings.TrimSpace(output.String()), err)
			}
			return 0, fmt.Errorf("kubectl port-forward %s %s exited before reporting a forwarded port: %s", namespace, target, strings.TrimSpace(output.String()))
		}
	}
}

// Stop terminates the port-forward process.
func (p *KubectlPortForwarder) Stop() error {
	if p.cmd != nil && p.cmd.Process != nil {
		return p.cmd.Process.Kill()
	}
	return nil
}

func waitForPort(port int, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)), 100*time.Millisecond)
		if err == nil {
			conn.Close()
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

func scanPortForwardOutput(reader io.Reader, lineCh chan<- string, errCh chan<- error) {
	scanner := bufio.NewScanner(reader)
	for scanner.Scan() {
		lineCh <- scanner.Text()
	}
	if err := scanner.Err(); err != nil {
		errCh <- err
	}
}

func appendPortForwardLog(output *bytes.Buffer, line string) {
	if output.Len() > 0 {
		output.WriteByte('\n')
	}
	output.WriteString(strings.TrimSpace(line))
}
