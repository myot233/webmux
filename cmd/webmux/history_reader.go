package main

import (
	"context"
	"os"

	"github.com/rainchan/webmux/internal/acp"
	"github.com/rainchan/webmux/internal/conversation"
	"github.com/rainchan/webmux/internal/runtime"
)

// The descriptor is private provider configuration, never launch config or an
// HTTP response. The live server keeps sole ownership of the SQLite lifecycle.
func historyReader(stateDir, executable string, store *conversation.Service) func(context.Context, conversation.ReaderScope) []acp.MCPServer {
	return func(ctx context.Context, scope conversation.ReaderScope) []acp.MCPServer {
		if !store.HistoryEnabled() {
			return nil
		}
		socket, err := runtime.SocketPath(stateDir, os.Getenv)
		if err != nil || executable == "" || store == nil {
			return nil
		}
		bin, err := os.Stat(executable)
		if err != nil || !bin.Mode().IsRegular() || bin.Mode().Perm()&0111 == 0 {
			return nil
		}
		info, err := os.Stat(socket)
		if err != nil || info.Mode()&os.ModeSocket == 0 {
			return nil
		}
		p, err := store.Preview(ctx, scope.HandoffID)
		if err != nil || p.SourceConversationID != scope.ConversationID {
			return nil
		}
		return []acp.MCPServer{{Name: "webmux-history", Command: executable, Args: []string{"mcp-serve", "--socket", socket, "--conversation", scope.ConversationID, "--handoff", scope.HandoffID}, Env: []acp.MCPEnv{}}}
	}
}
