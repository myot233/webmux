package wire

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/rainchan/webmux/internal/attentionkind"
	"github.com/rainchan/webmux/internal/codeview"
	"github.com/rainchan/webmux/internal/contract"
	"github.com/rainchan/webmux/internal/hybrid"
	"github.com/rainchan/webmux/internal/launch"
	"github.com/rainchan/webmux/internal/push"
	"github.com/rainchan/webmux/internal/session"
	"github.com/rainchan/webmux/internal/workspace"
)

func encode(t *testing.T, v any) string {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// Required arrays normalize nil to []; nullable fields retain null.
func TestRequiredArraysAndNullableCollections(t *testing.T) {
	for _, row := range []struct {
		name string
		got  any
		want []string
	}{
		{"nil sessions", Sessions(nil), []string{"[]"}},
		{"empty sessions", Sessions([]session.Info{}), []string{"[]"}},
		{"nil cmd", Session(session.Info{}), []string{`"cmd":[]`, `"workspace":null`, `"attention":null`, `"reportedCwd":null`, `"exitCode":null`}},
		{"empty cmd", Session(session.Info{Cmd: []string{}}), []string{`"cmd":[]`}},
		{"empty env", LaunchConfig(launch.Config{Env: map[string]string{}}), []string{`"env":{}`}},
		{"nil env", LaunchConfig(launch.Config{}), []string{`"env":{}`}},
		{"nil env names", LaunchSummaries([]launch.Summary{{}}, nil), []string{`"envNames":[]`, `"cmd":[]`, `"chatOptions":[]`}},
		{"nil preset arrays", LaunchPresets([]launch.Preset{{}}), []string{`"envNames":[]`, `"cmd":[]`}},
		{"nil branches", Branches(workspace.Branches{Branches: []workspace.Branch{{}}}), []string{`"worktreePaths":[]`, `"bases":[]`, `"mergedIntoDefault":false`}},
		{"nil remotes", Remotes(workspace.Remotes{}), []string{`"remotes":[]`}},
		{"nil fetch results", FetchRemotes(workspace.FetchRemotesResult{}), []string{`"results":[]`, `"worktrees":[]`}},
		{"nil fast-forward preview arrays", FastForwardPreview(workspace.FastForwardPreview{}), []string{`"expiresAt":null`, `"items":[]`, `"worktree":null`, `"sessions":[]`, `"blockers":[]`, `"warnings":[]`}},
		{"nil push preview arrays", PushPreview(workspace.PushPreview{}), []string{`"expiresAt":null`, `"items":[]`, `"blockers":[]`, `"warnings":[]`}},
		{"nil branch delete arrays", BranchDeletePreview(workspace.BranchDeletePreview{Items: []workspace.BranchDeleteItem{{}}}), []string{`"expiresAt":null`, `"items":[]`, `"blockers":[]`}},
		{"nil branch delete results", BranchDeleteResult(workspace.BranchDeleteResult{}), []string{`"results":[]`, `"worktrees":[]`}},
		{"push result", PushResult(workspace.PushResult{}), []string{`"status":""`, `"worktrees":[]`}},
		{"unknown worktree details", Worktree(workspace.Worktree{}), []string{`"upstream":null`, `"ahead":null`, `"behind":null`, `"upstreamGone":false`, `"lastCommit":null`, `"changes":null`, `"operation":null`}},
		{"unfetched repository", Repository(workspace.Repository{}), []string{`"lastFetchedAt":null`}},
		{"nil preview arrays", Preview(workspace.Preview{}, nil), []string{`"sessions":[]`, `"archived":[]`, `"blockers":[]`}},
		{"nil release preview arrays", ReleasePreview(workspace.ReleasePreview{}, nil), []string{`"archived":[]`, `"ignored":[]`, `"blockers":[]`}},
		{"nil release result arrays", ReleaseResult(workspace.ReleaseResult{}), []string{`"recordIds":[]`}},
		{"nil rebuild preview arrays", RebuildPreview(workspace.RebuildPreview{}), []string{`"blockers":[]`}},
		{"nil report arrays", PushTestReport(push.TestReport{}), []string{`"removed":[]`, `"errors":[]`}},
		{"nil listing children", CodeListing(codeview.Listing{}), []string{`"children":[]`}},
		{"nil change entries", CodeChanges(codeview.Changes{}), []string{`"entries":[]`}},
		{"empty env names", LaunchSummaries([]launch.Summary{{EnvNames: []string{}}}, nil), []string{`"envNames":[]`}},
		{"unknown chat options", LaunchSummaries([]launch.Summary{{}}, func(string) []hybrid.ConfigOption { return nil }), []string{`"chatOptions":[]`}},
		{"nil create warnings", CreateResult(workspace.CreateResult{}, session.Info{}), []string{`"configWarnings":[]`}},
		{"repository without worktrees", Repository(workspace.Repository{}), []string{`"worktrees":[]`}},
		{"repository with no worktrees", Repository(workspace.Repository{Worktrees: []workspace.Worktree{}}), []string{`"worktrees":[]`}},
		{"preview", Preview(workspace.Preview{Sessions: []session.Info{}, Blockers: []string{}}, nil), []string{`"sessions":[]`, `"blockers":[]`}},
		{"test report", PushTestReport(push.TestReport{Removed: []string{}, Errors: []string{}}), []string{`"removed":[]`, `"errors":[]`}},
		{"listing", CodeListing(codeview.Listing{Children: []codeview.Node{}}), []string{`"children":[]`}},
		{"node link omitted", CodeListing(codeview.Listing{Children: []codeview.Node{{}}}), []string{`"mediaType":""`}},
		{"check maps", CodeCheck(codeview.Check{Snapshots: map[string]string{}}), []string{`"snapshots":{}`, `"directories":{}`}},
	} {
		got := encode(t, row.got)
		for _, want := range row.want {
			if !strings.Contains(got, want) {
				t.Errorf("%s: %s lacks %s", row.name, got, want)
			}
		}
	}
	if got := encode(t, CodeListing(codeview.Listing{Children: []codeview.Node{{}}})); strings.Contains(got, "linkTarget") {
		t.Errorf("empty link not omitted: %s", got)
	}
}

// Worktree converts nested details field by field; a nil one stays null.
func TestWorktreeCopiesDetails(t *testing.T) {
	in := workspace.Worktree{
		ID: "w", Upstream: new("origin/main"), Ahead: new(2), Behind: new(0),
		LastCommit: &workspace.Commit{Hash: "abc", Subject: "s", Author: "a", CommittedAt: "2026-01-02T03:04:05Z"},
		Changes:    &workspace.WorktreeChanges{Staged: 1, Unstaged: 2, Untracked: 3, Conflicted: 4},
		Operation:  new("rebase"),
	}
	got := encode(t, Worktree(in))
	for _, want := range []string{
		`"upstream":"origin/main"`, `"ahead":2`, `"behind":0`, `"upstreamGone":false`, `"operation":"rebase"`,
		`"lastCommit":{"hash":"abc","subject":"s","author":"a","committedAt":"2026-01-02T03:04:05Z"}`,
		`"changes":{"staged":1,"unstaged":2,"untracked":3,"conflicted":4}`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("%s lacks %s", got, want)
		}
	}
}

