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
		AgentProviders: map[string][]string{
			"opencode": {"anthropic"},
		},
		ProxyAllowlist: []string{"github.com"},
	}

	policy := LoadPolicy(cfg, "opencode")
	if len(policy.AllowedDestinations) != 3 {
		t.Errorf("expected 3 allowed destinations, got %d", len(policy.AllowedDestinations))
	}
	want := map[string]bool{
		"api.anthropic.com":  true,
		"github.com":         true,
		"registry.npmjs.org": true,
	}
	for _, host := range policy.AllowedDestinations {
		delete(want, host)
	}
	if len(want) != 0 {
		t.Errorf("LoadPolicy(opencode) missing hosts: %v", want)
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

func TestLoadPolicyCodexIncludesRequiredHosts(t *testing.T) {
	t.Parallel()

	policy := LoadPolicy(&config.Config{}, "codex")
	if !policy.AllowsDestination("api.openai.com") {
		t.Error("LoadPolicy(codex) should allow api.openai.com")
	}
	if !policy.AllowsDestination("registry.npmjs.org") {
		t.Error("LoadPolicy(codex) should allow registry.npmjs.org")
	}
}
