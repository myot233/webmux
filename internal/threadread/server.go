package threadread

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"time"

	"github.com/rainchan/webmux/internal/mcpstdio"
)

// Serve exposes only the frozen history tool. Control tools belong to a separate
// server so broadening the control surface never broadens a handoff scope.
func Serve(ctx context.Context, input io.Reader, output io.Writer, r Reader) error {
	if !r.Valid() {
		return errors.New("invalid history scope")
	}
	return (mcpstdio.Server{
		Name: "webmux-thread-read", Instructions: Instructions, CallTimeout: 10 * time.Second,
		Tools: []mcpstdio.Tool{{Definition: toolDefinition(), Call: func(ctx context.Context, raw json.RawMessage) (any, error) { return r.Read(ctx, raw) }}},
	}).Serve(ctx, input, output)
}

func toolDefinition() map[string]any {
	return map[string]any{"name": "thread_read", "description": "Read frozen handoff history, including omitted items. " + Instructions, "annotations": map[string]any{"readOnlyHint": true, "destructiveHint": false, "openWorldHint": false}, "inputSchema": map[string]any{"type": "object", "additionalProperties": false, "required": []string{"view"}, "properties": map[string]any{
		"view":            map[string]any{"type": "string", "enum": []string{"activity"}},
		"afterPosition":   map[string]any{"type": "string", "pattern": "^(0|[1-9][0-9]{0,18})$"},
		"itemId":          map[string]any{"type": "string", "pattern": "^[a-f0-9]{32}$"},
		"textOffset":      map[string]any{"type": "integer", "minimum": 0},
		"limit":           map[string]any{"type": "integer", "default": 20, "minimum": 1, "maximum": 100},
		"maxBytesPerItem": map[string]any{"type": "integer", "default": 4000, "minimum": 4, "maximum": 65536},
	}}}
}
