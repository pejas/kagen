package e2e

import (
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/cucumber/godog"
	"github.com/pejas/kagen/internal/agent"
	"github.com/pejas/kagen/internal/devfile"
)

type testContext struct {
	tmpDir     string
	out        bytes.Buffer
	err        error
	exitCode   int
	lastOutput string
}

func (c *testContext) aDirectoryThatIsAGitRepository() error {
	var err error
	c.tmpDir, err = os.MkdirTemp("", "kagen-e2e-*")
	if err != nil {
		return err
	}

	// Initialize real git repo
	realGit, err := exec.LookPath("git")
	if err != nil {
		return fmt.Errorf("git not found in PATH")
	}

	runGit := func(args ...string) error {
		cmd := exec.Command(realGit, args...)
		cmd.Dir = c.tmpDir
		out, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("git %v failed: %s: %w", args, string(out), err)
		}
		return nil
	}

	if err := runGit("init"); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(c.tmpDir, "README.md"), []byte("# Test Repo"), 0644); err != nil {
		return err
	}
	_ = runGit("config", "user.email", "e2e@example.com")
	_ = runGit("config", "user.name", "E2E Tester")
	_ = runGit("config", "commit.gpgsign", "false")
	if err := runGit("add", "."); err != nil {
		return err
	}
	if err := runGit("commit", "-m", "initial commit"); err != nil {
		return err
	}

	return nil
}

func (c *testContext) iRun(command string) error {
	args := strings.Fields(command)
	if args[0] != "kagen" {
		return fmt.Errorf("unexpected command: %s", args[0])
	}

	binPath, err := kagenBinaryPath()
	if err != nil {
		return err
	}

	cmd := exec.Command(binPath, args[1:]...)
	cmd.Dir = c.tmpDir

	// Inherit environment but force non-interactive
	cmd.Env = os.Environ()
	cmd.Env = append(cmd.Env, "KAGEN_NON_INTERACTIVE=true")
	cmd.Env = append(cmd.Env, "KAGEN_E2E=true")

	c.out.Reset()
	cmd.Stdout = &c.out
	cmd.Stderr = &c.out

	err = cmd.Run()
	c.lastOutput = c.out.String()

	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			c.exitCode = exitErr.ExitCode()
			c.err = err
			return nil
		}
		return err
	}
	c.exitCode = 0
	c.err = nil
	return nil
}

func kagenBinaryPath() (string, error) {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		return "", fmt.Errorf("resolving e2e test path: runtime caller unavailable")
	}

	return filepath.Join(filepath.Dir(filename), "..", "..", "bin", "kagen"), nil
}

func (c *testContext) theFileShouldExist(filename string) error {
	path := filepath.Join(c.tmpDir, filename)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return fmt.Errorf("expected file %s to exist", filename)
	}
	return nil
}

func (c *testContext) theOutputShouldContain(expected string) error {
	// Support simple regex-like matching for paths (e.g. "Created ... devfile.yaml")
	parts := strings.Split(expected, "...")
	allFound := true
	for _, part := range parts {
		if !strings.Contains(c.lastOutput, strings.TrimSpace(part)) {
			allFound = false
			break
		}
	}
	if allFound {
		return nil
	}
	return fmt.Errorf("expected output to contain %q, but got:\n%s", expected, c.lastOutput)
}

func (c *testContext) theFileDoesNotExist(filename string) error {
	path := filepath.Join(c.tmpDir, filename)
	_ = os.Remove(path) // Ignore error if already gone
	return nil
}

func (c *testContext) theFileExists(filename string) error {
	path := filepath.Join(c.tmpDir, filename)
	content, err := devfile.DefaultForAgent(agent.Codex)
	if err != nil {
		return err
	}
	return os.WriteFile(path, []byte(content), 0644)
}

func (c *testContext) colimaIsRunning() error {
	cmd := exec.Command("colima", "status", "-p", "kagen")
	out, err := cmd.CombinedOutput()
	if err != nil {
		// If it's not running, we try without -p kagen just in case
		cmd = exec.Command("colima", "status")
		out, _ = cmd.CombinedOutput()
	}
	if !strings.Contains(strings.ToLower(string(out)), "running") {
		return fmt.Errorf("colima is not running: %s", string(out))
	}
	return nil
}

func (c *testContext) itShouldEnsureTheLocalRuntimeIsHealthy() error {
	return c.theOutputShouldContain("Ensuring local runtime is healthy")
}

func (c *testContext) itShouldEnsureClusterResourcesAreReady() error {
	return c.theOutputShouldContain("Ensuring cluster resources")
}

func (c *testContext) itShouldImportTheRepositoryToForgejo() error {
	// The CLI says "Importing repository to Forgejo..." or "Ensuring forgejo repo..."
	return c.theOutputShouldContain("Importing repository")
}

func (c *testContext) itShouldAttachToTheAgent(agent string) error {
	return c.theOutputShouldContain(fmt.Sprintf("Launching agent %s", agent))
}

func (c *testContext) theExitCodeShouldBe(expectedCode int) error {
	if c.exitCode != expectedCode {
		return fmt.Errorf("expected exit code %d, but got %d (err: %v)", expectedCode, c.exitCode, c.err)
	}
	return nil
}

