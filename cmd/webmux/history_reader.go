package main

import (
	"context"

	"github.com/rainchan/webmux/internal/acp"
	"github.com/rainchan/webmux/internal/conversation"
)

// The descriptor is private provider configuration, never launch config or an
// HTTP response. The live server keeps sole ownership of the SQLite lifecycle.
func historyReader(stateDir, executable string, store *conversation.Service) func(context.Context, conversation.ReaderScope) []acp.MCPServer {
	return func(ctx context.Context, scope conversation.ReaderScope) []acp.MCPServer {
		if !store.HistoryEnabled() {
			return nil
		}
		socket, ok := mcpSocket(stateDir, executable)
		if !ok || store == nil {
			return nil
		}
		p, err := store.Preview(ctx, scope.HandoffID)
		if err != nil || p.SourceConversationID != scope.ConversationID {
			return nil
		}
		return []acp.MCPServer{{Name: "webmux-history", Command: executable, Args: []string{"mcp-serve", "--socket", socket, "--conversation", scope.ConversationID, "--handoff", scope.HandoffID}, Env: []acp.MCPEnv{}}}
	}
}
