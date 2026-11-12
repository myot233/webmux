package clicmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strings"

	"github.com/coder/websocket"

	"github.com/rainchan/webmux/internal/contract"
	"github.com/rainchan/webmux/internal/localclient"
)

// chatStream is one /ws/s/{id}/chat subscription.
type chatStream struct {
	conn  *websocket.Conn
	state *contract.ChatState
	next  uint64
}

func openChat(ctx context.Context, client *localclient.Client, id string) (*chatStream, error) {
	conn, err := client.DialWebSocket(ctx, "/ws/s/"+url.PathEscape(id)+"/chat")
	if err != nil {
		return nil, err
	}
	conn.SetReadLimit(64 << 20)
	return &chatStream{conn: conn}, nil
}

func (c *chatStream) close() { _ = c.conn.CloseNow() }

func (c *chatStream) read(ctx context.Context) (contract.ChatFrame, error) {
	_, data, err := c.conn.Read(ctx)
	if err != nil {
		if websocket.CloseStatus(err) == 4404 {
			return contract.ChatFrame{}, errors.New("the session has no chat view or no longer exists")
		}
		return contract.ChatFrame{}, err
	}
	var frame contract.ChatFrame
	if err := json.Unmarshal(data, &frame); err != nil {
		return contract.ChatFrame{}, fmt.Errorf("chat frame: %w", err)
	}
	if frame.State != nil {
		c.state = frame.State
	}
	if frame.Type == contract.ChatFrameSnapshot {
		c.next = frame.Next
	}
	return frame, nil
}

// turnResult is the outcome of one tracked turn.
type turnResult struct {
	StopReason string               `json:"stopReason"`
	Error      string               `json:"error,omitempty"`
	Reply      string               `json:"reply"`
	Events     []contract.ChatEvent `json:"events"`
	// Request is an open permission request or form when the wait stopped
	// for it.
	Request *contract.ChatRequest `json:"request,omitempty"`
}

// turnWatch selects the turn turnWait tracks: the first whose turn.start
// passes accept. current names a turn already running: when its turn.start
// is not in the replayed history (the log dropped it), it is adopted once
// the replay is over. gone, when set, ends the wait with its error once a
// state frame shows the turn can no longer start (a queued prompt was
// removed).
type turnWatch struct {
	current       string
	accept        func(contract.ChatTurnStart) bool
	gone          func(*contract.ChatState) error
	stopAtRequest bool
}

// turnWait tracks one turn and collects its events until turn.end, or
// until a request opens when stopAtRequest.
func (c *chatStream) turnWait(ctx context.Context, w turnWatch) (turnResult, error) {
	var result turnResult
	turn := ""
	accept := w.accept
	if w.current != "" {
		accept = func(t contract.ChatTurnStart) bool { return t.TurnID == w.current }
	}
	messages := map[string]*strings.Builder{}
	var order []string
	// Requests of the tracked turn still open; a replayed request that was
	// already answered is closed again within the same history.
	open := map[string]contract.ChatRequest{}
	var opened []string
	for {
		frame, err := c.read(ctx)
		if err != nil {
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				return result, errTimeout
			}
			return result, err
		}
		if turn == "" && frame.State != nil && w.gone != nil {
			if err := w.gone(frame.State); err != nil {
				return result, err
			}
		}
		for _, ev := range frame.Items {
			if turn == "" {
				if ev.Kind == contract.ChatEventTurnStart && ev.TurnStart != nil && accept(*ev.TurnStart) {
					turn = ev.TurnStart.TurnID
					result.Events = append(result.Events, ev)
				}
				continue
			}
			result.Events = append(result.Events, ev)
			switch ev.Kind {
			case contract.ChatEventMessage:
				if ev.Message != nil && ev.Message.Role == "agent" {
					b, ok := messages[ev.Message.MessageID]
					if !ok {
						b = &strings.Builder{}
						messages[ev.Message.MessageID] = b
						order = append(order, ev.Message.MessageID)
					}
					b.WriteString(ev.Message.Text)
				}
			case contract.ChatEventRequest:
				if ev.Request != nil {
					open[ev.Request.RequestID] = *ev.Request
					opened = append(opened, ev.Request.RequestID)
				}
			case contract.ChatEventResolved:
				if ev.Resolved != nil {
					delete(open, ev.Resolved.RequestID)
				}
			case contract.ChatEventTurnEnd:
				if ev.TurnEnd != nil && ev.TurnEnd.TurnID == turn {
					result.StopReason = ev.TurnEnd.StopReason
					result.Error = ev.TurnEnd.Error
					result.Reply = lastReply(messages, order)
					return result, nil
				}
			}
		}
		// A request still open once the whole frame is read waits for an
		// answer.
		if w.stopAtRequest {
			for _, id := range opened {
				if request, ok := open[id]; ok {
					result.Request = &request
					result.Reply = lastReply(messages, order)
					return result, nil
				}
			}
		}
		// The replay is the events frame right after the snapshot; a running
		// turn whose start it no longer holds is followed from here.
		if turn == "" && w.current != "" && frame.Type == contract.ChatFrameEvents {
			turn = w.current
		}
	}
}

func lastReply(messages map[string]*strings.Builder, order []string) string {
	if len(order) == 0 {
		return ""
	}
	return strings.TrimSpace(messages[order[len(order)-1]].String())
}

// reportTurn prints a tracked turn and returns the exit status:
// end_turn is 0, a stop for a request or any other reason is 1.
func (e Env) reportTurn(result turnResult, asJSON bool) int {
	if asJSON {
		writeJSON(e.Stdout, result)
	} else if result.Reply != "" {
		_, _ = fmt.Fprintln(e.Stdout, result.Reply)
	}
	if result.Request != nil {
		_, _ = fmt.Fprintf(e.Stderr, "webmux: the agent is waiting for an answer: request %s (%s) %s\n",
			result.Request.RequestID, result.Request.Kind, result.Request.Title)
		for _, option := range result.Request.Options {
			_, _ = fmt.Fprintf(e.Stderr, "webmux:   --option %s\t%s\n", option.OptionID, option.Name)
		}
		_, _ = fmt.Fprintln(e.Stderr, "webmux: answer with webmux chat answer SESSION", result.Request.RequestID, "(see webmux chat status), or in the Web UI")
		return contract.ExitError
	}
	if result.StopReason != "end_turn" {
		reason := result.StopReason
		if result.Error != "" {
			reason += ": " + result.Error
		}
		_, _ = fmt.Fprintln(e.Stderr, "webmux: the turn ended with", reason)
		return contract.ExitError
	}
	return contract.ExitOK
}

func writeJSON(w io.Writer, v any) {
	encoder := json.NewEncoder(w)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	_ = encoder.Encode(v)
}
