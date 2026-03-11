package devfile

import (
	"fmt"

	"github.com/pejas/kagen/internal/agent"
)

// DefaultForAgent returns the default project devfile for the requested agent.
func DefaultForAgent(agentType agent.Type) (string, error) {
	switch agentType {
	case agent.Codex:
		return defaultCodexTemplate, nil
	case agent.Claude, agent.OpenCode:
		return "", fmt.Errorf("%s template not implemented yet", agentType)
	default:
		return "", fmt.Errorf("unsupported agent template: %s", agentType)
	}
}

const defaultCodexTemplate = `schemaVersion: 2.2.0
metadata:
  name: kagen-workspace
  version: 1.0.0
components:
  - name: agent
    attributes:
      kagen.agent/runtime: codex
    container:
      image: node:20-bookworm-slim
      command: ["/bin/sh", "-lc"]
      args:
        - |
          set -eu
          export DEBIAN_FRONTEND=noninteractive
          mkdir -p /home/kagen/.codex
          if ! command -v git >/dev/null 2>&1; then
            apt-get update
            apt-get install -y --no-install-recommends git ca-certificates curl ripgrep procps
            rm -rf /var/lib/apt/lists/*
          fi
          if ! command -v codex >/dev/null 2>&1; then
            npm install -g @openai/codex
          fi
          exec tail -f /dev/null
      env:
        - name: HOME
          value: /home/kagen
        - name: CODEX_HOME
          value: /home/kagen/.codex
      volumeMounts:
        - name: agent-home
          path: /home/kagen
  - name: agent-home
    volume:
      size: 5Gi
`
