package mcpcontrol

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/rainchan/webmux/internal/agentguide"
	"github.com/rainchan/webmux/internal/mcpprotocol"
)

func TestGuideReadArguments(t *testing.T) {
	c := Client{}
	for _, raw := range []string{`null`, `[]`, `{"topic":null}`, `{"topic":""}`, `{"topic":7}`, `{"topic":"Completion"}`, `{"topic":"../rules"}`, `{"url":"https://example.com"}`, `{"topic":"rules","extra":true}`, `{"topic":"` + strings.Repeat("a", 65) + `"}`} {
		if _, err := c.guideRead(context.Background(), json.RawMessage(raw)); err == nil {
			t.Errorf("accepted %s", raw)
		}
	}
	if _, err := c.guideRead(context.Background(), json.RawMessage(`{"topic":"unknown"}`)); err == nil {
		t.Fatal("accepted unknown topic")
	}
}

func TestGuideReadLimitsCompleteToolResult(t *testing.T) {
	c := Client{GuideContext: agentguide.Context{Transport: "mcp-stdio", Locality: "local"}}
	// Use escaped identity data: measuring only structuredContent would miss
	// the JSON text copy and its extra escaping.
	id := strings.Repeat("\"", 20000)
	c.GuideContext.SelfSessionID = &id
	_, err := c.guideRead(context.Background(), json.RawMessage(`{}`))
	if err == nil {
		t.Fatal("accepted oversized complete tool result")
	}
	r, err := agentguide.MCPGuide(c.guideContext(), "")
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := mcpprotocol.MCPResult(r, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(encoded) <= guideResultLimit {
		t.Fatalf("fixture is not oversized: %d", len(encoded))
	}
	low, high := 0, 20000
	for low < high {
		n := (low + high + 1) / 2
		id = strings.Repeat("\"", n)
		r, err := agentguide.MCPGuide(c.guideContext(), "")
		if err != nil {
			t.Fatal(err)
		}
		encoded, err := mcpprotocol.MCPResult(r, nil)
		if err != nil {
			t.Fatal(err)
		}
		if len(encoded) <= guideResultLimit {
			low = n
		} else {
			high = n - 1
		}
	}
	id = strings.Repeat("\"", low)
	if _, err := c.guideRead(context.Background(), json.RawMessage(`{}`)); err != nil {
		t.Fatalf("rejected last fitting result: %v", err)
	}
	id += "\""
	if _, err := c.guideRead(context.Background(), json.RawMessage(`{}`)); err == nil {
		t.Fatal("accepted first oversized result")
	}
}
