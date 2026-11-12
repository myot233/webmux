package mcpcontrol

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/coder/websocket"
	"github.com/rainchan/webmux/internal/contract"
)

func chatFixture(t *testing.T, frames []contract.ChatFrame) Client {
	t.Helper()
	return websocketClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" || r.URL.Path != "/ws/s/s/chat" {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			return
		}
		conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{Subprotocols: []string{"webmux"}})
		if err != nil {
			t.Error(err)
			return
		}
		defer func() { _ = conn.CloseNow() }()
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		for _, frame := range frames {
			data, err := json.Marshal(frame)
			if err != nil {
				t.Error(err)
				return
			}
			if err := conn.Write(ctx, websocket.MessageText, data); err != nil {
				return
			}
		}
		_, _, err = conn.Read(ctx)
		if err == nil {
			t.Error("wait wrote input to session")
		}
	})
}
func start(id, queue string) contract.ChatEvent {
	return contract.ChatEvent{Kind: contract.ChatEventTurnStart, TurnStart: &contract.ChatTurnStart{TurnID: id, QueueItemID: queue}}
}
func end(id string) contract.ChatEvent {
	return contract.ChatEvent{Kind: contract.ChatEventTurnEnd, TurnEnd: &contract.ChatTurnEnd{TurnID: id, StopReason: "end_turn"}}
}
func message(id, role, text string) contract.ChatEvent {
	return contract.ChatEvent{Kind: contract.ChatEventMessage, Message: &contract.ChatMessageChunk{MessageID: id, Role: role, Text: text}}
}
func TestWaitPinsTurnAndExcludesThought(t *testing.T) {
	c := chatFixture(t, []contract.ChatFrame{
		{Type: "snapshot", From: 1, State: &contract.ChatState{Turn: contract.ChatTurn{TurnID: "target", StopReason: "end_turn"}}},
		{Type: "events", Items: []contract.ChatEvent{start("old", ""), message("old-message", "agent", "old reply"), end("old"), start("target", ""), message("thought", "thought", "secret"), message("reply", "agent", "new "), message("reply", "agent", "reply"), end("target")}},
	})
	value, err := c.wait(context.Background(), json.RawMessage(`{"sessionId":"s","turnId":"target"}`))
	if err != nil {
		t.Fatal(err)
	}
	result := value.(waitResult)
	if result.Status != "ended" || result.TurnID != "target" || result.Reply != "new reply" || result.WaitTimedOut || result.ReplyTruncated {
		t.Fatalf("wrong turn result: %+v", result)
	}
}
func TestWaitFollowsQueuedInput(t *testing.T) {
	c := chatFixture(t, []contract.ChatFrame{
		{Type: "snapshot", From: 1, State: &contract.ChatState{Turn: contract.ChatTurn{TurnID: "old", Running: true}, Queue: contract.ChatQueue{Items: []contract.ChatQueueItem{{ID: "queued"}}}}},
		{Type: "events", Items: []contract.ChatEvent{start("old", ""), message("old-reply", "agent", "old")}},
		{Type: "events", Items: []contract.ChatEvent{end("old"), start("new", "queued"), message("answer", "agent", "done"), end("new")}},
	})
	value, err := c.wait(context.Background(), json.RawMessage(`{"sessionId":"s","queueItemId":"queued"}`))
	if err != nil {
		t.Fatal(err)
	}
	result := value.(waitResult)
	if result.TurnID != "new" || result.Reply != "done" || result.QueueItemID != "queued" {
		t.Fatalf("queue result %+v", result)
	}
}
func TestWaitTimeoutOnlyEndsObservation(t *testing.T) {
	c := chatFixture(t, []contract.ChatFrame{{Type: "snapshot", From: 1, State: &contract.ChatState{Turn: contract.ChatTurn{TurnID: "target", Running: true}}}, {Type: "events", Items: []contract.ChatEvent{start("target", "")}}})
	value, err := c.wait(context.Background(), json.RawMessage(`{"sessionId":"s","turnId":"target","timeoutMs":200}`))
	if err != nil {
		t.Fatal(err)
	}
	result := value.(waitResult)
	if !result.WaitTimedOut || result.Status != "running" || result.StopReason != "" {
		t.Fatalf("timeout result %+v", result)
	}
}
func TestWaitReturnsPendingUserRequest(t *testing.T) {
	request := contract.ChatRequest{RequestID: "approval", Kind: "permission", Title: "Run command?"}
	c := chatFixture(t, []contract.ChatFrame{{Type: "snapshot", From: 1, State: &contract.ChatState{Turn: contract.ChatTurn{TurnID: "target", Running: true}, Requests: []contract.ChatRequest{request}}}, {Type: "events", Items: []contract.ChatEvent{start("target", "")}}})
	value, err := c.wait(context.Background(), json.RawMessage(`{"sessionId":"s","turnId":"target"}`))
	if err != nil {
		t.Fatal(err)
	}
	result := value.(waitResult)
	if result.Status != "waiting_for_user" || len(result.Requests) != 1 || result.Requests[0].RequestID != "approval" {
		t.Fatalf("request result %+v", result)
	}
}
func TestWaitRefusesMissingHistory(t *testing.T) {
	tests := []struct {
		name, input string
		frames      []contract.ChatFrame
	}{
		{"first truncated", `{"sessionId":"s","firstTurn":true}`, []contract.ChatFrame{{Type: "snapshot", From: 10, Truncated: true}}},
		{"other turn", `{"sessionId":"s","turnId":"missing"}`, []contract.ChatFrame{{Type: "snapshot", From: 1, State: &contract.ChatState{Turn: contract.ChatTurn{TurnID: "other", Running: true}}}, {Type: "events", Items: []contract.ChatEvent{start("other", "")}}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := chatFixture(t, tt.frames)
			if _, err := c.wait(context.Background(), json.RawMessage(tt.input)); err == nil {
				t.Fatal("missing history silently adopted unrelated turn")
			}
		})
	}
}
func TestWaitReplyBoundPreservesUTF8(t *testing.T) {
	c := chatFixture(t, []contract.ChatFrame{{Type: "snapshot", From: 1, State: &contract.ChatState{}}, {Type: "events", Items: []contract.ChatEvent{start("target", ""), message("reply", "agent", strings.Repeat("字", 30000)), end("target")}}})
	value, err := c.wait(context.Background(), json.RawMessage(`{"sessionId":"s","firstTurn":true}`))
	if err != nil {
		t.Fatal(err)
	}
	result := value.(waitResult)
	if len(result.Reply) > 64<<10 || !result.ReplyTruncated || !utf8.ValidString(result.Reply) {
		t.Fatalf("invalid bounded reply: len=%d truncated=%v", len(result.Reply), result.ReplyTruncated)
	}
}
func TestWaitRejectsAmbiguousSelectors(t *testing.T) {
	c := testClient(t, func(*http.Request) string { t.Fatal("invalid wait reached API"); return "" })
	for _, raw := range []string{`{"sessionId":"s"}`, `{"sessionId":"s","turnId":"t","firstTurn":true}`, `{"sessionId":"s","until":"exit","turnId":"t"}`, `{"sessionId":"s","firstTurn":true,"timeoutMs":60001}`} {
		if _, err := c.wait(context.Background(), json.RawMessage(raw)); err == nil {
			t.Fatalf("accepted %s", raw)
		}
	}
}
func TestWaitExitDoesNotTreatIdleAsCompletion(t *testing.T) {
	c := websocketClient(t, func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{Subprotocols: []string{"webmux"}})
		if err != nil {
			t.Error(err)
			return
		}
		defer func() { _ = conn.CloseNow() }()
		ctx, cancel := context.WithTimeout(r.Context(), time.Second)
		defer cancel()
		code := 7
		for _, frame := range []contract.SessionEvent{{Type: "snapshot", Sessions: []contract.SessionInfo{{ID: "s", Status: "idle"}}}, {Type: "updated", Session: &contract.SessionInfo{ID: "s", Status: "exited", ExitCode: &code}}} {
			data, err := json.Marshal(frame)
			if err != nil {
				t.Error(err)
				return
			}
			if err := conn.Write(ctx, websocket.MessageText, data); err != nil {
				return
			}
		}
		_, _, _ = conn.Read(ctx)
	})
	value, err := c.wait(context.Background(), json.RawMessage(`{"sessionId":"s","until":"exit"}`))
	if err != nil {
		t.Fatal(err)
	}
	result := value.(waitResult)
	if result.Status != "exited" || result.ExitCode == nil || *result.ExitCode != 7 {
		t.Fatalf("exit result %+v", result)
	}
}

