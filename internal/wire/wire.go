// Package wire converts between service types and the wire types of
// internal/contract. It belongs to the controller layer: services never see
// the wire shapes, and the controller and hub never encode service types.
//
// Required response arrays and objects encode as [] and {}, even if nil.
// Nullable pointers keep null; request conversions preserve omissions.
package wire

import (
	"github.com/rainchan/webmux/internal/contract"
	"github.com/rainchan/webmux/internal/hybrid"
	"github.com/rainchan/webmux/internal/session"
	"github.com/rainchan/webmux/internal/workspace"
	"maps"
	"path/filepath"
	"slices"
)

// each maps a required response array, normalizing nil to an empty slice.
func each[S, D any](in []S, f func(S) D) []D {
	out := make([]D, len(in))
	for i, v := range in {
		out[i] = f(v)
	}
	return out
}

// required normalizes scalar response slices without copying populated arrays.
func required[T any](in []T) []T {
	if in == nil {
		return []T{}
	}
	return in
}

// requiredMap normalizes required response objects without changing requests.
func requiredMap[V any](in map[string]V) map[string]V {
	if in == nil {
		return map[string]V{}
	}
	return in
}

// Session converts one session snapshot.
func Session(i session.Info) contract.SessionInfo {
	out := contract.SessionInfo{
		ID: i.ID, Generation: i.Generation, Title: i.Title, Cwd: i.Cwd,
		Cmd: required(i.Cmd), Status: i.Status, ExitCode: i.ExitCode, Preview: i.Preview,
		CreatedAt: i.CreatedAt, LastActiveAt: i.LastActiveAt, Cols: i.Cols, Rows: i.Rows, Clients: i.Clients,
		ConfigID: i.ConfigID, ListeningPorts: ListeningPorts(i.ListeningPorts),
		ParentID: i.ParentID, Labels: maps.Clone(i.Labels),
	}
	if i.Workspace != nil {
		w := contract.SessionWorkspace(*i.Workspace)
		out.Workspace = &w
	}
	if i.Attention != nil {
		out.Attention = &contract.SessionAttention{Kind: i.Attention.Kind, Source: i.Attention.Source, Since: i.Attention.Since, Message: i.Attention.Message}
	}
	if i.ReportedCwd != nil {
		r := contract.SessionReportedCwd(*i.ReportedCwd)
		out.ReportedCwd = &r
	}
	out.State, out.Views, out.View = contract.SessionStateActive, []contract.SessionView{contract.ViewTerminal}, contract.ViewTerminal
	if h := i.Hybrid; h != nil {
		out.ConversationID, out.ProviderThreadID = h.ConversationID, h.ProviderThreadID
		if h.State != "" {
			out.State = contract.SessionState(h.State)
		}
		out.Views = []contract.SessionView{contract.ViewTerminal, contract.ViewChat}
		out.View = View(h.Side)
		if h.To != "" {
			out.Transition = &contract.SessionTransition{To: View(h.To), Phase: contract.TransitionPhase(h.Phase)}
		}
		out.Agent = &contract.SessionAgent{Name: h.Agent, Title: h.AgentTitle, Icon: h.AgentIcon, Work: h.Work}
		if h.AgentSessionID != nil {
			id := *h.AgentSessionID
			out.Agent.SessionID = &id
		}
	}
	return out
}

// Active reports whether a listed shell is an active session: every
// ordinary session, and a hybrid session until it is closed or interrupted.
// Only active sessions are listed and streamed; the others are described by
// their archive records.
func Active(i session.Info) bool {
	return i.Hybrid == nil || i.Hybrid.State == "" || i.Hybrid.State == string(hybrid.StateActive)
}

// View converts a hybrid side (tui or acp) to a view; anything but acp is
// the terminal.
func View(side string) contract.SessionView {
	if side == string(hybrid.SideACP) {
		return contract.ViewChat
	}
	return contract.ViewTerminal
}

// side converts a requested view to a hybrid side; empty stays empty and an
// unknown view becomes a side no service accepts, so it is rejected.
func side(v contract.SessionView) string {
	switch v {
	case "":
		return ""
	case contract.ViewTerminal:
		return string(hybrid.SideTUI)
	case contract.ViewChat:
		return string(hybrid.SideACP)
	}
	return "view:" + string(v)
}

func Sessions(in []session.Info) []contract.SessionInfo { return each(in, Session) }

