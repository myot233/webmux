package mcpstdio

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/rainchan/webmux/internal/mcpprotocol"
)

func TestToolDispatchAndDeadline(t *testing.T) {
	server, err := mcpprotocol.New(mcpprotocol.Config{Name: "fixture", CallTimeout: time.Second, Tools: []mcpprotocol.Tool{{
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
	}}})
	if err != nil {
		t.Fatal(err)
	}
	input := strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-25"}}`,
		`{"jsonrpc":"2.0","method":"notifications/initialized"}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"other","arguments":{}}}`,
		`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"read","arguments":{"fail":true}}}`,
		`{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"read","arguments":{}}}`,
	}, "\n")
	var out bytes.Buffer
	if err := Serve(context.Background(), strings.NewReader(input), &out, server); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 4 || !strings.Contains(lines[1], `"code":-32602`) || !strings.Contains(lines[2], `"isError":true`) || !strings.Contains(lines[3], `"structuredContent":{"ok":true}`) {
		t.Fatalf("responses: %s", out.String())
	}
}

func TestRejectInvalidToolRegistry(t *testing.T) {
	call := func(context.Context, json.RawMessage) (any, error) { return nil, nil }
	tool := mcpprotocol.Tool{Definition: map[string]any{"name": "duplicate"}, Call: call}
	for _, tools := range [][]mcpprotocol.Tool{{{}}, {tool, tool}} {
		if _, err := mcpprotocol.New(mcpprotocol.Config{Name: "fixture", CallTimeout: time.Second, Tools: tools}); err == nil {
			t.Fatal("invalid registry accepted")
		}
	}
}

func TestFrameLimitAndProtocolOnlyOutput(t *testing.T) {
	registry, err := mcpprotocol.New(mcpprotocol.Config{Name: "fixture", CallTimeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	input := "{\"jsonrpc\":\"2.0\",\"method\":\"ignored\"}\n{\"jsonrpc\":\"2.0\",\"id\":1,\"method\":\"ping\"}\n" + strings.Repeat("x", 64<<10)
	if err := Serve(context.Background(), strings.NewReader(input), &output, registry); err == nil {
		t.Fatal("oversized frame accepted")
	}
	var response struct {
		Result json.RawMessage `json:"result"`
	}
	if err := json.Unmarshal(bytes.TrimSpace(output.Bytes()), &response); err != nil {
		t.Fatalf("stdout is not one protocol response: %v", err)
	}
	if string(response.Result) != "{}" {
		t.Fatalf("ping result: %s", response.Result)
	}
}
