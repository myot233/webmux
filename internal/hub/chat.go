package hub

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"net/url"
	"strconv"

	"github.com/rainchan/webmux/internal/contract"
	"github.com/rainchan/webmux/internal/hybrid"
	"github.com/rainchan/webmux/internal/wire"
)

// ChatResume reads the chat socket's resume point: ?generation= of the log
// the client holds and seq= of the last event it applied (0 for none).
// Malformed values are dropped, never rejected: the client then gets the
// whole cached log.
func ChatResume(q url.Values) *hybrid.ResumePoint {
	generation := q.Get("generation")
	seq, err := strconv.ParseUint(q.Get("seq"), 10, 64)
	if err != nil || seq == math.MaxUint64 || !generationPattern.MatchString(generation) {
		return nil
	}
	return &hybrid.ResumePoint{Generation: generation, Seq: seq + 1}
}

// Chat is the only writer of a chat connection: the snapshot, the events
// from the resume point, then live events and state. A subscription the
// session dropped closes the connection (4408 when it fell behind); ending
// the connection only releases the subscription.
func Chat(ctx context.Context, sub *hybrid.Subscription, c MessageConn) {
	defer sub.Close()
	defer func() { _ = c.CloseNow() }()
	ctx = c.CloseRead(ctx)
	if !writeChat(ctx, c, wire.ChatSnapshot(sub.Snapshot)) || !writeChat(ctx, c, wire.ChatEvents(sub.Snapshot.Generation, sub.Events)) {
		return
	}
	for {
		// A dropped subscription ends the connection before its queued
		// frames: the client resumes from what it has.
		select {
		case <-sub.Done:
			closeChat(c, sub.Err())
			return
		default:
		}
		select {
		case <-ctx.Done():
			return
		case <-sub.Done:
			closeChat(c, sub.Err())
			return
		case f := <-sub.Out:
			if !writeChat(ctx, c, wire.ChatFrame(f)) {
				return
			}
		}
	}
}

func writeChat(ctx context.Context, c MessageConn, f contract.ChatFrame) bool {
	data, err := json.Marshal(f)
	if err != nil {
		_ = c.Close(StatusInternalError, "invalid chat frame")
		return false
	}
	if write(ctx, c, MessageText, data) != nil {
		_ = c.Close(StatusSlowClient, "write timeout")
		return false
	}
	return true
}

// closeChat closes a connection whose subscription the session ended.
func closeChat(c MessageConn, reason error) {
	switch {
	case errors.Is(reason, hybrid.ErrSlow):
		_ = c.Close(StatusSlowClient, "slow client")
	case errors.Is(reason, hybrid.ErrReset):
		_ = c.Close(StatusTryAgainLater, "chat log reset; reconnect")
	default:
		_ = c.Close(StatusSessionExited, "session ended")
	}
}
