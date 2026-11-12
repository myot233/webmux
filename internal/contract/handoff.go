package contract

import "time"

// Handoff previews are persistent drafts. Their text is separate from the
// current user request and from ordinary CreateSessionRequest.prompt.
type HandoffLocation struct {
	Cwd               string `json:"cwd"`
	RepositoryID      string `json:"repositoryId"`
	WorktreeID        string `json:"worktreeId"`
	NewWorktreeBase   string `json:"newWorktreeBase"`
	NewWorktreeBranch string `json:"newWorktreeBranch"`
}

type HandoffPreviewRequest struct {
	ClientRequestID        string            `json:"clientRequestId"`
	ConfigID               string            `json:"configId"`
	TargetProviderThreadID string            `json:"targetProviderThreadId,omitempty"`
	AgentConfig            map[string]string `json:"agentConfig"`
	UserText               string            `json:"userText"`
	Fresh                  bool              `json:"fresh,omitempty"`
	Fork                   bool              `json:"fork"`
	Location               HandoffLocation   `json:"location"`
	Attachments            []ChatAttachment  `json:"attachments"`
}

type HandoffEditRequest struct {
	Revision int64   `json:"revision"`
	Text     *string `json:"text,omitempty"`
	UserText *string `json:"userText,omitempty"`
}

type HandoffBudget struct {
	Window     int64  `json:"window"`
	NativeUsed int64  `json:"nativeUsed"`
	Current    int64  `json:"current"`
	Reserve    int64  `json:"reserve"`
	History    int64  `json:"history"`
	Used       int64  `json:"used"`
	Source     string `json:"source"`
	OverBudget bool   `json:"overBudget"`
}

type HandoffOmission struct {
	ActivityID string `json:"activityId"`
	Reason     string `json:"reason"`
}

// HandoffSelection uses activity positions, independently of text offsets.
type HandoffSelection struct {
	SourceLocation       HandoffLocation        `json:"sourceLocation"`
	SourceConversationID string                 `json:"sourceConversationId"`
	SourcePosition       string                 `json:"sourcePosition"`
	Revision             int64                  `json:"revision"`
	HistoryOrigin        string                 `json:"historyOrigin"`
	ReconcileRequired    bool                   `json:"reconcileRequired"`
	Manual               bool                   `json:"manual"`
	SelectedCount        int                    `json:"selectedCount"`
	OmittedCount         int                    `json:"omittedCount"`
	Items                []HandoffSelectionItem `json:"items"`
	NextPosition         string                 `json:"nextPosition"`
	Done                 bool                   `json:"done"`
}

type HandoffSelectionItem struct {
	ActivityID string `json:"activityId"`
	Position   string `json:"position"`
	Selected   bool   `json:"selected"`
	Reason     string `json:"reason"`
}

type HandoffPreview struct {
	SourceLocation         HandoffLocation   `json:"sourceLocation"`
	ID                     string            `json:"id"`
	SourceConversationID   string            `json:"sourceConversationId"`
	TargetConversationID   string            `json:"targetConversationId"`
	TargetProviderThreadID string            `json:"targetProviderThreadId"`
	ConfigID               string            `json:"configId"`
	AgentName              string            `json:"agentName"`
	AgentConfig            map[string]string `json:"agentConfig"`
	UserText               string            `json:"userText"`
	Fresh                  bool              `json:"fresh"`
	Fork                   bool              `json:"fork"`
	Location               HandoffLocation   `json:"location"`
	Attachments            []ChatAttachment  `json:"attachments"`
	Strategy               string            `json:"strategy"`
	State                  string            `json:"state"`
	SourcePosition         string            `json:"sourcePosition"`
	SourceRunOrdinal       string            `json:"sourceRunOrdinal"`
	FromRunOrdinal         string            `json:"fromRunOrdinal"`
	ToRunOrdinal           string            `json:"toRunOrdinal"`
	Revision               int64             `json:"revision"`
	HistoryOrigin          string            `json:"historyOrigin"`
	ReconcileRequired      bool              `json:"reconcileRequired"`
	Stale                  bool              `json:"stale"`
	Manual                 bool              `json:"manual"`
	RenderedText           string            `json:"renderedText"`
	SelectedItemIDs        []string          `json:"selectedItemIds"`
	OmittedItems           []HandoffOmission `json:"omittedItems"`
	OmittedItemIDs         []string          `json:"omittedItemIds"`
	SelectedCount          int               `json:"selectedCount"`
	OmittedCount           int               `json:"omittedCount"`
	SelectionTruncated     bool              `json:"selectionTruncated"`
	Budget                 HandoffBudget     `json:"budget"`
	NativeAvailable        bool              `json:"nativeAvailable"`
	HistoryToolAvailable   bool              `json:"historyToolAvailable"`
}

type ConversationMessageRequest struct {
	ClientRequestID string            `json:"clientRequestId"`
	HandoffID       string            `json:"handoffId"`
	Revision        int64             `json:"revision"`
	Text            string            `json:"text"`
	ConfigID        string            `json:"configId"`
	AgentConfig     map[string]string `json:"agentConfig"`
	Location        HandoffLocation   `json:"location"`
	Attachments     []ChatAttachment  `json:"attachments"`
	Retry           bool              `json:"retry"`
}

type HandoffDraft struct {
	Source   ConversationContext `json:"source"`
	Preview  *HandoffPreview     `json:"preview"`
	Delivery *HandoffDelivery    `json:"delivery"`
}

type HandoffDelivery struct {
	ClientRequestID        string       `json:"clientRequestId"`
	Retry                  bool         `json:"retry"`
	Session                *SessionInfo `json:"session"`
	ExecutionID            string       `json:"executionId"`
	DeliveryID             string       `json:"deliveryId"`
	HandoffID              string       `json:"handoffId"`
	SourceConversationID   string       `json:"sourceConversationId"`
	TargetConversationID   string       `json:"targetConversationId"`
	TargetProviderThreadID string       `json:"targetProviderThreadId"`
	SessionID              string       `json:"sessionId"`
	Status                 string       `json:"status"`
	HistoryStatus          string       `json:"historyStatus"`
	PromptStatus           string       `json:"promptStatus"`
	Phase                  string       `json:"phase"`
	Step                   string       `json:"step"`
	ErrorClass             string       `json:"errorClass"`
	WorktreeID             string       `json:"worktreeId"`
	WorktreePath           string       `json:"worktreePath"`
	NativeContinue         bool         `json:"nativeContinue"`
	ReplacementHandoffID   string       `json:"replacementHandoffId"`
	UpdatedAt              time.Time    `json:"updatedAt"`
}
