package mcpstdio

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestToolDispatchAndDeadline(t *testing.T) {
	server := Server{Name: "fixture", CallTimeout: time.Second, Tools: []Tool{{
		Definition: map[string]any{"name": "read"},
		Call: func(ctx context.Context, raw json.RawMessage) (any, error) {
			if _, ok := ctx.Deadline(); !ok {
				t.Fatal("tool has no deadline")
			}
			if string(raw) == `{"fail":true}` {
				return nil, errors.New("unavailable")
			}
			return map[string]bool{"ok": true}, nil
		},
	}}}
	input := strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-25"}}`,
		`{"jsonrpc":"2.0","method":"notifications/initialized"}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"other","arguments":{}}}`,
		`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"read","arguments":{"fail":true}}}`,
		`{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"read","arguments":{}}}`,
	}, "\n")
	var out bytes.Buffer
	if err := server.Serve(context.Background(), strings.NewReader(input), &out); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 4 || !strings.Contains(lines[1], `"code":-32602`) || !strings.Contains(lines[2], `"isError":true`) || !strings.Contains(lines[3], `"structuredContent":{"ok":true}`) {
		t.Fatalf("responses: %s", out.String())
	}
}

func TestRejectInvalidToolRegistry(t *testing.T) {
	call := func(context.Context, json.RawMessage) (any, error) { return nil, nil }
	tool := Tool{Definition: map[string]any{"name": "duplicate"}, Call: call}
	for _, tools := range [][]Tool{{{}}, {tool, tool}} {
		s := Server{Name: "fixture", CallTimeout: time.Second, Tools: tools}
		if err := s.Serve(context.Background(), strings.NewReader(""), &bytes.Buffer{}); err == nil {
			t.Fatal("invalid registry accepted")
		}
	}
}
