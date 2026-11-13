package mcpcontrol

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"regexp"

	"github.com/rainchan/webmux/internal/contract"
	"github.com/rainchan/webmux/internal/localclient"
	"github.com/rainchan/webmux/internal/mcpprotocol"
)

var requestID = regexp.MustCompile(`^[A-Za-z0-9_-]{8,64}$`)

func object(properties map[string]any, required ...string) map[string]any {
	if required == nil {
		required = []string{}
	}
	return map[string]any{"type": "object", "additionalProperties": false, "properties": properties, "required": required}
}
func boolean() map[string]any      { return map[string]any{"type": "boolean"} }
func stringsArray() map[string]any { return map[string]any{"type": "array", "items": text()} }
func stringMap() map[string]any {
	return map[string]any{"type": "object", "additionalProperties": text()}
}

func (c Client) actionTools() []mcpprotocol.Tool {
	return []mcpprotocol.Tool{
		tool("session_launch", "Create a session, optionally in a new worktree with an initial prompt. Requires a stable clientRequestId; retry the identical request after a lost response. Creation is deduplicated by the server for ten minutes, in memory only. Partial failures retain created resources.", false, createProperties(), []string{"clientRequestId"}, c.launch),
		tool("session_send", "Send input through the current session view. Reuse clientRequestId on retries. Chat returns turnId or queued itemId; terminal acceptance does not prove execution or completion. Read state before sending.", false, map[string]any{
			"sessionId": text(), "request": object(map[string]any{"clientRequestId": text(), "text": text(), "submit": boolean(), "keys": stringsArray(), "mode": enumeration("auto", "steer", "refuse", "now"), "attachments": attachmentSchema()}, "clientRequestId", "text"),
		}, []string{"sessionId", "request"}, c.send),
		tool("session_interrupt", "Interrupt the current chat turn or send the terminal interrupt key. Does not delete the session or its history. Reuse clientRequestId on retries.", false, map[string]any{"sessionId": text(), "clientRequestId": text()}, []string{"sessionId", "clientRequestId"}, c.interrupt),
	}
}
func attachmentSchema() map[string]any {
	return map[string]any{"type": "array", "items": object(map[string]any{"path": text(), "name": text(), "mimeType": text(), "kind": text(), "size": integer(0, 1<<30)}, "path")}
}
func createProperties() map[string]any {
	return map[string]any{
		"clientRequestId": text(), "configId": text(), "cmd": stringsArray(), "env": stringMap(), "cwd": text(), "name": text(),
		"workspace":   object(map[string]any{"repositoryId": text(), "worktreeId": text()}, "repositoryId", "worktreeId"),
		"newWorktree": object(map[string]any{"repositoryId": text(), "mode": enumeration("new", "existing"), "branch": text(), "base": text(), "path": text()}, "repositoryId", "mode"),
		"labels":      stringMap(), "view": enumeration("terminal", "chat"), "prompt": text(), "useInitInstruction": boolean(), "expandPrompt": boolean(),
		"attachments": attachmentSchema(), "agentConfig": stringMap(), "permissionMode": enumeration("ask", "auto"), "cols": integer(1, 1000), "rows": integer(1, 1000),
	}
}

func (c Client) launch(ctx context.Context, raw json.RawMessage) (any, error) {
	var req contract.CreateSessionRequest
	if decode(raw, &req) != nil || !requestID.MatchString(req.ClientRequestID) {
		return nil, errors.New("session_launch requires valid arguments and clientRequestId (8-64 letters, digits, - or _)")
	}
	var result contract.CreateSessionResult
	if err := c.API.DoBounded(ctx, http.MethodPost, "/api/sessions", req, responseLimit, &result); err != nil {
		var e *localclient.APIError
		if errors.As(err, &e) {
			var failure contract.CreateSessionFailure
			if json.Unmarshal(e.Body, &failure) == nil && failure.Step != "" {
				data, marshalErr := json.Marshal(failure)
				if marshalErr != nil {
					return nil, errors.New("cannot encode creation failure")
				}
				return nil, errors.New(string(data))
			}
		}
		return nil, apiError(err)
	}
	// Link lookup is advisory: a successful creation must never look failed just
	// because a second metadata read is unavailable.
	var details contract.SessionDetails
	_ = c.API.GetBounded(ctx, "/api/sessions/"+url.PathEscape(result.Session.ID)+"/details", responseLimit, &details)
	out := contract.MCPSessionLaunchResult{ClientRequestID: req.ClientRequestID, Session: result.Session, Worktree: result.Worktree, ConfigWarnings: result.ConfigWarnings, Link: details.Link, InitialInput: result.InitialInput}

	if result.InitialInput != nil {
		switch {
		case result.InitialInput.TurnID != "":
			out.Wait = &contract.MCPSessionWaitSelector{SessionID: result.Session.ID, TurnID: result.InitialInput.TurnID}
		case result.InitialInput.ItemID != "":
			out.Wait = &contract.MCPSessionWaitSelector{SessionID: result.Session.ID, QueueItemID: result.InitialInput.ItemID}
		}
	}

	return out, nil
}
func (c Client) send(ctx context.Context, raw json.RawMessage) (any, error) {
	var a contract.MCPSessionSendRequest
	if decode(raw, &a) != nil || !identity.MatchString(a.SessionID) || !requestID.MatchString(a.Request.ClientRequestID) {
		return nil, errors.New("invalid session_send arguments or clientRequestId")
	}
	var result contract.SessionInputResult
	if err := c.API.DoBounded(ctx, http.MethodPost, "/api/sessions/"+url.PathEscape(a.SessionID)+"/input", a.Request, responseLimit, &result); err != nil {
		return nil, apiError(err)
	}
	return contract.MCPSessionSendResult{ClientRequestID: a.Request.ClientRequestID, SessionID: a.SessionID, Receipt: result}, nil
}
func (c Client) interrupt(ctx context.Context, raw json.RawMessage) (any, error) {
	var a contract.MCPSessionInterruptRequest
	if decode(raw, &a) != nil || !identity.MatchString(a.SessionID) || !requestID.MatchString(a.ClientRequestID) {
		return nil, errors.New("invalid session_interrupt arguments or clientRequestId")
	}
	err := c.API.DoBounded(ctx, http.MethodPost, "/api/sessions/"+url.PathEscape(a.SessionID)+"/interrupt", contract.SessionCommandRequest{ClientRequestID: a.ClientRequestID}, responseLimit, nil)
	return contract.MCPSessionInterruptResult{SessionID: a.SessionID, ClientRequestID: a.ClientRequestID, Interrupted: err == nil}, apiError(err)
}
