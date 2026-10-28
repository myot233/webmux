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
		{"nil env names", LaunchSummaries([]launch.Summary{{}}), []string{`"envNames":[]`, `"cmd":[]`}},
		{"nil branches", Branches(workspace.Branches{Branches: []workspace.Branch{{}}}), []string{`"worktreePaths":[]`, `"bases":[]`}},
		{"nil preview arrays", Preview(workspace.Preview{}), []string{`"sessions":[]`, `"blockers":[]`}},
		{"nil report arrays", PushTestReport(push.TestReport{}), []string{`"removed":[]`, `"errors":[]`}},
		{"nil listing children", CodeListing(codeview.Listing{}), []string{`"children":[]`}},
		{"nil change entries", CodeChanges(codeview.Changes{}), []string{`"entries":[]`}},
		{"empty env names", LaunchSummaries([]launch.Summary{{EnvNames: []string{}}}), []string{`"envNames":[]`}},
		{"repository without worktrees", Repository(workspace.Repository{}), []string{`"worktrees":[]`}},
		{"repository with no worktrees", Repository(workspace.Repository{Worktrees: []workspace.Worktree{}}), []string{`"worktrees":[]`}},
		{"preview", Preview(workspace.Preview{Sessions: []session.Info{}, Blockers: []string{}}), []string{`"sessions":[]`, `"blockers":[]`}},
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

func TestSessionHybrid(t *testing.T) {
	if got := encode(t, Session(session.Info{})); strings.Contains(got, "hybrid") {
		t.Fatalf("ordinary session carries hybrid: %s", got)
	}
	in := session.Info{Hybrid: &session.HybridInfo{Side: "tui", State: "active", Agent: "omp"}}
	got := encode(t, Session(in).Hybrid)
	if want := `{"side":"tui","dormant":false,"state":"active","agent":"omp","agentSessionId":null}`; got != want {
		t.Fatalf("unknown agent session: %s, want %s", got, want)
	}
	id := "ses_abc"
	in.Hybrid = &session.HybridInfo{Side: "acp", Phase: "loading", Dormant: true, State: "active", Agent: "opencode", AgentSessionID: &id}
	out := Session(in)
	got = encode(t, out.Hybrid)
	if want := `{"side":"acp","phase":"loading","dormant":true,"state":"active","agent":"opencode","agentSessionId":"ses_abc"}`; got != want {
		t.Fatalf("known agent session: %s, want %s", got, want)
	}
	id = "changed"
	if *out.Hybrid.AgentSessionID != "ses_abc" {
		t.Fatal("agent session ID aliases the service value")
	}
}

func TestHybridRecord(t *testing.T) {
	at := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	rec := hybrid.Record{
		ID: "abcdefgh", Name: "omp", ConfigID: "cfgomp01", Agent: "omp", Cwd: "/w", Cmd: []string{"omp"}, Side: hybrid.SideACP,
		Config: map[string]string{"model": "fast"}, State: hybrid.StateClosed, Activity: "turn", QueuedCount: 2,
		CreatedAt: at, UpdatedAt: at, PID: 42,
	}
	got := encode(t, HybridRecord(rec, true))
	for _, want := range []string{`"configMissing":true`, `"agentSessionId":null`, `"previousIds":[]`, `"interruptedAt":null`, `"queuedCount":2`, `"activity":"turn"`} {
		if !strings.Contains(got, want) {
			t.Errorf("record %s lacks %s", got, want)
		}
	}
	for _, private := range []string{"fast", "42", "pid", "process"} {
		if strings.Contains(got, private) {
			t.Errorf("record %s exposes %s", got, private)
		}
	}
	rec.SetAgentSessionID("one")
	rec.SetAgentSessionID("two")
	rec.InterruptedAt = &at
	out := HybridRecord(rec, false)
	if *out.AgentSessionID != "two" || len(out.PreviousIDs) != 1 || out.PreviousIDs[0] != "one" || !out.InterruptedAt.Equal(at) || out.ConfigMissing {
		t.Fatalf("record %+v", out)
	}
	list := HybridRecords([]hybrid.Record{rec, {ID: "bcdefghi", ConfigID: "gone"}}, func(id string) bool { return id == "gone" })
	if len(list) != 2 || list[0].ConfigMissing || !list[1].ConfigMissing || encode(t, HybridRecords(nil, nil)) != "[]" {
		t.Fatalf("records %+v", list)
	}
	if got := encode(t, contract.HybridConflict{Error: "x", Reason: "config_missing"}); strings.Contains(got, "candidates") {
		t.Fatalf("conflict without candidates: %s", got)
	}
	if got := HybridCandidates(nil); got == nil {
		t.Fatal("nil candidates")
	}
	in := HybridResumeInput(contract.HybridResumeRequest{ClientRequestID: "req-0001", Side: contract.HybridSideTUI, AgentSessionID: "ses_x", ConfirmWithoutEnv: true})
	if in != (workspace.HybridResumeInput{ClientRequestID: "req-0001", Side: "tui", AgentSessionID: "ses_x", ConfirmWithoutEnv: true}) {
		t.Fatalf("resume input %+v", in)
	}
}

func TestRequestsConvertToServiceInputs(t *testing.T) {
	id := "abcdefgh"
	in := SessionInput(contractRequest(id))
	if in.Cwd != "/w" || in.Workspace == nil || in.Workspace.WorktreeID != "t" || *in.ConfigID != id || in.StartSide != "acp" {
		t.Fatalf("session input %+v", in)
	}
	if SessionInput(contractRequest("")).Workspace == nil {
		t.Fatal("reference dropped")
	}
}

func contractRequest(id string) contract.CreateSessionRequest {
	return contract.CreateSessionRequest{Cwd: "/w", Workspace: &contract.WorkspaceReference{RepositoryID: "r", WorktreeID: "t"}, ConfigID: &id, StartSide: contract.HybridSideACP}
}

func TestChatAttachmentsConvert(t *testing.T) {
	input := ChatPromptInput(contract.ChatPromptRequest{
		ClientRequestID: "req-1234", Text: "",
		Attachments: []contract.ChatAttachment{{Path: "/tmp/webmux-upload-a/note.txt", Kind: contract.AttachmentFile}},
	})
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
