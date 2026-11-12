package contract

import "time"

// SwitchTarget is what a worktree switches to, by Kind: "local" takes
// Branch; "remote" takes Ref, a short remote-tracking name such as
// "origin/fix/b", and creates the local branch Name (default: Ref without
// its remote) tracking it; "new" takes Name and StartPoint (a branch,
// remote-tracking branch or commit) and sets no upstream; "detach" takes
// Commit.
type SwitchTarget struct {
	Kind       string `json:"kind"`
	Branch     string `json:"branch,omitempty"`
	Ref        string `json:"ref,omitempty"`
	Name       string `json:"name,omitempty"`
	StartPoint string `json:"startPoint,omitempty"`
	Commit     string `json:"commit,omitempty"`
}

// SwitchPreviewRequest is the body of POST
// /api/repositories/{id}/worktrees/{worktreeId}/switch-preview.
type SwitchPreviewRequest struct {
	Target SwitchTarget `json:"target"`
}

// Issue is one blocker or warning of a preview. Path, when set, is the
// worktree a blocker refers to, such as the one occupying a branch.
type Issue struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Path    string `json:"path,omitempty"`
}

// SwitchTargetInfo is a switch target resolved to its commit. Branch is the
// branch the worktree ends on, null for a detached target; Upstream is the
// remote-tracking branch a remote target tracks, null otherwise.
type SwitchTargetInfo struct {
	Kind          string  `json:"kind"`
	Branch        *string `json:"branch"`
	Commit        string  `json:"commit"`
	Subject       string  `json:"subject"`
	CreatesBranch bool    `json:"createsBranch"`
	Upstream      *string `json:"upstream"`
}

// SwitchPreview describes switching a worktree's branch. With blockers it
// carries no confirmation: ConfirmationID is "" and ExpiresAt null.
// Sessions are the running sessions in the worktree; they keep running.
type SwitchPreview struct {
	ConfirmationID string           `json:"confirmationId"`
	ExpiresAt      *time.Time       `json:"expiresAt"`
	Worktree       Worktree         `json:"worktree"`
	Target         SwitchTargetInfo `json:"target"`
	Sessions       []SessionInfo    `json:"sessions"`
	Blockers       []Issue          `json:"blockers"`
	Warnings       []Issue          `json:"warnings"`
}

// SwitchRequest confirms a previewed switch; AcknowledgeSessions is
// required when the preview listed sessions. It answers 200 with the
// refreshed Worktree.
type SwitchRequest struct {
	ConfirmationID      string `json:"confirmationId"`
	AcknowledgeSessions bool   `json:"acknowledgeSessions"`
}

// CommitList is the first commits of a range, at most 20, newest first;
// Total counts the whole range.
type CommitList struct {
	Total int      `json:"total"`
	Items []Commit `json:"items"`
}

// FastForwardPreviewRequest is the body of POST
// /api/repositories/{id}/branches/fast-forward-preview.
type FastForwardPreviewRequest struct {
	Branch string `json:"branch"`
}

// FastForwardPreview describes moving Branch from From to its upstream's
// tip To (empty without an existing upstream), from the remote-tracking
// refs at hand. Worktree checks the branch out, null for none, and
// Sessions are its running sessions. With blockers ConfirmationID is ""
// and ExpiresAt null.
type FastForwardPreview struct {
	ConfirmationID string        `json:"confirmationId"`
	ExpiresAt      *time.Time    `json:"expiresAt"`
	Branch         string        `json:"branch"`
	From           string        `json:"from"`
	To             string        `json:"to"`
	Upstream       string        `json:"upstream"`
	Commits        CommitList    `json:"commits"`
	Worktree       *Worktree     `json:"worktree"`
	Sessions       []SessionInfo `json:"sessions"`
	Blockers       []Issue       `json:"blockers"`
	Warnings       []Issue       `json:"warnings"`
}

// FastForwardRequest confirms a previewed fast-forward; AcknowledgeSessions
// is required when the preview listed sessions.
type FastForwardRequest struct {
	ConfirmationID      string `json:"confirmationId"`
	AcknowledgeSessions bool   `json:"acknowledgeSessions"`
}

// FastForwardResult answers a fast-forward with the refreshed repository.
type FastForwardResult struct {
	Repository Repository `json:"repository"`
}

// PushPreviewRequest is the body of POST
// /api/repositories/{id}/branches/push-preview. Remote and Target apply
// only to publishing a branch without an existing upstream.
type PushPreviewRequest struct {
	Branch string `json:"branch"`
	Remote string `json:"remote,omitempty"`
	Target string `json:"target,omitempty"`
}

// PushPreview describes pushing Commit to branch Target of Remote, whose
// push URL is redacted. Publish marks a branch without an existing
// upstream, which gets Target as its upstream. With blockers
// ConfirmationID is "" and ExpiresAt null.
type PushPreview struct {
	ConfirmationID string     `json:"confirmationId"`
	ExpiresAt      *time.Time `json:"expiresAt"`
	Branch         string     `json:"branch"`
	Commit         string     `json:"commit"`
	Remote         string     `json:"remote"`
	PushURL        string     `json:"pushUrl"`
	Target         string     `json:"target"`
	Publish        bool       `json:"publish"`
	Commits        CommitList `json:"commits"`
	Blockers       []Issue    `json:"blockers"`
	Warnings       []Issue    `json:"warnings"`
}

// PushRequest confirms a previewed push.
type PushRequest struct {
	ConfirmationID string `json:"confirmationId"`
}

// PushResult answers a push, also a refused one: Status is "ok", "auth",
// "network", "timeout", "rejected" or "error", and Message explains a
// failure with credentials redacted.
type PushResult struct {
	Status     string     `json:"status"`
	Message    string     `json:"message"`
	Repository Repository `json:"repository"`
}

// BranchDeletePreviewRequest is the body of POST
// /api/repositories/{id}/branches/delete-preview: 1 to 50 local branches.
type BranchDeletePreviewRequest struct {
	Branches []string `json:"branches"`
}

// BranchDeleteItem previews deleting one branch at Commit. Group is
// "contained" when other refs keep all its commits and "unmerged"
// otherwise, with Unmerged listing the commits only it keeps. A blocked
// item cannot be deleted.
type BranchDeleteItem struct {
	Branch   string     `json:"branch"`
	Commit   string     `json:"commit"`
	Group    string     `json:"group"`
	Unmerged CommitList `json:"unmerged"`
	Blockers []Issue    `json:"blockers"`
}

// BranchDeletePreview previews deleting branches; without a deletable item
// ConfirmationID is "" and ExpiresAt null.
type BranchDeletePreview struct {
	ConfirmationID string             `json:"confirmationId"`
	ExpiresAt      *time.Time         `json:"expiresAt"`
	Items          []BranchDeleteItem `json:"items"`
}

// BranchDeleteRequest confirms a previewed delete; the unmerged group is
// deleted only with AcknowledgeUnmerged.
type BranchDeleteRequest struct {
	ConfirmationID      string `json:"confirmationId"`
	AcknowledgeUnmerged bool   `json:"acknowledgeUnmerged"`
}

// BranchDeleteItemResult is one branch's outcome: "deleted", "skipped" or
// "failed". A deleted branch's Message gives the command restoring it.
type BranchDeleteItemResult struct {
	Branch  string `json:"branch"`
	Status  string `json:"status"`
	Commit  string `json:"commit"`
	Message string `json:"message"`
}

// BranchDeleteResult answers a delete with each branch's outcome and the
// refreshed repository.
type BranchDeleteResult struct {
	Results    []BranchDeleteItemResult `json:"results"`
	Repository Repository               `json:"repository"`
}
