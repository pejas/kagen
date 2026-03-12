package session

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
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

// CreateKagenSession inserts a persisted kagen session record.
func (s *Store) CreateKagenSession(ctx context.Context, params CreateKagenSessionParams) (KagenSession, error) {
	if s == nil || s.db == nil {
		return KagenSession{}, fmt.Errorf("creating kagen session: store is closed")
	}

	now := time.Now().UTC()
	if params.UID == "" {
		params.UID = uuid.NewString()
	}
	if params.CreatedAt.IsZero() {
		params.CreatedAt = now
	}
	if params.LastUsedAt.IsZero() {
		params.LastUsedAt = params.CreatedAt
	}

	repoPath, err := normaliseRepoPath(params.RepoPath)
	if err != nil {
		return KagenSession{}, fmt.Errorf("normalising repository path: %w", err)
	}
	params.RepoPath = repoPath

	result, err := s.db.ExecContext(
		ctx,
		`INSERT INTO kagen_sessions (
			uid,
			repo_id,
			repo_path,
			base_branch,
			workspace_branch,
			head_sha_at_start,
			namespace,
			pod_name,
			status,
			created_at,
			last_used_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		params.UID,
		params.RepoID,
		params.RepoPath,
		params.BaseBranch,
		params.WorkspaceBranch,
		params.HeadSHAAtStart,
		params.Namespace,
		params.PodName,
		params.Status,
		formatTimestamp(params.CreatedAt),
		formatTimestamp(params.LastUsedAt),
	)
	if err != nil {
		return KagenSession{}, wrapWriteError("creating kagen session", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return KagenSession{}, fmt.Errorf("reading kagen session id: %w", err)
	}

	return KagenSession{
		ID:              id,
		UID:             params.UID,
		RepoID:          params.RepoID,
		RepoPath:        params.RepoPath,
		BaseBranch:      params.BaseBranch,
		WorkspaceBranch: params.WorkspaceBranch,
		HeadSHAAtStart:  params.HeadSHAAtStart,
		Namespace:       params.Namespace,
		PodName:         params.PodName,
		Status:          params.Status,
		CreatedAt:       params.CreatedAt.UTC(),
		LastUsedAt:      params.LastUsedAt.UTC(),
	}, nil
}

// CreateAgentSession inserts a persisted agent session record.
func (s *Store) CreateAgentSession(ctx context.Context, params CreateAgentSessionParams) (AgentSession, error) {
	if s == nil || s.db == nil {
		return AgentSession{}, fmt.Errorf("creating agent session: store is closed")
	}

	now := time.Now().UTC()
	if params.ID == "" {
		params.ID = uuid.NewString()
	}
	if params.CreatedAt.IsZero() {
		params.CreatedAt = now
	}
	if params.LastUsedAt.IsZero() {
		params.LastUsedAt = params.CreatedAt
	}

	_, err := s.db.ExecContext(
		ctx,
		`INSERT INTO agent_sessions (
			id,
			kagen_session_uid,
			agent_type,
			name,
			working_mode,
			branch,
			state_path,
			created_at,
			last_used_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		params.ID,
		params.KagenSessionUID,
		params.AgentType,
		params.Name,
		params.WorkingMode,
		nullableString(params.Branch),
		params.StatePath,
		formatTimestamp(params.CreatedAt),
		formatTimestamp(params.LastUsedAt),
	)
	if err != nil {
		return AgentSession{}, wrapWriteError("creating agent session", err)
	}

	return AgentSession{
		ID:              params.ID,
		KagenSessionUID: params.KagenSessionUID,
		AgentType:       params.AgentType,
		Name:            params.Name,
		WorkingMode:     params.WorkingMode,
		Branch:          params.Branch,
		StatePath:       params.StatePath,
		CreatedAt:       params.CreatedAt.UTC(),
		LastUsedAt:      params.LastUsedAt.UTC(),
	}, nil
}

// List returns persisted sessions ordered by most recently used first.
func (s *Store) List(ctx context.Context, opts ListOptions) ([]Summary, error) {
	if s == nil || s.db == nil {
		return nil, fmt.Errorf("listing sessions: store is closed")
	}

	query := `SELECT
		id,
		uid,
		repo_id,
		repo_path,
		base_branch,
		workspace_branch,
		head_sha_at_start,
		namespace,
		pod_name,
		status,
		created_at,
		last_used_at
	FROM kagen_sessions`
	args := []any{}
	if opts.RepoPath != "" {
		repoPath, err := normaliseRepoPath(opts.RepoPath)
		if err != nil {
			return nil, fmt.Errorf("normalising repository path: %w", err)
		}

		query += ` WHERE repo_path = ?`
		args = append(args, repoPath)
	}
	query += ` ORDER BY last_used_at DESC, id DESC`

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("listing sessions: %w", err)
	}

	sessions := make([]KagenSession, 0)
	for rows.Next() {
		session, err := scanKagenSession(rows)
		if err != nil {
			return nil, err
		}

		sessions = append(sessions, session)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, fmt.Errorf("iterating session rows: %w", err)
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("closing session rows: %w", err)
	}

	summaries := make([]Summary, 0, len(sessions))
	for _, session := range sessions {
		agentSessions, err := s.agentSessions(ctx, session.UID)
		if err != nil {
			return nil, err
		}

		summaries = append(summaries, Summary{
			Session:       session,
			AgentSessions: agentSessions,
			AgentTypes:    collectAgentTypes(agentSessions),
		})
	}

	return summaries, nil
}

