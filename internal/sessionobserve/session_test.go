package sessionobserve

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/rainchan/webmux/internal/contract"
)

func sessionReplay(t *testing.T, frames ...contract.SessionEvent) *SessionSubscription {
	t.Helper()
	client := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/ws/sessions" {
			t.Errorf("unexpected subscription %s", r.URL.Path)
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
		_, _, _ = conn.Read(ctx)
	})
	subscription, err := OpenSessions(context.Background(), client, 4<<20)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(subscription.Close)
	return subscription
}

func TestSessionExitUsesActualProcessStatusAndCode(t *testing.T) {
	code := 7
	subscription := sessionReplay(t,
		contract.SessionEvent{Type: "snapshot", Sessions: []contract.SessionInfo{{ID: "s", Status: contract.SessionStatusIdle}}},
		contract.SessionEvent{Type: "created", Session: &contract.SessionInfo{ID: "other", Status: contract.SessionStatusRunning}},
		contract.SessionEvent{Type: "updated", Session: &contract.SessionInfo{ID: "s", Status: contract.SessionStatusWaiting}},
		contract.SessionEvent{Type: "forwards"},
		contract.SessionEvent{Type: "updated", Session: &contract.SessionInfo{ID: "s", Status: contract.SessionStatusExited, ExitCode: &code}},
	)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	for i := range 5 {
		if _, err := subscription.Read(ctx); err != nil {
			t.Fatal(err)
		}
		info, exited, err := subscription.Exit("s")
		if err != nil || exited != (i == 4) {
			t.Fatalf("event %d reported exit: %+v %v %v", i, info, exited, err)
		}
		if exited && (info.ExitCode == nil || *info.ExitCode != 7) {
			t.Fatalf("lost actual process code: %+v", info)
		}
	}
}

func TestMissingAndRemovedSessionsDoNotInferExit(t *testing.T) {
	subscription := sessionReplay(t,
		contract.SessionEvent{Type: "snapshot", Sessions: []contract.SessionInfo{{ID: "s", Status: contract.SessionStatusRunning}}},
		contract.SessionEvent{Type: "removed", ID: "s"},
	)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if _, err := subscription.Read(ctx); err != nil {
		t.Fatal(err)
	}
	if _, exited, err := subscription.Exit("missing"); !errors.Is(err, ErrSessionNotFound) || exited {
		t.Fatalf("missing target inferred exit: exited=%v error=%v", exited, err)
	}
	if _, err := subscription.Read(ctx); err != nil {
		t.Fatal(err)
	}
	if _, exited, err := subscription.Exit("s"); !errors.Is(err, ErrSessionRemoved) || exited {
		t.Fatalf("removal inferred exit: exited=%v error=%v", exited, err)
	}
}

func TestExitWithoutCodeRemainsUnknown(t *testing.T) {
	subscription := sessionReplay(t, contract.SessionEvent{Type: "snapshot", Sessions: []contract.SessionInfo{{ID: "s", Status: contract.SessionStatusExited}}})
	if _, err := subscription.Read(context.Background()); err != nil {
		t.Fatal(err)
	}
	info, exited, err := subscription.Exit("s")
	if err != nil || !exited || info.ExitCode != nil {
		t.Fatalf("invented exit code: %+v, %v, %v", info, exited, err)
	}
}

func TestDisconnectedChatDoesNotInferCompletion(t *testing.T) {
	client := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{Subprotocols: []string{"webmux"}})
		if err != nil {
			t.Error(err)
			return
		}
		defer func() { _ = conn.CloseNow() }()
		for _, frame := range []contract.ChatFrame{snapshot(nil), events(start("target", "", ""), agent("answer", "partial"))} {
			data, err := json.Marshal(frame)
			if err != nil {
				t.Error(err)
				return
			}
			if err := conn.Write(r.Context(), websocket.MessageText, data); err != nil {
				return
			}
		}
	})
	subscription, err := OpenChat(context.Background(), client, "s", 64<<20)
	if err != nil {
		t.Fatal(err)
	}
	defer subscription.Close()
	result, err := subscription.WaitTurn(context.Background(), TurnSelector{TurnID: "target"}, TurnPolicy{})
	if err == nil || errors.Is(err, context.DeadlineExceeded) || result.Phase != "running" || result.Reply != "partial" || result.StopReason != "" {
		t.Fatalf("disconnect inferred completion: %+v, %v", result, err)
	}
}

func TestInvalidFrameIsNotAnObservation(t *testing.T) {
	client := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{Subprotocols: []string{"webmux"}})
		if err != nil {
			t.Error(err)
			return
		}
		defer func() { _ = conn.CloseNow() }()
		if err := conn.Write(r.Context(), websocket.MessageText, []byte(`{"type":"unknown"}`)); err != nil {
			return
		}
		_, _, _ = conn.Read(r.Context())
	})
	subscription, err := OpenChat(context.Background(), client, "s", 64<<20)
	if err != nil {
		t.Fatal(err)
	}
	defer subscription.Close()
	if _, err := subscription.Read(context.Background()); !errors.Is(err, ErrInvalidFrame) {
		t.Fatalf("invalid frame accepted: %v", err)
	}
}
