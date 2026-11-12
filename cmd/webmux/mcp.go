package main

import (
	"context"
	"flag"
	"fmt"
	"io"

	"github.com/rainchan/webmux/internal/localclient"
	"github.com/rainchan/webmux/internal/threadread"
)

func mcpCommand(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("webmux mcp-serve", flag.ContinueOnError)
	flags.SetOutput(stderr)
	socket := flags.String("socket", "", "local API socket PATH")
	source := flags.String("conversation", "", "fixed source Conversation ID")
	handoff := flags.String("handoff", "", "fixed handoff ID")
	if flags.Parse(args) != nil || flags.NArg() != 0 {
		return 2
	}
	client := localclient.New(localclient.UnixDialer{Path: *socket})
	defer client.Close()
	reader := threadread.Reader{Client: client, ConversationID: *source, HandoffID: *handoff}
	if *socket == "" || !reader.Valid() {
		_, _ = fmt.Fprintln(stderr, "mcp-serve requires socket, conversation and handoff scope")
		return 2
	}
	if err := threadread.Serve(context.Background(), stdin, stdout, reader); err != nil {
		_, _ = fmt.Fprintln(stderr, "history MCP transport closed")
		return 1
	}
	return 0
}
