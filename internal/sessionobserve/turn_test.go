package sessionobserve

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/coder/websocket"
	"github.com/rainchan/webmux/internal/contract"
	"github.com/rainchan/webmux/internal/localclient"
)

type fixtureDialer struct{ address string }

func (d fixtureDialer) DialContext(ctx context.Context) (net.Conn, error) {
	return (&net.Dialer{}).DialContext(ctx, "tcp", d.address)
}

func fixtureClient(t *testing.T, handler http.HandlerFunc) *localclient.Client {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	client := localclient.New(fixtureDialer{strings.TrimPrefix(server.URL, "http://")})
	t.Cleanup(client.Close)
	return client
}

func replay(t *testing.T, frames ...contract.ChatFrame) *ChatSubscription {
	t.Helper()
	client := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) {
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
		_, _, _ = conn.Read(ctx)
	})
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	subscription, err := OpenChat(ctx, client, "s", 64<<20)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(subscription.Close)
	return subscription
}

func snapshot(state *contract.ChatState) contract.ChatFrame {
	return contract.ChatFrame{Type: contract.ChatFrameSnapshot, From: 1, State: state}
}

func events(items ...contract.ChatEvent) contract.ChatFrame {
	return contract.ChatFrame{Type: contract.ChatFrameEvents, Items: items}
}

func start(id, prompt, queue string) contract.ChatEvent {
	return contract.ChatEvent{Kind: contract.ChatEventTurnStart, TurnStart: &contract.ChatTurnStart{TurnID: id, PromptID: prompt, QueueItemID: queue}}
}

func end(id string) contract.ChatEvent {
	return contract.ChatEvent{Kind: contract.ChatEventTurnEnd, TurnEnd: &contract.ChatTurnEnd{TurnID: id, StopReason: "end_turn"}}
}

func agent(id, text string) contract.ChatEvent {
	return contract.ChatEvent{Kind: contract.ChatEventMessage, Message: &contract.ChatMessageChunk{MessageID: id, Role: "agent", Text: text}}
}

func queued(ids ...string) *contract.ChatState {
	state := &contract.ChatState{}
	for _, id := range ids {
		state.Queue.Items = append(state.Queue.Items, contract.ChatQueueItem{ID: id})
	}
	return state
}

func TestQueueTransitions(t *testing.T) {
	tests := []struct {
		name     string
		phase    AttachPhase
		frames   []contract.ChatFrame
		wantErr  error
		wantTurn string
	}{
		{
			name: "seen then removed", frames: []contract.ChatFrame{
				snapshot(queued("q")), events(), {Type: "state", State: queued()},
			}, wantErr: ErrQueueRemoved,
		},
		{
			name: "initial absence before submission is not removal", phase: BeforeSubmit,
			frames: []contract.ChatFrame{
				snapshot(queued("old")), events(), {Type: "state", State: queued("old", "q")}, {Type: "state", State: queued("old")},
			}, wantErr: ErrQueueRemoved,
		},
		{
			name: "initial absence before submission can start later", phase: BeforeSubmit,
			frames: []contract.ChatFrame{
				snapshot(queued()), events(), events(start("target", "p", "q")), {Type: "state", State: queued()}, events(end("target")),
			}, wantTurn: "target",
		},
		{
			name: "initial absence after submission is missing history", frames: []contract.ChatFrame{
				snapshot(queued()), events(start("other", "", "different")), events(start("later", "", "q"), end("later")),
			}, wantErr: ErrHistoryUnavailable,
		},
		{
			name: "start beats removal in same frame", frames: []contract.ChatFrame{
				snapshot(queued("q")), events(),
				{Type: "events", State: queued(), Items: []contract.ChatEvent{start("target", "p", "q"), agent("reply", "done"), end("target")}},
			}, wantTurn: "target",
		},
		{
			name: "start beats subsequent removal", frames: []contract.ChatFrame{
				snapshot(queued("q")), events(), events(start("target", "p", "q")), {Type: "state", State: queued()}, events(end("target")),
			}, wantTurn: "target",
		},
		{
			name: "start in initial replay locates consumed queue", frames: []contract.ChatFrame{
				snapshot(queued()), events(start("target", "p", "q"), end("target")),
			}, wantTurn: "target",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			subscription := replay(t, tt.frames...)
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			result, err := subscription.WaitTurn(ctx, TurnSelector{QueueItemID: "q"}, TurnPolicy{AttachPhase: tt.phase})
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("result %+v, error %v; want %v", result, err, tt.wantErr)
			}
			if tt.wantTurn != "" && (result.TurnID != tt.wantTurn || result.Phase != "ended") {
				t.Fatalf("wrong selected turn: %+v", result)
			}
		})
	}
}

