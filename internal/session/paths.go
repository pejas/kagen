package session

import (
	"fmt"
	"os"
	"path/filepath"
)

func defaultDBPath() (string, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("finding user config directory: %w", err)
	}

	return filepath.Join(configDir, "kagen", "sessions.db"), nil
}

func normaliseRepoPath(path string) (string, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}

	resolved, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		if os.IsNotExist(err) {
			return filepath.Clean(absolute), nil
		}
		return "", err
	}

	return filepath.Clean(resolved), nil
}
