package mcpcontrol

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/rainchan/webmux/internal/contract"
	"github.com/rainchan/webmux/internal/mcpstdio"
)

func (c Client) waitTools() []mcpstdio.Tool {
	return []mcpstdio.Tool{tool("session_wait", "Wait for a specific chat turn, queued input, first launched turn, or terminal process exit. Never infer completion from idle. A timeout ends observation only; continue waiting with the same selector. Reports pending user requests without approving them.", true, map[string]any{
		"sessionId": text(), "until": enumeration("turn", "exit"), "turnId": text(), "queueItemId": text(), "firstTurn": boolean(), "timeoutMs": integer(0, 60000),
	}, []string{"sessionId"}, c.wait)}
}

type waitArguments struct {
	SessionID   string `json:"sessionId"`
	Until       string `json:"until,omitempty"`
	TurnID      string `json:"turnId,omitempty"`
	QueueItemID string `json:"queueItemId,omitempty"`
	FirstTurn   bool   `json:"firstTurn,omitempty"`
	TimeoutMS   int    `json:"timeoutMs,omitempty"`
}
type waitResult struct {
	SessionID      string                 `json:"sessionId"`
	TurnID         string                 `json:"turnId,omitempty"`
	QueueItemID    string                 `json:"queueItemId,omitempty"`
	Status         string                 `json:"status"`
	StopReason     string                 `json:"stopReason,omitempty"`
	Error          string                 `json:"error,omitempty"`
	Reply          string                 `json:"reply"`
	ReplyTruncated bool                   `json:"replyTruncated"`
	Unfinished     bool                   `json:"unfinished"`
	WaitTimedOut   bool                   `json:"waitTimedOut"`
	Requests       []contract.ChatRequest `json:"requests"`
	ExitCode       *int                   `json:"exitCode,omitempty"`
}

