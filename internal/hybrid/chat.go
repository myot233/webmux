package hybrid

import "time"

// The chat channel's service types. They mirror internal/contract's chat
// shapes field for field (internal/wire converts them); the service never
// encodes them. Values handed out are immutable snapshots: the actor builds
// a new value for every change.

// SideSwitching is the side reported while a switch runs; records never
// store it.
const SideSwitching Side = "switching"

// Phase is the current step of a switch; empty outside a switch.
type Phase string

const (
	PhaseInterrupting   Phase = "interrupting"
	PhaseAwaitingChoice Phase = "awaiting_choice"
	PhaseCancelling     Phase = "cancelling"
	PhaseStopping       Phase = "stopping"
	PhaseStarting       Phase = "starting"
	PhaseLoading        Phase = "loading"
	PhaseRollingBack    Phase = "rolling_back"
)

// Event kinds, identical to the contract's.
const (
	EventMessage    = "message.append"
	EventTool       = "tool.upsert"
	EventPlan       = "plan.set"
	EventCommands   = "commands.set"
	EventConfig     = "config.set"
	EventUsage      = "usage.set"
	EventInfo       = "info.set"
	EventCompaction = "compaction.upsert"
	EventNotice     = "notice"
	EventTurnStart  = "turn.start"
	EventTurnEnd    = "turn.end"
	EventRequest    = "request.open"
	EventResolved   = "request.close"
	EventMarker     = "marker"
)

// Message roles.
const (
	RoleUser    = "user"
	RoleAgent   = "agent"
	RoleThought = "thought"
)

// Turn stop reasons WebMux adds to the agent's.
const (
	StopError        = "error"
	StopAuthRequired = "auth_required"
)

// Request kinds and outcomes.
const (
	RequestPermission = "permission"
	RequestForm       = "form"

	OutcomeSelected  = "selected"
	OutcomeCancelled = "cancelled"
	OutcomeAccept    = "accept"
	OutcomeDecline   = "decline"
	OutcomeCancel    = "cancel"
)

// Marker kinds.
const (
	MarkerLoaded      = "loaded"
	MarkerSwitched    = "switched"
	MarkerTruncated   = "truncated"
	MarkerReconnected = "reconnected"
	MarkerResumed     = "resumed"
)

// Event is one normalized timeline event; Kind selects the set payload.
type Event struct {
	Seq        uint64
	Kind       string
	At         time.Time
	Message    *MessageChunk
	Tool       *Tool
	Plan       *Plan
	Commands   []Command
	Config     []ConfigOption
	Usage      *Usage
	Info       *Info
	Compaction *Compaction
	Notice     *Notice
	TurnStart  *TurnStart
	TurnEnd    *TurnEnd
	Request    *Request
	Resolved   *RequestOutcome
	Marker     *Marker
}

// MessageChunk appends Text to message MessageID.
type MessageChunk struct {
	MessageID string
	Role      string
	Text      string
	Truncated bool
}

// Tool is a field patch for one tool call: nil fields are unchanged, a
// non-nil Content or Locations replaces the previous value.
type Tool struct {
	ToolCallID string
	Kind       *string
	Title      *string
	Status     *string
	Content    []Content
	Locations  []Location
}

// Content is a renderable block: text, diff, resource_link, or unsupported
// with Unsupported naming the ACP type.
type Content struct {
	Type        string
	Text        string
	Path        string
	OldText     *string
	NewText     string
	URI         string
	Name        string
	MimeType    string
	Unsupported string
	Truncated   bool
}

// Location is a file position a tool call touches.
type Location struct {
	Path string
	Line *int
}

// Plan is the agent's whole plan.
type Plan struct {
	Entries []PlanEntry
}

// PlanEntry is one plan step; Status is pending, in_progress or completed.
type PlanEntry struct {
	Content  string
	Priority string
	Status   string
}

// Command is a slash command the agent offers.
type Command struct {
	Name        string
	Description string
	Hint        string
}

