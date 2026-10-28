package acp

import (
	"encoding/json"
	"fmt"
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
// as [] when nil: webmux passes no MCP servers.
type NewSessionRequest struct {
	Cwd        string            `json:"cwd"`
	MCPServers []json.RawMessage `json:"mcpServers"`
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
	SessionID  string            `json:"sessionId"`
	Cwd        string            `json:"cwd"`
	MCPServers []json.RawMessage `json:"mcpServers"`
}

// LoadSessionResponse is the session/load result.
type LoadSessionResponse struct {
	Modes         *SessionModeState `json:"modes,omitempty"`
	ConfigOptions []ConfigOption    `json:"configOptions,omitempty"`
}

// ResumeSessionRequest is the session/resume params; no history is
// replayed.
type ResumeSessionRequest struct {
	SessionID  string            `json:"sessionId"`
	Cwd        string            `json:"cwd"`
	MCPServers []json.RawMessage `json:"mcpServers"`
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

// Config option types.
const (
	ConfigSelect  = "select"
	ConfigBoolean = "boolean"
)

// ConfigValue is a config option value: a boolean when Bool is non-nil,
// otherwise the select value ID.
type ConfigValue struct {
	Bool *bool
	ID   string
}

// ConfigOption is one session configuration option (schema
// SessionConfigOption). Grouped select options are flattened in order, each
// carrying its group.
type ConfigOption struct {
	ID          string
	Name        string
	Description string
	Category    string
	Type        string
	Current     ConfigValue
	Options     []ConfigSelectOption
}

// ConfigSelectOption is one select choice.
type ConfigSelectOption struct {
	Value       string
	Name        string
	Description string
	Group       string
	GroupName   string
}

type configSelectOptionJSON struct {
	Value       string `json:"value"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

type configGroupJSON struct {
	Group   string                   `json:"group"`
	Name    string                   `json:"name"`
	Options []configSelectOptionJSON `json:"options"`
}

type configOptionJSON struct {
	ID           string          `json:"id"`
	Name         string          `json:"name"`
	Description  string          `json:"description,omitempty"`
	Category     string          `json:"category,omitempty"`
	Type         string          `json:"type"`
	CurrentValue json.RawMessage `json:"currentValue"`
	Options      json.RawMessage `json:"options,omitempty"`
}

// MarshalJSON writes the option in its schema shape.
func (o ConfigOption) MarshalJSON() ([]byte, error) {
	out := configOptionJSON{ID: o.ID, Name: o.Name, Description: o.Description, Category: o.Category, Type: o.Type}
	var err error
	if o.Current.Bool != nil {
		out.CurrentValue, err = json.Marshal(*o.Current.Bool)
	} else {
		out.CurrentValue, err = json.Marshal(o.Current.ID)
	}
	if err != nil {
		return nil, err
	}
	if o.Type == ConfigSelect {
		if out.Options, err = marshalSelectOptions(o.Options); err != nil {
			return nil, err
		}
	}
	return json.Marshal(out)
}

func marshalSelectOptions(opts []ConfigSelectOption) (json.RawMessage, error) {
	grouped := false
	for _, opt := range opts {
		if opt.Group != "" {
			grouped = true
			break
		}
	}
	if !grouped {
		flat := make([]configSelectOptionJSON, len(opts))
		for i, opt := range opts {
			flat[i] = configSelectOptionJSON{Value: opt.Value, Name: opt.Name, Description: opt.Description}
		}
		return json.Marshal(flat)
	}
	var groups []configGroupJSON
	for _, opt := range opts {
		if len(groups) == 0 || groups[len(groups)-1].Group != opt.Group {
			groups = append(groups, configGroupJSON{Group: opt.Group, Name: opt.GroupName})
		}
		g := &groups[len(groups)-1]
		g.Options = append(g.Options, configSelectOptionJSON{Value: opt.Value, Name: opt.Name, Description: opt.Description})
	}
	return json.Marshal(groups)
}

// UnmarshalJSON reads the schema shape.
func (o *ConfigOption) UnmarshalJSON(b []byte) error {
	var in configOptionJSON
	if err := json.Unmarshal(b, &in); err != nil {
		return err
	}
	*o = ConfigOption{ID: in.ID, Name: in.Name, Description: in.Description, Category: in.Category, Type: in.Type}
	switch in.Type {
	case ConfigBoolean:
		var v bool
		if err := json.Unmarshal(in.CurrentValue, &v); err != nil {
			return fmt.Errorf("config option currentValue: %w", err)
		}
		o.Current.Bool = &v
		return nil
	case ConfigSelect:
		if err := json.Unmarshal(in.CurrentValue, &o.Current.ID); err != nil {
			return fmt.Errorf("config option currentValue: %w", err)
		}
	default:
		return nil // future types keep their identity only
	}
	var items []struct {
		configSelectOptionJSON
		Group   string                   `json:"group"`
		Options []configSelectOptionJSON `json:"options"`
	}
	if len(in.Options) > 0 {
		if err := json.Unmarshal(in.Options, &items); err != nil {
			return fmt.Errorf("config option options: %w", err)
		}
	}
	for _, it := range items {
		if it.Group == "" {
			o.Options = append(o.Options, ConfigSelectOption{Value: it.Value, Name: it.Name, Description: it.Description})
			continue
		}
		for _, opt := range it.Options {
			o.Options = append(o.Options, ConfigSelectOption{Value: opt.Value, Name: opt.Name, Description: opt.Description, Group: it.Group, GroupName: it.Name})
		}
	}
	return nil
}

// SetConfigOptionRequest is the session/set_config_option params.
type SetConfigOptionRequest struct {
	SessionID string
	ConfigID  string
	Value     ConfigValue
}

type setConfigOptionJSON struct {
	SessionID string          `json:"sessionId"`
	ConfigID  string          `json:"configId"`
	Type      string          `json:"type,omitempty"`
	Value     json.RawMessage `json:"value"`
}

// MarshalJSON writes a boolean as {type: "boolean", value} and a select
// value as {value}.
func (r SetConfigOptionRequest) MarshalJSON() ([]byte, error) {
	out := setConfigOptionJSON{SessionID: r.SessionID, ConfigID: r.ConfigID}
	var err error
	if r.Value.Bool != nil {
		out.Type = ConfigBoolean
		out.Value, err = json.Marshal(*r.Value.Bool)
	} else {
		out.Value, err = json.Marshal(r.Value.ID)
	}
	if err != nil {
		return nil, err
	}
	return json.Marshal(out)
}

// UnmarshalJSON reads either value shape.
func (r *SetConfigOptionRequest) UnmarshalJSON(b []byte) error {
	var in setConfigOptionJSON
	if err := json.Unmarshal(b, &in); err != nil {
		return err
	}
	*r = SetConfigOptionRequest{SessionID: in.SessionID, ConfigID: in.ConfigID}
	if in.Type == ConfigBoolean {
		var v bool
		if err := json.Unmarshal(in.Value, &v); err != nil {
			return fmt.Errorf("config value: %w", err)
		}
		r.Value.Bool = &v
		return nil
	}
	if err := json.Unmarshal(in.Value, &r.Value.ID); err != nil {
		return fmt.Errorf("config value: %w", err)
	}
	return nil
}

// SetConfigOptionResponse is the session/set_config_option result: the
// complete option set after the change.
type SetConfigOptionResponse struct {
	ConfigOptions []ConfigOption `json:"configOptions"`
}

// Permission option kinds.
const (
	PermissionAllowOnce    = "allow_once"
	PermissionAllowAlways  = "allow_always"
	PermissionRejectOnce   = "reject_once"
	PermissionRejectAlways = "reject_always"
)

// PermissionOption is one choice offered by session/request_permission.
type PermissionOption struct {
	OptionID string `json:"optionId"`
	Name     string `json:"name"`
	Kind     string `json:"kind"`
}

// RequestPermissionRequest is the session/request_permission params.
type RequestPermissionRequest struct {
	SessionID string             `json:"sessionId"`
	ToolCall  ToolCallUpdate     `json:"toolCall"`
	Options   []PermissionOption `json:"options"`
}

// Permission outcomes.
const (
	OutcomeCancelled = "cancelled"
	OutcomeSelected  = "selected"
)

// PermissionOutcome is the answer: OutcomeSelected with OptionID, or
// OutcomeCancelled once the turn was cancelled.
type PermissionOutcome struct {
	Outcome  string `json:"outcome"`
	OptionID string `json:"optionId,omitempty"`
}

// RequestPermissionResponse is the session/request_permission result.
type RequestPermissionResponse struct {
	Outcome PermissionOutcome `json:"outcome"`
}

// Elicitation modes and actions.
const (
	ElicitationForm = "form"
	ElicitationURL  = "url"

	ElicitAccept  = "accept"
	ElicitDecline = "decline"
	ElicitCancel  = "cancel"
)

// CreateElicitationRequest is the elicitation/create params. A session
// scoped request has SessionID (and optionally ToolCallID); a request scoped
// one has RequestID.
type CreateElicitationRequest struct {
	Mode            string             `json:"mode"`
	Message         string             `json:"message"`
	SessionID       string             `json:"sessionId,omitempty"`
	ToolCallID      string             `json:"toolCallId,omitempty"`
	RequestID       json.RawMessage    `json:"requestId,omitempty"`
	RequestedSchema *ElicitationSchema `json:"requestedSchema,omitempty"`
}

// ElicitationSchema is a flat form: primitive properties only.
type ElicitationSchema struct {
	Type        string                         `json:"type,omitempty"`
	Title       string                         `json:"title,omitempty"`
	Description string                         `json:"description,omitempty"`
	Properties  map[string]ElicitationProperty `json:"properties,omitempty"`
	Required    []string                       `json:"required,omitempty"`
}

// ElicitationProperty is one form field. Type is string, number, integer,
// boolean or array (multi-select); the other fields apply per type.
type ElicitationProperty struct {
	Type        string            `json:"type"`
	Title       string            `json:"title,omitempty"`
	Description string            `json:"description,omitempty"`
	MinLength   *int              `json:"minLength,omitempty"`
	MaxLength   *int              `json:"maxLength,omitempty"`
	Pattern     string            `json:"pattern,omitempty"`
	Format      string            `json:"format,omitempty"`
	Default     json.RawMessage   `json:"default,omitempty"`
	Enum        []string          `json:"enum,omitempty"`
	OneOf       []EnumOption      `json:"oneOf,omitempty"`
	Minimum     *float64          `json:"minimum,omitempty"`
	Maximum     *float64          `json:"maximum,omitempty"`
	MinItems    *int              `json:"minItems,omitempty"`
	MaxItems    *int              `json:"maxItems,omitempty"`
	Items       *MultiSelectItems `json:"items,omitempty"`
}

// EnumOption is a titled choice.
type EnumOption struct {
	Const       string `json:"const"`
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
}

// MultiSelectItems lists the choices of a multi-select field: plain Enum
// values with Type "string", or titled AnyOf choices.
type MultiSelectItems struct {
	Type  string       `json:"type,omitempty"`
	Enum  []string     `json:"enum,omitempty"`
	AnyOf []EnumOption `json:"anyOf,omitempty"`
}

// CreateElicitationResponse is the elicitation/create result. Content
// values are strings, numbers, booleans or string slices.
type CreateElicitationResponse struct {
	Action  string         `json:"action"`
	Content map[string]any `json:"content,omitempty"`
}
