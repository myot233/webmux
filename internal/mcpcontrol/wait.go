package mcpcontrol

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/rainchan/webmux/internal/contract"
	"github.com/rainchan/webmux/internal/mcpprotocol"
	"github.com/rainchan/webmux/internal/sessionobserve"
)

func (c Client) waitTools() []mcpprotocol.Tool {
	return []mcpprotocol.Tool{tool("session_wait", "Wait for a specific chat turn, queued input, first launched turn, or terminal process exit. Never infer completion from idle. A timeout ends observation only; continue waiting with the same selector. Reports pending user requests without approving them.", true, map[string]any{
		"sessionId": text(), "until": enumeration("turn", "exit"), "turnId": text(), "queueItemId": text(), "firstTurn": boolean(), "timeoutMs": integer(0, 60000),
	}, []string{"sessionId"}, c.wait)}
}

func (c Client) wait(ctx context.Context, raw json.RawMessage) (any, error) {
	a := contract.MCPSessionWaitRequest{Until: "turn", TimeoutMS: 30000}
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
func initialWait(a contract.MCPSessionWaitRequest) contract.MCPSessionWaitResult {
	return contract.MCPSessionWaitResult{SessionID: a.SessionID, TurnID: a.TurnID, QueueItemID: a.QueueItemID, Status: "pending", Requests: []contract.ChatRequest{}}
}
func (c Client) waitExit(ctx context.Context, a contract.MCPSessionWaitRequest) (any, error) {
	result := initialWait(a)
	subscription, err := sessionobserve.OpenSessions(ctx, c.API, responseLimit)
	if err != nil {
		if ctx.Err() != nil {
			return observedWait(result, ctx.Err())
		}
		return nil, apiError(err)
	}
	defer subscription.Close()
	for {
		if _, err := subscription.Read(ctx); err != nil {
			if ctx.Err() != nil {
				return observedWait(result, ctx.Err())
			}
			if errors.Is(err, sessionobserve.ErrInvalidFrame) {
				return nil, errors.New("invalid session event")
			}
			return nil, errors.New("session observation disconnected")
		}
		info, exited, err := subscription.Exit(a.SessionID)
		if err != nil {
			return nil, err
		}
		result.Status = string(info.Status)
		if exited {
			result.ExitCode = info.ExitCode
			return result, nil
		}
		if a.TimeoutMS == 0 {
			return result, nil
		}
	}
}

func (c Client) waitTurn(ctx context.Context, a contract.MCPSessionWaitRequest) (any, error) {
	result := initialWait(a)
	// Retained history is a single large frame. ReplyLimit is independent
	// and applied as chunks are accumulated, not after a full reply copy.
	subscription, err := sessionobserve.OpenChat(ctx, c.API, a.SessionID, 64<<20)
	if err != nil {
		if ctx.Err() != nil {
			return observedWait(result, ctx.Err())
		}
		return nil, apiError(err)
	}
	defer subscription.Close()
	observation, err := subscription.WaitTurn(ctx, sessionobserve.TurnSelector{
		TurnID: a.TurnID, QueueItemID: a.QueueItemID, FirstTurn: a.FirstTurn,
	}, sessionobserve.TurnPolicy{
		StopAtRequest: true, ReplyLimit: 64 << 10, InitialOnly: a.TimeoutMS == 0,
	})
	result.TurnID = observation.TurnID
	result.QueueItemID = observation.QueueItemID
	result.Status = observation.Phase
	result.StopReason = observation.StopReason
	result.Error = observation.Error
	result.Reply = observation.Reply
	result.ReplyTruncated = observation.ReplyTruncated
	result.Unfinished = observation.Unfinished
	result.Requests = observation.Requests
	if err != nil && !errors.Is(err, context.DeadlineExceeded) && !errors.Is(err, context.Canceled) &&
		!errors.Is(err, sessionobserve.ErrQueueRemoved) && !errors.Is(err, sessionobserve.ErrHistoryUnavailable) {
		if errors.Is(err, sessionobserve.ErrInvalidFrame) {
			return nil, errors.New("invalid chat frame")
		}
		return nil, errors.New("chat observation disconnected; no completion inferred")
	}
	return observedWait(result, err)
}

func observedWait(result contract.MCPSessionWaitResult, err error) (any, error) {
	if errors.Is(err, context.DeadlineExceeded) {
		result.WaitTimedOut = true
		return result, nil
	}
	if err != nil {
		return nil, err
	}
	return result, nil
}
