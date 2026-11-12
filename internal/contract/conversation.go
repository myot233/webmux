package contract

import "time"

// Durable positions and byte offsets are decimal strings so clients do not
// round int64 cursors through JavaScript floating-point numbers.
type ConversationActivity struct {
	Exportable       bool                     `json:"exportable"`
	ExclusionReason  string                   `json:"exclusionReason"`
	ActivityID       string                   `json:"activityId"`
	ConversationID   string                   `json:"conversationId"`
	ProviderThreadID string                   `json:"providerThreadId"`
	RunID            string                   `json:"runId"`
	Position         string                   `json:"position"`
	RunOrdinal       string                   `json:"runOrdinal"`
	Kind             string                   `json:"kind"`
	Role             string                   `json:"role"`
	Status           string                   `json:"status"`
	Text             string                   `json:"text"`
	Source           string                   `json:"source"`
	ProviderItemID   string                   `json:"providerItemId"`
	ByteLength       string                   `json:"byteLength"`
	NextTextOffset   string                   `json:"nextTextOffset"`
	Truncated        bool                     `json:"truncated"`
	CreatedAt        time.Time                `json:"createdAt"`
	Attachments      []ConversationAttachment `json:"attachments"`
}

type ConversationAttachment struct {
	Name      string `json:"name"`
	MimeType  string `json:"mimeType"`
	Reference string `json:"reference"`
	Size      int64  `json:"size"`
}

type ConversationGap struct {
	ID               string    `json:"id"`
	ProviderThreadID string    `json:"providerThreadId"`
	Reason           string    `json:"reason"`
	CreatedAt        time.Time `json:"createdAt"`
}

type ConversationActivities struct {
	Items             []ConversationActivity `json:"items"`
	NextPosition      string                 `json:"nextPosition"`
	Gaps              []ConversationGap      `json:"gaps"`
	ReconcileRequired bool                   `json:"reconcileRequired"`
	HistoryOrigin     string                 `json:"historyOrigin"`
}

// ConversationContext contains working location metadata only.
type ConversationContext struct {
	ID           string `json:"id"`
	Cwd          string `json:"cwd"`
	RepositoryID string `json:"repositoryId"`
	WorktreeID   string `json:"worktreeId"`
}
