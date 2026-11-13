package sessionobserve

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/coder/websocket"
	"github.com/rainchan/webmux/internal/contract"
	"github.com/rainchan/webmux/internal/localclient"
)

var (
	ErrSessionNotFound = errors.New("session not found in active list; use session_get for archived records")
	ErrSessionRemoved  = errors.New("session removed before exit was observed")
)

// SessionSubscription applies the authoritative snapshot and list events.
// Multi-session conditions and timing remain the responsibility of the caller.
// The zero value can hold facts but must be opened before reading or closing.
type SessionSubscription struct {
	conn     *websocket.Conn
	sessions map[string]contract.SessionInfo
	removed  map[string]struct{}
}

func OpenSessions(ctx context.Context, client *localclient.Client, readLimit int64) (*SessionSubscription, error) {
	conn, err := client.DialWebSocket(ctx, "/ws/sessions")
	if err != nil {
		return nil, err
	}
	conn.SetReadLimit(readLimit)
	return &SessionSubscription{conn: conn}, nil
}

func (s *SessionSubscription) Close() { _ = s.conn.CloseNow() }

func (s *SessionSubscription) Read(ctx context.Context) (contract.SessionEvent, error) {
	_, data, err := s.conn.Read(ctx)
	if err != nil {
		if ctx.Err() != nil {
			return contract.SessionEvent{}, ctx.Err()
		}
		return contract.SessionEvent{}, fmt.Errorf("session observation disconnected: %w", err)
	}
	var event contract.SessionEvent
	if err := json.Unmarshal(data, &event); err != nil {
		return contract.SessionEvent{}, fmt.Errorf("%w: session event: %w", ErrInvalidFrame, err)
	}
	if err := s.Apply(event); err != nil {
		return contract.SessionEvent{}, err
	}
	return event, nil
}

// Apply replaces or updates facts with a decoded list event. It is also useful
// for clients that already receive these same events through their transport.
func (s *SessionSubscription) Apply(event contract.SessionEvent) error {
	if s.sessions == nil {
		s.sessions = make(map[string]contract.SessionInfo)
		s.removed = make(map[string]struct{})
	}
	switch event.Type {
	case "snapshot":
		clear(s.sessions)
		clear(s.removed)
		for _, info := range event.Sessions {
			s.sessions[info.ID] = info
		}
	case "created", "updated":
		if event.Session == nil {
			return fmt.Errorf("%w: session payload missing", ErrInvalidFrame)
		}
		s.sessions[event.Session.ID] = *event.Session
		delete(s.removed, event.Session.ID)
	case "removed":
		delete(s.sessions, event.ID)
		s.removed[event.ID] = struct{}{}
	case "forwards":
		// Forward-list updates share this stream but do not change session facts.
	default:
		return fmt.Errorf("%w: unknown session event %q", ErrInvalidFrame, event.Type)
	}
	return nil
}

// Session returns the latest target facts, distinguishing an explicit removal
// from absence in the initial active list. Neither fact proves process exit.
func (s *SessionSubscription) Session(id string) (contract.SessionInfo, error) {
	if info, ok := s.sessions[id]; ok {
		return info, nil
	}
	if _, ok := s.removed[id]; ok {
		return contract.SessionInfo{}, fmt.Errorf("%w: %s", ErrSessionRemoved, id)
	}
	return contract.SessionInfo{}, fmt.Errorf("%w: %s", ErrSessionNotFound, id)
}

// Exit reports only an observed exited status. Idle/waiting/attention are not
// exits, and the returned SessionInfo keeps the actual (possibly unknown) code.
func (s *SessionSubscription) Exit(id string) (contract.SessionInfo, bool, error) {
	info, err := s.Session(id)
	return info, info.Status == contract.SessionStatusExited && err == nil, err
}
