package forgejo

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/pejas/kagen/internal/cluster"
	kagerr "github.com/pejas/kagen/internal/errors"
	"github.com/pejas/kagen/internal/git"
	"github.com/pejas/kagen/internal/kubeexec"
	"github.com/pejas/kagen/internal/ui"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

const (
	forgejoConfigPath          = "/etc/gitea/app.ini"
	forgejoHTTPTimeout         = 2 * time.Second
	forgejoServiceName         = "forgejo"
	forgejoServiceHTTPPort     = 3000
	forgejoAdminUsername       = "kagen"
	forgejoAdminPassword       = "kagen-internal-secret"
	forgejoBootstrapSecretName = "forgejo-bootstrap-auth"
	forgejoSecretUsernameKey   = "username"
	forgejoSecretPasswordKey   = "password"
	forgejoRepoOwner           = "kagen"
	forgejoRepoName            = "workspace"
)

// ForgejoService implements the Service interface using client-go.
type ForgejoService struct {
	client     *kubernetes.Clientset
	pf         *cluster.PortForwarder
	exec       kubeexec.Runner
	httpClient *http.Client
}

// ReviewSession keeps a live Forgejo HTTP transport open for browser review.
type ReviewSession struct {
	baseURL   string
	repoURL   string
	auth      *git.BasicAuth
	forward   *cluster.ForwardSession
	stopOnce  sync.Once
	stopError error
}

// NewForgejoService returns a new ForgejoService.
func NewForgejoService(client *kubernetes.Clientset, pf *cluster.PortForwarder, execRunner kubeexec.Runner) *ForgejoService {
	return &ForgejoService{
		client: client,
		pf:     pf,
		exec:   execRunner,
		httpClient: &http.Client{
			Timeout: forgejoHTTPTimeout,
		},
	}
}

// StartReviewSession establishes the HTTP transport required to browse Forgejo.
func (f *ForgejoService) StartReviewSession(ctx context.Context, repo *git.Repository) (*ReviewSession, error) {
	ns := forgejoNamespace(repo)
	ui.Verbose("Starting Forgejo review session for namespace %s", ns)
	auth, err := f.credentials(ctx, ns)
	if err != nil {
		return nil, fmt.Errorf("loading forgejo credentials: %w", err)
	}

	forward, err := f.pf.Start(ctx, ns, "svc/"+forgejoServiceName, 0, forgejoServiceHTTPPort)
	if err != nil {
		return nil, fmt.Errorf("starting forgejo review transport: %w", err)
	}

	localPort := forward.LocalPort()
	if err := f.waitForAPI(ctx, localPort); err != nil {
		_ = forward.Stop()
		return nil, err
	}

	baseURL := fmt.Sprintf("http://127.0.0.1:%d", localPort)
	ui.Verbose("Forgejo review session ready on %s", baseURL)
	return &ReviewSession{
		baseURL: baseURL,
		repoURL: baseURL + "/" + forgejoRepoOwner + "/" + forgejoRepoName + ".git",
		auth:    auth,
		forward: forward,
	}, nil
}

// HasNewCommits checks if the review branch differs from the canonical branch.
func (f *ForgejoService) HasNewCommits(ctx context.Context, repo *git.Repository) (bool, error) {
	session, err := f.StartReviewSession(ctx, repo)
	if err != nil {
		return false, err
	}
	defer func() {
		_ = session.Stop()
	}()

	return session.HasNewCommits(ctx, repo)
}

// FetchReviewRefs fetches the canonical and review branches into local
// remote-tracking refs without persisting any host-side remote configuration.
func (f *ForgejoService) FetchReviewRefs(ctx context.Context, repo *git.Repository) error {
	return f.withGitTransport(ctx, repo, func(session *ReviewSession) error {
		refspecs := []string{"refs/heads/*:refs/remotes/kagen/*"}
		if err := repo.FetchURL(ctx, session.repoURL, session.auth, refspecs...); err != nil {
			return fmt.Errorf("fetching forgejo review refs: %w", err)
		}

		return nil
	})
}

// ReviewURL returns the live browser URL for the review branch.
func (s *ReviewSession) ReviewURL(repo *git.Repository) string {
	return fmt.Sprintf(
		"%s/%s/%s/src/branch/%s",
		s.baseURL,
		forgejoRepoOwner,
		forgejoRepoName,
		url.PathEscape(repo.KagenBranch()),
	)
}

