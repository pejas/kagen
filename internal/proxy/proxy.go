// Package proxy defines the network policy and proxy allowlist enforcement
// for the kagen cluster environment.
package proxy

import (
	"fmt"

	"github.com/pejas/kagen/internal/config"
	kagerr "github.com/pejas/kagen/internal/errors"
)

// Policy represents the set of allowed egress destinations and the
// enforcement state of the proxy.
type Policy struct {
	// AllowedDestinations is the list of permitted egress hosts/URLs.
	AllowedDestinations []string

	// Enforced indicates whether proxy enforcement is active.
	// If false, Validate will return ErrProxyNotActive (fail-closed).
	Enforced bool
}

// LoadPolicy creates a Policy from the kagen configuration.
// The policy is initially loaded as unenforced; the cluster layer
// is responsible for marking it as enforced once the proxy pod is ready.
func LoadPolicy(cfg *config.Config, agentType string) *Policy {
	return &Policy{
		AllowedDestinations: composedHosts(agentType, cfg.ProvidersForAgent(agentType), cfg.ProxyAllowlist),
		Enforced:            false,
	}
}

// Validate checks that proxy enforcement is active.
// Returns ErrProxyNotActive if the proxy is not enforced (fail-closed behavior).
func (p *Policy) Validate() error {
	if !p.Enforced {
		return fmt.Errorf("%w: will not allow direct outbound access", kagerr.ErrProxyNotActive)
	}
	return nil
}

// AllowsDestination checks whether the given destination is in the allowlist.
func (p *Policy) AllowsDestination(dest string) bool {
	for _, allowed := range p.AllowedDestinations {
		if allowed == dest {
			return true
		}
	}
	return false
}