func TestSessionCopiesNestedValues(t *testing.T) {
	code := 3
	since := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	in := session.Info{
		ID: "abcdefgh", Status: session.StatusExited, ExitCode: &code,
		Workspace:   &session.Workspace{RepositoryID: "r", WorktreeID: "w", WorktreePath: "/p"},
		Attention:   &session.Attention{Kind: attentionkind.Question, Source: session.SourceAgent, Since: since},
		ReportedCwd: &session.ReportedCwd{Path: "/x", Host: "h", Remote: true, Since: since},
	}
	out := Session(in)
	if out.Status != "exited" || *out.ExitCode != 3 || out.Workspace.WorktreePath != "/p" || out.Attention.Kind != "question" || out.Attention.Source != "agent" || !out.ReportedCwd.Remote {
		t.Fatalf("converted %+v", out)
	}
	in.Workspace.WorktreeID = "changed"
	if out.Workspace.WorktreeID != "w" {
		t.Fatal("workspace aliases the service value")
	}
}

func TestSessionViews(t *testing.T) {
	got := encode(t, Session(session.Info{}))
	for _, want := range []string{`"state":"active"`, `"views":["terminal"]`, `"view":"terminal"`, `"transition":null`, `"agent":null`} {
		if !strings.Contains(got, want) {
			t.Errorf("ordinary session %s lacks %s", got, want)
		}
	}
	if strings.Contains(got, "hybrid") || strings.Contains(got, "archive") {
		t.Fatalf("ordinary session carries hybrid or archive: %s", got)
	}
	in := session.Info{Hybrid: &session.HybridInfo{Side: "tui", State: "active", Agent: "omp", AgentTitle: "omp", AgentIcon: "pi"}}
	out := Session(in)
	if got := encode(t, out.Agent); got != `{"name":"omp","title":"omp","icon":"pi","sessionId":null}` {
		t.Fatalf("unknown agent session: %s", got)
	}
	if len(out.Views) != 2 || out.View != contract.ViewTerminal || out.Transition != nil {
		t.Fatalf("terminal view %+v", out)
	}
	id := "ses_abc"
	in.Hybrid = &session.HybridInfo{Side: "tui", To: "acp", Phase: "interrupting", State: "active", Agent: "opencode", AgentSessionID: &id, Work: "读取 internal/wire/wire.go"}
	out = Session(in)
	if out.View != contract.ViewTerminal || out.Transition == nil || *out.Transition != (contract.SessionTransition{To: contract.ViewChat, Phase: contract.PhaseInterrupting}) {
		t.Fatalf("transition %+v", out.Transition)
	}
	if got := encode(t, out.Agent); !strings.Contains(got, `"sessionId":"ses_abc"`) || !strings.Contains(got, `"work":"读取 internal/wire/wire.go"`) {
		t.Fatalf("agent %s", got)
	}
	if got := encode(t, Session(session.Info{Hybrid: &session.HybridInfo{Side: "acp", State: "active"}}).Agent); strings.Contains(got, `"work"`) {
		t.Fatalf("empty work detail must be omitted: %s", got)
	}
	id = "changed"
	if *out.Agent.SessionID != "ses_abc" {
		t.Fatal("agent session ID aliases the service value")
	}
}

