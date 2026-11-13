// Package sessionobserve interprets the bounded session and chat subscriptions
// exposed by the WebMux API. Subscriptions have one reader and are caller-owned.
package sessionobserve

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"

	"github.com/coder/websocket"
	"github.com/rainchan/webmux/internal/contract"
	"github.com/rainchan/webmux/internal/localclient"
)

var (
	ErrQueueRemoved       = errors.New("queued input removed before its turn started")
	ErrHistoryUnavailable = errors.New("selected turn history unavailable")
	ErrInvalidFrame       = errors.New("invalid observation frame")
)

// ChatSubscription reads one /ws/s/{id}/chat stream. Prime and WaitTurn preserve
// the initial replay without a second connection or a subscribe/send gap.
// Read is also available to consumers of raw chat state and bounded history.
// Do not mix raw reads with a turn wait on the same subscription.
type ChatSubscription struct {
	conn    *websocket.Conn
	state   *contract.ChatState
	initial []contract.ChatFrame
	primed  bool
}

func OpenChat(ctx context.Context, client *localclient.Client, id string, readLimit int64) (*ChatSubscription, error) {
	conn, err := client.DialWebSocket(ctx, "/ws/s/"+url.PathEscape(id)+"/chat")
	if err != nil {
		return nil, err
	}
	conn.SetReadLimit(readLimit)
	return &ChatSubscription{conn: conn}, nil
}

func (s *ChatSubscription) Close() { _ = s.conn.CloseNow() }

// State is the last replaceable state received from the server. Callers must
// treat it as immutable, just like payloads returned by Read.
func (s *ChatSubscription) State() *contract.ChatState { return s.state }

func (s *ChatSubscription) Read(ctx context.Context) (contract.ChatFrame, error) {
	_, data, err := s.conn.Read(ctx)
	if err != nil {
		if ctx.Err() != nil {
			return contract.ChatFrame{}, ctx.Err()
		}
		if websocket.CloseStatus(err) == 4404 {
			return contract.ChatFrame{}, errors.New("the session has no chat view or no longer exists")
		}
		return contract.ChatFrame{}, fmt.Errorf("chat observation disconnected; no completion inferred: %w", err)
	}
	var frame contract.ChatFrame
	if err := json.Unmarshal(data, &frame); err != nil {
		return contract.ChatFrame{}, fmt.Errorf("%w: chat frame: %w", ErrInvalidFrame, err)
	}
	switch frame.Type {
	case contract.ChatFrameSnapshot, contract.ChatFrameEvents, contract.ChatFrameState:
	default:
		return contract.ChatFrame{}, fmt.Errorf("%w: unknown chat frame %q", ErrInvalidFrame, frame.Type)
	}
	if frame.State != nil {
		s.state = frame.State
	}
	return frame, nil
}

// Prime reads the snapshot and its complete replay. It retains the actual
// decoded frames, not an extra copy of their events, until WaitTurn selects the
// relevant turn. Call before sending input when attaching before submission.
func (s *ChatSubscription) Prime(ctx context.Context) error {
	if s.primed {
		return nil
	}
	if _, err := s.initialSnapshot(ctx); err != nil {
		return err
	}
	for {
		frame, err := s.Read(ctx)
		if err != nil {
			return err
		}
		s.initial = append(s.initial, frame)
		if frame.Type == contract.ChatFrameEvents {
			s.primed = true
			return nil
		}
	}
}

func (s *ChatSubscription) initialSnapshot(ctx context.Context) (contract.ChatFrame, error) {
	if len(s.initial) != 0 {
		return s.initial[0], nil
	}
	frame, err := s.Read(ctx)
	if err != nil {
		return contract.ChatFrame{}, err
	}
	if frame.Type != contract.ChatFrameSnapshot {
		return contract.ChatFrame{}, fmt.Errorf("%w: chat snapshot expected", ErrInvalidFrame)
	}
	s.initial = append(s.initial, frame)
	return frame, nil
}
