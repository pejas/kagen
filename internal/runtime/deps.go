package runtime

import (
	"fmt"
	"os/exec"
)

// CheckConfigDependencies verifies that required external tools are available.
func CheckConfigDependencies() error {
	deps := []struct {
		name string
		hint string
	}{
		{"colima", "brew install colima"},
		{"kubectl", "brew install kubernetes-cli"},
	}

	for _, dep := range deps {
		if _, err := exec.LookPath(dep.name); err != nil {
			return fmt.Errorf("%s not found on PATH. Hint: %s", dep.name, dep.hint)
		}
	}

	return nil
}