// Archived is what an archived session's conversion needs besides its
// record: whether its launch config was deleted and its agent's label.
type Archived struct {
	ConversationID, ProviderThreadID string
	ConfigMissing                    bool
	AgentTitle                       string
	AgentIcon                        string
}

// ArchivedSession converts the record of a closed or interrupted session:
// runtime fields are zero, lastActiveAt is the record's update time. The
// option values and process identity stay on the server.
func ArchivedSession(r hybrid.Record, a Archived) contract.SessionInfo {
	title := r.Name
	if title == "" {
		title = r.Title
	}
	if title == "" {
		title = filepath.Base(r.Cwd)
	}
	out := contract.SessionInfo{
		ConversationID: a.ConversationID, ProviderThreadID: a.ProviderThreadID,
		ID: r.ID, Title: title, Cwd: r.Cwd, Cmd: required(r.Cmd), Status: contract.SessionStatusExited,
		CreatedAt: r.CreatedAt, LastActiveAt: r.UpdatedAt, ConfigID: r.ConfigID, ListeningPorts: []contract.ListeningPort{},
		State: contract.SessionState(r.State), Views: []contract.SessionView{contract.ViewTerminal, contract.ViewChat}, View: View(string(r.Side)),
		Agent: &contract.SessionAgent{Name: r.Agent, Title: a.AgentTitle, Icon: a.AgentIcon},
		Archive: &contract.SessionArchive{
			ConfigMissing: a.ConfigMissing, PreviousAgentSessionIDs: required(slices.Clone(r.PreviousIDs)),
			Activity: r.Activity, QueuedCount: r.QueuedCount, UpdatedAt: r.UpdatedAt,
		},
	}
	if r.AgentSessionID != "" {
		id := r.AgentSessionID
		out.Agent.SessionID = &id
	}
	if r.InterruptedAt != nil {
		at := *r.InterruptedAt
		out.Archive.InterruptedAt = &at
	}
	if r.Workspace != nil {
		// The record keeps no worktree path; the session's directory stands
		// in for it.
		out.Workspace = &contract.SessionWorkspace{RepositoryID: r.Workspace.RepositoryID, WorktreeID: r.Workspace.WorktreeID, WorktreePath: r.Cwd}
	}
	if r.Released != nil {
		rel := contract.HybridRelease(*r.Released)
		out.Released = &rel
	}
	return out
}

// CreateInput converts the body of POST /api/sessions.
func CreateInput(r contract.CreateSessionRequest) workspace.CreateInput {
	in := workspace.CreateInput{
		ClientRequestID: r.ClientRequestID, Cwd: r.Cwd, Cmd: r.Cmd, Name: r.Name, Env: r.Env, ConfigID: r.ConfigID,
		Side: side(r.View), Prompt: r.Prompt, UseInitInstruction: r.UseInitInstruction, ExpandPrompt: r.ExpandPrompt,
		AgentConfig: r.AgentConfig, PermissionMode: string(r.PermissionMode), Cols: r.Cols, Rows: r.Rows,
	}
	if r.Workspace != nil {
		ref := workspace.Reference(*r.Workspace)
		in.Workspace = &ref
	}
	if r.NewWorktree != nil {
		nw := workspace.NewWorktree(*r.NewWorktree)
		in.NewWorktree = &nw
	}
	if r.Attachments != nil {
		in.Attachments = each(r.Attachments, func(a contract.ChatAttachment) string { return a.Path })
	}
	return in
}

// CreateResult converts a created session; info is its snapshot after the
// request's labels were applied.
func CreateResult(r workspace.CreateResult, info session.Info) contract.CreateSessionResult {
	out := contract.CreateSessionResult{Session: Session(info), Worktree: OptionalWorktree(r.Worktree), ConfigWarnings: required(r.ConfigWarnings)}
	if r.InitialInput != nil {
		input := SessionInputResult(hybrid.InputResult{Side: hybrid.SideACP, Prompt: *r.InitialInput})
		out.InitialInput = &input
	}
	return out
}

// OptionalWorktree converts a worktree that may not exist; nil stays nil.
func OptionalWorktree(w *workspace.Worktree) *contract.Worktree {
	if w == nil {
		return nil
	}
	out := Worktree(*w)
	return &out
}

// InputKeys converts named keys to their bytes; ok is false for an unknown
// name.
func InputKeys(names []string) (keys [][]byte, ok bool) {
	for _, name := range names {
		key, known := contract.InputKeys[name]
		if !known {
			return nil, false
		}
		keys = append(keys, []byte(key))
	}
	return keys, true
}

