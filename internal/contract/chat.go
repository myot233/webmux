package contract

import (
	"encoding/json"
	"time"
)

// The chat channel of a hybrid session: GET /ws/s/{id}/chat carries text
// JSON frames (ChatFrame) and /api/sessions/{id}/chat/... carries commands.
// Events are WebMux's normalized model, not ACP: they upsert by messageId
// and toolCallId so another agent protocol only needs a new translator.

// Chat frame types.
const (
	ChatFrameSnapshot = "snapshot"
	ChatFrameEvents   = "events"
	ChatFrameState    = "state"
)

// ChatFrame is one server-to-client frame. A connection receives one
// snapshot, then events (the log from the resume point) and afterwards any
// mix of events and state. Exactly the fields of Type are set.
type ChatFrame struct {
	Type string `json:"type"`
	// snapshot: Reset tells the client to discard its timeline because the
	// requested generation or sequence is no longer in the log. From is the
	// oldest sequence still cached; Truncated reports that older events
	// were dropped by the byte limit.
	Generation string     `json:"generation,omitempty"`
	From       uint64     `json:"from,omitempty"`
	Next       uint64     `json:"next,omitempty"`
	Reset      bool       `json:"reset,omitempty"`
	Truncated  bool       `json:"truncated,omitempty"`
	State      *ChatState `json:"state,omitempty"`
	// events
	Items []ChatEvent `json:"items,omitempty"`
}

// ChatState is the replaceable, non-timeline state of the chat channel.
// Every state frame carries the whole object.
type ChatState struct {
	Side           HybridSide      `json:"side"`
	Phase          HybridPhase     `json:"phase,omitempty"`
	Dormant        bool            `json:"dormant"`
	Agent          string          `json:"agent"`
	AgentVersion   string          `json:"agentVersion,omitempty"`
	VerifiedBelow  string          `json:"verifiedBelow,omitempty"`
	AgentSessionID *string         `json:"agentSessionId"`
	Eligibility    ChatEligibility `json:"eligibility"`
	Turn           ChatTurn        `json:"turn"`
	Requests       []ChatRequest   `json:"requests"`
	Config         []ChatConfig    `json:"config"`
	Commands       []ChatCommand   `json:"commands"`
	Plan           *ChatPlan       `json:"plan"`
	Usage          *ChatUsage      `json:"usage"`
	Queue          ChatQueue       `json:"queue"`
	Steering       bool            `json:"steering"`
	AuthRequired   bool            `json:"authRequired"`
	// PromptCapabilities says which non-text prompt blocks the agent
	// accepts; false means the client must send attachments as paths.
	PromptCapabilities PromptCapabilities `json:"promptCapabilities"`
	PermissionMode     ChatPermissionMode `json:"permissionMode"`
	SwitchError        string             `json:"switchError,omitempty"`
	Unsupported        []string           `json:"unsupported"`
	IDChange           *ChatIDChange      `json:"idChange"`
	QueueLost          int                `json:"queueLost,omitempty"`
	Recovery           *ChatRecoveryTip   `json:"recovery"`
}

// ChatEligibility says whether switching to chat is possible from the
// terminal side. Reason is a stable code when Available is false:
// unknown_agent, not_root, no_session_id, not_listed, occupied, checking.
type ChatEligibility struct {
	Available bool   `json:"available"`
	Reason    string `json:"reason,omitempty"`
}

// PromptCapabilities is what the agent accepts in a prompt besides text.
type PromptCapabilities struct {
	Image           bool `json:"image"`
	EmbeddedContext bool `json:"embeddedContext"`
}

// Attachment kinds.
const (
	AttachmentImage = "image"
	AttachmentFile  = "file"
)

// ChatAttachment is a file uploaded for a chat prompt. The server derives
// Name, MimeType, Size and Kind from the stored file; only Path is trusted
// from a request.
type ChatAttachment struct {
	Path     string `json:"path"`
	Name     string `json:"name,omitempty"`
	MimeType string `json:"mimeType,omitempty"`
	Size     int64  `json:"size,omitempty"`
	Kind     string `json:"kind,omitempty"`
}