func (c Client) wait(ctx context.Context, raw json.RawMessage) (any, error) {
	a := waitArguments{Until: "turn", TimeoutMS: 30000}
	if decode(raw, &a) != nil || !identity.MatchString(a.SessionID) || a.TimeoutMS < 0 || a.TimeoutMS > 60000 {
		return nil, errors.New("invalid session_wait arguments")
	}
	selectors := 0
	if a.TurnID != "" {
		selectors++
		if !identity.MatchString(a.TurnID) {
			return nil, errors.New("invalid turnId")
		}
	}
	if a.QueueItemID != "" {
		selectors++
		if !identity.MatchString(a.QueueItemID) {
			return nil, errors.New("invalid queueItemId")
		}
	}
	if a.FirstTurn {
		selectors++
	}
	if a.Until == "turn" && selectors != 1 || a.Until == "exit" && selectors != 0 || a.Until != "turn" && a.Until != "exit" {
		return nil, errors.New("select exactly one of turnId, queueItemId or firstTurn for a turn; exit uses no turn selector")
	}
	// Zero is a bounded poll, allowing enough time to receive the initial replay.
	duration := time.Duration(a.TimeoutMS) * time.Millisecond
	if a.TimeoutMS == 0 {
		duration = 10 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, duration)
	defer cancel()
	if a.Until == "exit" {
		return c.waitExit(ctx, a)
	}
	return c.waitTurn(ctx, a)
}
func initialWait(a waitArguments) waitResult {
	return waitResult{SessionID: a.SessionID, TurnID: a.TurnID, QueueItemID: a.QueueItemID, Status: "pending", Requests: []contract.ChatRequest{}}
}
func (c Client) waitExit(ctx context.Context, a waitArguments) (any, error) {
	result := initialWait(a)
	conn, err := c.API.DialWebSocket(ctx, "/ws/sessions")
	if err != nil {
		return nil, apiError(err)
	}
	defer func() { _ = conn.CloseNow() }()
	conn.SetReadLimit(responseLimit)
	for {
		_, data, err := conn.Read(ctx)
		if err != nil {
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				result.WaitTimedOut = true
				return result, nil
			}
			return nil, errors.New("session observation disconnected")
		}
		var frame contract.SessionEvent
		if json.Unmarshal(data, &frame) != nil {
			return nil, errors.New("invalid session event")
		}
		var info *contract.SessionInfo
		if frame.Type == "snapshot" {
			for i := range frame.Sessions {
				if frame.Sessions[i].ID == a.SessionID {
					info = &frame.Sessions[i]
					break
				}
			}
			if info == nil {
				return nil, errors.New("session not found in active list; use session_get for archived records")
			}
		} else if frame.Session != nil && frame.Session.ID == a.SessionID {
			info = frame.Session
		} else if frame.Type == "removed" && frame.ID == a.SessionID {
			return nil, errors.New("session removed before exit was observed")
		}
		if info != nil {
			result.Status = string(info.Status)
			if info.Status == contract.SessionStatusExited {
				result.ExitCode = info.ExitCode
				return result, nil
			}
			if a.TimeoutMS == 0 {
				return result, nil
			}
		}
	}
}
func (c Client) waitTurn(ctx context.Context, a waitArguments) (any, error) {
	result := initialWait(a)
	conn, err := c.API.DialWebSocket(ctx, "/ws/s/"+url.PathEscape(a.SessionID)+"/chat")
	if err != nil {
		return nil, apiError(err)
	}
	defer func() { _ = conn.CloseNow() }()
	// The server sends retained chat history as one frame, as in the CLI.
	// Bound model-visible replies separately from the transport replay.
	conn.SetReadLimit(64 << 20)
	var state *contract.ChatState
	snapshot, replayed, collecting := false, false, false
	tracked, lastMessage := "", ""
	historyIncomplete := false
	for {
		_, data, err := conn.Read(ctx)
		if err != nil {
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				result.WaitTimedOut = true
				return result, nil
			}
			return nil, errors.New("chat observation disconnected; no completion inferred")
		}
		var frame contract.ChatFrame
		if json.Unmarshal(data, &frame) != nil {
			return nil, errors.New("invalid chat frame")
		}
		if frame.State != nil {
			state = frame.State
		}
		if frame.Type == contract.ChatFrameSnapshot {
			snapshot = true
			if a.FirstTurn && (frame.Truncated || frame.From > 1) {
				return nil, errors.New("first turn history unavailable; do not substitute a later turn")
			}
		}
		for _, event := range frame.Items {
			if event.TurnStart != nil {
				start := event.TurnStart
				match := a.TurnID != "" && start.TurnID == a.TurnID || a.QueueItemID != "" && start.QueueItemID == a.QueueItemID || a.FirstTurn && tracked == ""
				if match && tracked == "" {
					tracked = start.TurnID
					result.TurnID = tracked
					result.Status = "running"
				}
				collecting = tracked != "" && start.TurnID == tracked
			}
			if event.TurnEnd != nil && (event.TurnEnd.TurnID == tracked || a.TurnID != "" && event.TurnEnd.TurnID == a.TurnID) {
				if tracked == "" {
					result.ReplyTruncated = true
				}
				result.TurnID = event.TurnEnd.TurnID
				result.Status = "ended"
				result.Unfinished = event.TurnEnd.Unfinished
				result.StopReason = event.TurnEnd.StopReason
				result.Error = event.TurnEnd.Error
				return result, nil
			}
			if collecting && event.Message != nil && event.Message.Role == "agent" {
				if lastMessage != event.Message.MessageID {
					result.Reply = ""
					result.ReplyTruncated = historyIncomplete
					lastMessage = event.Message.MessageID
				}
				appendReply(&result, event.Message.Text)
				result.ReplyTruncated = result.ReplyTruncated || event.Message.Truncated
			}
		}
		if snapshot && frame.Type == contract.ChatFrameEvents && !replayed {
			replayed = true
			if tracked == "" && state != nil {
				switch {
				case a.TurnID != "" && state.Turn.TurnID == a.TurnID:
					tracked = a.TurnID
					collecting = true
					historyIncomplete = true
					result.ReplyTruncated = true
					if !state.Turn.Running {
						result.Status = "ended"
						result.StopReason = state.Turn.StopReason
						result.Unfinished = state.Turn.Unfinished
						result.ReplyTruncated = true
						return result, nil
					}
					result.Status = "running"
				case a.QueueItemID != "":
					found := false
					for _, item := range state.Queue.Items {
						if item.ID == a.QueueItemID {
							found = true
							result.Status = "queued"
							break
						}
					}
					if !found {
						return nil, errors.New("queued input absent and matching turn history unavailable")
					}
				default:
					return nil, errors.New("selected turn is not in retained history or current state")
				}
			}
		}
		if replayed && state != nil && tracked != "" && state.Turn.TurnID == tracked {
			if len(state.Requests) > 0 {
				result.Status = "waiting_for_user"
				result.Requests = state.Requests
				return result, nil
			}
			if !state.Turn.Running && state.Turn.StopReason != "" {
				result.Status = "ended"
				result.StopReason = state.Turn.StopReason
				result.Unfinished = state.Turn.Unfinished
				result.ReplyTruncated = true
				return result, nil
			}
		}
		if replayed && a.TimeoutMS == 0 {
			return result, nil
		}
	}
}
func appendReply(result *waitResult, text string) {
	const maxReplyBytes = 64 << 10
	remaining := maxReplyBytes - len(result.Reply)
	if len(text) > remaining {
		text = text[:remaining]
		for len(text) > 0 && !utf8.ValidString(text) {
			text = text[:len(text)-1]
		}
		result.ReplyTruncated = true
	}
	result.Reply += text
	// Preserve original whitespace across streaming chunks; trim only at display
	// time in clients so a split word cannot be accidentally concatenated.
	result.Reply = strings.ToValidUTF8(result.Reply, "�")
}
