package workspace

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/rainchan/webmux/internal/session"
)

const readTimeout = 10 * time.Second
const writeTimeout = 30 * time.Second
const confirmationLifetime = 5 * time.Minute

type Error struct {
	Status  int
	Message string
}

func (e *Error) Error() string { return e.Message }
func problem(status int, format string, args ...any) error {
	return &Error{status, fmt.Sprintf(format, args...)}
}
func Status(err error) int {
	var e *Error
	if errors.As(err, &e) {
		return e.Status
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return 504
	}
	return 500
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
}
type Branch struct {
	Name          string   `json:"name"`
	Head          string   `json:"head"`
	WorktreePaths []string `json:"worktreePaths"`
}
type Base struct {
	Name string `json:"name"`
	Head string `json:"head"`
}
type Branches struct {
	Branches      []Branch `json:"branches"`
	Bases         []Base   `json:"bases"`
	DefaultBase   *string  `json:"defaultBase"`
	DefaultParent string   `json:"defaultParent"`
}
type Reference struct {
	RepositoryID string `json:"repositoryId"`
	WorktreeID   string `json:"worktreeId"`
}
type CreateRequest struct {
	Path   string `json:"path"`
	Mode   string `json:"mode"`
	Branch string `json:"branch"`
	Base   string `json:"base"`
}
type RemoveRequest struct {
	ConfirmationID string `json:"confirmationId"`
	Mode           string `json:"mode"`
	ConfirmPath    string `json:"confirmPath"`
	CloseSessions  bool   `json:"closeSessions"`
}
type Preview struct {
	ConfirmationID string         `json:"confirmationId"`
	ExpiresAt      time.Time      `json:"expiresAt"`
	Worktree       Worktree       `json:"worktree"`
	Sessions       []session.Info `json:"sessions"`
	Blockers       []string       `json:"blockers"`
	ForceAllowed   bool           `json:"forceAllowed"`
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
