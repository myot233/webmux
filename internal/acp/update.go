package acp

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// session/update kinds this package decodes.
const (
	KindUserMessageChunk        = "user_message_chunk"
	KindAgentMessageChunk       = "agent_message_chunk"
	KindAgentThoughtChunk       = "agent_thought_chunk"
	KindToolCall                = "tool_call"
	KindToolCallUpdate          = "tool_call_update"
	KindPlan                    = "plan"
	KindAvailableCommandsUpdate = "available_commands_update"
	KindCurrentModeUpdate       = "current_mode_update"
	KindConfigOptionUpdate      = "config_option_update"
	KindSessionInfoUpdate       = "session_info_update"
	KindUsageUpdate             = "usage_update"
	KindNotice                  = "notice"
	KindCompactionUpdate        = "compaction_update"
	KindCompactionSummaryChunk  = "compaction_summary_chunk"
)

// Update is one session/update payload. The concrete type is one of the
// pointer types below; kinds this package does not know arrive as
// *UnknownUpdate with the raw JSON, so newer agents never break the
// connection.
type Update interface {
	UpdateKind() string
}

// ContentChunk is a streamed piece of a message. MessageID, when the agent
// sends one, groups the chunks of one message.
type ContentChunk struct {
	Content   ContentBlock `json:"content"`
	MessageID string       `json:"messageId,omitempty"`
}

// UserMessageChunk is a user_message_chunk update.
type UserMessageChunk ContentChunk

// AgentMessageChunk is an agent_message_chunk update.
type AgentMessageChunk ContentChunk

// AgentThoughtChunk is an agent_thought_chunk update.
type AgentThoughtChunk ContentChunk

// Tool call statuses.
const (
	ToolPending    = "pending"
	ToolInProgress = "in_progress"
	ToolCompleted  = "completed"
	ToolFailed     = "failed"
)

// ToolCall is a tool_call update: a new tool call.
type ToolCall struct {
	ToolCallID string             `json:"toolCallId"`
	Title      string             `json:"title"`
	Kind       string             `json:"kind,omitempty"`
	Status     string             `json:"status,omitempty"`
	Content    []ToolCallContent  `json:"content,omitempty"`
	Locations  []ToolCallLocation `json:"locations,omitempty"`
	RawInput   json.RawMessage    `json:"rawInput,omitempty"`
	RawOutput  json.RawMessage    `json:"rawOutput,omitempty"`
}

// ToolCallUpdate is a tool_call_update patch, and the tool call carried by
// a permission request. Nil fields are unchanged; a non-nil empty slice
// clears the collection.
type ToolCallUpdate struct {
	ToolCallID string             `json:"toolCallId"`
	Title      *string            `json:"title,omitempty"`
	Kind       *string            `json:"kind,omitempty"`
	Status     *string            `json:"status,omitempty"`
	Content    []ToolCallContent  `json:"content,omitzero"`
	Locations  []ToolCallLocation `json:"locations,omitzero"`
	RawInput   json.RawMessage    `json:"rawInput,omitempty"`
	RawOutput  json.RawMessage    `json:"rawOutput,omitempty"`
}

// Tool call content types.
const (
	ToolContentContent  = "content"
	ToolContentDiff     = "diff"
	ToolContentTerminal = "terminal"
)

// ToolCallContent is one item of tool output. Content is set for type
// "content"; Path, OldText and NewText for "diff"; TerminalID for
// "terminal". Other types keep their JSON in Raw.
type ToolCallContent struct {
	Type       string
	Content    *ContentBlock
	Path       string
	OldText    *string
	NewText    string
	TerminalID string
	Raw        json.RawMessage
}

type toolCallContentJSON struct {
	Type       string        `json:"type"`
	Content    *ContentBlock `json:"content,omitempty"`
	Path       string        `json:"path,omitempty"`
	OldText    *string       `json:"oldText,omitempty"`
	NewText    *string       `json:"newText,omitempty"`
	TerminalID string        `json:"terminalId,omitempty"`
}

