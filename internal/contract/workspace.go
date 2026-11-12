package contract

import "time"

// WorkspaceCapability reports whether Git and the repository registry work.
type WorkspaceCapability struct {
	GitAvailable      bool   `json:"gitAvailable"`
	RegistryAvailable bool   `json:"registryAvailable"`
	GitError          string `json:"gitError"`
	RegistryError     string `json:"registryError"`
}

// Repository is a registered Git repository and its worktrees.
// InitInstruction prefixes the first prompt of a quick start; "" for none.
// LastFetchedAt is the newest FETCH_HEAD modification time, approximate;
// null when there is none.
type Repository struct {
	ID              string     `json:"id"`
	Name            string     `json:"name"`
	CommonDir       string     `json:"commonDir"`
	AnchorPath      string     `json:"anchorPath"`
	Availability    string     `json:"availability"`
	Error           string     `json:"error"`
	CheckedAt       time.Time  `json:"checkedAt"`
	Worktrees       []Worktree `json:"worktrees"`
	InitInstruction string     `json:"initInstruction"`
	LastFetchedAt   *time.Time `json:"lastFetchedAt"`
}

// Worktree is one worktree of a repository. Upstream is null without an
// upstream or when the status is unreadable; UpstreamGone marks a configured
// upstream whose ref no longer exists. Ahead and Behind are null without an
// upstream, when it is gone, or when Git did not compute them. Changes is
// null when the status is unreadable. Operation is merge, rebase,
// cherry-pick, revert or bisect while one is in progress.
type Worktree struct {
	ID           string           `json:"id"`
	RepositoryID string           `json:"repositoryId"`
	Path         string           `json:"path"`
	Kind         string           `json:"kind"`
	Branch       *string          `json:"branch"`
	Head         *string          `json:"head"`
	Detached     bool             `json:"detached"`
	Locked       bool             `json:"locked"`
	LockReason   string           `json:"lockReason"`
	Prunable     bool             `json:"prunable"`
	Available    bool             `json:"available"`
	Dirty        *bool            `json:"dirty"`
	HasUntracked bool             `json:"hasUntracked"`
	HasIgnored   bool             `json:"hasIgnored"`
	Error        string           `json:"error"`
	Upstream     *string          `json:"upstream"`
	Ahead        *int             `json:"ahead"`
	Behind       *int             `json:"behind"`
	UpstreamGone bool             `json:"upstreamGone"`
	LastCommit   *Commit          `json:"lastCommit"`
	Changes      *WorktreeChanges `json:"changes"`
	Operation    *string          `json:"operation"`
}

// WorktreeChanges counts status entries; one entry can be staged and
// unstaged at once.
type WorktreeChanges struct {
	Staged     int `json:"staged"`
	Unstaged   int `json:"unstaged"`
	Untracked  int `json:"untracked"`
	Conflicted int `json:"conflicted"`
}

// Branch is a local branch. Upstream is the short upstream name, null
// without one; Ahead and Behind are null without an upstream or when it is
// gone. MergedIntoDefault reports that the main worktree's HEAD contains
// the branch, false for a bare or unborn main worktree.
type Branch struct {
	Name              string   `json:"name"`
	Head              string   `json:"head"`
	WorktreePaths     []string `json:"worktreePaths"`
	Subject           string   `json:"subject"`
	CommittedAt       string   `json:"committedAt"`
	Upstream          *string  `json:"upstream"`
	Ahead             *int     `json:"ahead"`
	Behind            *int     `json:"behind"`
	UpstreamGone      bool     `json:"upstreamGone"`
	MergedIntoDefault bool     `json:"mergedIntoDefault"`
}

// BranchBase is a local or remote-tracking branch offered as a start point;
// Name is the full refname and Kind is "local" or "remote".
type BranchBase struct {
	Name        string `json:"name"`
	Head        string `json:"head"`
	Subject     string `json:"subject"`
	Kind        string `json:"kind"`
	CommittedAt string `json:"committedAt"`
}

// Remote is one configured remote. Credentials in its URLs are redacted by
// the server.
type Remote struct {
	Name     string `json:"name"`
	FetchURL string `json:"fetchUrl"`
	PushURL  string `json:"pushUrl"`
}

// Remotes is the body of GET /api/repositories/{id}/remotes, sorted by name.
type Remotes struct {
	Remotes []Remote `json:"remotes"`
}

// FetchRemotesRequest is the empty body of POST
// /api/repositories/{id}/fetch.
type FetchRemotesRequest struct{}

// FetchResult is the outcome of fetching one remote. Status is "ok",
// "auth", "network", "timeout", "rejected" or "error"; Message is empty for
// "ok" and otherwise explains the failure with credentials redacted.
type FetchResult struct {
	Remote  string `json:"remote"`
	Status  string `json:"status"`
	Message string `json:"message"`
}

