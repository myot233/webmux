package hybrid

import (
	"context"
	"time"

	"github.com/rainchan/webmux/internal/acp"
	"github.com/rainchan/webmux/internal/conversation"
)

func (s *Service) HistoryReaderAvailable(ctx context.Context, scope conversation.ReaderScope) bool {
	if s.historyReader == nil {
		return false
	}
	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	return len(s.historyReader(ctx, scope)) > 0
}

func (a *actor) historyServers(ctx context.Context) []acp.MCPServer {
	if a.svc.historyReader == nil || a.svc.conversations == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	scope, err := a.svc.conversations.ReaderScope(ctx, a.id)
	if err != nil {
		return nil
	}
	// A failed optional lookup never prevents session/new, load or resume.
	return a.svc.historyReader(ctx, scope)
}