// MarshalJSON writes the fields of the item's type.
func (c ToolCallContent) MarshalJSON() ([]byte, error) {
	switch c.Type {
	case ToolContentContent:
		return json.Marshal(toolCallContentJSON{Type: c.Type, Content: c.Content})
	case ToolContentDiff:
		return json.Marshal(toolCallContentJSON{Type: c.Type, Path: c.Path, OldText: c.OldText, NewText: &c.NewText})
	case ToolContentTerminal:
		return json.Marshal(toolCallContentJSON{Type: c.Type, TerminalID: c.TerminalID})
	}
	if len(c.Raw) > 0 {
		return c.Raw, nil
	}
	return json.Marshal(toolCallContentJSON{Type: c.Type})
}

// UnmarshalJSON reads known types into fields and keeps others raw.
func (c *ToolCallContent) UnmarshalJSON(b []byte) error {
	var in toolCallContentJSON
	if err := json.Unmarshal(b, &in); err != nil {
		return err
	}
	*c = ToolCallContent{Type: in.Type, Content: in.Content, Path: in.Path, OldText: in.OldText, TerminalID: in.TerminalID}
	if in.NewText != nil {
		c.NewText = *in.NewText
	}
	switch in.Type {
	case ToolContentContent, ToolContentDiff, ToolContentTerminal:
	default:
		c.Raw = bytes.Clone(b)
	}
	return nil
}

// ToolCallLocation is a file a tool call touches.
type ToolCallLocation struct {
	Path string `json:"path"`
	Line *int   `json:"line,omitempty"`
}

// Plan is a plan update: the complete plan, replacing the previous one.
type Plan struct {
	Entries []PlanEntry `json:"entries"`
}

// PlanEntry is one step of a plan.
type PlanEntry struct {
	Content  string `json:"content"`
	Priority string `json:"priority"`
	Status   string `json:"status"`
}

// AvailableCommandsUpdate replaces the slash commands the agent offers.
type AvailableCommandsUpdate struct {
	AvailableCommands []AvailableCommand `json:"availableCommands"`
}

// AvailableCommand is one slash command.
type AvailableCommand struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Input       *AvailableCommandInput `json:"input,omitempty"`
}

// AvailableCommandInput hints at the free text a command takes.
type AvailableCommandInput struct {
	Hint string `json:"hint"`
}

// CurrentModeUpdate reports a legacy mode change.
type CurrentModeUpdate struct {
	CurrentModeID string `json:"currentModeId"`
}

// ConfigOptionUpdate replaces the complete config option set.
type ConfigOptionUpdate struct {
	ConfigOptions []ConfigOption `json:"configOptions"`
}

// SessionInfoUpdate patches session metadata.
type SessionInfoUpdate struct {
	Title     StringPatch `json:"title,omitzero"`
	UpdatedAt StringPatch `json:"updatedAt,omitzero"`
}

// StringPatch is a patched nullable string: unchanged when !Set, cleared
// when Set with a nil Value.
type StringPatch struct {
	Set   bool
	Value *string
}

// MarshalJSON writes the value or null; omitzero drops an unset patch.
func (p StringPatch) MarshalJSON() ([]byte, error) { return json.Marshal(p.Value) }

// UnmarshalJSON runs only for present keys, including null.
func (p *StringPatch) UnmarshalJSON(b []byte) error {
	p.Set = true
	p.Value = nil
	return json.Unmarshal(b, &p.Value)
}

// UsageUpdate reports context window use and, optionally, cost.
type UsageUpdate struct {
	Used int64 `json:"used"`
	Size int64 `json:"size"`
	Cost *Cost `json:"cost,omitempty"`
}

// Cost is a cumulative session cost.
type Cost struct {
	Amount   float64 `json:"amount"`
	Currency string  `json:"currency"`
}

// Notice severities.
const (
	NoticeInfo    = "info"
	NoticeWarning = "warning"
	NoticeError   = "error"
)

// Notice is a user-facing message outside the session history.
type Notice struct {
	Severity    string `json:"severity"`
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
}

// Compaction statuses.
const (
	CompactionInProgress = "in_progress"
	CompactionCompleted  = "completed"
	CompactionFailed     = "failed"
	CompactionCancelled  = "cancelled"
)