// ChatTurn is the current or last turn.
type ChatTurn struct {
	Running    bool   `json:"running"`
	TurnID     string `json:"turnId,omitempty"`
	StopReason string `json:"stopReason,omitempty"`
}

// ChatIDChange reports that the terminal switched to another agent session
// (for example /new or /resume) since the session was created.
type ChatIDChange struct {
	From string `json:"from"`
	To   string `json:"to"`
}

// ChatRecoveryTip tells an interrupted session's page what can be resumed.
type ChatRecoveryTip struct {
	State HybridState `json:"state"`
}

// ChatQueue is the server-side prompt queue shared by every device.
type ChatQueue struct {
	Paused bool            `json:"paused"`
	Items  []ChatQueueItem `json:"items"`
}

// ChatQueueItem is one prompt waiting to be sent.
type ChatQueueItem struct {
	ID          string           `json:"id"`
	Text        string           `json:"text"`
	CreatedAt   time.Time        `json:"createdAt"`
	Attachments []ChatAttachment `json:"attachments,omitempty"`
}

// Chat event kinds.
const (
	ChatEventMessage    = "message.append"
	ChatEventTool       = "tool.upsert"
	ChatEventPlan       = "plan.set"
	ChatEventCommands   = "commands.set"
	ChatEventConfig     = "config.set"
	ChatEventUsage      = "usage.set"
	ChatEventInfo       = "info.set"
	ChatEventCompaction = "compaction.upsert"
	ChatEventNotice     = "notice"
	ChatEventTurnStart  = "turn.start"
	ChatEventTurnEnd    = "turn.end"
	ChatEventRequest    = "request.open"
	ChatEventResolved   = "request.close"
	ChatEventMarker     = "marker"
)

// ChatEvent is one normalized timeline event. Kind selects the one payload
// field that is set.
type ChatEvent struct {
	Seq        uint64              `json:"seq"`
	Kind       string              `json:"kind"`
	At         time.Time           `json:"at"`
	Message    *ChatMessageChunk   `json:"message,omitempty"`
	Tool       *ChatTool           `json:"tool,omitempty"`
	Plan       *ChatPlan           `json:"plan,omitempty"`
	Commands   []ChatCommand       `json:"commands,omitempty"`
	Config     []ChatConfig        `json:"config,omitempty"`
	Usage      *ChatUsage          `json:"usage,omitempty"`
	Info       *ChatInfo           `json:"info,omitempty"`
	Compaction *ChatCompaction     `json:"compaction,omitempty"`
	Notice     *ChatNotice         `json:"notice,omitempty"`
	TurnStart  *ChatTurnStart      `json:"turnStart,omitempty"`
	TurnEnd    *ChatTurnEnd        `json:"turnEnd,omitempty"`
	Request    *ChatRequest        `json:"request,omitempty"`
	Resolved   *ChatRequestOutcome `json:"resolved,omitempty"`
	Marker     *ChatMarker         `json:"marker,omitempty"`
}

// ChatMessageChunk appends Text to the message MessageID. Role is user,
// agent or thought.
type ChatMessageChunk struct {
	MessageID string `json:"messageId"`
	Role      string `json:"role"`
	Text      string `json:"text"`
	Truncated bool   `json:"truncated,omitempty"`
	// Attachments are the files sent with a user prompt.
	Attachments []ChatAttachment `json:"attachments,omitempty"`
}

// ChatTool is a field patch for the tool call ToolCallID: nil fields are
// unchanged, a non-nil Content or Locations replaces the previous value.
// Status is pending, in_progress, completed or failed.
type ChatTool struct {
	ToolCallID string         `json:"toolCallId"`
	Kind       *string        `json:"kind,omitempty"`
	Title      *string        `json:"title,omitempty"`
	Status     *string        `json:"status,omitempty"`
	Content    []ChatContent  `json:"content,omitempty"`
	Locations  []ChatLocation `json:"locations,omitempty"`
}

