//go:build linux || darwin

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/rainchan/webmux/internal/acp"
	"github.com/rainchan/webmux/internal/api"
	"github.com/rainchan/webmux/internal/artifacts"
	"github.com/rainchan/webmux/internal/codeview"
	"github.com/rainchan/webmux/internal/conversation"
	"github.com/rainchan/webmux/internal/files"
	"github.com/rainchan/webmux/internal/handoff"
	"github.com/rainchan/webmux/internal/hybrid"
	"github.com/rainchan/webmux/internal/launch"
	"github.com/rainchan/webmux/internal/localclient"
	"github.com/rainchan/webmux/internal/session"
	"github.com/rainchan/webmux/internal/threadread"
	"github.com/rainchan/webmux/internal/workspace"
)

// Full live SQLite -> HTTP API -> child CLI stdio MCP -> independent fake
// provider. No test talks to a developer server or a real provider account.
func TestHandoffMCPReadsOmittedHistoryAndSurvivesProviderReopen(t *testing.T) {
	agent := fakeAgent(t)
	root, err := os.MkdirTemp("/tmp", "wmxmcp")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(root)
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	state := filepath.Join(root, "state")
	socket := filepath.Join(root, "api.sock")
	store := conversation.Open(conversation.Options{StateDir: state})
	defer store.Close()
	manager := session.NewManager(session.Options{DefaultCwd: root})
	defer manager.Shutdown(context.Background())
	launches := launch.Open(state)
	defer launches.Close()
	folders := artifacts.New(artifacts.Options{StateDir: state, Manager: manager})
	defer folders.Close()
	exe, _ := os.Executable()
	descriptor := func(ctx context.Context, scope conversation.ReaderScope) []acp.MCPServer {
		if _, err := store.Preview(ctx, scope.HandoffID); err != nil {
			return nil
		}
		return []acp.MCPServer{{Name: "webmux-history", Command: exe, Args: []string{"mcp-serve", "--socket", socket, "--conversation", scope.ConversationID, "--handoff", scope.HandoffID}, Env: []acp.MCPEnv{{Name: "WEBMUX_TEST_EXEC", Value: "argv"}}}}
	}
	hybrids := hybrid.New(hybrid.Options{StateDir: state, Manager: manager, Conversations: store, HistoryReader: descriptor,
		MCPServers: func(context.Context) []acp.MCPServer {
			return []acp.MCPServer{{Name: "webmux", Command: exe, Args: []string{"mcp", "--socket", socket}, Env: []acp.MCPEnv{{Name: "WEBMUX_TEST_EXEC", Value: "argv"}}}}
		},
	})
	defer hybrids.Close()
	workspaces := workspace.New(state, manager, launches, root)
	defer workspaces.Close()
	workspaces.UseHybrid(hybrids)
	service := handoff.New(handoff.Options{Conversations: store, Launch: launches, Hybrid: hybrids, Workspace: workspaces, TokenCap: 16000, MaxPromptBytes: hybrid.MaxPromptBytes, MaxPromptWireBytes: hybrid.MaxPromptWireBytes})
	defer service.Close()
	handler := api.New(api.Deps{Manager: manager, Workspace: workspaces, Code: codeview.New(manager, folders), Files: files.New(root), Conversations: store, Handoffs: service, Launch: launches, Hybrid: hybrids})
	listener, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatal(err)
	}
	server := &http.Server{Handler: handler, ReadHeaderTimeout: time.Second}
	defer server.Close()
	go func() { _ = server.Serve(listener) }()
	client := localclient.New(localclient.UnixDialer{Path: socket})
	defer client.Close()
	agentStore := filepath.Join(root, "provider")
	config, err := launches.Create(launch.Input{Name: "fake target", Cmd: []string{agent}, Hybrid: true, Env: map[string]string{"FAKEAGENT_STORE": agentStore, "FAKEAGENT_TURN_DELAY": "2ms", "FAKEAGENT_CONTROL_MCP": "1"}})
	if err != nil {
		t.Fatal(err)
	}
	source, err := hybrids.Create(ctx, hybrid.CreateInput{Cwd: root, ConfigID: config.ID, Cmd: config.Cmd, Env: config.Env, StartSide: hybrid.SideACP})
	if err != nil {
		t.Fatal(err)
	}
	assertControlMCP(t, agentStore, "new", *source.Info().Hybrid.AgentSessionID)
	binding, err := store.Binding(ctx, source.Info().ID)
	if err != nil {
		t.Fatal(err)
	}
	run, err := store.StartRun(ctx, binding.ProviderThreadID, "old-history")
	if err != nil {
		t.Fatal(err)
	}
	texts := map[string]string{}
	for i := range 45 {
		role := "agent"
		if i%2 == 0 {
			role = "user"
		}
		text := fmt.Sprintf("old-%d-", i) + strings.Repeat("中文🙂", 1600)
		item, err := store.WriteActivity(ctx, conversation.ActivityInput{ProviderThreadID: binding.ProviderThreadID, RunID: run.ID, LocalKey: strconv.Itoa(i), Kind: "message", Role: role, Status: "completed", Source: "local", Text: text})
		if err != nil {
			t.Fatal(err)
		}
		texts[item] = text
	}
	if err := store.FinishRun(ctx, run.ID, "completed"); err != nil {
		t.Fatal(err)
	}
	p, err := service.Preview(ctx, binding.ConversationID, handoff.Request{ClientRequestID: "mcp-preview", ConfigID: config.ID, Fork: true, UserText: "independent task"})
	if err != nil {
		t.Fatal(err)
	}
	if !p.HistoryToolAvailable || len(p.OmittedItemIDs) == 0 {
		t.Fatal("omissions/tool capability missing")
	}
	request := handoff.MessageRequest{ClientRequestID: "mcp-send", HandoffID: p.ID, Revision: p.Revision, Text: p.Request.UserText, ConfigID: config.ID, Location: p.Request.Location, AgentConfig: p.Request.AgentConfig, Attachments: p.Request.Attachments}
	if _, err := service.Deliver(ctx, binding.ConversationID, request); err != nil {
		t.Fatal(err)
	}
	var delivery conversation.Execution
	for {
		delivery, err = service.Delivery(ctx, p.ID)
		if err != nil {
			t.Fatal(err)
		}
		if delivery.Status != "pending" {
			break
		}
		select {
		case <-ctx.Done():
			t.Fatalf("delivery timeout: %+v", delivery)
		case <-time.After(10 * time.Millisecond):
		}
	}
	if delivery.Status != "inline" {
		t.Fatalf("delivery %+v", delivery)
	}
	assertControlMCP(t, agentStore, "new", delivery.NativeThreadID)
	var initial []json.RawMessage
	path := filepath.Join(agentStore, "mcp-new-"+delivery.NativeThreadID+".json")
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(body, &initial); err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, page := range initial {
		var data struct {
			IsError bool              `json:"isError"`
			Value   threadread.Result `json:"structuredContent"`
		}
		if err := json.Unmarshal(page, &data); err != nil || data.IsError {
			t.Fatalf("MCP result %s %v", page, err)
		}
		for _, item := range data.Value.Items {
			if seen[item.ActivityID] || !utf8.ValidString(item.Text) {
				t.Fatal("duplicate/invalid UTF8")
			}
			seen[item.ActivityID] = true
		}
	}
	if len(seen) != len(texts) {
		t.Fatalf("paged %d, want %d", len(seen), len(texts))
	}
	for _, id := range p.OmittedItemIDs {
		if !seen[id] {
			t.Fatal("omitted item inaccessible")
		}
	}
	reader := threadread.Reader{Client: client, ConversationID: binding.ConversationID, HandoffID: p.ID}
	id := p.OmittedItemIDs[0]
	offset := int64(0)
	var joined strings.Builder
	for {
		raw, _ := json.Marshal(threadread.Arguments{View: "activity", ItemID: id, TextOffset: offset, AfterPosition: "0", Limit: 1, MaxBytesPerItem: 401})
		got, err := reader.Read(ctx, raw)
		if err != nil {
			t.Fatal(err)
		}
		if len(got.Items) != 1 || !got.Items[0].Omitted || !utf8.ValidString(got.Items[0].Text) {
			t.Fatal("invalid long item")
		}
		joined.WriteString(got.Items[0].Text)
		if got.Done {
			break
		}
		offset, _ = strconv.ParseInt(got.NextTextOffset, 10, 64)
	}
	if joined.String() != texts[id] {
		t.Fatal("long item did not reconstruct")
	}
	for _, raw := range []string{`{"view":"activity","conversationId":"other"}`, `{"view":"activity","afterPosition":"01"}`, `{"view":"activity","limit":101}`, `{"view":"activity","textOffset":-1}`, `{"view":"activity","limit":100,"maxBytesPerItem":65536}`} {
		if _, err := reader.Read(ctx, json.RawMessage(raw)); err == nil {
			t.Fatalf("accepted %s", raw)
		}
	}
	// A valid ID in a different Conversation and future source turns are refused.
	targetPage, err := store.Activities(ctx, p.TargetConversationID, 0, 20, 4000)
	if err != nil {
		t.Fatal(err)
	}
	if len(targetPage.Items) > 0 {
		raw, _ := json.Marshal(map[string]any{"view": "activity", "itemId": targetPage.Items[0].Activity.ID})
		if _, err := reader.Read(ctx, raw); err == nil {
			t.Fatal("cross-conversation read")
		}
	}
	laterRun, err := store.StartRun(ctx, binding.ProviderThreadID, "future")
	if err != nil {
		t.Fatal(err)
	}
	later, err := store.WriteActivity(ctx, conversation.ActivityInput{ProviderThreadID: binding.ProviderThreadID, RunID: laterRun.ID, LocalKey: "future", Kind: "message", Role: "user", Source: "local", Status: "completed", Text: "future secret"})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.FinishRun(ctx, laterRun.ID, "completed"); err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(map[string]any{"view": "activity", "itemId": later})
	if _, err := reader.Read(ctx, raw); err == nil {
		t.Fatal("future turn visible")
	}
	// The provider owns the child, and every reopen receives the same scope.
	wait := func() {
		for {
			sub, err := hybrids.Subscribe(delivery.RuntimeID, nil)
			if err != nil {
				t.Fatal(err)
			}
			running := sub.Snapshot.State.Turn.Running
			sub.Close()
			if !running {
				return
			}
			select {
			case <-ctx.Done():
				t.Fatal("turn timeout")
			case <-time.After(10 * time.Millisecond):
			}
		}
	}
	wait()
	for _, method := range []string{"load", "resume"} {
		previousMethod := "new"
		if method == "resume" {
			previousMethod = "load"
		}
		var pids []int
		for _, prefix := range []string{"mcp-pid-", "mcp-control-pid-"} {
			pidBody, err := os.ReadFile(filepath.Join(agentStore, prefix+previousMethod+"-"+delivery.NativeThreadID))
			if err != nil {
				t.Fatal(err)
			}
			pid, err := strconv.Atoi(string(pidBody))
			if err != nil {
				t.Fatal(err)
			}
			pids = append(pids, pid)
		}
		if err := manager.Close(delivery.RuntimeID); err != nil {
			t.Fatal(err)
		}
		for {
			rec, e := hybrids.Record(delivery.RuntimeID)
			_, shellErr := manager.Get(delivery.RuntimeID)
			if e == nil && rec.State == hybrid.StateClosed && rec.PID == 0 && shellErr != nil {
				break
			}
			select {
			case <-ctx.Done():
				t.Fatal("close timeout")
			case <-time.After(10 * time.Millisecond):
			}
		}
		for _, pid := range pids {
			if err := syscall.Kill(pid, 0); !errors.Is(err, syscall.ESRCH) {
				t.Fatalf("MCP child still exists after close: %d %v", pid, err)
			}
		}
		if _, err := hybrids.Resume(ctx, delivery.RuntimeID, hybrid.ResumeInput{ClientRequestID: "mcp-" + method, Side: hybrid.SideACP, Env: config.Env, RequireNativeResume: method == "resume"}); err != nil {
			t.Fatal(err)
		}
		assertControlMCP(t, agentStore, method, delivery.NativeThreadID)
		if _, err := os.Stat(filepath.Join(agentStore, "mcp-"+method+"-"+delivery.NativeThreadID+".json")); err != nil {
			t.Fatal(err)
		}
	}
	if err := manager.Close(delivery.RuntimeID); err != nil {
		t.Fatal(err)
	}

	// Reuse an initially ordinary active target twice. Its first handoff must add
	// MCP, and its second must replace the old frozen --handoff scope by resume.
	ordinary, err := hybrids.Create(ctx, hybrid.CreateInput{Cwd: root, ConfigID: config.ID, Cmd: config.Cmd, Env: config.Env, StartSide: hybrid.SideACP})
	if err != nil {
		t.Fatal(err)
	}
	ordinaryBinding, err := store.Binding(ctx, ordinary.Info().ID)
	if err != nil {
		t.Fatal(err)
	}
	for attempt := range 2 {
		run, err := store.StartRun(ctx, ordinaryBinding.ProviderThreadID, fmt.Sprintf("active-history-%d", attempt))
		if err != nil {
			t.Fatal(err)
		}
		for i := range 30 {
			if _, err := store.WriteActivity(ctx, conversation.ActivityInput{ProviderThreadID: ordinaryBinding.ProviderThreadID, RunID: run.ID, LocalKey: fmt.Sprintf("%d-%d", attempt, i), Kind: "message", Role: "agent", Status: "completed", Source: "local", Text: fmt.Sprintf("scope-%d-item-%d ", attempt, i) + strings.Repeat("x", 300)}); err != nil {
				t.Fatal(err)
			}
		}
		if err := store.FinishRun(ctx, run.ID, "completed"); err != nil {
			t.Fatal(err)
		}
		q, err := service.Preview(ctx, ordinaryBinding.ConversationID, handoff.Request{ClientRequestID: fmt.Sprintf("active-preview-%d", attempt), ConfigID: config.ID, UserText: fmt.Sprintf("task-%d", attempt)})
		if err != nil {
			t.Fatal(err)
		}
		if q.TargetProviderThreadID != ordinaryBinding.ProviderThreadID || !q.HistoryToolAvailable {
			t.Fatal("active native target was not reused")
		}
		if _, err := service.Deliver(ctx, ordinaryBinding.ConversationID, handoff.MessageRequest{ClientRequestID: fmt.Sprintf("active-send-%d", attempt), HandoffID: q.ID, Revision: q.Revision, Text: q.Request.UserText, ConfigID: q.Request.ConfigID, AgentConfig: q.Request.AgentConfig, Location: q.Request.Location, Attachments: q.Request.Attachments}); err != nil {
			t.Fatal(err)
		}
		var e conversation.Execution
		for {
			e, err = service.Delivery(ctx, q.ID)
			if err != nil {
				t.Fatal(err)
			}
			if e.Status != "pending" {
				break
			}
			select {
			case <-ctx.Done():
				t.Fatal("active delivery timeout")
			case <-time.After(10 * time.Millisecond):
			}
		}
		if e.Status != "inline" || e.RuntimeID != ordinary.Info().ID {
			t.Fatalf("active delivery: %+v", e)
		}
		data, err := os.ReadFile(filepath.Join(agentStore, "mcp-resume-"+e.NativeThreadID+".json"))
		if err != nil {
			t.Fatal(err)
		}
		var pages []struct {
			Value threadread.Result `json:"structuredContent"`
		}
		if err := json.Unmarshal(data, &pages); err != nil || len(pages) == 0 {
			t.Fatal("missing MCP results")
		}
		for _, page := range pages {
			if page.Value.SourceConversationID != q.SourceConversationID || page.Value.SourcePosition != strconv.FormatInt(q.SourcePosition, 10) {
				t.Fatal("active target kept previous MCP source point")
			}
		}
		for {
			sub, err := hybrids.Subscribe(e.RuntimeID, nil)
			if err != nil {
				t.Fatal(err)
			}
			running := sub.Snapshot.State.Turn.Running
			sub.Close()
			if !running {
				break
			}
			select {
			case <-ctx.Done():
				t.Fatal("active turn timeout")
			case <-time.After(10 * time.Millisecond):
			}
		}
	}
	if err := manager.Close(ordinary.Info().ID); err != nil {
		t.Fatal(err)
	}
	// Disabling the optional descriptor does not disable handoff budget/delivery.
	plainManager := session.NewManager(session.Options{DefaultCwd: root})
	defer plainManager.Shutdown(context.Background())
	plain := hybrid.New(hybrid.Options{StateDir: filepath.Join(root, "plain-state"), Manager: plainManager})
	defer plain.Close()
	if plain.HistoryReaderAvailable(ctx, conversation.ReaderScope{ConversationID: binding.ConversationID, HandoffID: p.ID}) {
		t.Fatal("nil reader advertised")
	}
}