func (c *testContext) thereAreUncommittedLocalChanges() error {
	path := filepath.Join(c.tmpDir, "dirty.txt")
	return os.WriteFile(path, []byte("dirty"), 0644)
}

func (c *testContext) itShouldCreateAWIPCommit() error {
	return c.theOutputShouldContain("WIP commit")
}

func (c *testContext) itShouldFetchChangesFromForgejo() error {
	return c.theOutputShouldContain("Connecting to in-cluster Forgejo")
}

func (c *testContext) itShouldMergeTheChangesIntoTheCurrentBranch() error {
	return c.theOutputShouldContain("Merging changes")
}

func InitializeScenario(ctx *godog.ScenarioContext) {
	c := &testContext{}

	ctx.Before(func(ctx context.Context, sc *godog.Scenario) (context.Context, error) {
		return ctx, nil
	})

	ctx.After(func(ctx context.Context, sc *godog.Scenario, err error) (context.Context, error) {
		if c.tmpDir != "" {
			_ = cleanupNamespace(namespaceForTmpDir(c.tmpDir))
			os.RemoveAll(c.tmpDir)
		}
		return ctx, nil
	})

	ctx.Step(`^a directory that is a git repository$`, c.aDirectoryThatIsAGitRepository)
	ctx.Step(`^I run "([^"]*)"$`, c.iRun)
	ctx.Step(`^the file "([^"]*)" should exist$`, c.theFileShouldExist)
	ctx.Step(`^the output should contain "([^"]*)"$`, c.theOutputShouldContain)
	ctx.Step(`^the file "([^"]*)" does not exist$`, c.theFileDoesNotExist)
	ctx.Step(`^the file "([^"]*)" exists$`, c.theFileExists)
	ctx.Step(`^the exit code should be (\d+)$`, c.theExitCodeShouldBe)
	ctx.Step(`^colima is running$`, c.colimaIsRunning)
	ctx.Step(`^it should ensure the local runtime is healthy$`, c.itShouldEnsureTheLocalRuntimeIsHealthy)
	ctx.Step(`^it should ensure cluster resources are ready$`, c.itShouldEnsureClusterResourcesAreReady)
	ctx.Step(`^it should import the repository to Forgejo$`, c.itShouldImportTheRepositoryToForgejo)
	ctx.Step(`^it should attach to the agent "([^"]*)"$`, c.itShouldAttachToTheAgent)
	ctx.Step(`^there are uncommitted local changes$`, c.thereAreUncommittedLocalChanges)
	ctx.Step(`^it should create a WIP commit$`, c.itShouldCreateAWIPCommit)
	ctx.Step(`^it should fetch changes from Forgejo$`, c.itShouldFetchChangesFromForgejo)
	ctx.Step(`^it should merge the changes into the current branch$`, c.itShouldMergeTheChangesIntoTheCurrentBranch)
}

func TestFeatures(t *testing.T) {
	if _, err := exec.LookPath("kubectl"); err != nil {
		t.Skip("skipping e2e tests: kubectl not found in PATH")
	}
	if _, err := exec.LookPath("colima"); err != nil {
		t.Skip("skipping e2e: colima not found in PATH")
	}
	wd, _ := os.Getwd()
	binPath := filepath.Join(wd, "..", "..", "bin", "kagen")
	if _, err := os.Stat(binPath); err != nil {
		t.Skip("skipping e2e: bin/kagen not built")
	}

	if err := cleanupE2ENamespaces(); err != nil {
		t.Fatalf("cleaning e2e namespaces before test run: %v", err)
	}

	suite := godog.TestSuite{
		ScenarioInitializer: InitializeScenario,
		Options: &godog.Options{
			Format:   "pretty",
			Paths:    []string{"../../features"},
			TestingT: t,
		},
	}

	if suite.Run() != 0 {
		t.Fatal("non-zero status returned, failed to run feature tests")
	}
}

func namespaceForTmpDir(tmpDir string) string {
	h := sha1.New()
	h.Write([]byte(tmpDir))
	id := hex.EncodeToString(h.Sum(nil))[:8]
	return "kagen-" + id
}

func cleanupE2ENamespaces() error {
	cmd := exec.Command("kubectl", "get", "ns", "-l", "kagen.io/e2e=true", "-o", "name")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("listing e2e namespaces: %s: %w", string(out), err)
	}

	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line == "" {
			continue
		}
		ns := strings.TrimPrefix(line, "namespace/")
		if err := cleanupNamespace(ns); err != nil {
			return err
		}
	}

	return nil
}

func cleanupNamespace(ns string) error {
	deleteCmd := exec.Command("kubectl", "delete", "ns", ns, "--ignore-not-found=true", "--wait=false")
	if out, err := deleteCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("deleting namespace %s: %s: %w", ns, string(out), err)
	}

	waitCmd := exec.Command("kubectl", "wait", "--for=delete", "ns/"+ns, "--timeout=90s")
	if out, err := waitCmd.CombinedOutput(); err != nil {
		waitOutput := string(out)
		if strings.Contains(waitOutput, "not found") {
			return nil
		}
		return fmt.Errorf("waiting for namespace %s deletion: %s: %w", ns, waitOutput, err)
	}

	time.Sleep(500 * time.Millisecond)
	return nil
}
