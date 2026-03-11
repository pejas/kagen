package forgejo

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
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
	_ = repo.AddRemote("kagen", remoteURL)

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

	for i := 0; i < 5; i++ {
		out, err := f.exec.Run(ctx, namespace, podName, createAdminCmd)
		if err == nil || strings.Contains(out, "already exists") || strings.Contains(err.Error(), "already exists") {
			break
		}
		fmt.Fprintf(os.Stderr, "ℹ Retry user creation (%d/5)... output: %s, err: %v\n", i+1, out, err)
		time.Sleep(2 * time.Second)
	}

	listCmd := []string{"forgejo", "--config", forgejoConfigPath, "admin", "user", "list"}
	out, _ := f.exec.Run(ctx, namespace, podName, listCmd)
	if !strings.Contains(out, "kagen") {
		fmt.Fprintf(os.Stderr, "⚠ User 'kagen' not found in user list: %s\n", out)
	}

	time.Sleep(2 * time.Second)
	return nil
}

func (f *ForgejoService) ensureForgejoRepo(ctx context.Context, namespace, podName string) (int, error) {
	var (
		lastErr   error
		localPort int
	)

	for i := 0; i < 5; i++ {
		localPort, lastErr = f.pf.Start(ctx, namespace, "pod/"+podName, 3000)
		if lastErr != nil {
			lastErr = fmt.Errorf("starting port-forward to forgejo: %w", lastErr)
			time.Sleep(1 * time.Second)
			continue
		}

		if err := f.waitForAPI(ctx, localPort); err != nil {
			_ = f.pf.Stop()
			lastErr = err
			time.Sleep(1 * time.Second)
			continue
		}

		if err := f.createRepo(localPort, "workspace"); err != nil {
			_ = f.pf.Stop()
			lastErr = err
			time.Sleep(1 * time.Second)
			continue
		}

		return localPort, nil
	}

	return 0, fmt.Errorf("failed to create forgejo repo after retries: %w", lastErr)
}

func (f *ForgejoService) pushRepo(ctx context.Context, repo *git.Repository) error {
	var lastErr error
	for i := 0; i < 5; i++ {
		if err := repo.Push(ctx, "kagen", "HEAD:"+repo.KagenBranch()); err == nil {
			return nil
		} else {
			lastErr = err
			time.Sleep(1 * time.Second)
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
			resp, err := http.Get(versionURL)
			if err == nil {
				resp.Body.Close()
				if resp.StatusCode == http.StatusOK {
					return nil
				}
			}
			time.Sleep(500 * time.Millisecond)
		}
	}

	return fmt.Errorf("timed out waiting for forgejo API on local port %d", port)
}

func (f *ForgejoService) createRepo(port int, name string) error {
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

	req, err := http.NewRequest(http.MethodPost, apiURL, bytes.NewBuffer(body))
	if err != nil {
		return err
	}
	req.SetBasicAuth("kagen", "kagen-internal-secret")
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusConflict {
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	return nil
}
