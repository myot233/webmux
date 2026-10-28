package acp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// Method names (schema meta.json).
const (
	MethodInitialize             = "initialize"
	MethodSessionNew             = "session/new"
	MethodSessionLoad            = "session/load"
	MethodSessionResume          = "session/resume"
	MethodSessionList            = "session/list"
	MethodSessionClose           = "session/close"
	MethodSessionPrompt          = "session/prompt"
	MethodSessionCancel          = "session/cancel"
	MethodSessionSetConfigOption = "session/set_config_option"
	MethodSessionUpdate          = "session/update"
	MethodRequestPermission      = "session/request_permission"
	MethodElicitationCreate      = "elicitation/create"
	MethodCancelRequest          = "$/cancel_request"
)

// Default request deadlines, applied unless ctx expires sooner. A request
// that runs out sends $/cancel_request and fails with
// context.DeadlineExceeded. session/prompt has none: a turn lasts as long
// as the agent works.
const (
	InitializeTimeout = 30 * time.Second
	LoadTimeout       = 120 * time.Second
	RequestTimeout    = 30 * time.Second
)

func (c *Conn) request(ctx context.Context, timeout time.Duration, method string, params, result any) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	return c.call(ctx, method, params, result, callOptions{mode: cancelRequest})
}

// Initialize negotiates protocol version 1 and advertises the fixed client
// capabilities (notices, compaction, form elicitation; no fs or terminal).
// meta is sent as the request's _meta, for extensions such as steering. An
// agent answering with another version yields ErrUnsupportedVersion along
// with its response; the caller must end the agent.
func (c *Conn) Initialize(ctx context.Context, info Implementation, meta json.RawMessage) (InitializeResponse, error) {
	req := InitializeRequest{
		ProtocolVersion:    ProtocolVersion,
		ClientCapabilities: clientCapabilities(),
		ClientInfo:         &info,
		Meta:               meta,
	}
	var resp InitializeResponse
	if err := c.request(ctx, InitializeTimeout, MethodInitialize, req, &resp); err != nil {
		return InitializeResponse{}, err
	}
	if resp.ProtocolVersion != ProtocolVersion {
		return resp, fmt.Errorf("%w: agent speaks %d", ErrUnsupportedVersion, resp.ProtocolVersion)
	}
	return resp, nil
}

// NewSession creates an agent session.
func (c *Conn) NewSession(ctx context.Context, req NewSessionRequest) (NewSessionResponse, error) {
	if req.MCPServers == nil {
		req.MCPServers = []json.RawMessage{}
	}
	var resp NewSessionResponse
	err := c.request(ctx, RequestTimeout, MethodSessionNew, req, &resp)
	return resp, err
}

// LoadSession opens a session and has the agent replay its history. Every
// replayed update reaches the Handler before LoadSession returns.
func (c *Conn) LoadSession(ctx context.Context, req LoadSessionRequest) (LoadSessionResponse, error) {
	if req.MCPServers == nil {
		req.MCPServers = []json.RawMessage{}
	}
	var resp LoadSessionResponse
	err := c.request(ctx, LoadTimeout, MethodSessionLoad, req, &resp)
	return resp, err
}

// ResumeSession opens a session without replaying its history.
func (c *Conn) ResumeSession(ctx context.Context, req ResumeSessionRequest) (ResumeSessionResponse, error) {
	if req.MCPServers == nil {
		req.MCPServers = []json.RawMessage{}
	}
	var resp ResumeSessionResponse
	err := c.request(ctx, RequestTimeout, MethodSessionResume, req, &resp)
	return resp, err
}

// ListSessions returns one page of sessions, optionally filtered by cwd.
func (c *Conn) ListSessions(ctx context.Context, req ListSessionsRequest) (ListSessionsResponse, error) {
	var resp ListSessionsResponse
	err := c.request(ctx, RequestTimeout, MethodSessionList, req, &resp)
	return resp, err
}

// CloseSession cancels any turn in the session and releases it.
func (c *Conn) CloseSession(ctx context.Context, sessionID string) error {
	return c.request(ctx, RequestTimeout, MethodSessionClose, CloseSessionRequest{SessionID: sessionID}, nil)
}

// Prompt runs one turn. If ctx ends first, Prompt sends session/cancel and
// keeps waiting for the agent's answer, normally StopCancelled, so the
// caller learns when the turn has really ended; closing the Conn abandons
// the wait. Every update of the turn reaches the Handler before Prompt
// returns.
func (c *Conn) Prompt(ctx context.Context, req PromptRequest) (PromptResponse, error) {
	if req.Prompt == nil {
		req.Prompt = []ContentBlock{}
	}
	var resp PromptResponse
	err := c.call(ctx, MethodSessionPrompt, req, &resp, callOptions{mode: cancelTurn, sessionID: req.SessionID})
	return resp, err
}

// Cancel sends session/cancel. The running Prompt call then returns with
// the agent's answer. Pending permission and elicitation handlers are
// answered by the caller (cancelled/cancel), as ACP requires.
func (c *Conn) Cancel(ctx context.Context, sessionID string) error {
	if err := c.notify(ctx, MethodSessionCancel, CancelNotification{SessionID: sessionID}); err != nil {
		return fmt.Errorf("acp: %s: %w", MethodSessionCancel, err)
	}
	return nil
}

// SetConfigOption changes one option and returns the complete new set.
func (c *Conn) SetConfigOption(ctx context.Context, req SetConfigOptionRequest) (SetConfigOptionResponse, error) {
	var resp SetConfigOptionResponse
	err := c.request(ctx, RequestTimeout, MethodSessionSetConfigOption, req, &resp)
	return resp, err
}

// CallExtension calls an agent extension method, whose name must start
// with "_". Callers only use extensions the agent advertised in _meta.
func (c *Conn) CallExtension(ctx context.Context, method string, params, result any) error {
	if !strings.HasPrefix(method, "_") {
		return fmt.Errorf("acp: %q is not an extension method", method)
	}
	return c.request(ctx, RequestTimeout, method, params, result)
}
