package session

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
)

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
