package runtime

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/pejas/kagen/internal/config"
)

const (
	colimaProfile = "kagen"
	kubeContext   = "colima-kagen"
)

// ColimaManager handles the lifecycle of the Colima/K3s runtime.
type ColimaManager struct {
	cfg config.RuntimeConfig
}

// NewColimaManager returns a new ColimaManager with the provided config.
func NewColimaManager(cfg config.RuntimeConfig) *ColimaManager {
	return &ColimaManager{cfg: cfg}
}

// EnsureRunning starts Colima if it is not already running.
func (c *ColimaManager) EnsureRunning(ctx context.Context) error {
	// 1. Check dependencies first.
	if err := CheckConfigDependencies(); err != nil {
		return err
	}

	// 2. Check current status.
	status, err := c.Status(ctx)
	if err != nil {
		return fmt.Errorf("checking status: %w", err)
	}

	if status == StatusRunning {
		return nil
	}

	// 3. Start Colima.
	args := []string{
		"start",
		"--profile", colimaProfile,
		"--kubernetes",
		"--cpu", fmt.Sprintf("%d", c.cfg.CPU),
		"--memory", fmt.Sprintf("%d", c.cfg.Memory),
		"--disk", fmt.Sprintf("%d", c.cfg.Disk),
	}

	cmd := exec.CommandContext(ctx, "colima", args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("starting colima: %w (output: %s)", err, string(out))
	}

	// 4. Verify K3s health.
	return c.waitReady(ctx)
}

// Status determined the current state by running 'colima status'.
func (c *ColimaManager) Status(ctx context.Context) (Status, error) {
	cmd := exec.CommandContext(ctx, "colima", "status", "--profile", colimaProfile)
	out, err := cmd.CombinedOutput()
	if err != nil {
		// Exit code 1 usually means strictly "not running" or "not found".
		if strings.Contains(string(out), "not found") || strings.Contains(string(out), "is not running") {
			return StatusStopped, nil
		}
		return StatusUnknown, fmt.Errorf("colima status: %w (output: %s)", err, string(out))
	}

	if strings.Contains(string(out), "colima [profile kagen] is running") {
		return StatusRunning, nil
	}

	return StatusStopped, nil
}

// KubeContext returns the context name for the kagen profile.
func (c *ColimaManager) KubeContext() string {
	return kubeContext
}

func (c *ColimaManager) waitReady(ctx context.Context) error {
	timeout, err := time.ParseDuration(c.cfg.StartupTimeout)
	if err != nil {
		timeout = 5 * time.Minute
	}

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			// check if node is ready
			checkCmd := exec.CommandContext(ctx, "kubectl", "--context", kubeContext, "get", "nodes")
			if err := checkCmd.Run(); err == nil {
				return nil
			}
			time.Sleep(5 * time.Second)
		}
	}

	return fmt.Errorf("timed out waiting for kubernetes node to be ready")
}
