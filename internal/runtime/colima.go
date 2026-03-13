package runtime

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/pejas/kagen/internal/config"
	"github.com/pejas/kagen/internal/ui"
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
	ui.Verbose("Checking runtime dependencies")
	if err := CheckConfigDependencies(); err != nil {
		return err
	}

	// 2. Check current status.
	status, err := c.Status(ctx)
	if err != nil {
		return fmt.Errorf("checking status: %w", err)
	}
	ui.Verbose("Colima profile %q status is %s", colimaProfile, status)

	if status == StatusRunning {
		ui.Verbose("Reusing existing kube context %s", kubeContext)
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
	ui.Verbose(
		"Starting Colima profile %q with cpu=%d memory=%dGi disk=%dGi",
		colimaProfile,
		c.cfg.CPU,
		c.cfg.Memory,
		c.cfg.Disk,
	)

	cmd := exec.CommandContext(ctx, "colima", args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("starting colima: %w (output: %s)", err, string(out))
	}

	// 4. Verify K3s health.
	ui.Verbose("Colima started; waiting for Kubernetes readiness on context %s", kubeContext)
	return c.waitReady(ctx)
}

// Stop shuts down Colima if it is currently running.
func (c *ColimaManager) Stop(ctx context.Context) error {
	if err := checkColimaDependency(); err != nil {
		return err
	}

	status, err := c.Status(ctx)
	if err != nil {
		return fmt.Errorf("checking status: %w", err)
	}
	if status == StatusStopped {
		ui.Verbose("Colima profile %q is already stopped", colimaProfile)
		return nil
	}

	ui.Verbose("Stopping Colima profile %q", colimaProfile)
	cmd := exec.CommandContext(ctx, "colima", "stop", "--profile", colimaProfile)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("stopping colima: %w (output: %s)", err, string(out))
	}

	return nil
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
	ui.Verbose("Waiting up to %s for Kubernetes nodes to become ready", timeout)

	deadline := time.Now().Add(timeout)
	attempt := 0
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			attempt++
			// check if node is ready
			checkCmd := exec.CommandContext(ctx, "kubectl", "--context", kubeContext, "get", "nodes")
			out, err := checkCmd.CombinedOutput()
			if err == nil {
				ui.Verbose("Kubernetes readiness succeeded on attempt %d", attempt)
				return nil
			}
			if ui.VerboseEnabled() && (attempt == 1 || attempt%3 == 0) {
				ui.Verbose("Kubernetes readiness attempt %d still pending: %s", attempt, trimCommandOutput(string(out)))
			}
			time.Sleep(5 * time.Second)
		}
	}

	return fmt.Errorf("timed out waiting for kubernetes node to be ready")
}

func trimCommandOutput(out string) string {
	trimmed := strings.TrimSpace(out)
	if trimmed == "" {
		return "no command output"
	}

	trimmed = strings.Join(strings.Fields(trimmed), " ")
	if len(trimmed) > 160 {
		return trimmed[:157] + "..."
	}

	return trimmed
}