// ScriptResize converts the body of POST /api/sessions/{id}/resize; label
// names the controller while the request holds control.
func ScriptResize(r contract.SessionResizeRequest, label string) session.ScriptResize {
	return session.ScriptResize{Cols: r.Cols, Rows: r.Rows, Take: r.Take, Label: label}
}

// SessionInput converts the body of POST /api/sessions/{id}/input; label
// names the terminal controller. Keys must already be valid.
func SessionInput(r contract.SessionInputRequest, label string) hybrid.InputRequest {
	keys, _ := InputKeys(r.Keys)
	return hybrid.InputRequest{
		ClientRequestID: r.ClientRequestID, Label: label, Text: r.Text, Submit: r.Submit, Keys: keys,
		Attachments: promptAttachments(r.Attachments), Mode: r.Mode,
	}
}

// SessionInputResult converts where input went.
func SessionInputResult(r hybrid.InputResult) contract.SessionInputResult {
	out := contract.SessionInputResult{View: View(string(r.Side))}
	if r.Side == hybrid.SideACP {
		queued := r.Prompt.Queued
		out.Queued, out.ItemID, out.Position = &queued, r.Prompt.ItemID, r.Prompt.Position
		out.TurnID, out.PromptID = r.Prompt.TurnID, r.Prompt.PromptID
	}
	return out
}

func Capability(c workspace.Capability) contract.WorkspaceCapability {
	return contract.WorkspaceCapability(c)
}

// PendingUpdate describes an installed version awaiting a restart; it is
// nil when nothing different is installed.
func PendingUpdate(installed, launcher string) *contract.PendingUpdate {
	if installed == "" {
		return nil
	}
	return &contract.PendingUpdate{InstalledVersion: installed, Launcher: launcher}
}

func Worktree(w workspace.Worktree) contract.Worktree {
	out := contract.Worktree{
		ID: w.ID, RepositoryID: w.RepositoryID, Path: w.Path, Kind: w.Kind, Branch: w.Branch, Head: w.Head,
		Detached: w.Detached, Locked: w.Locked, LockReason: w.LockReason, Prunable: w.Prunable, Available: w.Available,
		Dirty: w.Dirty, HasUntracked: w.HasUntracked, HasIgnored: w.HasIgnored, Error: w.Error,
		Upstream: w.Upstream, Ahead: w.Ahead, Behind: w.Behind, UpstreamGone: w.UpstreamGone, Operation: w.Operation,
	}
	if w.LastCommit != nil {
		commit := contract.Commit(*w.LastCommit)
		out.LastCommit = &commit
	}
	if w.Changes != nil {
		changes := contract.WorktreeChanges(*w.Changes)
		out.Changes = &changes
	}
	return out
}

func Repository(r workspace.Repository) contract.Repository {
	return contract.Repository{
		ID: r.ID, Name: r.Name, CommonDir: r.CommonDir, AnchorPath: r.AnchorPath,
		Availability: r.Availability, Error: r.Error, CheckedAt: r.CheckedAt,
		Worktrees: each(r.Worktrees, Worktree), InitInstruction: r.InitInstruction, LastFetchedAt: r.LastFetchedAt,
	}
}

func Repositories(in []workspace.Repository) []contract.Repository { return each(in, Repository) }

func Branches(b workspace.Branches) contract.Branches {
	branches := each(b.Branches, func(v workspace.Branch) contract.Branch {
		return contract.Branch{
			Name: v.Name, Head: v.Head, WorktreePaths: required(v.WorktreePaths), Subject: v.Subject, CommittedAt: v.CommittedAt,
			Upstream: v.Upstream, Ahead: v.Ahead, Behind: v.Behind, UpstreamGone: v.UpstreamGone, MergedIntoDefault: v.MergedIntoDefault,
		}
	})
	bases := each(b.Bases, func(v workspace.Base) contract.BranchBase { return contract.BranchBase(v) })
	return contract.Branches{
		Branches:           branches,
		Bases:              bases,
		DefaultBase:        b.DefaultBase,
		DefaultBaseSubject: b.DefaultBaseSubject,
		DefaultBranch:      b.DefaultBranch,
		DefaultParent:      b.DefaultParent,
	}
}

func Remotes(r workspace.Remotes) contract.Remotes {
	return contract.Remotes{Remotes: each(r.Remotes, func(v workspace.Remote) contract.Remote { return contract.Remote(v) })}
}

