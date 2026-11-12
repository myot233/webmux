package workspace

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/rainchan/webmux/internal/hybrid"
	"github.com/rainchan/webmux/internal/session"
)

const readTimeout = 10 * time.Second
const writeTimeout = 30 * time.Second
const maxBatchWriteTimeout = 3 * time.Minute
const confirmationLifetime = 5 * time.Minute
const maxBatchWorktrees = 50

// Error kinds. Each wraps a transport-neutral category; the controller maps
// kinds to HTTP statuses.
var (
	ErrInvalid     = errors.New("invalid workspace request")
	ErrNotFound    = errors.New("workspace item not found")
	ErrConflict    = errors.New("workspace state conflict")
	ErrTooLarge    = errors.New("workspace output too large")
	ErrInternal    = errors.New("workspace internal error")
	ErrUnavailable = errors.New("workspace unavailable")
	ErrTimeout     = errors.New("workspace operation timed out")
)

// Error carries the user-facing message; its kind is reachable through
// errors.Is.
type Error struct {
	kind    error
	message string
}

func (e *Error) Error() string { return e.message }
func (e *Error) Unwrap() error { return e.kind }
func problemf(kind error, format string, args ...any) error {
	return &Error{kind: kind, message: fmt.Sprintf(format, args...)}
}

// errorKind classifies err like the controller does: its own kind, a timeout for
// cancellation, otherwise an internal error.
func errorKind(err error) error {
	var e *Error
	if errors.As(err, &e) {
		return e.kind
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return ErrTimeout
	}
	return ErrInternal
}

type Capability struct {
	GitAvailable      bool   `json:"gitAvailable"`
	RegistryAvailable bool   `json:"registryAvailable"`
	GitError          string `json:"gitError"`
	RegistryError     string `json:"registryError"`
}
type Repository struct {
	ID           string     `json:"id"`
	Name         string     `json:"name"`
	CommonDir    string     `json:"commonDir"`
	AnchorPath   string     `json:"anchorPath"`
	Availability string     `json:"availability"`
	Error        string     `json:"error"`
	CheckedAt    time.Time  `json:"checkedAt"`
	Worktrees    []Worktree `json:"worktrees"`
	// InitInstruction prefixes the first prompt of a quick start; empty
	// for none.
	InitInstruction string `json:"initInstruction"`
	// LastFetchedAt is the newest FETCH_HEAD modification time of the
	// repository and its linked worktrees; nil when none exists. It is
	// approximate: fetches that skip FETCH_HEAD are not seen.
	LastFetchedAt *time.Time `json:"lastFetchedAt"`
}
type Worktree struct {
	ID           string  `json:"id"`
	RepositoryID string  `json:"repositoryId"`
	Path         string  `json:"path"`
	Kind         string  `json:"kind"`
	Branch       *string `json:"branch"`
	Head         *string `json:"head"`
	Detached     bool    `json:"detached"`
	Locked       bool    `json:"locked"`
	LockReason   string  `json:"lockReason"`
	Prunable     bool    `json:"prunable"`
	Available    bool    `json:"available"`
	Dirty        *bool   `json:"dirty"`
	HasUntracked bool    `json:"hasUntracked"`
	HasIgnored   bool    `json:"hasIgnored"`
	Error        string  `json:"error"`
	// Upstream is the short upstream name ("origin/main"); nil without one
	// or when the status could not be read.
	Upstream *string `json:"upstream"`
	// Ahead and Behind count commits against the upstream; nil without an
	// upstream, when it is gone, or when Git did not compute them.
	Ahead        *int `json:"ahead"`
	Behind       *int `json:"behind"`
	UpstreamGone bool `json:"upstreamGone"`
	// LastCommit describes Head; nil for bare, unavailable or unborn
	// worktrees and when it could not be read.
	LastCommit *Commit `json:"lastCommit"`
	// Changes is nil when the status could not be read.
	Changes *WorktreeChanges `json:"changes"`
	// Operation is the Git operation in progress: merge, rebase,
	// cherry-pick, revert or bisect; nil for none.
	Operation *string `json:"operation"`
}