// CompactionUpdate creates or patches a context compaction. A nil Summary
// is unchanged; a non-nil one replaces it.
type CompactionUpdate struct {
	CompactionID string         `json:"compactionId"`
	Status       string         `json:"status"`
	Summary      []ContentBlock `json:"summary,omitzero"`
	Error        *string        `json:"error,omitempty"`
}

// CompactionSummaryChunk appends to a compaction's summary.
type CompactionSummaryChunk struct {
	CompactionID string       `json:"compactionId"`
	Content      ContentBlock `json:"content"`
}

// UnknownUpdate is an update kind this package does not decode.
type UnknownUpdate struct {
	Kind string
	Raw  json.RawMessage
}

// UpdateKind implementations.
func (*UserMessageChunk) UpdateKind() string        { return KindUserMessageChunk }
func (*AgentMessageChunk) UpdateKind() string       { return KindAgentMessageChunk }
func (*AgentThoughtChunk) UpdateKind() string       { return KindAgentThoughtChunk }
func (*ToolCall) UpdateKind() string                { return KindToolCall }
func (*ToolCallUpdate) UpdateKind() string          { return KindToolCallUpdate }
func (*Plan) UpdateKind() string                    { return KindPlan }
func (*AvailableCommandsUpdate) UpdateKind() string { return KindAvailableCommandsUpdate }
func (*CurrentModeUpdate) UpdateKind() string       { return KindCurrentModeUpdate }
func (*ConfigOptionUpdate) UpdateKind() string      { return KindConfigOptionUpdate }
func (*SessionInfoUpdate) UpdateKind() string       { return KindSessionInfoUpdate }
func (*UsageUpdate) UpdateKind() string             { return KindUsageUpdate }
func (*Notice) UpdateKind() string                  { return KindNotice }
func (*CompactionUpdate) UpdateKind() string        { return KindCompactionUpdate }
func (*CompactionSummaryChunk) UpdateKind() string  { return KindCompactionSummaryChunk }
func (u *UnknownUpdate) UpdateKind() string         { return u.Kind }

// newUpdate returns an empty value for a known kind, or nil.
func newUpdate(kind string) Update {
	switch kind {
	case KindUserMessageChunk:
		return &UserMessageChunk{}
	case KindAgentMessageChunk:
		return &AgentMessageChunk{}
	case KindAgentThoughtChunk:
		return &AgentThoughtChunk{}
	case KindToolCall:
		return &ToolCall{}
	case KindToolCallUpdate:
		return &ToolCallUpdate{}
	case KindPlan:
		return &Plan{}
	case KindAvailableCommandsUpdate:
		return &AvailableCommandsUpdate{}
	case KindCurrentModeUpdate:
		return &CurrentModeUpdate{}
	case KindConfigOptionUpdate:
		return &ConfigOptionUpdate{}
	case KindSessionInfoUpdate:
		return &SessionInfoUpdate{}
	case KindUsageUpdate:
		return &UsageUpdate{}
	case KindNotice:
		return &Notice{}
	case KindCompactionUpdate:
		return &CompactionUpdate{}
	case KindCompactionSummaryChunk:
		return &CompactionSummaryChunk{}
	}
	return nil
}

func isKnownUpdateKind(kind string) bool { return newUpdate(kind) != nil }

// SessionNotification is the session/update params.
type SessionNotification struct {
	SessionID string
	Update    Update
}

type sessionNotificationJSON struct {
	SessionID string          `json:"sessionId"`
	Update    json.RawMessage `json:"update"`
}

// MarshalJSON adds the sessionUpdate discriminator to the update.
func (n SessionNotification) MarshalJSON() ([]byte, error) {
	u, err := marshalUpdate(n.Update)
	if err != nil {
		return nil, err
	}
	return json.Marshal(sessionNotificationJSON{SessionID: n.SessionID, Update: u})
}

func marshalUpdate(u Update) (json.RawMessage, error) {
	if u == nil {
		return nil, fmt.Errorf("session update is nil")
	}
	if unknown, ok := u.(*UnknownUpdate); ok {
		return unknown.Raw, nil
	}
	body, err := json.Marshal(u)
	if err != nil {
		return nil, err
	}
	kind, err := json.Marshal(u.UpdateKind())
	if err != nil {
		return nil, err
	}
	out := make([]byte, 0, len(body)+len(kind)+20)
	out = append(out, `{"sessionUpdate":`...)
	out = append(out, kind...)
	if len(body) > 2 {
		out = append(out, ',')
	}
	return append(out, body[1:]...), nil
}

