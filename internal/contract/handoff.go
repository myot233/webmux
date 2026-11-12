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
	Fork                   bool              `json:"fork"`
	Location               HandoffLocation   `json:"location"`
	Attachments            []ChatAttachment  `json:"attachments"`
}

type HandoffEditRequest struct {
	Revision int64  `json:"revision"`
	Text     string `json:"text"`
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

type HandoffPreview struct {
	ID                     string            `json:"id"`
	SourceConversationID   string            `json:"sourceConversationId"`
	TargetConversationID   string            `json:"targetConversationId"`
	TargetProviderThreadID string            `json:"targetProviderThreadId"`
	ConfigID               string            `json:"configId"`
	AgentName              string            `json:"agentName"`
	AgentConfig            map[string]string `json:"agentConfig"`
	UserText               string            `json:"userText"`
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

type HandoffDelivery struct {
	ExecutionID            string    `json:"executionId"`
	DeliveryID             string    `json:"deliveryId"`
	HandoffID              string    `json:"handoffId"`
	SourceConversationID   string    `json:"sourceConversationId"`
	TargetConversationID   string    `json:"targetConversationId"`
	TargetProviderThreadID string    `json:"targetProviderThreadId"`
	SessionID              string    `json:"sessionId"`
	Status                 string    `json:"status"`
	Phase                  string    `json:"phase"`
	Step                   string    `json:"step"`
	ErrorClass             string    `json:"errorClass"`
	WorktreeID             string    `json:"worktreeId"`
	WorktreePath           string    `json:"worktreePath"`
	UpdatedAt              time.Time `json:"updatedAt"`
}