func TestArchivedSession(t *testing.T) {
	at := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	rec := hybrid.Record{
		ID: "abcdefgh", Title: "session title", ConfigID: "cfgomp01", Agent: "omp", Cwd: "/w", Cmd: []string{"omp"}, Side: hybrid.SideACP,
		Config: map[string]string{"model": "fast"}, State: hybrid.StateClosed, Activity: "turn", QueuedCount: 2,
		CreatedAt: at.Add(-time.Hour), UpdatedAt: at, PID: 42,
	}
	out := ArchivedSession(rec, Archived{ConfigMissing: true, AgentTitle: "Oh My Pi", AgentIcon: "pi"})
	got := encode(t, out)
	for _, want := range []string{`"state":"closed"`, `"status":"exited"`, `"generation":""`, `"clients":0`, `"preview":""`, `"listeningPorts":[]`,
		`"view":"chat"`, `"configMissing":true`, `"previousAgentSessionIds":[]`, `"interruptedAt":null`, `"queuedCount":2`, `"activity":"turn"`, `"title":"session title"`} {
		if !strings.Contains(got, want) {
			t.Errorf("archived %s lacks %s", got, want)
		}
	}
	if !out.LastActiveAt.Equal(at) || !out.Archive.UpdatedAt.Equal(at) || out.Agent.Title != "Oh My Pi" || out.Agent.SessionID != nil {
		t.Fatalf("archived %+v", out)
	}
	for _, private := range []string{"fast", "42", "pid", "process"} {
		if strings.Contains(got, private) {
			t.Errorf("archived %s exposes %s", got, private)
		}
	}
	rec.Name = "named"
	rec.SetAgentSessionID("one")
	rec.SetAgentSessionID("two")
	rec.InterruptedAt = &at
	rec.Workspace = &hybrid.WorkspaceRef{RepositoryID: "r1", WorktreeID: "w1"}
	out = ArchivedSession(rec, Archived{})
	if out.Title != "named" || *out.Agent.SessionID != "two" || len(out.Archive.PreviousAgentSessionIDs) != 1 || !out.Archive.InterruptedAt.Equal(at) {
		t.Fatalf("archived %+v", out)
	}
	if out.Workspace == nil || out.Workspace.RepositoryID != "r1" || out.Workspace.WorktreeID != "w1" {
		t.Fatalf("archived workspace %+v", out.Workspace)
	}
	if got := encode(t, contract.SessionConflict{Error: "x", Reason: "config_missing"}); strings.Contains(got, "candidates") || strings.Contains(got, "submission") {
		t.Fatalf("conflict without candidates: %s", got)
	}
	if got := AgentCandidates(nil); got == nil {
		t.Fatal("nil candidates")
	}
	in := ResumeInput(contract.ResumeRequest{ClientRequestID: "req-0001", View: contract.ViewTerminal, AgentSessionID: "ses_x", ConfirmWithoutEnv: true})
	if in != (workspace.HybridResumeInput{ClientRequestID: "req-0001", Side: "tui", AgentSessionID: "ses_x", ConfirmWithoutEnv: true}) {
		t.Fatalf("resume input %+v", in)
	}
	if got := ViewChangeInput(contract.ViewChangeRequest{To: contract.ViewChat}); got.To != hybrid.SideACP {
		t.Fatalf("view change input %+v", got)
	}
}

