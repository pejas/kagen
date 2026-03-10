package proxy

import (
	"errors"
	"testing"

	"github.com/pejas/kagen/internal/config"
	kagerr "github.com/pejas/kagen/internal/errors"
)

func TestLoadPolicyFromConfig(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{
		ProxyAllowlist: []string{"api.anthropic.com", "api.openai.com"},
	}

	policy := LoadPolicy(cfg)
	if len(policy.AllowedDestinations) != 2 {
		t.Errorf("expected 2 allowed destinations, got %d", len(policy.AllowedDestinations))
	}
	if policy.Enforced {
		t.Error("expected policy to be unenforced initially")
	}
}

func TestValidateFailsClosed(t *testing.T) {
	t.Parallel()

	policy := &Policy{Enforced: false}
	err := policy.Validate()
	if !errors.Is(err, kagerr.ErrProxyNotActive) {
		t.Errorf("expected ErrProxyNotActive, got %v", err)
	}
}

func TestValidatePassesWhenEnforced(t *testing.T) {
	t.Parallel()

	policy := &Policy{Enforced: true}
	if err := policy.Validate(); err != nil {
		t.Errorf("expected nil error when enforced, got %v", err)
	}
}

func TestAllowsDestination(t *testing.T) {
	t.Parallel()

	policy := &Policy{
		AllowedDestinations: []string{"api.anthropic.com", "api.openai.com"},
	}

	if !policy.AllowsDestination("api.anthropic.com") {
		t.Error("expected api.anthropic.com to be allowed")
	}
	if policy.AllowsDestination("evil.example.com") {
		t.Error("expected evil.example.com to be denied")
	}
}

func TestEmptyAllowlistDeniesAll(t *testing.T) {
	t.Parallel()

	policy := &Policy{AllowedDestinations: nil}
	if policy.AllowsDestination("anything.com") {
		t.Error("expected empty allowlist to deny all destinations")
	}
}
