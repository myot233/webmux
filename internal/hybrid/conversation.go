package hybrid

import (
	"context"
	"fmt"

	"github.com/rainchan/webmux/internal/conversation"
)

func conversationInput(r Record, origin string) conversation.RuntimeInput {
	in := conversation.RuntimeInput{RuntimeID: r.ID, Cwd: r.Cwd, ConfigID: r.ConfigID, AgentName: r.Agent, AgentSessionID: r.AgentSessionID, Status: string(r.State), Config: r.Config, HistoryOrigin: origin, CreatedAt: r.CreatedAt}
	if r.Workspace != nil {
		in.RepositoryID = r.Workspace.RepositoryID
		in.WorktreeID = r.Workspace.WorktreeID
	}
	return in
}

// migrateConversations reads only validated legacy metadata. It does not start
// agents, alter argv/env or guess at the provider's historical transcript.
func (s *Service) migrateConversations() error {
	if s.conversations == nil {
		return nil
	}
	if err := s.records.unavailable(); err != nil {
		return err
	}
	ctx := context.Background()
	records := s.records.all()
	ids := make([]string, 0, len(records))
	for _, r := range records {
		if _, err := s.conversations.EnsureRuntime(ctx, conversationInput(r, "unknown")); err != nil {
			return err
		}
		if err := s.conversations.SyncRuntime(ctx, conversationInput(r, "")); err != nil {
			return err
		}
		ids = append(ids, r.ID)
	}
	return s.conversations.PruneRuntimes(ctx, ids)
}

func (s *Service) ensureConversation(ctx context.Context, r Record) error {
	if s.conversations == nil {
		return nil
	}
	_, err := s.conversations.EnsureRuntime(ctx, conversationInput(r, "complete"))
	// New sessions are captured from creation; TUI intervals record explicit gaps.
	if err != nil {
		return fmt.Errorf("%w: conversation identity unavailable", ErrUnavailable)
	}
	return nil
}

func (s *Service) syncConversation(r Record) {
	if s.conversations == nil {
		return
	}
	if err := s.conversations.SyncRuntime(context.Background(), conversationInput(r, "")); err != nil {
		s.log.Warn("conversation metadata update failed", "id", r.ID)
	}
}

func (s *Service) forgetConversation(ctx context.Context, id string) error {
	if s.conversations == nil {
		return nil
	}
	if err := s.conversations.ForgetRuntime(ctx, id); err != nil {
		return fmt.Errorf("%w: conversation cleanup incomplete", ErrUnavailable)
	}
	return nil
}
