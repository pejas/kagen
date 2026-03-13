package session

import (
	"database/sql"
	"fmt"
)

func scanSummaries(rows *sql.Rows) ([]Summary, error) {
	summaries := make([]Summary, 0)
	indexByUID := make(map[string]int)

	for rows.Next() {
		session, agentSession, hasAgentSession, err := scanSummaryRow(rows)
		if err != nil {
			return nil, err
		}

		index, ok := indexByUID[session.UID]
		if !ok {
			summaries = append(summaries, Summary{Session: session})
			index = len(summaries) - 1
			indexByUID[session.UID] = index
		}
		if hasAgentSession {
			summaries[index].AgentSessions = append(summaries[index].AgentSessions, agentSession)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating session rows: %w", err)
	}

	for i := range summaries {
		summaries[i].AgentTypes = collectAgentTypes(summaries[i].AgentSessions)
	}

	return summaries, nil
}

func scanSummaryRow(scanner rowScanner) (KagenSession, AgentSession, bool, error) {
	var (
		sessionCreatedAt string
		sessionLastUsed  string
		agentID          sql.NullString
		agentUID         sql.NullString
		agentType        sql.NullString
		agentName        sql.NullString
		agentMode        sql.NullString
		agentBranch      sql.NullString
		agentStatePath   sql.NullString
		agentCreatedAt   sql.NullString
		agentLastUsed    sql.NullString
		session          KagenSession
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
		&sessionCreatedAt,
		&sessionLastUsed,
		&agentID,
		&agentUID,
		&agentType,
		&agentName,
		&agentMode,
		&agentBranch,
		&agentStatePath,
		&agentCreatedAt,
		&agentLastUsed,
	); err != nil {
		return KagenSession{}, AgentSession{}, false, fmt.Errorf("scanning session summary row: %w", err)
	}

	var err error
	session.CreatedAt, err = parseTimestamp(sessionCreatedAt)
	if err != nil {
		return KagenSession{}, AgentSession{}, false, fmt.Errorf("parsing created_at for session %d: %w", session.ID, err)
	}
	session.LastUsedAt, err = parseTimestamp(sessionLastUsed)
	if err != nil {
		return KagenSession{}, AgentSession{}, false, fmt.Errorf("parsing last_used_at for session %d: %w", session.ID, err)
	}

	if !agentID.Valid {
		return session, AgentSession{}, false, nil
	}

	agentSession := AgentSession{
		ID:              agentID.String,
		KagenSessionUID: agentUID.String,
		AgentType:       agentType.String,
		Name:            agentName.String,
		WorkingMode:     agentMode.String,
		Branch:          agentBranch.String,
		StatePath:       agentStatePath.String,
	}
	agentSession.CreatedAt, err = parseTimestamp(agentCreatedAt.String)
	if err != nil {
		return KagenSession{}, AgentSession{}, false, fmt.Errorf("parsing agent session created_at for %s: %w", agentSession.ID, err)
	}
	agentSession.LastUsedAt, err = parseTimestamp(agentLastUsed.String)
	if err != nil {
		return KagenSession{}, AgentSession{}, false, fmt.Errorf("parsing agent session last_used_at for %s: %w", agentSession.ID, err)
	}

	return session, agentSession, true, nil
}
