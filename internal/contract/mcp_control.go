package contract

import "encoding/json"

// MCP control payloads retain the REST field types while declaring the tool wire shapes.
type MCPSessionLaunchResult struct {
	ClientRequestID string                  `json:"clientRequestId"`
	Session         SessionInfo             `json:"session"`
	Worktree        *Worktree               `json:"worktree"`
	ConfigWarnings  []string                `json:"configWarnings"`
	Link            *string                 `json:"link"`
	InitialInput    *SessionInputResult     `json:"initialInput"`
	Wait            *MCPSessionWaitSelector `json:"wait,omitempty"`
}

// MCPSessionWaitSelector is present only for a submitted initial input, with
// exactly one of TurnID and QueueItemID populated.
type MCPSessionWaitSelector struct {
	SessionID   string `json:"sessionId"`
	TurnID      string `json:"turnId,omitempty"`
	QueueItemID string `json:"queueItemId,omitempty"`
}

type MCPSessionSendRequest struct {
	SessionID string              `json:"sessionId"`
	Request   SessionInputRequest `json:"request"`
}

type MCPSessionSendResult struct {
	ClientRequestID string             `json:"clientRequestId"`
	SessionID       string             `json:"sessionId"`
	Receipt         SessionInputResult `json:"receipt"`
}

type MCPSessionInterruptRequest struct {
	SessionID       string `json:"sessionId"`
	ClientRequestID string `json:"clientRequestId"`
}

type MCPSessionInterruptResult struct {
	SessionID       string `json:"sessionId"`
	ClientRequestID string `json:"clientRequestId"`
	Interrupted     bool   `json:"interrupted"`
}

type MCPControlCapabilitiesRequest struct{}

type MCPControlLaunchConfig struct {
	ID      string      `json:"id"`
	Name    string      `json:"name"`
	Chat    bool        `json:"chat"`
	Support ChatSupport `json:"chatSupport"`
}

type MCPControlCapabilitiesResult struct {
	Tools         []string                 `json:"tools"`
	LaunchConfigs []MCPControlLaunchConfig `json:"launchConfigs"`
	Views         []string                 `json:"views"`
	Instructions  string                   `json:"instructions"`
	Transport     string                   `json:"transport"`
	Locality      string                   `json:"locality"`
	SelfSessionID *string                  `json:"selfSessionId"`
}

type MCPSessionListRequest struct {
	State        string        `json:"state,omitempty"`
	ParentID     string        `json:"parentId,omitempty"`
	RepositoryID string        `json:"repositoryId,omitempty"`
	Status       SessionStatus `json:"status,omitempty"`
	Offset       int           `json:"offset,omitempty"`
	Limit        int           `json:"limit,omitempty"`
}

type MCPSessionListResult struct {
	Sessions   []SessionInfo `json:"sessions"`
	NextOffset int           `json:"nextOffset"`
	Done       bool          `json:"done"`
	Total      int           `json:"total"`
}

type MCPSessionGetRequest struct {
	SessionID string `json:"sessionId"`
}

type MCPSessionReadRequest struct {
	SessionID       string `json:"sessionId"`
	View            string `json:"view,omitempty"`
	AfterPosition   string `json:"afterPosition,omitempty"`
	ItemID          string `json:"itemId,omitempty"`
	TextOffset      string `json:"textOffset,omitempty"`
	Limit           int    `json:"limit,omitempty"`
	MaxBytesPerItem int    `json:"maxBytesPerItem,omitempty"`
	Generation      string `json:"generation,omitempty"`
	Offset          string `json:"offset,omitempty"`
	MaxBytes        int    `json:"maxBytes,omitempty"`
}

type MCPActivityItemResult struct {
	View         string               `json:"view"`
	Item         ConversationActivity `json:"item"`
	Done         bool                 `json:"done"`
	Instructions string               `json:"instructions"`
}

type MCPActivityPageResult struct {
	View              string                 `json:"view"`
	ConversationID    string                 `json:"conversationId"`
	Items             []ConversationActivity `json:"items"`
	NextPosition      string                 `json:"nextPosition"`
	Done              bool                   `json:"done"`
	Gaps              []ConversationGap      `json:"gaps"`
	HistoryOrigin     string                 `json:"historyOrigin"`
	ReconcileRequired bool                   `json:"reconcileRequired"`
	Instructions      string                 `json:"instructions"`
}

type MCPTerminalPageResult struct {
	View string `json:"view"`
	TerminalOutput
	Instructions string `json:"instructions"`
}

type MCPSessionWaitRequest struct {
	SessionID   string `json:"sessionId"`
	Until       string `json:"until,omitempty"`
	TurnID      string `json:"turnId,omitempty"`
	QueueItemID string `json:"queueItemId,omitempty"`
	FirstTurn   bool   `json:"firstTurn,omitempty"`
	TimeoutMS   int    `json:"timeoutMs,omitempty"`
}

type MCPSessionWaitResult struct {
	SessionID      string        `json:"sessionId"`
	TurnID         string        `json:"turnId,omitempty"`
	QueueItemID    string        `json:"queueItemId,omitempty"`
	Status         string        `json:"status"`
	StopReason     string        `json:"stopReason,omitempty"`
	Error          string        `json:"error,omitempty"`
	Reply          string        `json:"reply"`
	ReplyTruncated bool          `json:"replyTruncated"`
	Unfinished     bool          `json:"unfinished"`
	WaitTimedOut   bool          `json:"waitTimedOut"`
	Requests       []ChatRequest `json:"requests"`
	ExitCode       *int          `json:"exitCode,omitempty"`
}

// MCPGuideReadRequest retains the raw topic to distinguish omission from null
// and reject non-string values before looking up a guide topic.
type MCPGuideReadRequest struct {
	Topic json.RawMessage `json:"topic"`
}
