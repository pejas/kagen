package runtime

import (
	"fmt"
	"os/exec"
)

// CheckConfigDependencies verifies that required external tools are available.
func CheckConfigDependencies() error {
	return checkDependencies(
		dependency{name: "colima", hint: "brew install colima"},
		dependency{name: "kubectl", hint: "brew install kubernetes-cli"},
	)
}

type dependency struct {
	name string
	hint string
}

func checkColimaDependency() error {
	return checkDependencies(dependency{name: "colima", hint: "brew install colima"})
}

func checkDependencies(deps ...dependency) error {
	for _, dep := range deps {
		if _, err := exec.LookPath(dep.name); err != nil {
			return fmt.Errorf("%s not found on PATH. Hint: %s", dep.name, dep.hint)
		}
	}

	return nil
}