// The provider discovers the control tools and calls the running local API,
// both before handoff and alongside its scoped history reader after reopening.
func assertControlMCP(t *testing.T, store, method, sid string) {
	t.Helper()
	body, err := os.ReadFile(filepath.Join(store, "mcp-control-"+method+"-"+sid+".json"))
	if err != nil {
		t.Fatal(err)
	}
	var results []json.RawMessage
	if err := json.Unmarshal(body, &results); err != nil || len(results) != 2 {
		t.Fatalf("control MCP results %s: %v", body, err)
	}
	var listed struct{ Tools []struct{ Name string } }
	if err := json.Unmarshal(results[0], &listed); err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{}
	for _, tool := range listed.Tools {
		names[tool.Name] = true
	}
	for _, name := range []string{"capabilities", "session_list", "session_get", "session_launch", "session_send", "session_interrupt", "session_read", "session_wait"} {
		if !names[name] {
			t.Fatalf("missing control tool %s in %s", name, results[0])
		}
	}
	var called struct {
		IsError bool            `json:"isError"`
		Content json.RawMessage `json:"structuredContent"`
	}
	if err := json.Unmarshal(results[1], &called); err != nil || called.IsError || len(called.Content) == 0 {
		t.Fatalf("capabilities failed: %s %v", results[1], err)
	}
}
