// Package session persists kagen session metadata across CLI invocations.
package session

import "time"

// KagenSession is the persisted control-plane record for a workspace session.
type KagenSession struct {
	ID              int64
	UID             string
	RepoID          string
	RepoPath        string
	BaseBranch      string
	WorkspaceBranch string
	HeadSHAAtStart  string
	Namespace       string
	PodName         string
	Status          string
	CreatedAt       time.Time
	LastUsedAt      time.Time
}

// AgentSession is the persisted runtime record nested under a kagen session.
type AgentSession struct {
	ID              string
	KagenSessionUID string
	AgentType       string
	Name            string
	WorkingMode     string
	Branch          string
	StatePath       string
	CreatedAt       time.Time
	LastUsedAt      time.Time
}

// Summary is the list-oriented view of a persisted session.
type Summary struct {
	Session       KagenSession
	AgentSessions []AgentSession
	AgentTypes    []string
}

// ListOptions scopes session listing queries.
type ListOptions struct {
	RepoPath string
}

// CreateKagenSessionParams describes a new persisted kagen session.
type CreateKagenSessionParams struct {
	UID             string
	RepoID          string
	RepoPath        string
	BaseBranch      string
	WorkspaceBranch string
	HeadSHAAtStart  string
	Namespace       string
	PodName         string
	Status          string
	CreatedAt       time.Time
	LastUsedAt      time.Time
}

// CreateAgentSessionParams describes a new persisted agent session.
type CreateAgentSessionParams struct {
	ID              string
	KagenSessionUID string
	AgentType       string
	Name            string
	WorkingMode     string
	Branch          string
	StatePath       string
	CreatedAt       time.Time
	LastUsedAt      time.Time
}