func TestWaitUsesTurnEndBackgroundStateBeforeStateFrame(t *testing.T) {
	ended := end("target")
	ended.TurnEnd.Unfinished = true
	c := chatFixture(t, []contract.ChatFrame{
		{Type: "snapshot", From: 1, State: &contract.ChatState{Turn: contract.ChatTurn{TurnID: "target", Running: true, Unfinished: false}}},
		{Type: "events", Items: []contract.ChatEvent{start("target", "")}},
		{Type: "events", Items: []contract.ChatEvent{ended}},
		{Type: "state", State: &contract.ChatState{Turn: contract.ChatTurn{TurnID: "target", Unfinished: true}}},
	})
	value, err := c.wait(context.Background(), json.RawMessage(`{"sessionId":"s","turnId":"target"}`))
	if err != nil {
		t.Fatal(err)
	}
	if result := value.(waitResult); !result.Unfinished || result.Status != "ended" {
		t.Fatalf("stale unfinished state: %+v", result)
	}
}
func TestWaitKeepsIncompleteHistoryFlagAcrossLiveChunks(t *testing.T) {
	c := chatFixture(t, []contract.ChatFrame{
		{Type: "snapshot", From: 10, Truncated: true, State: &contract.ChatState{Turn: contract.ChatTurn{TurnID: "target", Running: true}}},
		{Type: "events", Items: []contract.ChatEvent{message("reply", "agent", "retained prefix")}},
		{Type: "events", Items: []contract.ChatEvent{message("reply", "agent", "live suffix"), end("target")}},
	})
	value, err := c.wait(context.Background(), json.RawMessage(`{"sessionId":"s","turnId":"target"}`))
	if err != nil {
		t.Fatal(err)
	}
	result := value.(waitResult)
	if result.Reply != "live suffix" || !result.ReplyTruncated {
		t.Fatalf("partial reply advertised as complete: %+v", result)
	}
}

func TestWaitReadsLargeReplayButReturnsOnlySelectedReply(t *testing.T) {
	c := chatFixture(t, []contract.ChatFrame{
		{Type: "snapshot", From: 1, State: &contract.ChatState{Turn: contract.ChatTurn{TurnID: "target"}}},
		{Type: "events", Items: []contract.ChatEvent{start("old", ""), message("old-reply", "agent", strings.Repeat("x", 5<<20)), end("old"), start("target", ""), message("answer", "agent", "short answer"), end("target")}},
	})
	value, err := c.wait(context.Background(), json.RawMessage(`{"sessionId":"s","turnId":"target"}`))
	if err != nil {
		t.Fatal(err)
	}
	result := value.(waitResult)
	if result.Reply != "short answer" || result.ReplyTruncated || result.Status != "ended" {
		t.Fatalf("large unrelated history affected reply: %+v", result)
	}
}
