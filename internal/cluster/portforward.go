package cluster

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// KubectlPortForwarder implements PortForwarder using os/exec and kubectl.
type KubectlPortForwarder struct {
	cmd *exec.Cmd
}

// NewPortForwarder returns a new KubectlPortForwarder.
func NewPortForwarder() *KubectlPortForwarder {
	return &KubectlPortForwarder{}
}

// Start begins the port-forward in the background.
// namespace: the K8s namespace
// target: the target service or pod name (e.g., "svc/forgejo" or "pod/agent")
// port: the container/service port to forward
// Returns the local port assigned by kubectl (if 0 was requested, it finds a random one), or an error.
func (p *KubectlPortForwarder) Start(ctx context.Context, namespace, target string, port int) (int, error) {
	// If port is 0, we'll let kubectl pick one by passing ":<port>"
	localPortStr := "0"
	if port > 0 {
		// Try to find a free port on localhost if 0 was requested,
		// but kubectl does this better.
	}

	args := []string{"port-forward", "-n", namespace, target, fmt.Sprintf("%s:%d", localPortStr, port)}
	p.cmd = exec.CommandContext(ctx, "kubectl", args...)

	var stderr bytes.Buffer
	p.cmd.Stderr = &stderr
	stdout, err := p.cmd.StdoutPipe()
	if err != nil {
		return 0, fmt.Errorf("getting stdout pipe: %w", err)
	}

	if err := p.cmd.Start(); err != nil {
		return 0, fmt.Errorf("starting port-forward: %w", err)
	}

	// Wait for the "Forwarding from 127.0.0.1:XXXXX -> YYYY" message.
	buf := make([]byte, 1024)
	n, err := stdout.Read(buf)
	if err != nil {
		_ = p.cmd.Process.Kill()
		return 0, fmt.Errorf("reading kubectl output: %w (stderr: %s)", err, stderr.String())
	}

	output := string(buf[:n])
	if !strings.Contains(output, "Forwarding from") {
		_ = p.cmd.Process.Kill()
		return 0, fmt.Errorf("unexpected kubectl output: %s", output)
	}

	// Parse the assigned local port
	// Example: "Forwarding from 127.0.0.1:56789 -> 3000"
	parts := strings.Split(output, ":")
	if len(parts) < 2 {
		_ = p.cmd.Process.Kill()
		return 0, fmt.Errorf("failed to parse port from output: %s", output)
	}
	portPart := strings.Split(parts[1], " ")[0]
	localPort, err := strconv.Atoi(portPart)
	if err != nil {
		_ = p.cmd.Process.Kill()
		return 0, fmt.Errorf("invalid port in output: %s", portPart)
	}

	// Verify the port is actually listening
	if err := waitForPort(localPort, 5*time.Second); err != nil {
		_ = p.cmd.Process.Kill()
		return 0, err
	}

	return localPort, nil
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
