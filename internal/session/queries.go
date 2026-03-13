package session

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

const sessionSummaryColumns = `
	ks.id,
	ks.uid,
	ks.repo_id,
	ks.repo_path,
	ks.base_branch,
	ks.workspace_branch,
	ks.head_sha_at_start,
	ks.namespace,
	ks.pod_name,
	ks.status,
	ks.created_at,
	ks.last_used_at
`

const agentSessionSummaryColumns = `
	ag.id,
	ag.kagen_session_uid,
	ag.agent_type,
	ag.name,
	ag.working_mode,
	ag.branch,
	ag.state_path,
	ag.created_at,
	ag.last_used_at
`

// List returns persisted sessions ordered by most recently used first.
func (s *Store) List(ctx context.Context, opts ListOptions) ([]Summary, error) {
	if s == nil || s.db == nil {
		return nil, fmt.Errorf("listing sessions: store is closed")
	}

	query := `
		SELECT ` + sessionSummaryColumns + `, ` + agentSessionSummaryColumns + `
		FROM kagen_sessions ks
		LEFT JOIN agent_sessions ag ON ag.kagen_session_uid = ks.uid
	`
	args := []any{}
	if opts.RepoPath != "" {
		repoPath, err := normaliseRepoPath(opts.RepoPath)
		if err != nil {
			return nil, fmt.Errorf("normalising repository path: %w", err)
		}

		query += ` WHERE ks.repo_path = ?`
		args = append(args, repoPath)
	}
	query += summaryOrderByClause()

	return s.listSummaries(ctx, "listing sessions", query, args...)
}

// GetSummary returns a single persisted session summary by numeric ID.
func (s *Store) GetSummary(ctx context.Context, id int64) (Summary, bool, error) {
	if s == nil || s.db == nil {
		return Summary{}, false, fmt.Errorf("getting session summary: store is closed")
	}

	query := `
		SELECT ` + sessionSummaryColumns + `, ` + agentSessionSummaryColumns + `
		FROM kagen_sessions ks
		LEFT JOIN agent_sessions ag ON ag.kagen_session_uid = ks.uid
		WHERE ks.id = ?
	` + summaryOrderByClause()

	summaries, err := s.listSummaries(ctx, fmt.Sprintf("getting session summary %d", id), query, id)
	if err != nil {
		return Summary{}, false, err
	}
	if len(summaries) == 0 {
		return Summary{}, false, nil
	}

	return summaries[0], true, nil
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

	query := `
		SELECT ` + sessionSummaryColumns + `, ` + agentSessionSummaryColumns + `
		FROM (
			SELECT *
			FROM kagen_sessions
			WHERE repo_path = ?
			  AND status = 'ready'
			ORDER BY last_used_at DESC, id DESC
			LIMIT 1
		) ks
		LEFT JOIN agent_sessions ag ON ag.kagen_session_uid = ks.uid
	` + summaryOrderByClause()

	summaries, err := s.listSummaries(ctx, "finding ready session", query, normalisedRepoPath)
	if err != nil {
		return Summary{}, false, err
	}
	if len(summaries) == 0 {
		return Summary{}, false, nil
	}

	return summaries[0], true, nil
}

func (s *Store) listSummaries(ctx context.Context, action, query string, args ...any) ([]Summary, error) {
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", action, err)
	}
	defer rows.Close()

	summaries, err := scanSummaries(rows)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}

		return nil, fmt.Errorf("%s: %w", action, err)
	}

	return summaries, nil
}

func summaryOrderByClause() string {
	return `
		ORDER BY
			ks.last_used_at DESC,
			ks.id DESC,
			ag.agent_type ASC,
			ag.last_used_at DESC,
			ag.created_at DESC,
			ag.id DESC
	`
}

func collectAgentTypes(agentSessions []AgentSession) []string {
	if len(agentSessions) == 0 {
		return nil
	}

	agentTypes := make([]string, 0, len(agentSessions))
	lastAgentType := ""
	for _, agentSession := range agentSessions {
		if strings.EqualFold(agentSession.AgentType, lastAgentType) {
			continue
		}

		agentTypes = append(agentTypes, agentSession.AgentType)
		lastAgentType = agentSession.AgentType
	}

	return agentTypes
}
