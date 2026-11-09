// Package wire converts between service types and the wire types of
// internal/contract. It belongs to the controller layer: services never see
// the wire shapes, and the controller and hub never encode service types.
//
// Required response arrays and objects encode as [] and {}, even if nil.
// Nullable pointers keep null; request conversions preserve omissions.
package wire

import (
	"github.com/rainchan/webmux/internal/agentprofile"
	"github.com/rainchan/webmux/internal/codeview"
	"github.com/rainchan/webmux/internal/contract"
	"github.com/rainchan/webmux/internal/hoststat"
	"github.com/rainchan/webmux/internal/hybrid"
	"github.com/rainchan/webmux/internal/launch"
	"github.com/rainchan/webmux/internal/push"
	"github.com/rainchan/webmux/internal/session"
	"github.com/rainchan/webmux/internal/tailscale"
	"github.com/rainchan/webmux/internal/voice"
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
	ConfigMissing bool
	AgentTitle    string
	AgentIcon     string
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
	return contract.CreateSessionResult{Session: Session(info), Worktree: OptionalWorktree(r.Worktree), ConfigWarnings: required(r.ConfigWarnings)}
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

func Worktree(w workspace.Worktree) contract.Worktree { return contract.Worktree(w) }

func Repository(r workspace.Repository) contract.Repository {
	return contract.Repository{
		ID: r.ID, Name: r.Name, CommonDir: r.CommonDir, AnchorPath: r.AnchorPath,
		Availability: r.Availability, Error: r.Error, CheckedAt: r.CheckedAt,
		Worktrees: each(r.Worktrees, Worktree), InitInstruction: r.InitInstruction,
	}
}

func Repositories(in []workspace.Repository) []contract.Repository { return each(in, Repository) }

func Branches(b workspace.Branches) contract.Branches {
	branches := each(b.Branches, func(v workspace.Branch) contract.Branch {
		return contract.Branch{Name: v.Name, Head: v.Head, WorktreePaths: required(v.WorktreePaths)}
	})
	bases := each(b.Bases, func(v workspace.Base) contract.BranchBase {
		return contract.BranchBase{Name: v.Name, Head: v.Head, Subject: v.Subject}
	})
	return contract.Branches{
		Branches:           branches,
		Bases:              bases,
		DefaultBase:        b.DefaultBase,
		DefaultBaseSubject: b.DefaultBaseSubject,
		DefaultBranch:      b.DefaultBranch,
		DefaultParent:      b.DefaultParent,
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

func BatchRemoveResult(r workspace.BatchRemoveResult) contract.BatchRemoveResult {
	return contract.BatchRemoveResult{
		Results: each(r.Results, func(v workspace.BatchRemoveItemResult) contract.BatchRemoveItemResult {
			return contract.BatchRemoveItemResult(v)
		}),
	}
}

func previewSettings(p *launch.PreviewSettings) *contract.PreviewSettings {
	if p == nil {
		return nil
	}
	return &contract.PreviewSettings{Lines: p.Lines, Skip: p.Skip}
}

func launchPreview(p *contract.PreviewSettings) *launch.PreviewSettings {
	if p == nil {
		return nil
	}
	return &launch.PreviewSettings{Lines: p.Lines, Skip: p.Skip}
}

func LaunchConfig(c launch.Config) contract.LaunchConfig {
	return contract.LaunchConfig{
		ID: c.ID, Name: c.Name, Icon: c.Icon, Cmd: required(c.Cmd), Env: requiredMap(c.Env), Preview: previewSettings(c.Preview),
		Chat: c.Hybrid, ChatSupport: ChatSupport(c.HybridSupport()),
		CreatedAt: c.CreatedAt, UpdatedAt: c.UpdatedAt,
	}
}

// LaunchSummaries converts the launch config list; chatOptions returns the
// remembered agent options of a config (nil: none known).
func LaunchSummaries(in []launch.Summary, chatOptions func(configID string) []hybrid.ConfigOption) []contract.LaunchConfigSummary {
	return each(in, func(s launch.Summary) contract.LaunchConfigSummary {
		var options []hybrid.ConfigOption
		if chatOptions != nil {
			options = chatOptions(s.ID)
		}
		return contract.LaunchConfigSummary{
			ID: s.ID, Name: s.Name, Icon: s.Icon, Cmd: required(s.Cmd), EnvNames: required(s.EnvNames), Preview: previewSettings(s.Preview),
			Chat: s.Hybrid, ChatSupport: ChatSupport(s.HybridSupport), ChatOptions: each(options, chatConfig),
			CreatedAt: s.CreatedAt, UpdatedAt: s.UpdatedAt,
		}
	})
}

func LaunchInput(r contract.LaunchConfigInput) launch.Input {
	return launch.Input{Name: r.Name, Icon: r.Icon, Cmd: r.Cmd, Env: r.Env, Preview: launchPreview(r.Preview), Hybrid: r.Chat}
}

func LaunchPresets(in []launch.Preset) []contract.LaunchPreset {
	return each(in, func(p launch.Preset) contract.LaunchPreset {
		return contract.LaunchPreset{
			Name: p.Name, Icon: p.Icon, Cmd: required(p.Cmd), EnvNames: required(p.EnvNames), Chat: p.Hybrid,
		}
	})
}

func ChatSupport(s agentprofile.Support) contract.ChatSupport {
	return contract.ChatSupport{Supported: s.Supported, Agent: s.Agent, Reason: string(s.Reason), Connector: AgentConnector(s.Connector),
		SupportedAgents: required(s.Agents),
	}
}

// AgentConnector converts a profile's connector; nil stays nil. Its
// verified version stays on the server.
func AgentConnector(c *agentprofile.Connector) *contract.AgentConnector {
	if c == nil {
		return nil
	}
	return &contract.AgentConnector{Executable: c.Executable, Title: c.Title, InstallCommand: c.InstallCommand, DocsURL: c.DocsURL}
}

func PushTestReport(r push.TestReport) contract.PushTestReport {
	out := contract.PushTestReport(r)
	out.Removed, out.Errors = required(r.Removed), required(r.Errors)
	return out
}

func CodeContext(c codeview.Context) contract.CodeContext { return contract.CodeContext(c) }

func codeNode(n codeview.Node) contract.CodeNode { return contract.CodeNode(n) }

func CodeListing(l codeview.Listing) contract.CodeListing {
	return contract.CodeListing{Children: each(l.Children, codeNode), Next: l.Next, Limited: l.Limited, Complete: l.Complete, Version: l.Version, Checked: l.Checked}
}

func CodeLocation(l codeview.Location) contract.CodeLocation { return contract.CodeLocation(l) }

func codeDocument(d codeview.Document) contract.CodeDocument {
	return contract.CodeDocument{
		NodeID: d.NodeID, Path: d.Path, Absolute: d.Absolute, Source: d.Source, OID: d.OID,
		Version: d.Version, Kind: d.Kind, Text: d.Text, Lines: d.Lines, Referenceable: d.Referenceable,
		Reason: d.Reason, MediaType: d.MediaType, Size: d.Size, Downloadable: d.Downloadable,
	}
}

func CodeFile(f codeview.File) contract.CodeFile {
	return contract.CodeFile{Snapshot: f.Snapshot, Document: codeDocument(f.Document), Checked: f.Checked}
}

func codeChange(c codeview.Change) contract.CodeChange { return contract.CodeChange(c) }

func CodeChanges(c codeview.Changes) contract.CodeChanges {
	return contract.CodeChanges{Next: c.Next, Total: c.Total, Entries: each(c.Entries, codeChange), Version: c.Version, Checked: c.Checked}
}

func CodeDiff(d codeview.Diff) contract.CodeDiff {
	return contract.CodeDiff{
		Snapshot: d.Snapshot, Old: codeDocument(d.Old), New: codeDocument(d.New),
		Kind: d.Kind, Reason: d.Reason, Checked: d.Checked, Change: codeChange(d.Change),
	}
}

func CodeCheck(c codeview.Check) contract.CodeCheck {
	out := contract.CodeCheck(c)
	out.Snapshots, out.Directories = requiredMap(c.Snapshots), requiredMap(c.Directories)
	return out
}

func CodeCheckRequest(r contract.CodeCheckRequest) codeview.CheckRequest {
	return codeview.CheckRequest(r)
}

func CodeMutation(m codeview.Mutation) contract.CodeMutation {
	return contract.CodeMutation{Nodes: each(m.Nodes, codeNode)}
}

func CodeUsage(u codeview.Usage) contract.CodeUsage { return contract.CodeUsage(u) }

// SessionEvent converts one session list event.
func SessionEvent(e session.Event) contract.SessionEvent {
	out := contract.SessionEvent{Type: e.Type, Sessions: Sessions(e.Sessions), ID: e.ID}
	if e.Session != nil {
		info := Session(*e.Session)
		out.Session = &info
	}
	return out
}

func StreamPosition(p session.Position) contract.StreamPosition {
	return contract.StreamPosition(p)
}

// TailscaleStatus converts the read-only detection result. Runtime state and
// configured mode are layered on by the controller.
func TailscaleStatus(s tailscale.Status) contract.TailscaleStatus {
	out := contract.TailscaleStatus{
		Provider: s.Provider, ConfiguredProvider: s.ConfiguredProvider,
		BackendState: s.BackendState, DNSName: s.DNSName,
		HTTPSReady: s.HTTPSReady, FunnelAvailable: s.FunnelAvailable,
		Entries: each(s.Entries, func(e tailscale.Entry) contract.TailscaleEntry {
			return contract.TailscaleEntry{Host: e.Host, Port: e.Port, Public: e.Public}
		}),
	}
	if s.WSL != nil {
		out.WSL = &contract.TailscaleWSL{
			NetworkingMode: s.WSL.NetworkingMode, WindowsDetected: s.WSL.WindowsDetected,
			WindowsRunning: s.WSL.WindowsRunning, WindowsDNSName: s.WSL.WindowsDNSName,
		}
	}
	return out
}

func SubmissionStatus(s session.SubmissionStatus) contract.SubmissionStatus {
	return contract.SubmissionStatus{ID: s.ID, State: string(s.State), Error: s.Error}
}

func ControlState(c session.ControlState) contract.ControlState {
	out := contract.ControlState{Cols: c.Cols, Rows: c.Rows, ColorSchemeReports: c.ColorSchemeReports}
	if c.Controller != nil {
		controller := contract.TerminalController(*c.Controller)
		out.Controller = &controller
	}
	return out
}

func VoiceEvent(e voice.Event) contract.VoiceServerEvent {
	return contract.VoiceServerEvent{
		Type:  string(e.Type),
		Text:  e.Text,
		Error: e.Error,
	}
}

// Health is the GET /api/health body; nil load or memory is left out.
func Health(sessions int, load *hoststat.Load, mem *hoststat.Memory) contract.APIHealth {
	h := contract.APIHealth{OK: true, Sessions: sessions}
	if load != nil {
		h.Load = &contract.HostLoad{One: load.One, Five: load.Five, Fifteen: load.Fifteen, CPUs: load.CPUs}
	}
	if mem != nil {
		h.Memory = &contract.HostMemory{Total: mem.Total, Available: mem.Available}
	}
	return h
}

func ChatMessageMatches(matches []hybrid.MessageMatch) []contract.ChatMessageMatch {
	out := make([]contract.ChatMessageMatch, len(matches))
	for i, match := range matches {
		out[i] = contract.ChatMessageMatch(match)
	}
	return out
}