// FetchRemotesResponse answers POST /api/repositories/{id}/fetch: one
// result per remote, sorted by name, and the repository after the fetch.
type FetchRemotesResponse struct {
	Results    []FetchResult `json:"results"`
	Repository Repository    `json:"repository"`
}

// Branches is the body of GET /api/repositories/{id}/branches.
type Branches struct {
	Branches           []Branch     `json:"branches"`
	Bases              []BranchBase `json:"bases"`
	DefaultBase        *string      `json:"defaultBase"`
	DefaultBaseSubject string       `json:"defaultBaseSubject"`
	DefaultBranch      *string      `json:"defaultBranch"`
	DefaultParent      string       `json:"defaultParent"`
}

// Commit is one recent commit offered as a worktree start point.
type Commit struct {
	Hash        string `json:"hash"`
	Subject     string `json:"subject"`
	Author      string `json:"author"`
	CommittedAt string `json:"committedAt"`
}

// Commits is the body of GET /api/repositories/{id}/commits?from=<oid>.
type Commits struct {
	Commits []Commit `json:"commits"`
}

// RefreshBranchBaseRequest selects the local branch whose upstream is fetched.
type RefreshBranchBaseRequest struct {
	Branch string `json:"branch"`
}

// RefreshBranchBaseResponse describes a remote base or a local fallback.
// UpstreamHead is the fetched upstream tip, also when the local base is kept
// because the branch is ahead or diverged; it is empty when no fetch succeeded.
type RefreshBranchBaseResponse struct {
	Branch       string `json:"branch"`
	Base         string `json:"base"`
	BaseSubject  string `json:"baseSubject"`
	BaseSource   string `json:"baseSource"`
	Upstream     string `json:"upstream"`
	UpstreamHead string `json:"upstreamHead"`
	FetchWarning string `json:"fetchWarning"`
}

// RegisterRepositoryRequest is the body of POST /api/repositories. An
// already registered repository is returned unchanged.
type RegisterRepositoryRequest struct {
	Path string `json:"path"`
}

// UpdateRepositoryRequest is the body of PATCH /api/repositories/{id}; an
// empty instruction clears it. It answers 200 with the Repository.
type UpdateRepositoryRequest struct {
	InitInstruction string `json:"initInstruction"`
}

// CreateWorktreeRequest is the body of POST /api/repositories/{id}/worktrees.
type CreateWorktreeRequest struct {
	Path   string `json:"path"`
	Mode   string `json:"mode"`
	Branch string `json:"branch"`
	Base   string `json:"base"`
}

// RemovalPreviewRequest is the empty body of a removal preview.
type RemovalPreviewRequest struct{}

// RemovalPreview describes what removing a worktree would affect.
type RemovalPreview struct {
	ConfirmationID string        `json:"confirmationId"`
	ExpiresAt      time.Time     `json:"expiresAt"`
	Worktree       Worktree      `json:"worktree"`
	Sessions       []SessionInfo `json:"sessions"`
	// Archived are the closed and interrupted sessions in the worktree; a
	// successful removal keeps them, marked released at Commit so they can
	// be rebuilt. With an empty Commit (HEAD does not resolve) they stay
	// unchanged and cannot be rebuilt.
	Archived []SessionInfo `json:"archived"`
	// Commit is the full OID the removal keeps, empty when HEAD does not
	// resolve; Push is its push check.
	Commit       string    `json:"commit"`
	Push         PushCheck `json:"push"`
	Blockers     []string  `json:"blockers"`
	Warnings     []string  `json:"warnings"`
	ForceAllowed bool      `json:"forceAllowed"`
}

// HybridRelease records that a linked worktree directory was released while an
// archived session record kept its git state.
type HybridRelease struct {
	ID           string    `json:"id"`
	RepositoryID string    `json:"repositoryId"`
	Path         string    `json:"path"`
	Commit       string    `json:"commit"`
	Branch       string    `json:"branch,omitempty"`
	ReleasedAt   time.Time `json:"releasedAt"`
}

// RebuildPreviewRequest is the empty body of POST
// /api/hybrid-sessions/{id}/rebuild-preview.
type RebuildPreviewRequest struct{}

// RebuildPreview describes how an archived record's released worktree would be
// rebuilt.
type RebuildPreview struct {
	ReleaseID string   `json:"releaseId"`
	Path      string   `json:"path"`
	Commit    string   `json:"commit"`
	Branch    string   `json:"branch"`
	Mode      string   `json:"mode"`
	Reason    string   `json:"reason,omitempty"`
	Blockers  []string `json:"blockers"`
}

// RebuildRequest is the body of POST /api/hybrid-sessions/{id}/rebuild.
type RebuildRequest struct {
	ReleaseID string `json:"releaseId"`
	Mode      string `json:"mode"`
	Commit    string `json:"commit"`
}

