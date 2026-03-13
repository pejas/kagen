package cmd

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/pejas/kagen/internal/git"
)

type fakeReviewSession struct {
	hasNewCommits bool
	hasErr        error
	reviewURL     string
	stopCalls     int
	done          chan struct{}
	waitErr       error
}

func (f *fakeReviewSession) HasNewCommits(_ context.Context, _ *git.Repository) (bool, error) {
	if f.hasErr != nil {
		return false, f.hasErr
	}

	return f.hasNewCommits, nil
}

func (f *fakeReviewSession) ReviewURL(_ *git.Repository) string {
	return f.reviewURL
}

func (f *fakeReviewSession) Stop() error {
	f.stopCalls++
	return nil
}

func (f *fakeReviewSession) Done() <-chan struct{} {
	return f.done
}

func (f *fakeReviewSession) Wait() error {
	return f.waitErr
}

func TestOpenReviewOpensLiveReviewURLAndWaitsForShutdown(t *testing.T) {
	t.Parallel()

	repo := &git.Repository{CurrentBranch: "feature/x"}
	session := &fakeReviewSession{
		hasNewCommits: true,
		reviewURL:     "http://127.0.0.1:54321/kagen/workspace/src/branch/kagen%2Ffeature%2Fx",
	}

	var openedURL string
	waitCalls := 0

	err := openReview(
		t.Context(),
		repo,
		func(context.Context, *git.Repository) (reviewSession, error) {
			return session, nil
		},
		func(url string) error {
			openedURL = url
			return nil
		},
		func(context.Context) error {
			waitCalls++
			return nil
		},
	)
	if err != nil {
		t.Fatalf("openReview() returned error: %v", err)
	}
	if openedURL != session.reviewURL {
		t.Fatalf("opened URL = %q, want %q", openedURL, session.reviewURL)
	}
	if waitCalls != 1 {
		t.Fatalf("wait call count = %d, want 1", waitCalls)
	}
	if session.stopCalls != 1 {
		t.Fatalf("stop call count = %d, want 1", session.stopCalls)
	}
}

func TestOpenReviewSkipsBrowserWhenNoReviewableChangesExist(t *testing.T) {
	t.Parallel()

	repo := &git.Repository{CurrentBranch: "feature/x"}
	session := &fakeReviewSession{}
	browserCalled := false
	waitCalled := false

	err := openReview(
		t.Context(),
		repo,
		func(context.Context, *git.Repository) (reviewSession, error) {
			return session, nil
		},
		func(string) error {
			browserCalled = true
			return nil
		},
		func(context.Context) error {
			waitCalled = true
			return nil
		},
	)
	if err != nil {
		t.Fatalf("openReview() returned error: %v", err)
	}
	if browserCalled {
		t.Fatal("browser should not open when there are no reviewable changes")
	}
	if waitCalled {
		t.Fatal("wait function should not be called when there are no reviewable changes")
	}
}

func TestOpenReviewPropagatesReviewSessionErrors(t *testing.T) {
	t.Parallel()

	repo := &git.Repository{CurrentBranch: "feature/x"}
	session := &fakeReviewSession{hasErr: errors.New("boom")}

	err := openReview(
		t.Context(),
		repo,
		func(context.Context, *git.Repository) (reviewSession, error) {
			return session, nil
		},
		func(string) error { return nil },
		func(context.Context) error { return nil },
	)
	if err == nil {
		t.Fatal("openReview() expected error")
	}
}

func TestOpenReviewFailsWhenTransportTerminatesEarly(t *testing.T) {
	t.Parallel()

	done := make(chan struct{})
	close(done)

	repo := &git.Repository{CurrentBranch: "feature/x"}
	session := &fakeReviewSession{
		hasNewCommits: true,
		reviewURL:     "http://127.0.0.1:54321/kagen/workspace/src/branch/kagen%2Ffeature%2Fx",
		done:          done,
		waitErr:       errors.New("port-forward exited"),
	}

	err := openReview(
		t.Context(),
		repo,
		func(context.Context, *git.Repository) (reviewSession, error) {
			return session, nil
		},
		func(string) error { return nil },
		func(context.Context) error {
			select {}
		},
	)
	if err == nil {
		t.Fatal("openReview() expected transport termination error")
	}
	if !strings.Contains(err.Error(), "review transport terminated") {
		t.Fatalf("openReview() error = %v, want transport termination context", err)
	}
}