func TestExactSelectionAndMissingHistory(t *testing.T) {
	running := &contract.ChatState{Turn: contract.ChatTurn{TurnID: "target", Running: true}}
	ended := &contract.ChatState{Turn: contract.ChatTurn{TurnID: "target", StopReason: "cancelled", Unfinished: true}}
	tests := []struct {
		name           string
		selector       TurnSelector
		frames         []contract.ChatFrame
		wantErr        error
		wantReply      string
		wantIncomplete bool
		wantReason     string
	}{
		{
			name: "prompt ignores old and later unrelated turns", selector: TurnSelector{PromptID: "p"},
			frames:    []contract.ChatFrame{snapshot(running), events(start("old", "old-p", ""), agent("old", "old reply"), end("old"), start("target", "p", ""), agent("answer", "selected"), start("other", "other-p", ""), agent("other", "unrelated"), end("other"), end("target"))},
			wantReply: "selected", wantReason: "end_turn",
		},
		{
			name: "end retained without start", selector: TurnSelector{TurnID: "target"},
			frames:         []contract.ChatFrame{snapshot(running), events(agent("unknown", "cannot attribute"), end("target"))},
			wantIncomplete: true, wantReason: "end_turn",
		},
		{
			name: "matching ended state", selector: TurnSelector{TurnID: "target"},
			frames:         []contract.ChatFrame{snapshot(ended), events(agent("unknown", "cannot attribute"))},
			wantIncomplete: true, wantReason: "cancelled",
		},
		{
			name: "matching running state stays incomplete across live chunks", selector: TurnSelector{TurnID: "target"},
			frames:    []contract.ChatFrame{{Type: "snapshot", From: 20, Truncated: true, State: running}, events(agent("answer", "unidentified prefix")), events(agent("answer", " live suffix "), end("target"))},
			wantReply: " live suffix ", wantIncomplete: true, wantReason: "end_turn",
		},
		{
			name: "wrong state never substitutes its turn", selector: TurnSelector{TurnID: "missing"},
			frames: []contract.ChatFrame{snapshot(running), events(start("target", "p", ""), end("target"))}, wantErr: ErrHistoryUnavailable,
		},
		{
			name: "first turn rejects truncated replay", selector: TurnSelector{FirstTurn: true},
			frames: []contract.ChatFrame{{Type: "snapshot", From: 1, Truncated: true}, events(start("later", "", ""), end("later"))}, wantErr: ErrHistoryUnavailable,
		},
		{
			name: "first turn rejects advanced replay origin", selector: TurnSelector{FirstTurn: true},
			frames: []contract.ChatFrame{{Type: "snapshot", From: 2}, events(start("later", "", ""), end("later"))}, wantErr: ErrHistoryUnavailable,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			subscription := replay(t, tt.frames...)
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			result, err := subscription.WaitTurn(ctx, tt.selector, TurnPolicy{})
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("result %+v, error %v; want %v", result, err, tt.wantErr)
			}
			if err == nil && (result.StopReason != tt.wantReason || result.Reply != tt.wantReply || result.HistoryIncomplete != tt.wantIncomplete || result.ReplyTruncated != tt.wantIncomplete) {
				t.Fatalf("wrong history observation: %+v", result)
			}
		})
	}
}

func TestRequestsResolvedBeforeReturning(t *testing.T) {
	request := contract.ChatRequest{RequestID: "approval", Kind: "permission", Title: "Run command?"}
	open := contract.ChatEvent{Kind: contract.ChatEventRequest, Request: &request}
	close := contract.ChatEvent{Kind: contract.ChatEventResolved, Resolved: &contract.ChatRequestOutcome{RequestID: request.RequestID, Outcome: "selected"}}
	tests := []struct {
		name         string
		frames       []contract.ChatFrame
		stop         bool
		wantPhase    string
		wantRequests int
	}{
		{"resolved with start already evicted", []contract.ChatFrame{snapshot(&contract.ChatState{Turn: contract.ChatTurn{TurnID: "target", Running: true}, Requests: []contract.ChatRequest{request}}), events(close), events(end("target"))}, true, "ended", 0},
		{"resolved in same replay", []contract.ChatFrame{snapshot(nil), events(start("target", "", ""), open, close), events(end("target"))}, true, "ended", 0},
		{"stale snapshot request resolved in replay", []contract.ChatFrame{snapshot(&contract.ChatState{Turn: contract.ChatTurn{TurnID: "target", Running: true}, Requests: []contract.ChatRequest{request}}), events(start("target", "", ""), open, close), events(end("target"))}, true, "ended", 0},
		{"unresolved live request", []contract.ChatFrame{snapshot(nil), events(start("target", "", "")), events(open)}, true, "waiting_for_user", 1},
		{"unresolved current state", []contract.ChatFrame{snapshot(&contract.ChatState{Turn: contract.ChatTurn{TurnID: "target", Running: true}, Requests: []contract.ChatRequest{request}}), events()}, true, "waiting_for_user", 1},
		{"auto wait does not approve", []contract.ChatFrame{snapshot(nil), events(start("target", "", ""), open), events(close, end("target"))}, false, "ended", 0},
		{"other turn requests are excluded", []contract.ChatFrame{snapshot(&contract.ChatState{Turn: contract.ChatTurn{TurnID: "other", Running: true}, Requests: []contract.ChatRequest{request}}), events(start("target", "", ""), agent("reply", "target"), start("other", "", ""), open), events(end("target"))}, true, "ended", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			subscription := replay(t, tt.frames...)
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			result, err := subscription.WaitTurn(ctx, TurnSelector{TurnID: "target"}, TurnPolicy{StopAtRequest: tt.stop})
			if err != nil || result.Phase != tt.wantPhase || len(result.Requests) != tt.wantRequests {
				t.Fatalf("result %+v, error %v", result, err)
			}
			if tt.wantRequests != 0 && result.Requests[0].RequestID != request.RequestID {
				t.Fatalf("lost pending request: %+v", result)
			}
		})
	}
}

