package contract

import "time"

// The REST and WebSocket shapes below mirror web/src/schemas.ts. Field
// names, omitempty and nil-versus-empty behavior are part of the wire
// contract and are pinned by internal/api/testdata/wire.

// SessionStatus is the display state of a session. waiting means the session
// needs the user: an agent's blocked state or unread done/error, a latched
// attention signal, or, without agent signals, a prompt-like last line or a
// process reading the terminal. idle means long silence without those signs,
// not completion.
type SessionStatus string

const (
	SessionStatusRunning SessionStatus = "running"
	SessionStatusWaiting SessionStatus = "waiting"
	SessionStatusIdle    SessionStatus = "idle"
	SessionStatusExited  SessionStatus = "exited"
)

// AttentionKind names the signal that a session needs the user.
type AttentionKind string

const (
	AttentionKindQuestion   AttentionKind = "question"
	AttentionKindPermission AttentionKind = "permission"
	AttentionKindDone       AttentionKind = "done"
	AttentionKindError      AttentionKind = "error"
	AttentionKindNotify     AttentionKind = "notify"
	AttentionKindBell       AttentionKind = "bell"
)

// Valid reports whether k is one of the named kinds.
func (k AttentionKind) Valid() bool {
	switch k {
	case AttentionKindQuestion, AttentionKindPermission, AttentionKindError, AttentionKindDone, AttentionKindNotify, AttentionKindBell:
		return true
	}
	return false
}

// AttentionSource identifies the component that raised an attention signal.
type AttentionSource string

const (
	AttentionSourceAgent    AttentionSource = "agent"
	AttentionSourceTerminal AttentionSource = "terminal"
	// AttentionSourceCLI is attention raised through POST
	// /api/sessions/{id}/attention, normally by webmux notify.
	AttentionSourceCLI AttentionSource = "cli"
)

// SessionInfo is one session in GET /api/sessions, POST /api/sessions,
// /ws/sessions events and the 0x02 terminal frame.
type SessionInfo struct {
	ConversationID   string              `json:"conversationId,omitempty"`
	ProviderThreadID string              `json:"providerThreadId,omitempty"`
	ID               string              `json:"id"`
	Generation       string              `json:"generation"`
	Title            string              `json:"title"`
	Cwd              string              `json:"cwd"`
	Cmd              []string            `json:"cmd"`
	Status           SessionStatus       `json:"status"`
	ExitCode         *int                `json:"exitCode"`
	Preview          string              `json:"preview"`
	CreatedAt        time.Time           `json:"createdAt"`
	LastActiveAt     time.Time           `json:"lastActiveAt"`
	Cols             uint16              `json:"cols"`
	Rows             uint16              `json:"rows"`
	Clients          int                 `json:"clients"`
	Workspace        *SessionWorkspace   `json:"workspace"`
	Attention        *SessionAttention   `json:"attention"`
	ReportedCwd      *SessionReportedCwd `json:"reportedCwd"`
	// ConfigID is the saved launch config used at creation, if any. It is
	// omitted for sessions started from an explicit command.
	ConfigID string `json:"configId,omitempty"`
	// State is active for every running session; closed and interrupted
	// sessions are archived, with zero runtime fields.
	State SessionState `json:"state"`
	// Views are the views the session supports: ["terminal"], or
	// ["terminal","chat"] for a session with a chat view.
	Views []SessionView `json:"views"`
	// View is the last settled view; during a transition it is still the
	// view the session comes from.
	View SessionView `json:"view"`
	// Transition is the view change in progress, null otherwise.
	Transition *SessionTransition `json:"transition"`
	// Agent is the agent behind the chat view, null without one.
	Agent *SessionAgent `json:"agent"`
	// Archive is set only on a closed or interrupted session.
	Archive *SessionArchive `json:"archive,omitempty"`
	// Released is set when the session's linked worktree directory was
	// released while archived.
	Released *HybridRelease `json:"released,omitempty"`
	// ListeningPorts are the loopback or wildcard TCP ports the session's
	// process tree listens on, ascending; empty where detection is
	// unsupported.
	ListeningPorts []ListeningPort `json:"listeningPorts"`
	// ParentID is the session whose process created this one through the
	// local API; sessions created elsewhere omit it.
	ParentID string `json:"parentId,omitempty"`
	// Labels are caller-defined key-value pairs; empty is omitted.
	Labels map[string]string `json:"labels,omitempty"`
}

// SessionWorkspace is the registered worktree a session was started in.
type SessionWorkspace struct {
	RepositoryID string `json:"repositoryId"`
	WorktreeID   string `json:"worktreeId"`
	WorktreePath string `json:"worktreePath"`
}

// SessionAttention is a latched signal that the session needs the user.
type SessionAttention struct {
	Kind   AttentionKind   `json:"kind"`
	Source AttentionSource `json:"source"`
	Since  time.Time       `json:"since"`
	// Message is the text a cli source attached; other sources omit it.
	Message string `json:"message,omitempty"`
}

// AttentionRequest is the body of POST /api/sessions/{id}/attention.
type AttentionRequest struct {
	Kind    AttentionKind `json:"kind"`
	Message string        `json:"message,omitempty"`
}

