package clicmd

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/rainchan/webmux/internal/contract"
	"github.com/rainchan/webmux/internal/localclient"
	"github.com/rainchan/webmux/internal/sessionobserve"
)

type observationDialer struct{ address string }

func (d observationDialer) DialContext(ctx context.Context) (net.Conn, error) {
	return (&net.Dialer{}).DialContext(ctx, "tcp", d.address)
}

func observationClient(t *testing.T, handler http.HandlerFunc) *localclient.Client {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	client := localclient.New(observationDialer{strings.TrimPrefix(server.URL, "http://")})
	t.Cleanup(client.Close)
	return client
}

func events(items ...contract.ChatEvent) contract.ChatFrame {
	return contract.ChatFrame{Type: contract.ChatFrameEvents, Items: items}
}

func turnStart(id string) contract.ChatEvent {
	return contract.ChatEvent{Kind: contract.ChatEventTurnStart, TurnStart: &contract.ChatTurnStart{TurnID: id, PromptID: "p" + id}}
}

func turnEnd(id, reason string) contract.ChatEvent {
	return contract.ChatEvent{Kind: contract.ChatEventTurnEnd, TurnEnd: &contract.ChatTurnEnd{TurnID: id, StopReason: reason}}
}

func agent(message, text string) contract.ChatEvent {
	return contract.ChatEvent{Kind: contract.ChatEventMessage, Message: &contract.ChatMessageChunk{MessageID: message, Role: "agent", Text: text}}
}

func TestSendReplyHasNoSubscribeSendGap(t *testing.T) {
	tests := []struct {
		name        string
		receipt     contract.SessionInputResult
		initialTurn string
		live        []contract.ChatFrame
		wantCode    int
		wantReply   string
	}{
		{
			name: "direct turn ends before receipt returns", receipt: contract.SessionInputResult{View: contract.ViewChat, TurnID: "new", PromptID: "pnew"}, initialTurn: "old",
			live:      []contract.ChatFrame{events(turnStart("new"), agent("answer", "  new reply  "), turnEnd("new", "end_turn"))},
			wantReply: "new reply",
		},
		{
			name: "prompt receipt selects only its own turn", receipt: contract.SessionInputResult{View: contract.ViewChat, PromptID: "pnew"}, initialTurn: "old",
			live:      []contract.ChatFrame{events(turnStart("unrelated"), agent("unrelated", "wrong"), turnEnd("unrelated", "end_turn"), turnStart("new"), agent("answer", "prompt reply"), turnEnd("new", "end_turn"))},
			wantReply: "prompt reply",
		},
		{
			name: "steer follows confirmed current turn", receipt: contract.SessionInputResult{View: contract.ViewChat}, initialTurn: "new",
			live: []contract.ChatFrame{events(agent("answer", "  steered reply  "), turnEnd("new", "end_turn"))}, wantReply: "steered reply",
		},
		{
			name: "queued item deletion fails instead of timing out", receipt: contract.SessionInputResult{View: contract.ViewChat, Queued: new(true), ItemID: "q"}, initialTurn: "old",
			live: []contract.ChatFrame{{Type: "state", State: &contract.ChatState{Queue: contract.ChatQueue{Items: []contract.ChatQueueItem{{ID: "q"}}}}}, {Type: "state", State: &contract.ChatState{}}}, wantCode: contract.ExitError,
		},
		{
			name: "timeout keeps original CLI code", receipt: contract.SessionInputResult{View: contract.ViewChat, TurnID: "new"}, initialTurn: "old",
			live: []contract.ChatFrame{events(turnStart("new"), agent("answer", "not finished"))}, wantCode: contract.ExitTimeout,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := make(chan struct{})
			published := make(chan struct{})
			client := observationClient(t, func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/ws/s/s/chat":
					conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{Subprotocols: []string{"webmux"}})
					if err != nil {
						t.Error(err)
						return
					}
					defer func() { _ = conn.CloseNow() }()
					ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
					defer cancel()
					writeFrame := func(frame contract.ChatFrame) bool {
						data, err := json.Marshal(frame)
						if err != nil {
							t.Error(err)
							return false
						}
						return conn.Write(ctx, websocket.MessageText, data) == nil
					}
					if !writeFrame(contract.ChatFrame{Type: "snapshot", From: 1, State: &contract.ChatState{Turn: contract.ChatTurn{TurnID: tt.initialTurn, Running: true}}}) ||
						!writeFrame(events(turnStart(tt.initialTurn), agent("initial", "old reply"))) {
						return
					}
					select {
					case <-input:
					case <-ctx.Done():
						return
					}
					for _, frame := range tt.live {
						if !writeFrame(frame) {
							return
						}
					}
					// Publish the whole turn before releasing the REST receipt.
					close(published)
					_, _, _ = conn.Read(ctx)
				case "/api/sessions/s/input":
					if r.Method != http.MethodPost {
						t.Errorf("unexpected observation write %s", r.Method)
						return
					}
					var request contract.SessionInputRequest
					if err := json.NewDecoder(r.Body).Decode(&request); err != nil || request.ClientRequestID != "request" {
						t.Errorf("input: %+v, %v", request, err)
						return
					}
					close(input)
					select {
					case <-published:
					case <-r.Context().Done():
						return
					}
					if err := json.NewEncoder(w).Encode(tt.receipt); err != nil {
						t.Error(err)
					}
				default:
					t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
				}
			})
			var stdout, stderr bytes.Buffer
			env := Env{Stdout: &stdout, Stderr: &stderr}
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			waitCtx := ctx
			if tt.wantCode == contract.ExitTimeout {
				var stop context.CancelFunc
				waitCtx, stop = context.WithTimeout(ctx, 200*time.Millisecond)
				defer stop()
			}
			code := env.sendReply(ctx, waitCtx, client, "s", contract.SessionInputRequest{ClientRequestID: "request", Text: "do work"}, sendOptions{asJSON: true})
			if code != tt.wantCode {
				t.Fatalf("exit %d, want %d: %s", code, tt.wantCode, stderr.String())
			}
			if tt.wantCode == contract.ExitOK {
				var result turnResult
				if err := json.Unmarshal(stdout.Bytes(), &result); err != nil || result.Reply != tt.wantReply || result.StopReason != "end_turn" {
					t.Fatalf("reply %s, error %v", stdout.String(), err)
				}
				if strings.Contains(stdout.String(), `"unfinished"`) {
					t.Fatal("core migration changed the CLI JSON contract")
				}
			}
		})
	}
}