func FetchRemotes(r workspace.FetchRemotesResult) contract.FetchRemotesResponse {
	return contract.FetchRemotesResponse{
		Results:    each(r.Results, func(v workspace.FetchResult) contract.FetchResult { return contract.FetchResult(v) }),
		Repository: Repository(r.Repository),
	}
}

func Commits(in []workspace.Commit) contract.Commits {
	return contract.Commits{Commits: each(in, func(v workspace.Commit) contract.Commit { return contract.Commit(v) })}
}

func RefreshBranchBase(r workspace.BranchBaseRefresh) contract.RefreshBranchBaseResponse {
	return contract.RefreshBranchBaseResponse(r)
}

// Preview converts a removal preview; archived describes each archived
// record.
func Preview(p workspace.Preview, archived func(hybrid.Record) Archived) contract.RemovalPreview {
	return contract.RemovalPreview{
		ConfirmationID: p.ConfirmationID, ExpiresAt: p.ExpiresAt, Worktree: Worktree(p.Worktree),
		Sessions: Sessions(p.Sessions), Archived: each(p.Archived, func(r hybrid.Record) contract.SessionInfo { return ArchivedSession(r, archived(r)) }),
		Commit: p.Commit, Push: PushCheck(p.Push), Blockers: required(p.Blockers), Warnings: required(p.Warnings), ForceAllowed: p.ForceAllowed,
	}
}

// BatchPreview converts a batch removal preview.
func BatchPreview(p workspace.BatchPreview, archived func(hybrid.Record) Archived) contract.BatchRemovalPreview {
	return contract.BatchRemovalPreview{
		ConfirmationID: p.ConfirmationID,
		ExpiresAt:      p.ExpiresAt,
		Items: each(p.Items, func(item workspace.BatchPreviewItem) contract.RemovalPreviewItem {
			return contract.RemovalPreviewItem{
				Worktree:     Worktree(item.Worktree),
				Sessions:     Sessions(item.Sessions),
				Archived:     each(item.Archived, func(r hybrid.Record) contract.SessionInfo { return ArchivedSession(r, archived(r)) }),
				Commit:       item.Commit,
				Push:         PushCheck(item.Push),
				Blockers:     required(item.Blockers),
				Warnings:     required(item.Warnings),
				ForceAllowed: item.ForceAllowed,
			}
		}),
	}
}

// PushCheck converts a push check; its branches are never null.
func PushCheck(c workspace.PushCheck) contract.PushCheck {
	state := string(c.State)
	if state == "" {
		state = string(workspace.PushUnknown)
	}
	return contract.PushCheck{State: state, Branches: required(c.Branches), More: c.More}
}

func ReleasePreview(p workspace.ReleasePreview, archived func(hybrid.Record) Archived) contract.ReleasePreview {
	return contract.ReleasePreview{
		ConfirmationID: p.ConfirmationID, ExpiresAt: p.ExpiresAt, Worktree: Worktree(p.Worktree), Commit: p.Commit, Push: PushCheck(p.Push),
		Archived:    each(p.Archived, func(r hybrid.Record) contract.SessionInfo { return ArchivedSession(r, archived(r)) }),
		Ignored:     each(p.Ignored, func(e workspace.IgnoredEntry) contract.IgnoredEntry { return contract.IgnoredEntry(e) }),
		IgnoredMore: p.IgnoredMore, IgnoredBytes: p.IgnoredBytes, SizeComplete: p.SizeComplete, Blockers: required(p.Blockers),
	}
}

func WorktreeUsage(u workspace.Usage) contract.WorktreeUsage {
	return contract.WorktreeUsage(u)
}

func ReleaseWorktree(r contract.ReleaseWorktreeRequest) workspace.ReleaseRequest {
	return workspace.ReleaseRequest(r)
}

func ReleaseResult(r workspace.ReleaseResult) contract.ReleaseResult {
	return contract.ReleaseResult{ReleaseID: r.ReleaseID, RecordIDs: required(r.RecordIDs)}
}

func RebuildPreview(p workspace.RebuildPreview) contract.RebuildPreview {
	return contract.RebuildPreview{
		ReleaseID: p.ReleaseID, Path: p.Path, Commit: p.Commit, Branch: p.Branch,
		Mode: p.Mode, Reason: p.Reason, Blockers: required(p.Blockers),
	}
}

func RebuildWorktree(r contract.RebuildRequest) workspace.RebuildRequest {
	return workspace.RebuildRequest(r)
}

func CreateWorktree(r contract.CreateWorktreeRequest) workspace.CreateRequest {
	return workspace.CreateRequest(r)
}

