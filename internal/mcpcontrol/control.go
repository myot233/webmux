// Package mcpcontrol exposes session control as a local API client. It does not
// construct services or access their storage directly.
package mcpcontrol

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"regexp"
	"slices"
	"time"

	"github.com/rainchan/webmux/internal/agentguide"
	"github.com/rainchan/webmux/internal/contract"
	"github.com/rainchan/webmux/internal/jsonutil"
	"github.com/rainchan/webmux/internal/localclient"
	"github.com/rainchan/webmux/internal/mcpprotocol"
)

const responseLimit = 4 << 20

var identity = regexp.MustCompile(`^[A-Za-z0-9_-]{1,128}$`)

// Client uses the injected API connection's authentication and caller identity.
type Client struct {
	API          *localclient.Client
	GuideContext agentguide.Context
	guide        *guideSnapshot
}

func (c Client) Registry(versions ...string) (*mcpprotocol.Registry, error) {
	guide := guideSnapshot{context: c.normalizedGuideContext()}
	c.guide = &guide
	tools := c.tools()
	guide.context.Tools = toolSummaries(tools)
	guide.instructions = agentguide.Instructions(guide.context)
	return mcpprotocol.New(mcpprotocol.Config{Name: "webmux-control", Instructions: guide.instructions, CallTimeout: 185 * time.Second, Tools: tools, Versions: versions})
}

func (c Client) tools() []mcpprotocol.Tool {
	return append(append(append(append(c.actionTools(), c.readTools()...), c.waitTools()...), c.guideTools()...), []mcpprotocol.Tool{
		tool("capabilities", "List available launch configurations and supported WebMux control tools. Does not start an agent or probe authentication.", true, map[string]any{}, nil, c.capabilities),
		tool("session_list", "List sessions with bounded pagination; optionally filter by state, parent, repository or status.", true, map[string]any{
			"state": enumeration("active", "closed", "interrupted", "archived", "all"), "parentId": text(), "repositoryId": text(), "status": enumeration("running", "waiting", "idle", "exited"),
			"offset": integer(0, 100000), "limit": integer(1, 100),
		}, nil, c.list),
		tool("session_get", "Read session metadata and its browser link, including parent, children and worktree. Does not read output or environment values.", true, map[string]any{"sessionId": text()}, []string{"sessionId"}, c.get),
	}...)
}

func tool(name, description string, readOnly bool, properties map[string]any, required []string, call func(context.Context, json.RawMessage) (any, error)) mcpprotocol.Tool {
	if required == nil {
		required = []string{}
	}
	return mcpprotocol.Tool{Definition: map[string]any{
		"name": name, "description": description,
		"annotations": map[string]any{"readOnlyHint": readOnly, "destructiveHint": !readOnly, "openWorldHint": false},
		"inputSchema": map[string]any{"type": "object", "additionalProperties": false, "properties": properties, "required": required},
	}, Call: call}
}
func text() map[string]any { return map[string]any{"type": "string"} }
func enumeration(values ...string) map[string]any {
	return map[string]any{"type": "string", "enum": values}
}
func integer(minimum, maximum int) map[string]any {
	return map[string]any{"type": "integer", "minimum": minimum, "maximum": maximum}
}

func decode(raw json.RawMessage, out any) error {
	if len(raw) == 0 {
		raw = json.RawMessage(`{}`)
	}
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) || jsonutil.Decode(raw, out) != nil {
		return errors.New("invalid tool arguments")
	}
	return nil
}

// apiError preserves API conflict details without exposing dialer URLs or local
// paths from transport failures. Tool handlers never log inputs or responses.
func apiError(err error) error {
	if err == nil {
		return nil
	}
	var e *localclient.APIError
	if errors.As(err, &e) {
		return errors.New(e.Error())
	}
	return errors.New("WebMux local API unavailable or response exceeds limit")
}

func (c Client) capabilities(ctx context.Context, raw json.RawMessage) (any, error) {
	if err := decode(raw, &struct{}{}); err != nil {
		return nil, err
	}
	var configs []contract.LaunchConfigSummary
	if err := c.API.GetBounded(ctx, "/api/launch-configs", responseLimit, &configs); err != nil {
		return nil, apiError(err)
	}
	// Only return what is needed for selection; command arguments and environment
	// names do not need to enter the model's context.
	type launch struct {
		ID      string               `json:"id"`
		Name    string               `json:"name"`
		Chat    bool                 `json:"chat"`
		Support contract.ChatSupport `json:"chatSupport"`
	}
	launches := make([]launch, 0, len(configs))
	for _, config := range configs {
		launches = append(launches, launch{config.ID, config.Name, config.Chat, config.ChatSupport})
	}
	guide := c.guideContext()
	names := make([]string, 0, len(guide.Tools))
	for _, t := range guide.Tools {
		names = append(names, t.Name)
	}
	return map[string]any{"tools": names, "launchConfigs": launches, "views": []string{"terminal", "chat"}, "instructions": c.guideInstructions(guide), "transport": guide.Transport, "locality": guide.Locality, "selfSessionId": guide.SelfSessionID}, nil
}

type listArguments struct {
	State        string                 `json:"state,omitempty"`
	ParentID     string                 `json:"parentId,omitempty"`
	RepositoryID string                 `json:"repositoryId,omitempty"`
	Status       contract.SessionStatus `json:"status,omitempty"`
	Offset       int                    `json:"offset,omitempty"`
	Limit        int                    `json:"limit,omitempty"`
}

func (c Client) list(ctx context.Context, raw json.RawMessage) (any, error) {
	a := listArguments{State: "active", Limit: 20}
	if err := decode(raw, &a); err != nil {
		return nil, err
	}
	if !slices.Contains([]string{"active", "closed", "interrupted", "archived", "all"}, a.State) || a.Offset < 0 || a.Offset > 100000 || a.Limit < 1 || a.Limit > 100 || (a.Status != "" && !slices.Contains([]contract.SessionStatus{contract.SessionStatusRunning, contract.SessionStatusWaiting, contract.SessionStatusIdle, contract.SessionStatusExited}, a.Status)) {
		return nil, errors.New("invalid session_list filter or page")
	}
	var sessions []contract.SessionInfo
	if err := c.API.GetBounded(ctx, "/api/sessions?state="+url.QueryEscape(a.State), responseLimit, &sessions); err != nil {
		return nil, apiError(err)
	}
	filtered := make([]contract.SessionInfo, 0)
	for _, s := range sessions {
		if a.ParentID != "" && s.ParentID != a.ParentID || a.RepositoryID != "" && (s.Workspace == nil || s.Workspace.RepositoryID != a.RepositoryID) || a.Status != "" && s.Status != a.Status {
			continue
		}
		filtered = append(filtered, s)
	}
	start := min(a.Offset, len(filtered))
	end := min(start+a.Limit, len(filtered))
	return map[string]any{"sessions": filtered[start:end], "nextOffset": end, "done": end == len(filtered), "total": len(filtered)}, nil
}

func (c Client) get(ctx context.Context, raw json.RawMessage) (any, error) {
	var a struct {
		SessionID string `json:"sessionId"`
	}
	if decode(raw, &a) != nil || !identity.MatchString(a.SessionID) {
		return nil, errors.New("invalid sessionId")
	}
	var result contract.SessionDetails
	err := c.API.GetBounded(ctx, "/api/sessions/"+url.PathEscape(a.SessionID)+"/details", responseLimit, &result)
	return result, apiError(err)
}