func TestCLITurnPresentationPreservesInternalUnfinished(t *testing.T) {
	observed := sessionobserve.TurnObservation{StopReason: "end_turn", Reply: "  reply\n", Unfinished: true}
	result := cliTurn(observed)
	if result.Reply != "reply" || !result.Unfinished {
		t.Fatalf("presentation lost observed facts: %+v", result)
	}
}

func TestRawChatHistoryStillReadsBoundedReplay(t *testing.T) {
	client := observationClient(t, func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{Subprotocols: []string{"webmux"}})
		if err != nil {
			t.Error(err)
			return
		}
		defer func() { _ = conn.CloseNow() }()
		for _, frame := range []contract.ChatFrame{{Type: "snapshot", From: 10, Truncated: true, State: &contract.ChatState{Turn: contract.ChatTurn{Running: true}}}, events(agent("m", "retained reply"))} {
			data, err := json.Marshal(frame)
			if err != nil {
				t.Error(err)
				return
			}
			if err := conn.Write(r.Context(), websocket.MessageText, data); err != nil {
				return
			}
		}
		_, _, _ = conn.Read(r.Context())
	})
	subscription, err := sessionobserve.OpenChat(context.Background(), client, "s", 64<<20)
	if err != nil {
		t.Fatal(err)
	}
	defer subscription.Close()
	snapshot, items, err := readChatHistory(context.Background(), subscription)
	if err != nil || !snapshot.Truncated || snapshot.From != 10 || len(items) != 1 || items[0].Message.Text != "retained reply" || subscription.State() == nil || !subscription.State().Turn.Running {
		t.Fatalf("bounded replay changed: %+v, %+v, %v", snapshot, items, err)
	}
}