// ChatContent is a renderable content block. Type is text, diff,
// resource_link or unsupported (with Unsupported naming the ACP type).
type ChatContent struct {
	Type        string  `json:"type"`
	Text        string  `json:"text,omitempty"`
	Path        string  `json:"path,omitempty"`
	OldText     *string `json:"oldText,omitempty"`
	NewText     string  `json:"newText,omitempty"`
	URI         string  `json:"uri,omitempty"`
	Name        string  `json:"name,omitempty"`
	MimeType    string  `json:"mimeType,omitempty"`
	Unsupported string  `json:"unsupported,omitempty"`
	Truncated   bool    `json:"truncated,omitempty"`
}

// ChatLocation is a file position a tool call touches.
type ChatLocation struct {
	Path string `json:"path"`
	Line *int   `json:"line,omitempty"`
}

// ChatPlan replaces the agent's plan.
type ChatPlan struct {
	Entries []ChatPlanEntry `json:"entries"`
}

// ChatPlanEntry is one plan step; Status is pending, in_progress or
// completed.
type ChatPlanEntry struct {
	Content  string `json:"content"`
	Priority string `json:"priority"`
	Status   string `json:"status"`
}

// ChatCommand is a slash command the agent offers.
type ChatCommand struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Hint        string `json:"hint,omitempty"`
}

// ChatConfig is an agent configuration option. Type is select or boolean;
// Value holds the select value and Enabled the boolean value.
type ChatConfig struct {
	ID       string             `json:"id"`
	Name     string             `json:"name"`
	Category string             `json:"category,omitempty"`
	Type     string             `json:"type"`
	Value    string             `json:"value,omitempty"`
	Enabled  *bool              `json:"enabled,omitempty"`
	Options  []ChatConfigChoice `json:"options"`
}

