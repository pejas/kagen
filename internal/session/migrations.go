package session

import (
	"context"
	"database/sql"
	"fmt"
)

func (s *Store) initialise(ctx context.Context) error {
	if _, err := s.db.ExecContext(ctx, `PRAGMA foreign_keys = ON`); err != nil {
		return fmt.Errorf("enabling foreign keys: %w", err)
	}
	if _, err := s.db.ExecContext(ctx, `PRAGMA busy_timeout = 5000`); err != nil {
		return fmt.Errorf("setting busy timeout: %w", err)
	}

	version, err := s.schemaVersion(ctx)
	if err != nil {
		return err
	}
	if version > schemaVersion {
		return fmt.Errorf("session store schema version %d is newer than supported version %d", version, schemaVersion)
	}
	if version == schemaVersion {
		return nil
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("starting schema migration: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	for migration := version + 1; migration <= schemaVersion; migration++ {
		if err = applyMigration(ctx, tx, migration); err != nil {
			return err
		}
	}

	if _, err = tx.ExecContext(ctx, fmt.Sprintf(`PRAGMA user_version = %d`, schemaVersion)); err != nil {
		return fmt.Errorf("updating schema version: %w", err)
	}
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("committing schema migration: %w", err)
	}

	return nil
}

func (s *Store) schemaVersion(ctx context.Context) (int, error) {
	row := s.db.QueryRowContext(ctx, `PRAGMA user_version`)

	var version int
	if err := row.Scan(&version); err != nil {
		return 0, fmt.Errorf("reading schema version: %w", err)
	}

	return version, nil
}

func applyMigration(ctx context.Context, tx *sql.Tx, version int) error {
	switch version {
	case 1:
		statements := []string{
			`CREATE TABLE IF NOT EXISTS kagen_sessions (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				uid TEXT NOT NULL UNIQUE,
				repo_id TEXT NOT NULL,
				repo_path TEXT NOT NULL,
				base_branch TEXT NOT NULL,
				workspace_branch TEXT NOT NULL,
				head_sha_at_start TEXT NOT NULL,
				namespace TEXT NOT NULL,
				pod_name TEXT NOT NULL,
				status TEXT NOT NULL,
				created_at TEXT NOT NULL,
				last_used_at TEXT NOT NULL
			)`,
			`CREATE INDEX IF NOT EXISTS idx_kagen_sessions_repo_path_last_used
				ON kagen_sessions (repo_path, last_used_at DESC, id DESC)`,
			`CREATE INDEX IF NOT EXISTS idx_kagen_sessions_repo_id_last_used
				ON kagen_sessions (repo_id, last_used_at DESC, id DESC)`,
			`CREATE TABLE IF NOT EXISTS agent_sessions (
				id TEXT PRIMARY KEY,
				kagen_session_uid TEXT NOT NULL,
				agent_type TEXT NOT NULL,
				name TEXT,
				working_mode TEXT NOT NULL,
				branch TEXT,
				state_path TEXT NOT NULL,
				created_at TEXT NOT NULL,
				last_used_at TEXT NOT NULL,
				FOREIGN KEY (kagen_session_uid) REFERENCES kagen_sessions(uid) ON DELETE CASCADE
			)`,
			`CREATE INDEX IF NOT EXISTS idx_agent_sessions_kagen_session_uid
				ON agent_sessions (kagen_session_uid, agent_type)`,
		}
		for _, statement := range statements {
			if _, err := tx.ExecContext(ctx, statement); err != nil {
				return fmt.Errorf("applying schema migration %d: %w", version, err)
			}
		}

		return nil
	default:
		return fmt.Errorf("unsupported schema migration %d", version)
	}
}