// UnmarshalJSON decodes known kinds into their types and keeps unknown
// kinds as *UnknownUpdate. A known kind with malformed fields is an error.
func (n *SessionNotification) UnmarshalJSON(b []byte) error {
	var in sessionNotificationJSON
	if err := json.Unmarshal(b, &in); err != nil {
		return err
	}
	if in.SessionID == "" || len(in.Update) == 0 {
		return fmt.Errorf("session update without sessionId or update")
	}
	var head struct {
		Kind string `json:"sessionUpdate"`
	}
	if err := json.Unmarshal(in.Update, &head); err != nil {
		return fmt.Errorf("session update: %w", err)
	}
	u := newUpdate(head.Kind)
	if u == nil {
		*n = SessionNotification{SessionID: in.SessionID, Update: &UnknownUpdate{Kind: head.Kind, Raw: in.Update}}
		return nil
	}
	if err := json.Unmarshal(in.Update, u); err != nil {
		return fmt.Errorf("session update %s: %w", head.Kind, err)
	}
	*n = SessionNotification{SessionID: in.SessionID, Update: u}
	return nil
}

// Content block types.
const (
	ContentText         = "text"
	ContentImage        = "image"
	ContentAudio        = "audio"
	ContentResourceLink = "resource_link"
	ContentResource     = "resource"
)

// ContentBlock is message or tool content. Text is set for "text"; URI,
// Name, Title, Description, MimeType and Size for "resource_link". Other
// types (images, audio, embedded resources, future types) are kept whole
// in Raw and written back unchanged.
type ContentBlock struct {
	Type        string
	Text        string
	URI         string
	Name        string
	Title       string
	Description string
	MimeType    string
	Size        *int64
	Raw         json.RawMessage
}

// TextBlock returns a text content block.
func TextBlock(text string) ContentBlock { return ContentBlock{Type: ContentText, Text: text} }

type textBlockJSON struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type resourceLinkJSON struct {
	Type        string `json:"type"`
	URI         string `json:"uri"`
	Name        string `json:"name"`
	Title       string `json:"title,omitempty"`
	Description string `json:"description,omitempty"`
	MimeType    string `json:"mimeType,omitempty"`
	Size        *int64 `json:"size,omitempty"`
}

// MarshalJSON writes the fields of the block's type.
func (c ContentBlock) MarshalJSON() ([]byte, error) {
	switch c.Type {
	case ContentText:
		return json.Marshal(textBlockJSON{Type: c.Type, Text: c.Text})
	case ContentResourceLink:
		return json.Marshal(resourceLinkJSON{Type: c.Type, URI: c.URI, Name: c.Name, Title: c.Title, Description: c.Description, MimeType: c.MimeType, Size: c.Size})
	}
	if len(c.Raw) > 0 {
		return c.Raw, nil
	}
	return json.Marshal(struct {
		Type string `json:"type"`
	}{c.Type})
}

// UnmarshalJSON reads text and resource links into fields and keeps other
// types raw.
func (c *ContentBlock) UnmarshalJSON(b []byte) error {
	var in struct {
		Type        string `json:"type"`
		Text        string `json:"text"`
		URI         string `json:"uri"`
		Name        string `json:"name"`
		Title       string `json:"title"`
		Description string `json:"description"`
		MimeType    string `json:"mimeType"`
		Size        *int64 `json:"size"`
	}
	if err := json.Unmarshal(b, &in); err != nil {
		return err
	}
	switch in.Type {
	case ContentText:
		*c = ContentBlock{Type: in.Type, Text: in.Text}
	case ContentResourceLink:
		*c = ContentBlock{Type: in.Type, URI: in.URI, Name: in.Name, Title: in.Title, Description: in.Description, MimeType: in.MimeType, Size: in.Size}
	default:
		*c = ContentBlock{Type: in.Type, MimeType: in.MimeType, Raw: bytes.Clone(b)}
	}
	return nil
}
