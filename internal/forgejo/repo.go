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
	"github.com/pejas/kagen/internal/ui"
)

// ImportRepo ensures the repository exists in Forgejo and prepares it for the first push.
func (f *ForgejoService) ImportRepo(ctx context.Context, repo *git.Repository) error {
	ns := forgejoNamespace(repo)
	podName, err := f.getForgejoPod(ctx, ns)
	if err != nil {
		return err
	}
	ui.Verbose("Using Forgejo pod %s/%s for repository import", ns, podName)
	if err := f.ensureAdminUser(ctx, ns, podName); err != nil {
		return err
	}

	auth, err := f.credentials(ctx, ns)
	if err != nil {
		return fmt.Errorf("loading forgejo credentials: %w", err)
	}

	session, err := f.ensureForgejoRepo(ctx, ns, podName, auth)
	if err != nil {
		return err
	}
	defer func() {
		_ = session.Stop()
	}()

	return f.pushRepo(ctx, repo, session)
}

func (f *ForgejoService) ensureAdminUser(ctx context.Context, namespace, podName string) error {
	createAdminCmd := []string{
		"forgejo", "--config", forgejoConfigPath, "admin", "user", "create",
		"--username", forgejoAdminUsername,
		"--password", forgejoAdminPassword,
		"--email", forgejoAdminUsername + "@internal.local",
		"--admin",
		"--must-change-password=false",
	}

	var lastErr error
	for i := 0; i < 5; i++ {
		out, err := f.exec.Run(ctx, namespace, podName, createAdminCmd)
		if err == nil || strings.Contains(out, "already exists") || (err != nil && strings.Contains(err.Error(), "already exists")) {
			ui.Verbose("Forgejo admin user %q is ready", forgejoAdminUsername)
			lastErr = nil
			break
		}
		lastErr = fmt.Errorf("kubectl exec %s/%s: %s: %w", namespace, podName, out, err)
		if ui.VerboseEnabled() {
			ui.Verbose("Forgejo admin bootstrap retry %d/5 failed: %v", i+1, lastErr)
		}
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

func (f *ForgejoService) ensureForgejoRepo(ctx context.Context, namespace, podName string, auth *git.BasicAuth) (*ReviewSession, error) {
	var (
		lastErr error
		session *ReviewSession
	)

	for i := 0; i < 5; i++ {
		forward, err := f.pf.Start(ctx, namespace, "pod/"+podName, 0, forgejoServiceHTTPPort)
		lastErr = err
		if lastErr != nil {
			lastErr = fmt.Errorf("starting port-forward to forgejo: %w", lastErr)
			if ui.VerboseEnabled() {
				ui.Verbose("Forgejo repo bootstrap port-forward retry %d/5 failed: %v", i+1, lastErr)
			}
			if sleepErr := sleepContext(ctx, time.Second); sleepErr != nil {
				return nil, sleepErr
			}
			continue
		}

		localPort := forward.LocalPort()
		if err := f.waitForAPI(ctx, localPort); err != nil {
			_ = forward.Stop()
			lastErr = err
			if ui.VerboseEnabled() {
				ui.Verbose("Forgejo repo bootstrap API wait retry %d/5 failed: %v", i+1, lastErr)
			}
			if sleepErr := sleepContext(ctx, time.Second); sleepErr != nil {
				return nil, sleepErr
			}
			continue
		}

		session = &ReviewSession{
			baseURL: fmt.Sprintf("http://127.0.0.1:%d", localPort),
			repoURL: fmt.Sprintf("http://127.0.0.1:%d/%s/%s.git", localPort, forgejoRepoOwner, forgejoRepoName),
			auth:    auth,
			forward: forward,
		}

		if err := f.createRepo(ctx, session, forgejoRepoName); err != nil {
			_ = forward.Stop()
			lastErr = err
			if ui.VerboseEnabled() {
				ui.Verbose("Forgejo repo creation retry %d/5 failed: %v", i+1, lastErr)
			}
			if sleepErr := sleepContext(ctx, time.Second); sleepErr != nil {
				return nil, sleepErr
			}
			continue
		}

		ui.Verbose("Forgejo repository %s/%s is ready", forgejoRepoOwner, forgejoRepoName)
		return session, nil
	}

	return nil, fmt.Errorf("failed to create forgejo repo after retries: %w", lastErr)
}

func (f *ForgejoService) pushRepo(ctx context.Context, repo *git.Repository, session *ReviewSession) error {
	refspecs := []string{
		"HEAD:" + repo.CurrentBranch,
		"HEAD:" + repo.KagenBranch(),
	}

	var lastErr error
	for i := 0; i < 5; i++ {
		if err := repo.PushURL(ctx, session.repoURL, session.auth, refspecs...); err == nil {
			ui.Verbose("Pushed canonical and review refs to Forgejo on attempt %d", i+1)
			return nil
		} else {
			lastErr = err
			if ui.VerboseEnabled() {
				ui.Verbose("Forgejo push retry %d/5 failed: %v", i+1, lastErr)
			}
			if sleepErr := sleepContext(ctx, time.Second); sleepErr != nil {
				return sleepErr
			}
		}
	}

	return fmt.Errorf("pushing to forgejo: %w", lastErr)
}

func (f *ForgejoService) createRepo(ctx context.Context, session *ReviewSession, name string) error {
	apiURL := session.baseURL + "/api/v1/user/repos"
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
	req.SetBasicAuth(session.auth.Username, session.auth.Password)
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