// ConfigOption is an agent configuration option: a select (Value) or a
// boolean (Enabled).
type ConfigOption struct {
	ID       string
	Name     string
	Category string
	Type     string
	Value    string
	Enabled  *bool
	Options  []ConfigChoice
}

// ConfigChoice is one select value.
type ConfigChoice struct {
	Value       string
	Name        string
	Description string
}

// Usage is the context window use and cost.
type Usage struct {
	Used     int64
	Size     int64
	Cost     *float64
	Currency string
}

// Info patches session metadata.
type Info struct {
	Title *string
}

// Compaction is a context compaction; Status is running, completed or
// failed.
type Compaction struct {
	ID      string
	Status  string
	Summary string
}

// Notice is an agent or WebMux notice; Level is info, warning or error.
type Notice struct {
	Level string
	Text  string
}

// TurnStart opens a turn for prompt PromptID.
type TurnStart struct {
	TurnID   string
	PromptID string
}

// TurnEnd closes a turn. StopReason is an ACP stop reason, error or
// auth_required.
type TurnEnd struct {
	TurnID     string
	StopReason string
	Error      string
}

// Request is a pending permission request or form.
type Request struct {
	RequestID  string
	Kind       string
	ToolCallID string
	Title      string
	Options    []PermissionOption
	Form       *Form
}

// PermissionOption is one permission choice.
type PermissionOption struct {
	OptionID string
	Name     string
	Kind     string
}

// Form is a form-mode elicitation. Supported is false when a field cannot
// be rendered; the client then offers only decline and cancel.
type Form struct {
	Message   string
	Fields    []FormField
	Supported bool
}

// FormField is one field; Type is string, enum, boolean or unsupported.
// Default is the agent's raw JSON default, passed through unchanged.
type FormField struct {
	Name        string
	Title       string
	Description string
	Type        string
	Required    bool
	Enum        []string
	EnumNames   []string
	Default     []byte
}

// RequestOutcome closes a request.
type RequestOutcome struct {
	RequestID string
	Outcome   string
	OptionID  string
}

// Marker is a WebMux timeline marker.
type Marker struct {
	Kind string
	Side Side
}

// ChatState is the replaceable, non-timeline state of the chat channel.
type ChatState struct {
	Side           Side
	Phase          Phase
	Dormant        bool
	Agent          string
	AgentVersion   string
	VerifiedBelow  string
	AgentSessionID *string
	Eligibility    Eligibility
	Turn           Turn
	Requests       []Request
	Config         []ConfigOption
	Commands       []Command
	Plan           *Plan
	Usage          *Usage
	Queue          Queue
	Steering       bool
	AuthRequired   bool
	SwitchError    string
	Unsupported    []string
	IDChange       *IDChange
	// QueueLost is how many queued prompts the interruption a session was
	// resumed from lost; the queue lives only in memory.
	QueueLost int
	Recovery  *RecoveryTip
}

// Eligibility says whether switching to chat is possible from the terminal.
type Eligibility struct {
	Available bool
	Reason    string
}

// Turn is the current or last turn.
type Turn struct {
	Running    bool
	TurnID     string
	StopReason string
}

// IDChange reports that the terminal moved to another agent session.
type IDChange struct {
	From string
	To   string
}

// RecoveryTip tells an interrupted or closed session's page its state.
type RecoveryTip struct {
	State State
}

// Queue is the shared prompt queue.
type Queue struct {
	Paused bool
	Items  []QueueItem
}

// QueueItem is one prompt waiting to be sent.
type QueueItem struct {
	ID        string
	Text      string
	CreatedAt time.Time
}

// Answer answers a pending request: permissions use Outcome selected with
// OptionID, or cancelled; forms use accept with Content, decline or cancel.
// Content values are strings, numbers, booleans or string slices.
type Answer struct {
	Outcome  string
	OptionID string
	Content  map[string]any
}