// WorktreeChanges counts status entries. One entry can be staged and
// unstaged at once.
type WorktreeChanges struct {
	Staged     int `json:"staged"`
	Unstaged   int `json:"unstaged"`
	Untracked  int `json:"untracked"`
	Conflicted int `json:"conflicted"`
}
type Branch struct {
	Name          string   `json:"name"`
	Head          string   `json:"head"`
	WorktreePaths []string `json:"worktreePaths"`
	Subject       string   `json:"subject"`
	CommittedAt   string   `json:"committedAt"`
	// Upstream is the short upstream name; nil without one.
	Upstream *string `json:"upstream"`
	// Ahead and Behind are nil without an upstream or when it is gone.
	Ahead        *int `json:"ahead"`
	Behind       *int `json:"behind"`
	UpstreamGone bool `json:"upstreamGone"`
	// MergedIntoDefault reports that the main worktree's HEAD contains
	// the branch; false for a bare or unborn main worktree.
	MergedIntoDefault bool `json:"mergedIntoDefault"`
}
type Base struct {
	Name    string `json:"name"`
	Head    string `json:"head"`
	Subject string `json:"subject"`
	// Kind is "local" for refs/heads and "remote" for refs/remotes.
	Kind        string `json:"kind"`
	CommittedAt string `json:"committedAt"`
}

// Remote is one configured remote; credentials in its URLs are redacted.
type Remote struct {
	Name     string `json:"name"`
	FetchURL string `json:"fetchUrl"`
	PushURL  string `json:"pushUrl"`
}
type Remotes struct {
	Remotes []Remote `json:"remotes"`
}

// Commit is one entry of the history shown when choosing a start commit.
type Commit struct {
	Hash        string `json:"hash"`
	Subject     string `json:"subject"`
	Author      string `json:"author"`
	CommittedAt string `json:"committedAt"`
}
type Branches struct {
	Branches           []Branch `json:"branches"`
	Bases              []Base   `json:"bases"`
	DefaultBase        *string  `json:"defaultBase"`
	DefaultBaseSubject string   `json:"defaultBaseSubject"`
	DefaultBranch      *string  `json:"defaultBranch"`
	DefaultParent      string   `json:"defaultParent"`
}
type BranchBaseRefresh struct {
	Branch       string `json:"branch"`
	Base         string `json:"base"`
	BaseSubject  string `json:"baseSubject"`
	BaseSource   string `json:"baseSource"`
	Upstream     string `json:"upstream"`
	UpstreamHead string `json:"upstreamHead"`
	FetchWarning string `json:"fetchWarning"`
}
type Reference struct {
	RepositoryID string
	WorktreeID   string
}
type CreateRequest struct {
	Path   string
	Mode   string
	Branch string
	Base   string
}
type RemoveRequest struct {
	ConfirmationID string
	Mode           string
	ConfirmPath    string
	CloseSessions  bool
}
type Preview struct {
	ConfirmationID string         `json:"confirmationId"`
	ExpiresAt      time.Time      `json:"expiresAt"`
	Worktree       Worktree       `json:"worktree"`
	Sessions       []session.Info `json:"sessions"`
	// Archived are the closed and interrupted hybrid records in the
	// worktree; a successful removal keeps them, marked released at Commit
	// so they can be rebuilt. Without a Commit (HEAD does not resolve) they
	// stay as they were and cannot be rebuilt.
	Archived []hybrid.Record `json:"archived"`
	// Commit is the full OID the removal pins, empty when HEAD does not
	// resolve; Push is its push check.
	Commit       string    `json:"commit"`
	Push         PushCheck `json:"push"`
	Blockers     []string  `json:"blockers"`
	Warnings     []string  `json:"warnings"`
	ForceAllowed bool      `json:"forceAllowed"`
}

type BatchPreviewItem struct {
	Worktree     Worktree        `json:"worktree"`
	Sessions     []session.Info  `json:"sessions"`
	Archived     []hybrid.Record `json:"archived"`
	Commit       string          `json:"commit"`
	Push         PushCheck       `json:"push"`
	Blockers     []string        `json:"blockers"`
	Warnings     []string        `json:"warnings"`
	ForceAllowed bool            `json:"forceAllowed"`
}

type BatchPreview struct {
	ConfirmationID string             `json:"confirmationId"`
	ExpiresAt      time.Time          `json:"expiresAt"`
	Items          []BatchPreviewItem `json:"items"`
}

type BatchRemoveRequest struct {
	ConfirmationID string
	IncludeForce   bool
	CloseSessions  bool
}

type BatchRemoveItemResult struct {
	WorktreeID string `json:"worktreeId"`
	Path       string `json:"path"`
	Branch     string `json:"branch,omitempty"`
	Status     string `json:"status"`
	Error      string `json:"error,omitempty"`
}

type BatchRemoveResult struct {
	Results []BatchRemoveItemResult `json:"results"`
}

func randomID() (string, error) {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw[:]), nil
}
func worktreeID(repository, path string) string {
	sum := sha256.Sum256([]byte(repository + "\x00" + path))
	return hex.EncodeToString(sum[:16])
}
func strptr(s string) *string { return &s }