func TestRequestsConvertToServiceInputs(t *testing.T) {
	id := "abcdefgh"
	in := CreateInput(contractRequest(id))
	if in.Cwd != "/w" || in.Workspace == nil || in.Workspace.WorktreeID != "t" || *in.ConfigID != id || in.Side != "acp" {
		t.Fatalf("session input %+v", in)
	}
	if CreateInput(contractRequest("")).Workspace == nil {
		t.Fatal("reference dropped")
	}
	nw := CreateInput(contract.CreateSessionRequest{NewWorktree: &contract.NewWorktree{RepositoryID: "r", Mode: "new"}, Attachments: []contract.ChatAttachment{{Path: "/a"}}})
	if nw.NewWorktree == nil || nw.NewWorktree.RepositoryID != "r" || len(nw.Attachments) != 1 || nw.Attachments[0] != "/a" {
		t.Fatalf("create input %+v", nw)
	}
	if keys, ok := InputKeys([]string{"esc", "enter"}); !ok || string(keys[0]) != "\x1b" || string(keys[1]) != "\r" {
		t.Fatalf("keys %q", keys)
	}
	if _, ok := InputKeys([]string{"f13"}); ok {
		t.Fatal("unknown key accepted")
	}
}

func contractRequest(id string) contract.CreateSessionRequest {
	return contract.CreateSessionRequest{Cwd: "/w", Workspace: &contract.WorkspaceReference{RepositoryID: "r", WorktreeID: "t"}, ConfigID: &id, View: contract.ViewChat}
}

func TestChatAttachmentsConvert(t *testing.T) {
	input := SessionInput(contract.SessionInputRequest{
		ClientRequestID: "req-1234", Text: "",
		Attachments: []contract.ChatAttachment{{Path: "/tmp/webmux-upload-a/note.txt", Kind: contract.AttachmentFile}},
	}, "script")
	if len(input.Attachments) != 1 || input.Attachments[0].Path != "/tmp/webmux-upload-a/note.txt" {
		t.Fatalf("prompt input %+v", input)
	}
	event := ChatEvent(hybrid.Event{
		Kind: hybrid.EventMessage,
		Message: &hybrid.MessageChunk{
			MessageID: "u1", Role: hybrid.RoleUser,
			Attachments: []hybrid.Attachment{{Path: "/tmp/a.png", Name: "a.png", Kind: hybrid.AttachmentImage}},
		},
	})
	if event.Message == nil || len(event.Message.Attachments) != 1 || event.Message.Attachments[0].Kind != contract.AttachmentImage {
		t.Fatalf("message %+v", event.Message)
	}
	state := ChatState(hybrid.ChatState{
		Queue: hybrid.Queue{Items: []hybrid.QueueItem{{
			ID: "q1", Attachments: []hybrid.Attachment{{Path: "/tmp/b.txt", Name: "b.txt", Size: 3}},
		}}},
		PromptCapabilities: hybrid.PromptCapabilities{Image: true},
	})
	if len(state.Queue.Items) != 1 || state.Queue.Items[0].Attachments[0].Name != "b.txt" ||
		!state.PromptCapabilities.Image || state.PromptCapabilities.EmbeddedContext {
		t.Fatalf("state %+v", state)
	}
}

func TestPendingUpdate(t *testing.T) {
	if got := PendingUpdate("", contract.LauncherSystemd); got != nil {
		t.Fatalf("no installed version = %+v, want nil", got)
	}
	if got := encode(t, PendingUpdate("0.5.0", contract.LauncherSystemd)); got != `{"installedVersion":"0.5.0","launcher":"systemd"}` {
		t.Fatalf("pending update = %s", got)
	}
}
