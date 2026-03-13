package proxy

import "slices"

type hostCatalogue struct {
	CommonBootstrap []string
	AgentRequired   map[string][]string
	ProviderHosts   map[string][]string
}

var requiredHosts = hostCatalogue{
	CommonBootstrap: []string{},
	AgentRequired: map[string][]string{
		"claude": {
			"api.anthropic.com",
		},
		"codex": {
			"auth.openai.com",
			"api.openai.com",
		},
		"opencode": {
			"opencode.ai",
		},
	},
	ProviderHosts: map[string][]string{
		"anthropic": {
			"api.anthropic.com",
		},
		"openai": {
			"api.openai.com",
		},
	},
}

func composedHosts(agent string, providers []string, extra []string) []string {
	all := append([]string{}, requiredHosts.CommonBootstrap...)
	all = append(all, requiredHosts.AgentRequired[agent]...)
	for _, provider := range providers {
		all = append(all, requiredHosts.ProviderHosts[provider]...)
	}
	all = append(all, extra...)

	return uniqueSorted(all)
}

func uniqueSorted(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}

	slices.Sort(out)
	return out
}
