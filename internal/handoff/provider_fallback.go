package handoff

import (
	"context"
	"maps"

	"github.com/rainchan/webmux/internal/conversation"
)

func (s *Service) prepareFreshFallback(ctx context.Context, e conversation.Execution, p conversation.Preview) error {
	// Resume can have opened a process before reporting failure. Verify every
	// other runtime and any lingering shell is quiescent before offering fresh.
	ids, err := s.options.Conversations.RuntimeIDs(ctx, p.SourceConversationID)
	if err != nil {
		return err
	}
	for _, id := range ids {
		if err := s.quiesceProvider(ctx, id); err != nil {
			return err
		}
	}
	replacement, err := s.Preview(ctx, p.SourceConversationID, Request{ClientRequestID: "fresh-" + e.ID, ConfigID: p.Request.ConfigID, AgentConfig: maps.Clone(p.Request.AgentConfig), UserText: p.Request.UserText, ForceFresh: true, Location: p.Request.Location, Attachments: p.Request.Attachments})
	if err != nil {
		return err
	}
	return s.options.Conversations.LinkFreshPreview(ctx, e.ID, replacement.ID)
}
