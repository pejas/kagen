package session

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

const schemaVersion = 1

// Store wraps the persistent session database.
type Store struct {
	db *sql.DB
}

type rowScanner interface {
	Scan(dest ...any) error
}

// OpenDefault opens the session store in the platform-specific config directory.
func OpenDefault() (*Store, error) {
	path, err := defaultDBPath()
	if err != nil {
		return nil, err
	}

	return Open(path)
}

// Open opens the session store at the provided path, initialising schema when needed.
func Open(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("creating session store directory: %w", err)
	}

	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("opening session store: %w", err)
	}
	db.SetMaxOpenConns(1)

	store := &Store{db: db}
	if err := store.initialise(context.Background()); err != nil {
		_ = db.Close()
		return nil, err
	}

	return store, nil
}

// Close closes the underlying database handle.
func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}

	return s.db.Close()
}
