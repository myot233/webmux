// Package mcpstdio provides bounded MCP stdio transport for local API clients.
package mcpstdio

import (
	"bufio"
	"context"
	"encoding/json"
	"io"

	"github.com/rainchan/webmux/internal/mcpprotocol"
)

// Serve handles one connection serially with a 64 KiB input frame limit.
func Serve(ctx context.Context, input io.Reader, output io.Writer, registry *mcpprotocol.Registry) error {
	session := registry.NewSession()
	scan := bufio.NewScanner(input)
	scan.Buffer(make([]byte, 4096), 64<<10)
	enc := json.NewEncoder(output)
	for scan.Scan() {
		req, response := mcpprotocol.Decode(scan.Bytes())
		if response == nil {
			var err error
			response, err = session.Dispatch(ctx, req)
			if err != nil {
				return err
			}
		}
		if response != nil {
			if err := enc.Encode(response); err != nil {
				return err
			}
		}
	}
	return scan.Err()
}
