package devfile

import (
	"fmt"

	"github.com/pejas/kagen/internal/agent"
)

// DefaultForAgent returns the default project devfile for the requested agent.
func DefaultForAgent(agentType agent.Type) (string, error) {
	switch agentType {
	case agent.Codex, agent.Claude, agent.OpenCode:
		return defaultProjectTemplate, nil
	default:
		return "", fmt.Errorf("unsupported agent template: %s", agentType)
	}
}

const defaultProjectTemplate = `schemaVersion: 2.2.0
metadata:
  name: kagen-workspace
  version: 1.0.0
components:
  - name: workspace
    container:
      image: vxcontrol/codebase:latest
      command: ["/bin/sh", "-lc"]
      args:
        - |
          exec tail -f /dev/null
`
