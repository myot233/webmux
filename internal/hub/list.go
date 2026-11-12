package hub

import (
	"context"
	"encoding/json"

	"github.com/rainchan/webmux/internal/contract"
	"github.com/rainchan/webmux/internal/forward"
	"github.com/rainchan/webmux/internal/session"
	"github.com/rainchan/webmux/internal/wire"
)

// List streams /ws/sessions: a snapshot of the sessions and forwards, then
// session events and a forwards event after each forward change. forwards
// may be nil, which lists none.
func List(ctx context.Context, m *session.Manager, forwards *forward.Service, c MessageConn) {
	ctx, cancel := context.WithCancel(ctx)
	// Subscribing to forwards before reading the list leaves no change
	// unannounced; a signal already covered by the snapshot only repeats it.
	var changes <-chan struct{}
	if forwards != nil {
		fsub := forwards.Subscribe()
		defer forwards.Unsubscribe(fsub)
		changes = fsub.C
	}
	sub := m.Subscribe()
	defer m.Unsubscribe(sub)
	reader := c.CloseRead(ctx)
	defer func() {
		cancel()
		_ = c.CloseNow()
	}()
	send := func(event contract.SessionEvent) bool {
		data, err := json.Marshal(event)
		if err != nil || write(ctx, c, MessageText, data) != nil {
			_ = c.Close(StatusSlowClient, "write timeout")
			return false
		}
		return true
	}
	// Subscribe queued the snapshot first; it must precede forwards events.
	var view activeView
	snapshot := wire.SessionEvent(view.filter(<-sub.Out))
	snapshot.Forwards = forwardList(forwards)
	if !send(snapshot) {
		return
	}
	for {
		select {
		case <-reader.Done():
			return
		case <-sub.Slow:
			_ = c.Close(StatusSlowClient, "slow client")
			return
		case event := <-sub.Out:
			event, ok := view.event(event)
			if ok && !send(wire.SessionEvent(event)) {
				return
			}
		case <-changes:
			if !send(contract.SessionEvent{Type: "forwards", Forwards: forwardList(forwards)}) {
				return
			}
		}
	}
}

// activeView keeps the session list a connection sees to active sessions:
// a hybrid shell that was closed or interrupted leaves it with a removed
// event, and comes back with a created event if it becomes active again.
type activeView struct {
	hidden map[string]bool
}

// filter drops the inactive sessions of a snapshot and remembers them.
func (v *activeView) filter(e session.Event) session.Event {
	v.hidden = map[string]bool{}
	sessions := make([]session.Info, 0, len(e.Sessions))
	for _, info := range e.Sessions {
		if wire.Active(info) {
			sessions = append(sessions, info)
		} else {
			v.hidden[info.ID] = true
		}
	}
	e.Sessions = sessions
	return e
}

// event translates one list event for the connection; ok is false when it
// concerns a session the connection does not see.
func (v *activeView) event(e session.Event) (session.Event, bool) {
	switch e.Type {
	case "snapshot":
		return v.filter(e), true
	case "removed":
		if v.hidden[e.ID] {
			delete(v.hidden, e.ID)
			return e, false
		}
		return e, true
	}
	if e.Session == nil {
		return e, true
	}
	id := e.Session.ID
	switch {
	case !wire.Active(*e.Session) && v.hidden[id]:
		return e, false
	case !wire.Active(*e.Session):
		v.hidden[id] = true
		if e.Type == "created" {
			return e, false
		}
		return session.Event{Type: "removed", ID: id}, true
	case v.hidden[id]:
		delete(v.hidden, id)
		e.Type = "created"
	}
	return e, true
}

func forwardList(forwards *forward.Service) []contract.Forward {
	if forwards == nil {
		return []contract.Forward{}
	}
	return wire.Forwards(forwards.List())
}