func TestEndEventOverridesStaleState(t *testing.T) {
	ended := end("target")
	ended.TurnEnd.Unfinished = true
	ended.TurnEnd.Error = "background command still running"
	subscription := replay(t,
		snapshot(&contract.ChatState{Turn: contract.ChatTurn{TurnID: "target", Running: true, Unfinished: false}}),
		events(start("target", "", "")), events(ended),
	)
	result, err := subscription.WaitTurn(context.Background(), TurnSelector{TurnID: "target"}, TurnPolicy{})
	if err != nil || result.Phase != "ended" || !result.Unfinished || result.Error != ended.TurnEnd.Error || result.StopReason != "end_turn" {
		t.Fatalf("end lost authority: %+v, %v", result, err)
	}
}

func TestReplyLimitsAndOptionalEvents(t *testing.T) {
	tests := []struct {
		name      string
		limit     int
		collect   bool
		chunks    []contract.ChatEvent
		want      string
		truncated bool
	}{
		{"UTF8 boundary remains a prefix across later chunks", 5, false, []contract.ChatEvent{agent("m", "a字字"), agent("m", "suffix")}, "a字", true},
		{"byte truncation survives a new message", 5, false, []contract.ChatEvent{agent("m1", "too long"), agent("m2", "short")}, "short", true},
		{"whitespace and full CLI reply", 0, true, []contract.ChatEvent{agent("m", "  first "), agent("m", strings.Repeat("字", 30000)), agent("m", " last  ")}, "  first " + strings.Repeat("字", 30000) + " last  ", false},
		{"merge by ID without selecting an older interleaved message", 0, true, []contract.ChatEvent{agent("old", "older"), agent("last", "new "), agent("old", " ignored"), agent("last", "reply")}, "new reply", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			items := append([]contract.ChatEvent{start("target", "", "")}, tt.chunks...)
			items = append(items, end("target"), start("later", "", ""), agent("later", "unrelated"))
			subscription := replay(t, snapshot(nil), events(items...))
			result, err := subscription.WaitTurn(context.Background(), TurnSelector{TurnID: "target"}, TurnPolicy{ReplyLimit: tt.limit, CollectEvents: tt.collect})
			if err != nil || result.Reply != tt.want || result.ReplyTruncated != tt.truncated || !utf8.ValidString(result.Reply) {
				t.Fatalf("reply length %d, truncated %v, error %v", len(result.Reply), result.ReplyTruncated, err)
			}
			if tt.collect && len(result.Events) != len(tt.chunks)+2 || !tt.collect && result.Events != nil {
				t.Fatalf("wrong event collection: %d events", len(result.Events))
			}
		})
	}
}

func TestTimeoutReturnsPartialObservationWithoutEndingTurn(t *testing.T) {
	subscription := replay(t, snapshot(nil), events(start("target", "", ""), agent("answer", "partial")))
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	result, err := subscription.WaitTurn(ctx, TurnSelector{TurnID: "target"}, TurnPolicy{})
	if !errors.Is(err, context.DeadlineExceeded) || result.Phase != "running" || result.Reply != "partial" || result.StopReason != "" {
		t.Fatalf("timeout inferred completion: %+v, %v", result, err)
	}
}

func TestInitialOnlyReadsReplayWithoutTimingOut(t *testing.T) {
	subscription := replay(t, snapshot(nil), events(start("target", "", ""), agent("answer", "current")))
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	result, err := subscription.WaitTurn(ctx, TurnSelector{TurnID: "target"}, TurnPolicy{InitialOnly: true})
	if err != nil || result.Phase != "running" || result.Reply != "current" || result.StopReason != "" {
		t.Fatalf("poll result: %+v, %v", result, err)
	}
}

func TestCanceledObservationIsNotCompletion(t *testing.T) {
	subscription := replay(t, snapshot(nil), events(start("target", "", "")))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	result, err := subscription.WaitTurn(ctx, TurnSelector{TurnID: "target"}, TurnPolicy{})
	if !errors.Is(err, context.Canceled) || result.StopReason != "" {
		t.Fatalf("cancellation inferred completion: %+v, %v", result, err)
	}
}