// SessionReportedCwd is the directory a program reported through OSC 7.
type SessionReportedCwd struct {
	Path   string    `json:"path"`
	Host   string    `json:"host"`
	Remote bool      `json:"remote"`
	Since  time.Time `json:"since"`
}

// WorkspaceReference selects a registered worktree.
type WorkspaceReference struct {
	RepositoryID string `json:"repositoryId"`
	WorktreeID   string `json:"worktreeId"`
}

// CreateSessionRequest is the body of POST /api/sessions, the only way to
// create a session. The location is one of Cwd, Workspace and NewWorktree;
// the command is ConfigID or Cmd with Env. View (terminal or chat, default
// terminal) chat needs a config with the chat view; AgentConfig and
// PermissionMode are only valid with View chat, and Attachments need a
// Prompt. UseInitInstruction needs Workspace or NewWorktree. omitempty only
// shapes what Go clients send; the server treats an absent field like its
// zero value.
type CreateSessionRequest struct {
	// ClientRequestID dedupes retries for ten minutes; empty never dedupes.
	ClientRequestID string              `json:"clientRequestId,omitempty"`
	Cwd             string              `json:"cwd,omitempty"`
	Workspace       *WorkspaceReference `json:"workspace,omitempty"`
	NewWorktree     *NewWorktree        `json:"newWorktree,omitempty"`
	Cmd             []string            `json:"cmd,omitempty"`
	Env             map[string]string   `json:"env,omitempty"`
	ConfigID        *string             `json:"configId,omitempty"`
	Name            string              `json:"name,omitempty"`
	// Labels are set on the new session; invalid labels reject the request
	// before anything starts.
	Labels map[string]string `json:"labels,omitempty"`
	View   SessionView       `json:"view,omitempty"`
	// Prompt is the user's first message, sent once the session is up: as
	// the first chat prompt on the chat view, otherwise typed into the
	// terminal with Enter. UseInitInstruction puts the repository's
	// initialization instruction before it. ExpandPrompt replaces the
	// prompt's {{dir}} (the session's artifacts folder) and {{webmux}} (its
	// WebMux brief) once the session exists; without it the prompt is sent
	// as written.
	Prompt             string `json:"prompt,omitempty"`
	UseInitInstruction bool   `json:"useInitInstruction,omitempty"`
	ExpandPrompt       bool   `json:"expandPrompt,omitempty"`
	// Attachments are files returned by POST /api/uploads; only their path
	// is read. They go with the first prompt.
	Attachments []ChatAttachment `json:"attachments,omitempty"`
	// AgentConfig maps agent option IDs to values: the select value, or
	// "true"/"false" for a boolean.
	AgentConfig map[string]string `json:"agentConfig,omitempty"`
	// PermissionMode is ask, auto or empty for the default.
	PermissionMode ChatPermissionMode `json:"permissionMode,omitempty"`
	// Cols and Rows are the PTY's initial size (the terminal side of a chat
	// session too). Both absent or 0 is 80x24; otherwise both are required,
	// each 1..1000. The first browser to control the session resizes it to
	// its own window.
	Cols int `json:"cols,omitempty"`
	Rows int `json:"rows,omitempty"`
}

// NewWorktree asks POST /api/sessions to create a worktree first. Mode is
// new (a new Branch from Base; an empty Branch is generated together with
// the path) or existing (check out the existing Branch). An empty Path
// derives the directory from the branch.
type NewWorktree struct {
	RepositoryID string `json:"repositoryId"`
	Mode         string `json:"mode"`
	Branch       string `json:"branch,omitempty"`
	Base         string `json:"base,omitempty"`
	Path         string `json:"path,omitempty"`
}

// CreateSessionResult is the 201 body of POST /api/sessions. Worktree is
// set only when the request created one; ConfigWarnings name each
// AgentConfig item the agent did not take.
type CreateSessionResult struct {
	Session        SessionInfo `json:"session"`
	Worktree       *Worktree   `json:"worktree,omitempty"`
	ConfigWarnings []string    `json:"configWarnings"`
}

// Create steps, as CreateSessionFailure.Step.
const (
	CreateStepPrecheck = "precheck"
	CreateStepWorktree = "worktree"
	CreateStepSession  = "session"
	CreateStepConfig   = "config"
	CreateStepPrompt   = "prompt"
)

// CreateSessionFailure is the body of every failed POST /api/sessions. Step
// is precheck (nothing was created), worktree, session, config or prompt.
// Reason, Eligibility and Connector are those of a SessionConflict; Reason
// is conflict for a branch or path that already exists. Worktree and
// SessionID name what had been created; nothing is rolled back.
type CreateSessionFailure struct {
	Error       string          `json:"error"`
	Step        string          `json:"step"`
	Reason      string          `json:"reason,omitempty"`
	Eligibility string          `json:"eligibility,omitempty"`
	Connector   *AgentConnector `json:"connector,omitempty"`
	Worktree    *Worktree       `json:"worktree,omitempty"`
	SessionID   string          `json:"sessionId,omitempty"`
}