func RemoveWorktree(r contract.RemoveWorktreeRequest) workspace.RemoveRequest {
	return workspace.RemoveRequest(r)
}

func BatchRemoveWorktrees(r contract.BatchRemoveWorktreesRequest) workspace.BatchRemoveRequest {
	return workspace.BatchRemoveRequest(r)
}

func SwitchTarget(t contract.SwitchTarget) workspace.SwitchTarget {
	return workspace.SwitchTarget(t)
}

func Issues(in []workspace.Issue) []contract.Issue {
	return each(in, func(i workspace.Issue) contract.Issue { return contract.Issue(i) })
}

func SwitchPreview(p workspace.SwitchPreview) contract.SwitchPreview {
	return contract.SwitchPreview{
		ConfirmationID: p.ConfirmationID, ExpiresAt: p.ExpiresAt, Worktree: Worktree(p.Worktree),
		Target:   contract.SwitchTargetInfo(p.Target),
		Sessions: Sessions(p.Sessions), Blockers: Issues(p.Blockers), Warnings: Issues(p.Warnings),
	}
}

func SwitchWorktree(r contract.SwitchRequest) workspace.SwitchRequest {
	return workspace.SwitchRequest(r)
}

func CommitList(l workspace.CommitList) contract.CommitList {
	return contract.CommitList{Total: l.Total, Items: each(l.Items, func(v workspace.Commit) contract.Commit { return contract.Commit(v) })}
}

func FastForwardPreview(p workspace.FastForwardPreview) contract.FastForwardPreview {
	out := contract.FastForwardPreview{
		ConfirmationID: p.ConfirmationID, ExpiresAt: p.ExpiresAt, Branch: p.Branch, From: p.From, To: p.To, Upstream: p.Upstream,
		Commits: CommitList(p.Commits), Sessions: Sessions(p.Sessions), Blockers: Issues(p.Blockers), Warnings: Issues(p.Warnings),
	}
	if p.Worktree != nil {
		tree := Worktree(*p.Worktree)
		out.Worktree = &tree
	}
	return out
}

func FastForward(r contract.FastForwardRequest) workspace.FastForwardRequest {
	return workspace.FastForwardRequest(r)
}

func FastForwardResult(r workspace.FastForwardResult) contract.FastForwardResult {
	return contract.FastForwardResult{Repository: Repository(r.Repository)}
}

func PushPreview(p workspace.PushPreview) contract.PushPreview {
	return contract.PushPreview{
		ConfirmationID: p.ConfirmationID, ExpiresAt: p.ExpiresAt, Branch: p.Branch, Commit: p.Commit, Remote: p.Remote, PushURL: p.PushURL,
		Target: p.Target, Publish: p.Publish, Commits: CommitList(p.Commits), Blockers: Issues(p.Blockers), Warnings: Issues(p.Warnings),
	}
}

func Push(r contract.PushRequest) workspace.PushRequest {
	return workspace.PushRequest(r)
}

func PushResult(r workspace.PushResult) contract.PushResult {
	return contract.PushResult{Status: r.Status, Message: r.Message, Repository: Repository(r.Repository)}
}

func BranchDeletePreview(p workspace.BranchDeletePreview) contract.BranchDeletePreview {
	return contract.BranchDeletePreview{
		ConfirmationID: p.ConfirmationID, ExpiresAt: p.ExpiresAt,
		Items: each(p.Items, func(v workspace.BranchDeleteItem) contract.BranchDeleteItem {
			return contract.BranchDeleteItem{Branch: v.Branch, Commit: v.Commit, Group: v.Group, Unmerged: CommitList(v.Unmerged), Blockers: Issues(v.Blockers)}
		}),
	}
}

func DeleteBranches(r contract.BranchDeleteRequest) workspace.BranchDeleteRequest {
	return workspace.BranchDeleteRequest(r)
}

func BranchDeleteResult(r workspace.BranchDeleteResult) contract.BranchDeleteResult {
	return contract.BranchDeleteResult{
		Results: each(r.Results, func(v workspace.BranchDeleteItemResult) contract.BranchDeleteItemResult {
			return contract.BranchDeleteItemResult(v)
		}),
		Repository: Repository(r.Repository),
	}
}

func BatchRemoveResult(r workspace.BatchRemoveResult) contract.BatchRemoveResult {
	return contract.BatchRemoveResult{
		Results: each(r.Results, func(v workspace.BatchRemoveItemResult) contract.BatchRemoveItemResult {
			return contract.BatchRemoveItemResult(v)
		}),
	}
}
