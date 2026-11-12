package acp

import (
	"encoding/json"
)

// ProtocolVersion is the only ACP major version this package speaks.
const ProtocolVersion = 1

// Implementation names a client or agent (schema Implementation).
type Implementation struct {
	Name    string `json:"name"`
	Title   string `json:"title,omitempty"`
	Version string `json:"version"`
}

// Capability is an opt-in capability object: present (even as {}) means
// supported, nil means not advertised.
type Capability struct {
	Meta json.RawMessage `json:"_meta,omitempty"`
}

// ClientCapabilities lists what the client advertises. fs and terminal are
// deliberately absent: omitting them declares them unsupported.
type ClientCapabilities struct {
	Session     *ClientSessionCapabilities `json:"session,omitempty"`
	Elicitation *ElicitationCapabilities   `json:"elicitation,omitempty"`
}

// ClientSessionCapabilities opts in to session updates that agents only
// send when asked for.
type ClientSessionCapabilities struct {
	Notices    *Capability `json:"notices,omitempty"`
	Compaction *Capability `json:"compaction,omitempty"`
}

// ElicitationCapabilities advertises elicitation modes.
type ElicitationCapabilities struct {
	Form *Capability `json:"form,omitempty"`
	URL  *Capability `json:"url,omitempty"`
}

// clientCapabilities is the fixed set webmux advertises: notices,
// compaction and form elicitation; no fs, terminal or URL elicitation.
func clientCapabilities() ClientCapabilities {
	return ClientCapabilities{
		Session:     &ClientSessionCapabilities{Notices: &Capability{}, Compaction: &Capability{}},
		Elicitation: &ElicitationCapabilities{Form: &Capability{}},
	}
}

// InitializeRequest is the initialize params.
type InitializeRequest struct {
	ProtocolVersion    int                `json:"protocolVersion"`
	ClientCapabilities ClientCapabilities `json:"clientCapabilities"`
	ClientInfo         *Implementation    `json:"clientInfo,omitempty"`
	Meta               json.RawMessage    `json:"_meta,omitempty"`
}

// InitializeResponse is the initialize result.
type InitializeResponse struct {
	ProtocolVersion   int               `json:"protocolVersion"`
	AgentCapabilities AgentCapabilities `json:"agentCapabilities"`
	AuthMethods       []AuthMethod      `json:"authMethods,omitempty"`
	AgentInfo         *Implementation   `json:"agentInfo,omitempty"`
	Meta              json.RawMessage   `json:"_meta,omitempty"`
}

// AgentCapabilities is what the agent advertises. session/load is still
// declared by LoadSession; the other session methods by
// SessionCapabilities.
type AgentCapabilities struct {
	LoadSession         bool                `json:"loadSession,omitempty"`
	PromptCapabilities  PromptCapabilities  `json:"promptCapabilities,omitzero"`
	SessionCapabilities SessionCapabilities `json:"sessionCapabilities,omitzero"`
	Meta                json.RawMessage     `json:"_meta,omitempty"`
}

// PromptCapabilities lists optional prompt content types.
type PromptCapabilities struct {
	Image           bool `json:"image,omitempty"`
	Audio           bool `json:"audio,omitempty"`
	EmbeddedContext bool `json:"embeddedContext,omitempty"`
}

// SessionCapabilities lists optional session methods.
type SessionCapabilities struct {
	List   *Capability `json:"list,omitempty"`
	Resume *Capability `json:"resume,omitempty"`
	Close  *Capability `json:"close,omitempty"`
	Delete *Capability `json:"delete,omitempty"`
}

// AuthMethod is one advertised authentication method. Type is empty for
// agent-handled methods.
type AuthMethod struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Type        string `json:"type,omitempty"`
}

// NewSessionRequest is the session/new params. MCPServers is always sent,
// as [] when nil. Only target-scoped history servers are supplied.
type NewSessionRequest struct {
	Cwd        string      `json:"cwd"`
	MCPServers []MCPServer `json:"mcpServers"`
}

// NewSessionResponse is the session/new result.
type NewSessionResponse struct {
	SessionID     string            `json:"sessionId"`
	Modes         *SessionModeState `json:"modes,omitempty"`
	ConfigOptions []ConfigOption    `json:"configOptions,omitempty"`
}

// LoadSessionRequest is the session/load params. The agent replays the
// history as session/update notifications before it responds.
type LoadSessionRequest struct {
	SessionID  string      `json:"sessionId"`
	Cwd        string      `json:"cwd"`
	MCPServers []MCPServer `json:"mcpServers"`
}

// LoadSessionResponse is the session/load result.
type LoadSessionResponse struct {
	Modes         *SessionModeState `json:"modes,omitempty"`
	ConfigOptions []ConfigOption    `json:"configOptions,omitempty"`
}

// ResumeSessionRequest is the session/resume params; no history is
// replayed.
type ResumeSessionRequest struct {
	SessionID  string      `json:"sessionId"`
	Cwd        string      `json:"cwd"`
	MCPServers []MCPServer `json:"mcpServers"`
}

// ResumeSessionResponse is the session/resume result.
type ResumeSessionResponse struct {
	Modes         *SessionModeState `json:"modes,omitempty"`
	ConfigOptions []ConfigOption    `json:"configOptions,omitempty"`
}

// ListSessionsRequest is the session/list params. Cursor is the opaque
// NextCursor of a previous page.
type ListSessionsRequest struct {
	Cwd    string `json:"cwd,omitempty"`
	Cursor string `json:"cursor,omitempty"`
}

// ListSessionsResponse is the session/list result.
type ListSessionsResponse struct {
	Sessions   []SessionInfo `json:"sessions"`
	NextCursor string        `json:"nextCursor,omitempty"`
}

// SessionInfo is one listed session.
type SessionInfo struct {
	SessionID string `json:"sessionId"`
	Cwd       string `json:"cwd"`
	Title     string `json:"title,omitempty"`
	UpdatedAt string `json:"updatedAt,omitempty"`
}

// CloseSessionRequest is the session/close params.
type CloseSessionRequest struct {
	SessionID string `json:"sessionId"`
}

// PromptRequest is the session/prompt params.
type PromptRequest struct {
	SessionID string          `json:"sessionId"`
	Prompt    []ContentBlock  `json:"prompt"`
	Meta      json.RawMessage `json:"_meta,omitempty"`
}

// Stop reasons of a prompt turn.
const (
	StopEndTurn         = "end_turn"
	StopMaxTokens       = "max_tokens"
	StopMaxTurnRequests = "max_turn_requests"
	StopRefusal         = "refusal"
	StopCancelled       = "cancelled"
)

// PromptResponse is the session/prompt result.
type PromptResponse struct {
	StopReason string `json:"stopReason"`
}

// CancelNotification is the session/cancel params.
type CancelNotification struct {
	SessionID string `json:"sessionId"`
}

// CancelRequestNotification is the $/cancel_request params.
type CancelRequestNotification struct {
	RequestID json.RawMessage `json:"requestId"`
}

// SessionModeState is the legacy mode selector some agents still send.
type SessionModeState struct {
	CurrentModeID  string        `json:"currentModeId"`
	AvailableModes []SessionMode `json:"availableModes"`
}

// SessionMode is one legacy mode.
type SessionMode struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}