// HasNewCommits reports whether the in-cluster review branch has diverged from
// the canonical branch.
func (s *ReviewSession) HasNewCommits(ctx context.Context, repo *git.Repository) (bool, error) {
	reviewSHA, reviewExists, err := s.remoteBranchSHA(ctx, repo, repo.KagenBranch())
	if err != nil {
		return false, err
	}
	if !reviewExists {
		return false, nil
	}

	baseSHA, baseExists, err := s.remoteBranchSHA(ctx, repo, repo.CurrentBranch)
	if err != nil {
		return false, err
	}
	if !baseExists {
		return reviewSHA != repo.HeadSHA, nil
	}

	return reviewSHA != baseSHA, nil
}

// Stop tears down the live Forgejo transport.
func (s *ReviewSession) Stop() error {
	s.stopOnce.Do(func() {
		if s.forward != nil {
			ui.Verbose("Stopping Forgejo review session for %s", s.baseURL)
			s.stopError = s.forward.Stop()
		}
	})

	return s.stopError
}

// Done closes when the underlying review transport exits.
func (s *ReviewSession) Done() <-chan struct{} {
	if s == nil || s.forward == nil {
		ch := make(chan struct{})
		close(ch)
		return ch
	}

	return s.forward.Done()
}

// Wait blocks until the underlying review transport exits.
func (s *ReviewSession) Wait() error {
	if s == nil || s.forward == nil {
		return nil
	}

	return s.forward.Wait()
}

func (s *ReviewSession) remoteBranchSHA(ctx context.Context, repo *git.Repository, branch string) (string, bool, error) {
	ref := "refs/heads/" + branch
	sha, ok, err := repo.RemoteRefSHA(ctx, s.repoURL, s.auth, ref)
	if err != nil {
		return "", false, fmt.Errorf("resolving remote ref %s: %w", ref, err)
	}

	return sha, ok, nil
}

func (f *ForgejoService) withGitTransport(ctx context.Context, repo *git.Repository, fn func(*ReviewSession) error) error {
	session, err := f.StartReviewSession(ctx, repo)
	if err != nil {
		return err
	}
	defer func() {
		_ = session.Stop()
	}()

	return fn(session)
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
				_ = resp.Body.Close()
				if resp.StatusCode == http.StatusOK {
					ui.Verbose("Forgejo API responded on local port %d after %d attempt(s)", port, i+1)
					return nil
				}
			}
			if ui.VerboseEnabled() && (i == 0 || (i+1)%5 == 0) {
				ui.Verbose("Waiting for Forgejo API on local port %d (attempt %d/30)", port, i+1)
			}
			if sleepErr := sleepContext(ctx, 500*time.Millisecond); sleepErr != nil {
				return sleepErr
			}
		}
	}

	return fmt.Errorf("timed out waiting for forgejo API on local port %d", port)
}

func (f *ForgejoService) credentials(ctx context.Context, namespace string) (*git.BasicAuth, error) {
	if f.client == nil {
		return defaultForgejoAuth(), nil
	}

	secret, err := f.client.CoreV1().Secrets(namespace).Get(ctx, forgejoBootstrapSecretName, metav1.GetOptions{})
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return defaultForgejoAuth(), nil
		}
		return nil, err
	}

	return authFromSecret(secret)
}

func authFromSecret(secret *corev1.Secret) (*git.BasicAuth, error) {
	if secret == nil {
		return defaultForgejoAuth(), nil
	}

	username := string(secret.Data[forgejoSecretUsernameKey])
	password := string(secret.Data[forgejoSecretPasswordKey])
	if username == "" || password == "" {
		return nil, fmt.Errorf("%w: forgejo bootstrap secret is missing username or password", kagerr.ErrClusterUnhealthy)
	}

	return &git.BasicAuth{Username: username, Password: password}, nil
}

func defaultForgejoAuth() *git.BasicAuth {
	return &git.BasicAuth{
		Username: forgejoAdminUsername,
		Password: forgejoAdminPassword,
	}
}

func forgejoNamespace(repo *git.Repository) string {
	return fmt.Sprintf("kagen-%s", repo.ID())
}