// GetSummary returns a single persisted session summary by numeric ID.
func (s *Store) GetSummary(ctx context.Context, id int64) (Summary, bool, error) {
	if s == nil || s.db == nil {
		return Summary{}, false, fmt.Errorf("getting session summary: store is closed")
	}

	row := s.db.QueryRowContext(
		ctx,
		`SELECT
			id,
			uid,
			repo_id,
			repo_path,
			base_branch,
			workspace_branch,
			head_sha_at_start,
			namespace,
			pod_name,
			status,
			created_at,
			last_used_at
		FROM kagen_sessions
		WHERE id = ?`,
		id,
	)

	session, err := scanKagenSession(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Summary{}, false, nil
		}

		return Summary{}, false, fmt.Errorf("getting session summary %d: %w", id, err)
	}

	agentSessions, err := s.agentSessions(ctx, session.UID)
	if err != nil {
		return Summary{}, false, err
	}

	return Summary{
		Session:       session,
		AgentSessions: agentSessions,
		AgentTypes:    collectAgentTypes(agentSessions),
	}, true, nil
}

// FindMostRecentReady returns the most recently used ready session for the repository.
func (s *Store) FindMostRecentReady(ctx context.Context, repoPath string) (Summary, bool, error) {
	if s == nil || s.db == nil {
		return Summary{}, false, fmt.Errorf("finding ready session: store is closed")
	}

	normalisedRepoPath, err := normaliseRepoPath(repoPath)
	if err != nil {
		return Summary{}, false, fmt.Errorf("normalising repository path: %w", err)
	}

	row := s.db.QueryRowContext(
		ctx,
		`SELECT
			id,
			uid,
			repo_id,
			repo_path,
			base_branch,
			workspace_branch,
			head_sha_at_start,
			namespace,
			pod_name,
			status,
			created_at,
			last_used_at
		FROM kagen_sessions
		WHERE repo_path = ?
		  AND status = 'ready'
		ORDER BY last_used_at DESC, id DESC
		LIMIT 1`,
		normalisedRepoPath,
	)

	session, err := scanKagenSession(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Summary{}, false, nil
		}

		return Summary{}, false, fmt.Errorf("finding ready session: %w", err)
	}

	agentSessions, err := s.agentSessions(ctx, session.UID)
	if err != nil {
		return Summary{}, false, err
	}

	return Summary{
		Session:       session,
		AgentSessions: agentSessions,
		AgentTypes:    collectAgentTypes(agentSessions),
	}, true, nil
}

