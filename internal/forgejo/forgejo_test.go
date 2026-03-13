package forgejo

import (
	"testing"

	"github.com/pejas/kagen/internal/git"
	corev1 "k8s.io/api/core/v1"
)

func TestReviewSessionReviewURLUsesActiveBaseURL(t *testing.T) {
	t.Parallel()

	session := &ReviewSession{baseURL: "http://127.0.0.1:54321"}
	repo := &git.Repository{CurrentBranch: "feature/x"}

	want := "http://127.0.0.1:54321/kagen/workspace/src/branch/kagen%2Ffeature%2Fx"
	if got := session.ReviewURL(repo); got != want {
		t.Fatalf("ReviewURL() = %q, want %q", got, want)
	}
}

func TestAuthFromSecretRejectsMissingValues(t *testing.T) {
	t.Parallel()

	_, err := authFromSecret(&corev1.Secret{
		Data: map[string][]byte{
			forgejoSecretUsernameKey: []byte(forgejoAdminUsername),
		},
	})
	if err == nil {
		t.Fatal("authFromSecret() expected error for missing password")
	}
}
