package forgejo

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/pejas/kagen/internal/git"
)

// ImportRepo ensures the repository exists in Forgejo and prepares it for the first push.
func (f *ForgejoService) ImportRepo(ctx context.Context, repo *git.Repository) error {
	ns := fmt.Sprintf("kagen-%s", repo.ID())
	podName, err := f.getForgejoPod(ctx, ns)
	if err != nil {
		return err
	}
	if err := f.ensureAdminUser(ctx, ns, podName); err != nil {
		return err
	}

	localPort, err := f.ensureForgejoRepo(ctx, ns, podName)
	if err != nil {
		return err
	}
	defer f.pf.Stop()

	remoteURL := fmt.Sprintf("http://kagen:kagen-internal-secret@127.0.0.1:%d/kagen/workspace.git", localPort)
	if err := repo.AddRemote("kagen", remoteURL); err != nil {
		return fmt.Errorf("configuring forgejo remote: %w", err)
	}

	return f.pushRepo(ctx, repo)
}

func (f *ForgejoService) ensureAdminUser(ctx context.Context, namespace, podName string) error {
	createAdminCmd := []string{
		"forgejo", "--config", forgejoConfigPath, "admin", "user", "create",
		"--username", "kagen",
		"--password", "kagen-internal-secret",
		"--email", "kagen@internal.local",
		"--admin",
		"--must-change-password=false",
	}

	var lastErr error
	for i := 0; i < 5; i++ {
		out, err := f.exec.Run(ctx, namespace, podName, createAdminCmd)
		if err == nil || strings.Contains(out, "already exists") || (err != nil && strings.Contains(err.Error(), "already exists")) {
			lastErr = nil
			break
		}
		lastErr = fmt.Errorf("kubectl exec %s/%s: %s: %w", namespace, podName, out, err)
		if sleepErr := sleepContext(ctx, 2*time.Second); sleepErr != nil {
			return sleepErr
		}
	}

	if lastErr != nil {
		return fmt.Errorf("creating forgejo admin user: %w", lastErr)
	}

	listCmd := []string{"forgejo", "--config", forgejoConfigPath, "admin", "user", "list"}
	out, err := f.exec.Run(ctx, namespace, podName, listCmd)
	if err != nil {
		return fmt.Errorf("listing forgejo admin users: %w", err)
	}
	if !strings.Contains(out, "kagen") {
		return fmt.Errorf("forgejo admin user kagen not found after creation")
	}

	return sleepContext(ctx, 2*time.Second)
}

func (f *ForgejoService) ensureForgejoRepo(ctx context.Context, namespace, podName string) (int, error) {
	var (
		lastErr   error
		localPort int
	)

	for i := 0; i < 5; i++ {
		localPort, lastErr = f.pf.Start(ctx, namespace, "pod/"+podName, 0, 3000)
		if lastErr != nil {
			lastErr = fmt.Errorf("starting port-forward to forgejo: %w", lastErr)
			if sleepErr := sleepContext(ctx, time.Second); sleepErr != nil {
				return 0, sleepErr
			}
			continue
		}

		if err := f.waitForAPI(ctx, localPort); err != nil {
			_ = f.pf.Stop()
			lastErr = err
			if sleepErr := sleepContext(ctx, time.Second); sleepErr != nil {
				return 0, sleepErr
			}
			continue
		}

		if err := f.createRepo(ctx, localPort, "workspace"); err != nil {
			_ = f.pf.Stop()
			lastErr = err
			if sleepErr := sleepContext(ctx, time.Second); sleepErr != nil {
				return 0, sleepErr
			}
			continue
		}

		return localPort, nil
	}

	return 0, fmt.Errorf("failed to create forgejo repo after retries: %w", lastErr)
}

func (f *ForgejoService) pushRepo(ctx context.Context, repo *git.Repository) error {
	refspecs := []string{
		"HEAD:" + repo.CurrentBranch,
		"HEAD:" + repo.KagenBranch(),
	}

	var lastErr error
	for i := 0; i < 5; i++ {
		if err := repo.PushRefspecs(ctx, "kagen", refspecs...); err == nil {
			return nil
		} else {
			lastErr = err
			if sleepErr := sleepContext(ctx, time.Second); sleepErr != nil {
				return sleepErr
			}
		}
	}

	return fmt.Errorf("pushing to forgejo: %w", lastErr)
}

func (f *ForgejoService) waitForAPI(ctx context.Context, port int) error {
	versionURL := fmt.Sprintf("http://127.0.0.1:%d/api/v1/version", port)

	for i := 0; i < 30; i++ {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, versionURL, nil)
			if err != nil {
				return fmt.Errorf("creating forgejo API readiness request: %w", err)
			}
			resp, err := f.httpClient.Do(req)
			if err == nil {
				resp.Body.Close()
				if resp.StatusCode == http.StatusOK {
					return nil
				}
			}
			if sleepErr := sleepContext(ctx, 500*time.Millisecond); sleepErr != nil {
				return sleepErr
			}
		}
	}

	return fmt.Errorf("timed out waiting for forgejo API on local port %d", port)
}

func (f *ForgejoService) createRepo(ctx context.Context, port int, name string) error {
	apiURL := fmt.Sprintf("http://127.0.0.1:%d/api/v1/user/repos", port)
	body, err := json.Marshal(map[string]interface{}{
		"name":           name,
		"private":        true,
		"auto_init":      false,
		"default_branch": "main",
	})
	if err != nil {
		return fmt.Errorf("marshalling repo create request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, bytes.NewBuffer(body))
	if err != nil {
		return fmt.Errorf("creating forgejo repo request: %w", err)
	}
	req.SetBasicAuth("kagen", "kagen-internal-secret")
	req.Header.Set("Content-Type", "application/json")

	resp, err := f.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("creating forgejo repo: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusConflict {
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	return nil
}

func sleepContext(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
