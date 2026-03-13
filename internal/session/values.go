package session

import (
	"fmt"
	"strings"
	"time"
)

func formatTimestamp(value time.Time) string {
	return value.UTC().Format(time.RFC3339Nano)
}

func parseTimestamp(value string) (time.Time, error) {
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return time.Time{}, err
	}

	return parsed.UTC(), nil
}

func nullableString(value string) any {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}

	return trimmed
}

func wrapWriteError(prefix string, err error) error {
	switch {
	case strings.Contains(strings.ToLower(err.Error()), "unique constraint failed"):
		return fmt.Errorf("%s: record already exists: %w", prefix, err)
	case strings.Contains(strings.ToLower(err.Error()), "foreign key constraint failed"):
		return fmt.Errorf("%s: referenced kagen session does not exist: %w", prefix, err)
	default:
		return fmt.Errorf("%s: %w", prefix, err)
	}
}
