package conversation

// PreviewRequest freezes source, target and config privately in SQLite.
// UserText is an independently editable draft until an execution seals its revision.
// It excludes launch argv/env, attachment bodies and provider private state.
type PreviewRequest struct {
	ConfigID, AgentName, ConfigFingerprint, TargetProviderThreadID string
	AgentConfig                                                    map[string]string
	TargetConfigBaseline                                           map[string]string
	UserText                                                       string
	Fork, ForceFresh                                               bool
	NativePlanned                                                  bool
	Location, SourceLocation                                       PreviewLocation
	Attachments                                                    []PreviewAttachment
}

type PreviewLocation struct{ Cwd, RepositoryID, WorktreeID, NewWorktreeBase, NewWorktreeBranch string }
type PreviewAttachment struct {
	Name, MimeType, Reference, Kind string
	Size                            int64
}
type PreviewBudget struct {
	Window, NativeUsed, Current, Reserve, History, Used int64
	Source                                              string
	OverBudget                                          bool
}

type Preview struct {
	ID, SourceConversationID, TargetConversationID, TargetProviderThreadID                       string
	Strategy, State, HistoryOrigin                                                               string
	SourcePosition, SourceRunOrdinal, FromRunOrdinal, ToRunOrdinal, Revision, TargetSeenPosition int64
	HistoryToolAvailable                                                                         bool
	ReconcileRequired, Stale                                                                     bool
	Request                                                                                      PreviewRequest
	Budget                                                                                       PreviewBudget
	RenderedText                                                                                 string
	ManualText                                                                                   *string
	SelectedItemIDs, OmittedItemIDs                                                              []string
}

type PreviewReservation struct {
	SourceConversationID, ClientRequestID, RequestHash string
	Request                                            PreviewRequest
}

type PreviewResult struct {
	RenderedText                    string
	Budget                          PreviewBudget
	SelectedItemIDs, OmittedItemIDs []string
}

const previewSchema = `
CREATE TABLE handoff_preview (
 handoff_id TEXT PRIMARY KEY REFERENCES context_handoff(id) ON DELETE CASCADE,
 source_position INTEGER NOT NULL, source_run_ordinal INTEGER NOT NULL,
 history_origin TEXT NOT NULL, reconcile_required INTEGER NOT NULL,
 request BLOB NOT NULL, budget BLOB NOT NULL,
 rendered_text TEXT NOT NULL DEFAULT '', manual_text TEXT,
 state TEXT NOT NULL DEFAULT 'preparing'
);
`
