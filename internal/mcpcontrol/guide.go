package mcpcontrol

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"

	"github.com/rainchan/webmux/internal/agentguide"
	"github.com/rainchan/webmux/internal/contract"
	"github.com/rainchan/webmux/internal/mcpprotocol"
)

type guideSnapshot struct {
	context      agentguide.Context
	instructions string
}

const guideResultLimit = 64 << 10

var guideTopicPattern = regexp.MustCompile(`^[a-z][a-z0-9-]{0,63}$`)

func (c Client) guideTools() []mcpprotocol.Tool {
	return []mcpprotocol.Tool{tool("guide_read", "Read the transport-aware WebMux operation guide index or a curated topic.", true, map[string]any{"topic": map[string]any{"type": "string", "minLength": 1, "maxLength": 64, "pattern": "^[a-z][a-z0-9-]*$"}}, nil, c.guideRead)}
}

func (c Client) guideRead(_ context.Context, raw json.RawMessage) (any, error) {
	var a contract.MCPGuideReadRequest
	if err := decode(raw, &a); err != nil {
		return nil, err
	}
	var topic string
	if a.Topic != nil && (json.Unmarshal(a.Topic, &topic) != nil || !guideTopicPattern.MatchString(topic)) {
		return nil, errors.New("invalid tool arguments: topic must be a 1–64 character topic identifier")
	}
	r, err := agentguide.MCPGuide(c.guideContext(), topic)
	if err != nil {
		return nil, err
	}
	encoded, err := mcpprotocol.MCPResult(r, nil)
	if err != nil {
		return nil, err
	}
	if len(encoded) > guideResultLimit {
		return nil, errors.New("guide_too_large: complete tool result exceeds 65536-byte limit")
	}
	return mcpprotocol.EncodedResult(encoded), nil
}

func (c Client) normalizedGuideContext() agentguide.Context {
	context := c.GuideContext
	if context.Transport == "" {
		context.Transport = "mcp-stdio"
	}
	if context.Locality == "" {
		context.Locality = "local"
	}
	if context.Locality != "local" {
		context.SelfSessionID = nil
	}
	if context.SelfSessionID != nil {
		id := *context.SelfSessionID
		context.SelfSessionID = &id
	}
	context.Tools = nil
	return context
}

func (c Client) guideContext() agentguide.Context {
	if c.guide != nil {
		return c.guide.context
	}
	context := c.normalizedGuideContext()
	context.Tools = toolSummaries(c.tools())
	return context
}

func toolSummaries(tools []mcpprotocol.Tool) []agentguide.ToolSummary {
	summaries := make([]agentguide.ToolSummary, 0, len(tools))
	for _, tool := range tools {
		summaries = append(summaries, agentguide.ToolSummary{Name: tool.Definition["name"].(string), Description: tool.Definition["description"].(string)})
	}
	return summaries
}

func (c Client) guideInstructions(guide agentguide.Context) string {
	if c.guide != nil {
		return c.guide.instructions
	}
	return agentguide.Instructions(guide)
}