// ChatConfigChoice is one select value.
type ChatConfigChoice struct {
	Value       string `json:"value"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

// ChatUsage is the context window use and cost reported by the agent.
type ChatUsage struct {
	Used     int64    `json:"used"`
	Size     int64    `json:"size"`
	Cost     *float64 `json:"cost,omitempty"`
	Currency string   `json:"currency,omitempty"`
}

// ChatInfo updates session metadata such as the title.
type ChatInfo struct {
	Title *string `json:"title,omitempty"`
}

// ChatCompaction is a context compaction; Status is running, completed or
// failed and Summary is set once known.
type ChatCompaction struct {
	ID      string `json:"id"`
	Status  string `json:"status"`
	Summary string `json:"summary,omitempty"`
}

// ChatNotice is an agent or WebMux notice; Level is info, warning or error.
type ChatNotice struct {
	Level string `json:"level"`
	Text  string `json:"text"`
}

// ChatTurnStart opens a turn for the prompt identified by PromptID.
type ChatTurnStart struct {
	TurnID   string `json:"turnId"`
	PromptID string `json:"promptId"`
}

// ChatTurnEnd closes a turn. StopReason is end_turn, max_tokens,
// max_turn_requests, refusal, cancelled, error or auth_required.
type ChatTurnEnd struct {
	TurnID     string `json:"turnId"`
	StopReason string `json:"stopReason"`
	Error      string `json:"error,omitempty"`
}

// ChatRequest is a pending permission request or form. Kind is permission
// or form.
type ChatRequest struct {
	RequestID  string       `json:"requestId"`
	Kind       string       `json:"kind"`
	ToolCallID string       `json:"toolCallId,omitempty"`
	Title      string       `json:"title,omitempty"`
	Options    []ChatOption `json:"options,omitempty"`
	Form       *ChatForm    `json:"form,omitempty"`
}

// ChatOption is a permission choice; Kind is allow_once, allow_always,
// reject_once or reject_always.
type ChatOption struct {
	OptionID string `json:"optionId"`
	Name     string `json:"name"`
	Kind     string `json:"kind"`
}

// ChatForm is a form-mode elicitation. Supported is false when a field
// type cannot be rendered; the client then offers only decline and cancel.
type ChatForm struct {
	Message   string          `json:"message"`
	Fields    []ChatFormField `json:"fields"`
	Supported bool            `json:"supported"`
}

// ChatFormField is one field; Type is string, enum, boolean or unsupported.
type ChatFormField struct {
	Name        string          `json:"name"`
	Title       string          `json:"title,omitempty"`
	Description string          `json:"description,omitempty"`
	Type        string          `json:"type"`
	Required    bool            `json:"required"`
	Enum        []string        `json:"enum,omitempty"`
	EnumNames   []string        `json:"enumNames,omitempty"`
	Default     json.RawMessage `json:"default,omitempty"`
}

// ChatRequestOutcome closes a request. Outcome is selected or cancelled
// for permissions and accept, decline or cancel for forms.
type ChatRequestOutcome struct {
	RequestID string `json:"requestId"`
	Outcome   string `json:"outcome"`
	OptionID  string `json:"optionId,omitempty"`
}

// ChatMarker is a WebMux timeline marker; Kind is loaded, switched,
// truncated, reconnected or resumed.
type ChatMarker struct {
	Kind string     `json:"kind"`
	Side HybridSide `json:"side,omitempty"`
}

// ChatPromptRequest is the body of POST /api/sessions/{id}/chat/prompts.
// Mode is auto (send now or queue) or steer (inject into the running turn).
type ChatPromptRequest struct {
	ClientRequestID string `json:"clientRequestId"`
	Text            string `json:"text"`
	Mode            string `json:"mode,omitempty"`
	// Attachments are files returned by the upload endpoint. Their other
	// fields are ignored; the server reads the stored file.
	Attachments []ChatAttachment `json:"attachments,omitempty"`
}

// ChatPromptResult reports whether the prompt was sent or queued.
type ChatPromptResult struct {
	Queued bool   `json:"queued"`
	ItemID string `json:"itemId,omitempty"`
}

// ChatQueueRequest is the body of the queue commands under
// /api/sessions/{id}/chat/queue: PATCH {qid} edits Text or moves the item
// up (Move "up"); DELETE {qid}, POST {qid}/send-now, POST resume and
// POST clear carry only the request ID.
type ChatQueueRequest struct {
	ClientRequestID string  `json:"clientRequestId"`
	Text            *string `json:"text,omitempty"`
	Move            string  `json:"move,omitempty"`
}

// ChatCommandRequest is the body of commands without parameters, such as
// POST /api/sessions/{id}/chat/cancel.
type ChatCommandRequest struct {
	ClientRequestID string `json:"clientRequestId"`
}

// ChatAnswerRequest is the body of POST .../chat/requests/{requestId}.
// Permissions set Outcome selected with OptionID, or cancelled. Forms set
// Outcome accept with Content, decline or cancel.
type ChatAnswerRequest struct {
	ClientRequestID string                     `json:"clientRequestId"`
	Outcome         string                     `json:"outcome"`
	OptionID        string                     `json:"optionId,omitempty"`
	Content         map[string]json.RawMessage `json:"content,omitempty"`
}

// ChatConfigRequest is the body of PUT .../chat/config/{configId}: Value
// for select options, Enabled for boolean options.
type ChatConfigRequest struct {
	ClientRequestID string  `json:"clientRequestId"`
	Value           *string `json:"value,omitempty"`
	Enabled         *bool   `json:"enabled,omitempty"`
}

// ChatPermissionMode is how the chat side answers the agent's permission
// requests: ask shows each to the user, auto approves it on the server
// (the first allow_once option, else allow_always). Forms always ask.
type ChatPermissionMode string

const (
	ChatPermissionAsk  ChatPermissionMode = "ask"
	ChatPermissionAuto ChatPermissionMode = "auto"
)

// ChatPermissionModeRequest is the body of PUT
// /api/sessions/{id}/chat/permission-mode.
type ChatPermissionModeRequest struct {
	ClientRequestID string             `json:"clientRequestId"`
	Mode            ChatPermissionMode `json:"mode"`
}
