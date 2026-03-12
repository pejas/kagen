package git

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// UserConfig holds git user configuration (name and email).
type UserConfig struct {
	Name  string
	Email string
}

// GetHostUser reads the user.name and user.email from the host git config.
// Falls back to OS username and empty email if git config is not set.
func GetHostUser() (*UserConfig, error) {
	name, err := getGitConfig("user.name")
	if err != nil || name == "" {
		// Fallback to OS username
		name = getOSUsername()
	}

	email, err := getGitConfig("user.email")
	if err != nil {
		email = ""
	}

	return &UserConfig{
		Name:  name,
		Email: email,
	}, nil
}

// AddSubaddress transforms an email address by adding a subaddress tag.
// Following RFC 5233, it adds +tag before the @ symbol.
// If the email already contains a +, it appends the new tag.
// Examples:
//   - user@example.com + opencode -> user+opencode@example.com
//   - user+existing@example.com + opencode -> user+existing+opencode@example.com
func AddSubaddress(email, tag string) string {
	if email == "" {
		return ""
	}

	parts := strings.SplitN(email, "@", 2)
	if len(parts) != 2 {
		// Invalid email format, return as-is
		return email
	}

	localPart := parts[0]
	domain := parts[1]

	// Append the tag to the local part
	return fmt.Sprintf("%s+%s@%s", localPart, tag, domain)
}

// getGitConfig retrieves a git config value using git config command.
func getGitConfig(key string) (string, error) {
	cmd := exec.Command("git", "config", "--global", key)
	out, err := cmd.Output()
	if err != nil {
		// Try local config if global fails
		cmd = exec.Command("git", "config", key)
		out, err = cmd.Output()
		if err != nil {
			return "", err
		}
	}
	return strings.TrimSpace(string(out)), nil
}

// getOSUsername returns the current OS username as a fallback.
func getOSUsername() string {
	if user := os.Getenv("USER"); user != "" {
		return user
	}
	if user := os.Getenv("USERNAME"); user != "" {
		return user
	}
	return "unknown"
}
