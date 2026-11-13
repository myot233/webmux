package clicmd

import (
	"context"
	"flag"
	"io"
	"os"

	"github.com/rainchan/webmux/internal/agentguide"
	"github.com/rainchan/webmux/internal/attach"
	"github.com/rainchan/webmux/internal/mcpcontrol"
	"github.com/rainchan/webmux/internal/mcpstdio"
)

var mcpCommand = &Command{
	Name: "mcp", Group: GroupGuide,
	Summary:     "Serve WebMux session tools over MCP stdio.",
	Description: "Uses the same local API connection as other session commands. The handoff history MCP remains separate and read-only.",
	Output:      "MCP JSON-RPC on stdout; diagnostics on stderr.",
	Exit:        []ExitItem{{Code: 0, Text: "Input closed."}, {Code: 1, Text: "Transport failed."}, {Code: 2, Text: "Usage error."}},
	Examples:    []Example{{Command: "webmux mcp", Text: "Connect an MCP client to the default instance."}, {Command: "webmux mcp --socket /path/webmux.sock", Text: "Connect to an explicit instance."}},
	SeeAlso:     []string{"webmux guide", "webmux ls"},
	Bind: func(_ *flag.FlagSet) Action {
		return func(ctx context.Context, e Env, args []string) int {
			if len(args) != 0 {
				return e.Usagef("mcp accepts no positional arguments")
			}
			client, code := e.connect()
			if client == nil {
				return code
			}
			defer client.Close()
			if e.Stdin == nil {
				return e.Usagef("mcp requires standard input")
			}
			// File stdin needs cancellable OS reads: os.File.Close can wait on a
			// blocking read. Non-file owned readers (such as io.Pipe) close on cancel.
			input := e.Stdin
			if file, ok := input.(*os.File); ok {
				input = contextInput{ctx: ctx, file: file}
			}
			done, stopped := make(chan struct{}), make(chan struct{})
			go func() {
				defer close(stopped)
				select {
				case <-ctx.Done():
					if closer, ok := input.(io.Closer); ok {
						_ = closer.Close()
					}
				case <-done:
				}
			}()
			// The local connection preserves API caller identity, but stdio
			// alone does not establish an authoritative guide selfSessionId.
			registry, err := (mcpcontrol.Client{API: client, GuideContext: agentguide.Context{Transport: "mcp-stdio", Locality: "local"}}).Registry()
			if err == nil {
				err = mcpstdio.Serve(ctx, input, e.Stdout, registry)
			}
			close(done)
			<-stopped
			if ctx.Err() != nil {
				return e.Fail(ctx.Err())
			}
			if err != nil {
				return e.Fail(err)
			}
			return 0
		}
	},
}

type contextInput struct {
	ctx  context.Context
	file *os.File
}

func (r contextInput) Read(p []byte) (int, error) { return attach.ReadInput(r.ctx, r.file, p) }
