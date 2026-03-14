package cmd

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/pejas/kagen/internal/session"
	"github.com/pejas/kagen/internal/ui"
)

func newListCommand() *cobra.Command {
	var all bool

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List persisted kagen sessions",
		Long: `Lists persisted kagen sessions from the local session store.

Without --all, only sessions for the current repository are shown.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runList(cmd.Context(), all)
		},
	}

	cmd.Flags().BoolVar(&all, "all", false, "show sessions across all repositories")

	return cmd
}

func runList(ctx context.Context, all bool) (err error) {
	ctx = rootContext(ctx)

	store, err := session.OpenDefault()
	if err != nil {
		return fmt.Errorf("opening session store: %w", err)
	}
	defer func() {
		if closeErr := store.Close(); closeErr != nil && err == nil {
			err = fmt.Errorf("closing session store: %w", closeErr)
		}
	}()

	opts := session.ListOptions{}
	if !all {
		repo, err := discoverRepository()
		if err != nil {
			return err
		}
		opts.RepoPath = repo.Path
	}

	summaries, err := store.List(ctx, opts)
	if err != nil {
		return fmt.Errorf("listing sessions: %w", err)
	}

	if len(summaries) == 0 {
		ui.Info("No persisted sessions found.")
		return nil
	}

	ui.Header("Sessions")

	headers := []string{"ID", "STATUS", "BRANCH", "LAST USED", "AGENT SESSIONS"}
	rows := make([][]string, 0, len(summaries))
	for _, summary := range summaries {
		row := []string{
			strconv.FormatInt(summary.Session.ID, 10),
			summary.Session.Status,
			summary.Session.WorkspaceBranch,
			formatListTimestamp(summary.Session.LastUsedAt),
			formatAgentSessions(summary.AgentSessions),
		}
		if all {
			row = append([]string{summary.Session.RepoPath}, row...)
		}
		rows = append(rows, row)
	}
	if all {
		headers = append([]string{"REPOSITORY"}, headers...)
	}

	ui.Table(headers, rows)
	return nil
}

func formatAgentSessions(agentSessions []session.AgentSession) string {
	if len(agentSessions) == 0 {
		return "-"
	}

	counts := make(map[string]int, len(agentSessions))
	ordered := make([]string, 0, len(agentSessions))
	for _, agentSession := range agentSessions {
		if counts[agentSession.AgentType] == 0 {
			ordered = append(ordered, agentSession.AgentType)
		}
		counts[agentSession.AgentType]++
	}

	parts := make([]string, 0, len(ordered))
	for _, agentType := range ordered {
		count := counts[agentType]
		if count == 1 {
			parts = append(parts, agentType)
			continue
		}

		parts = append(parts, fmt.Sprintf("%s x%d", agentType, count))
	}

	return strings.Join(parts, ", ")
}

func formatListTimestamp(value time.Time) string {
	return value.UTC().Format("2006-01-02 15:04:05Z")
}
