package devfile

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

var errDevfileNotFound = errors.New("devfile not found")

// ErrDevfileNotFound returns the sentinel error used when no devfile exists.
func ErrDevfileNotFound() error {
	return errDevfileNotFound
}

// FindPath returns the preferred devfile path for the repository.
func FindPath(dir string) (string, error) {
	candidates := []string{"devfile.yaml", "devfile.yml"}
	for _, name := range candidates {
		path := filepath.Join(dir, name)
		if _, err := os.Stat(path); err == nil {
			return path, nil
		} else if !errors.Is(err, os.ErrNotExist) {
			return "", fmt.Errorf("stat %s: %w", path, err)
		}
	}

	return "", errDevfileNotFound
}
