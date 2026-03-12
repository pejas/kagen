package forgejo

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/pejas/kagen/internal/cluster"
	kagerr "github.com/pejas/kagen/internal/errors"
	"github.com/pejas/kagen/internal/git"
	"github.com/pejas/kagen/internal/kubeexec"
	"k8s.io/client-go/kubernetes"
)

// ForgejoService implements the Service interface using client-go.
type ForgejoService struct {
	client     *kubernetes.Clientset
	pf         cluster.PortForwarder
	exec       kubeexec.Runner
	httpClient *http.Client
}

const forgejoConfigPath = "/etc/gitea/app.ini"
const forgejoHTTPTimeout = 2 * time.Second

// NewForgejoService returns a new ForgejoService.
func NewForgejoService(client *kubernetes.Clientset, pf cluster.PortForwarder, execRunner kubeexec.Runner) *ForgejoService {
	return &ForgejoService{
		client: client,
		pf:     pf,
		exec:   execRunner,
		httpClient: &http.Client{
			Timeout: forgejoHTTPTimeout,
		},
	}
}

// GetReviewURL returns the local browser URL for the repository review in Forgejo.
func (f *ForgejoService) GetReviewURL(repo *git.Repository) (string, error) {
	return fmt.Sprintf(
		"http://localhost:3000/kagen/workspace/src/branch/%s",
		url.PathEscape(repo.KagenBranch()),
	), nil
}

// HasNewCommits checks if there are commits in Forgejo not yet pulled local.
func (f *ForgejoService) HasNewCommits(ctx context.Context, repo *git.Repository) (bool, error) {
	return false, fmt.Errorf("forgejo has new commits: %w", kagerr.ErrNotImplemented)
}