// UpdateKagenSessionStatus updates the status of a persisted kagen session.
func (s *Store) UpdateKagenSessionStatus(ctx context.Context, id int64, status string) error {
	if s == nil || s.db == nil {
		return fmt.Errorf("updating kagen session status: store is closed")
	}

	result, err := s.db.ExecContext(
		ctx,
		`UPDATE kagen_sessions SET status = ? WHERE id = ?`,
		status,
		id,
	)
	if err != nil {
		return fmt.Errorf("updating kagen session status for %d: %w", id, err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("reading affected rows for session %d status update: %w", id, err)
	}
	if rowsAffected == 0 {
		return fmt.Errorf("updating kagen session status for %d: session not found", id)
	}

	return nil
}

// RecordAttach updates last-used timestamps for the kagen session and its agent session.
func (s *Store) RecordAttach(ctx context.Context, sessionID int64, agentSessionID string, attachedAt time.Time) error {
	if s == nil || s.db == nil {
		return fmt.Errorf("recording attach: store is closed")
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("recording attach: starting transaction: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	formatted := formatTimestamp(attachedAt)

	sessionResult, err := tx.ExecContext(
		ctx,
		`UPDATE kagen_sessions SET last_used_at = ? WHERE id = ?`,
		formatted,
		sessionID,
	)
	if err != nil {
		return fmt.Errorf("recording attach for session %d: %w", sessionID, err)
	}

	sessionRowsAffected, err := sessionResult.RowsAffected()
	if err != nil {
		return fmt.Errorf("reading session attach rows affected for %d: %w", sessionID, err)
	}
	if sessionRowsAffected == 0 {
		return fmt.Errorf("recording attach for session %d: session not found", sessionID)
	}

	agentResult, err := tx.ExecContext(
		ctx,
		`UPDATE agent_sessions
		SET last_used_at = ?
		WHERE id = ?`,
		formatted,
		agentSessionID,
	)
	if err != nil {
		return fmt.Errorf("recording attach for session %d agent session %s: %w", sessionID, agentSessionID, err)
	}

	agentRowsAffected, err := agentResult.RowsAffected()
	if err != nil {
		return fmt.Errorf("reading agent attach rows affected for session %d: %w", sessionID, err)
	}
	if agentRowsAffected == 0 {
		return fmt.Errorf("recording attach for session %d: agent session %s not found", sessionID, agentSessionID)
	}

	if err = tx.Commit(); err != nil {
		return fmt.Errorf("recording attach: committing transaction: %w", err)
	}

	return nil
}

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

func (s *Store) agentSessions(ctx context.Context, sessionUID string) ([]AgentSession, error) {
	rows, err := s.db.QueryContext(
		ctx,
		`SELECT
			id,
			kagen_session_uid,
			agent_type,
			name,
			working_mode,
			branch,
			state_path,
			created_at,
			last_used_at
		FROM agent_sessions
		WHERE kagen_session_uid = ?
		ORDER BY agent_type ASC, last_used_at DESC, created_at DESC, id DESC`,
		sessionUID,
	)
	if err != nil {
		return nil, fmt.Errorf("listing agent sessions for %s: %w", sessionUID, err)
	}
	defer rows.Close()

	agentSessions := make([]AgentSession, 0)
	for rows.Next() {
		agentSession, err := scanAgentSession(rows)
		if err != nil {
			return nil, fmt.Errorf("scanning agent session for %s: %w", sessionUID, err)
		}

		agentSessions = append(agentSessions, agentSession)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating agent sessions for %s: %w", sessionUID, err)
	}

	return agentSessions, nil
}

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

func scanKagenSession(scanner rowScanner) (KagenSession, error) {
	var (
		createdAt string
		lastUsed  string
		session   KagenSession
	)
	if err := scanner.Scan(
		&session.ID,
		&session.UID,
		&session.RepoID,
		&session.RepoPath,
		&session.BaseBranch,
		&session.WorkspaceBranch,
		&session.HeadSHAAtStart,
		&session.Namespace,
		&session.PodName,
		&session.Status,
		&createdAt,
		&lastUsed,
	); err != nil {
		return KagenSession{}, fmt.Errorf("scanning session row: %w", err)
	}

	var err error
	session.CreatedAt, err = parseTimestamp(createdAt)
	if err != nil {
		return KagenSession{}, fmt.Errorf("parsing created_at for session %d: %w", session.ID, err)
	}
	session.LastUsedAt, err = parseTimestamp(lastUsed)
	if err != nil {
		return KagenSession{}, fmt.Errorf("parsing last_used_at for session %d: %w", session.ID, err)
	}

	return session, nil
}

func scanAgentSession(scanner rowScanner) (AgentSession, error) {
	var (
		createdAt string
		lastUsed  string
		name      sql.NullString
		branch    sql.NullString
		session   AgentSession
	)
	if err := scanner.Scan(
		&session.ID,
		&session.KagenSessionUID,
		&session.AgentType,
		&name,
		&session.WorkingMode,
		&branch,
		&session.StatePath,
		&createdAt,
		&lastUsed,
	); err != nil {
		return AgentSession{}, err
	}

	session.Name = name.String
	session.Branch = branch.String

	var err error
	session.CreatedAt, err = parseTimestamp(createdAt)
	if err != nil {
		return AgentSession{}, fmt.Errorf("parsing agent session created_at for %s: %w", session.ID, err)
	}
	session.LastUsedAt, err = parseTimestamp(lastUsed)
	if err != nil {
		return AgentSession{}, fmt.Errorf("parsing agent session last_used_at for %s: %w", session.ID, err)
	}

	return session, nil
}

func collectAgentTypes(agentSessions []AgentSession) []string {
	if len(agentSessions) == 0 {
		return nil
	}

	agentTypes := make([]string, 0, len(agentSessions))
	lastAgentType := ""
	for _, agentSession := range agentSessions {
		if agentSession.AgentType == lastAgentType {
			continue
		}

		agentTypes = append(agentTypes, agentSession.AgentType)
		lastAgentType = agentSession.AgentType
	}

	return agentTypes
}