// InputKeys are the named keys of SessionInputRequest.Keys and the bytes
// the terminal view writes for them.
var InputKeys = map[string]string{
	"backspace": "\x7f", "ctrl-c": "\x03", "ctrl-d": "\x04", "down": "\x1b[B", "enter": "\r",
	"esc": "\x1b", "left": "\x1b[D", "right": "\x1b[C", "tab": "\t", "up": "\x1b[A",
}

// SessionInputRequest is the body of POST /api/sessions/{id}/input. On the
// terminal view Text (with Enter when Submit) is one acknowledged
// submission, then each of Keys follows; Attachments and Mode are the chat
// view's and fail there with wrong_view. On the chat view Text and
// Attachments are a prompt and Mode is auto (send now or queue), steer
// (inject into the running turn), refuse (fail with 409 busy instead of
// queueing) or now (cancel the running turn and send next); Submit is
// ignored and Keys fail with wrong_view.
type SessionInputRequest struct {
	ClientRequestID string           `json:"clientRequestId"`
	Text            string           `json:"text"`
	Submit          bool             `json:"submit,omitempty"`
	Keys            []string         `json:"keys,omitempty"`
	Attachments     []ChatAttachment `json:"attachments,omitempty"`
	Mode            string           `json:"mode,omitempty"`
}

// SessionInputResult is the 200 body of POST /api/sessions/{id}/input. On
// the chat view a sent prompt names its turn (TurnID, PromptID as in
// turn.start) and a queued one its queue item and 1-based Position, which
// the turn that sends it names as queueItemId; steering names neither. The
// terminal view sets View only.
type SessionInputResult struct {
	View     SessionView `json:"view"`
	Queued   *bool       `json:"queued,omitempty"`
	ItemID   string      `json:"itemId,omitempty"`
	Position int         `json:"position,omitempty"`
	TurnID   string      `json:"turnId,omitempty"`
	PromptID string      `json:"promptId,omitempty"`
}

// SessionCommandRequest is the body of commands without parameters, such
// as POST /api/sessions/{id}/interrupt and the chat queue commands.
type SessionCommandRequest struct {
	ClientRequestID string `json:"clientRequestId"`
}

// SessionResizeRequest is the body of POST /api/sessions/{id}/resize, which
// answers 200 with the SessionInfo. Cols and Rows are each 1..1000. A
// terminal controlled by a browser or an attached client refuses with 409
// unless Take is set: the script then takes control for the resize. A
// terminal nobody controls, or that server-side input controls, resizes
// without it.
type SessionResizeRequest struct {
	Cols int  `json:"cols"`
	Rows int  `json:"rows"`
	Take bool `json:"take,omitempty"`
}

// LabelsUpdate is the body of PATCH /api/sessions/{id}/labels: Set adds or
// replaces labels, then Remove deletes keys, as one change. It answers 200
// with the SessionInfo.
type LabelsUpdate struct {
	Set    map[string]string `json:"set,omitempty"`
	Remove []string          `json:"remove,omitempty"`
}

// SessionDetails is GET /api/sessions/{id}/details: the session's metadata
// gathered from every service. It never carries the token, launch
// environment values or another session's output.
type SessionDetails struct {
	Session             SessionInfo          `json:"session"`
	Link                *string              `json:"link"`
	Repository          *DetailsRepository   `json:"repository"`
	Worktree            *DetailsWorktree     `json:"worktree"`
	LaunchConfig        *DetailsLaunchConfig `json:"launchConfig"`
	LaunchConfigMissing bool                 `json:"launchConfigMissing,omitempty"`
	// ArtifactsDir is the session's artifacts folder when it exists; the
	// endpoint never creates it.
	ArtifactsDir *string           `json:"artifactsDir"`
	Forwards     []Forward         `json:"forwards"`
	ParentID     string            `json:"parentId,omitempty"`
	Labels       map[string]string `json:"labels"`
	// Children are the sessions whose ParentID is this session, oldest
	// first.
	Children []string `json:"children"`
}

// DetailsRepository names the registered repository of a session.
type DetailsRepository struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Path string `json:"path"`
}

// DetailsWorktree names the registered worktree of a session.
type DetailsWorktree struct {
	ID     string  `json:"id"`
	Branch *string `json:"branch"`
	Path   string  `json:"path"`
	Main   bool    `json:"main"`
}

// DetailsLaunchConfig names the saved launch config a session came from.
type DetailsLaunchConfig struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Icon string `json:"icon,omitempty"`
	Chat bool   `json:"chat"`
}

// RenameSessionRequest is the body of PATCH /api/sessions/{id}. An empty
// name hands the title back to the terminal or agent.
type RenameSessionRequest struct {
	Name string `json:"name"`
}

// AgentGuidePrompt is the 200 body of POST /api/sessions/{id}/agent-guide:
// the brief {{webmux}} expands to for the session, and the facts in it.
// Bin and Socket are null when the server does not know them.
type AgentGuidePrompt struct {
	Prompt   string  `json:"prompt"`
	GuideURL string  `json:"guideUrl"`
	Dir      string  `json:"dir"`
	Bin      *string `json:"bin"`
	Socket   *string `json:"socket"`
}