// ReleasePreviewRequest is the empty body of a release preview.
type ReleasePreviewRequest struct{}

// ReleasePreview describes releasing a worktree's directory: the commit and
// archived records it keeps, and the ignored files it deletes. Bytes and
// IgnoredBytes are omitted when not counted; SizeComplete is false when the
// count ran out of budget.
type ReleasePreview struct {
	ConfirmationID string         `json:"confirmationId"`
	ExpiresAt      time.Time      `json:"expiresAt"`
	Worktree       Worktree       `json:"worktree"`
	Commit         string         `json:"commit"`
	Push           PushCheck      `json:"push"`
	Archived       []SessionInfo  `json:"archived"`
	Ignored        []IgnoredEntry `json:"ignored"`
	IgnoredMore    int            `json:"ignoredMore"`
	IgnoredBytes   *int64         `json:"ignoredBytes,omitempty"`
	SizeComplete   bool           `json:"sizeComplete"`
	Blockers       []string       `json:"blockers"`
}

// PushCheck says whether Commit is contained in a remote-tracking ref of the
// repository. State is "pushed", "unpushed" (remotes exist, none contains
// it), "noRemote" or "unknown" (no commit, or the check failed). It reads
// local refs only, so it can lag behind the remote. Branches name at most a
// few containing remote-tracking branches such as "origin/main"; More counts
// the rest.
type PushCheck struct {
	State    string   `json:"state"`
	Branches []string `json:"branches"`
	More     int      `json:"more"`
}

// IgnoredEntry is a top-level ignored path relative to the worktree;
// directories end with "/".
type IgnoredEntry struct {
	Path  string `json:"path"`
	Bytes *int64 `json:"bytes,omitempty"`
}

// WorktreeUsageRequest is the empty body of a worktree size measurement.
type WorktreeUsageRequest struct{}

// WorktreeUsage is the disk space of a worktree's directory: bytes of regular
// files and the entry count, without following symbolic links. Complete is
// false when the count ran out of budget or skipped an unreadable directory,
// so both numbers are lower bounds.
type WorktreeUsage struct {
	Bytes    int64 `json:"bytes"`
	Entries  int   `json:"entries"`
	Complete bool  `json:"complete"`
}

// ReleaseWorktreeRequest confirms a previewed release.
type ReleaseWorktreeRequest struct {
	ConfirmationID string `json:"confirmationId"`
}

// ReleaseResult names the release and the records it marked.
type ReleaseResult struct {
	ReleaseID string   `json:"releaseId"`
	RecordIDs []string `json:"recordIds"`
}

// RemoveWorktreeRequest confirms a previewed removal.
type RemoveWorktreeRequest struct {
	ConfirmationID string `json:"confirmationId"`
	Mode           string `json:"mode"`
	ConfirmPath    string `json:"confirmPath"`
	CloseSessions  bool   `json:"closeSessions"`
}

// BatchRemovalPreviewRequest selects the worktrees of a repository to preview.
type BatchRemovalPreviewRequest struct {
	WorktreeIDs []string `json:"worktreeIds"`
}

// RemovalPreviewItem describes what removing one worktree in a batch affects.
type RemovalPreviewItem struct {
	Worktree     Worktree      `json:"worktree"`
	Sessions     []SessionInfo `json:"sessions"`
	Archived     []SessionInfo `json:"archived"`
	Commit       string        `json:"commit"`
	Push         PushCheck     `json:"push"`
	Blockers     []string      `json:"blockers"`
	Warnings     []string      `json:"warnings"`
	ForceAllowed bool          `json:"forceAllowed"`
}

// BatchRemovalPreview describes what removing a batch of worktrees affects.
type BatchRemovalPreview struct {
	ConfirmationID string               `json:"confirmationId"`
	ExpiresAt      time.Time            `json:"expiresAt"`
	Items          []RemovalPreviewItem `json:"items"`
}

// BatchRemoveWorktreesRequest confirms a previewed batch removal.
type BatchRemoveWorktreesRequest struct {
	ConfirmationID string `json:"confirmationId"`
	IncludeForce   bool   `json:"includeForce"`
	CloseSessions  bool   `json:"closeSessions"`
}

// BatchRemoveItemResult reports the outcome for one worktree in a batch removal.
type BatchRemoveItemResult struct {
	WorktreeID string `json:"worktreeId"`
	Path       string `json:"path"`
	Branch     string `json:"branch,omitempty"`
	Status     string `json:"status"`
	Error      string `json:"error,omitempty"`
}

// BatchRemoveResult lists the per-worktree outcomes of a batch removal.
type BatchRemoveResult struct {
	Results []BatchRemoveItemResult `json:"results"`
}
