package hub

import (
	"bytes"
	"compress/flate"
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/rainchan/webmux/internal/contract"
	"github.com/rainchan/webmux/internal/hybrid"
	"github.com/rainchan/webmux/internal/session"
	"github.com/rainchan/webmux/internal/wire"
)

var (
	clientPattern     = regexp.MustCompile(`^[A-Za-z0-9_-]{8,64}$`)
	generationPattern = regexp.MustCompile(`^[0-9a-f]{16}$`)
)

// AttachOptions reads the optional client identity and resume point from the
// terminal WebSocket query. Malformed values are dropped, never rejected:
// an anonymous client simply observes or replays in full.
func AttachOptions(q url.Values) session.AttachOptions {
	var o session.AttachOptions
	if client := q.Get("client"); clientPattern.MatchString(client) {
		o.Client = client
	}
	label := strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || r == unicode.ReplacementChar {
			return -1
		}
		return r
	}, q.Get("label"))
	if runes := []rune(strings.TrimSpace(label)); len(runes) > 40 {
		o.Label = string(runes[:40])
	} else {
		o.Label = string(runes)
	}
	generation := q.Get("generation")
	offset, err := strconv.ParseUint(q.Get("offset"), 10, 64)
	if err == nil && generationPattern.MatchString(generation) {
		o.Resume = &session.ResumePoint{Generation: generation, Offset: offset}
	}
	return o
}

type submission struct {
	contract.SubmissionHeader
	body   []byte
	submit []byte
}

// parseSubmission splits a 0x03 payload: a JSON header line, then the body
// bytes followed by the submit key bytes. Terminal bytes never enter JSON.
func parseSubmission(p []byte) (*submission, bool) {
	header, rest, found := bytes.Cut(p, []byte{'\n'})
	var item submission
	if !found || json.Unmarshal(header, &item.SubmissionHeader) != nil || !clientPattern.MatchString(item.ID) {
		return nil, false
	}
	if item.BodyBytes < 0 || item.BodyBytes > len(rest) || len(rest) == 0 {
		return &item, false
	}
	item.body, item.submit = rest[:item.BodyBytes], rest[item.BodyBytes:]
	return &item, true
}

// Buffer short input bursts; overflow disconnects a slow connection.
const terminalInputQueueCapacity = 16

type input struct {
	data   []byte
	reply  bool
	submit *submission
}

// jsonFrame encodes a JSON control frame. Its value is a contract type,
// which archtest rule R8 checks at every call.
func jsonFrame(kind byte, v any) ([]byte, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	return contract.Frame(kind, data), nil
}

var errInvalidMessage = errors.New("invalid session message")

// encode rejects malformed payloads instead of dropping them or choosing one
// arbitrarily. Raw bytes remain shared; only typed control payloads are encoded.
func encode(m session.Message) (frame []byte, exit bool, err error) {
	if !m.Valid() {
		return nil, false, errInvalidMessage
	}
	switch {
	case m.Frame != nil:
		return m.Frame, false, nil
	case m.Info != nil:
		frame, err = jsonFrame(contract.FrameState, wire.Session(*m.Info))
	case m.Exit != nil:
		frame, err = jsonFrame(contract.FrameExit, contract.ExitFrame{ExitCode: *m.Exit})
		exit = true
	case m.Position != nil:
		frame, err = jsonFrame(contract.FramePosition, wire.StreamPosition(*m.Position))
	case m.Submission != nil:
		frame, err = jsonFrame(contract.FrameSubmission, wire.SubmissionStatus(*m.Submission))
	case m.Control != nil:
		frame, err = jsonFrame(contract.FrameControl, wire.ControlState(*m.Control))
	}
	return frame, exit, err
}

func write(ctx context.Context, c MessageConn, kind MessageType, p []byte) error {
	deadline, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	return c.Write(deadline, kind, p)
}

// movedReason is the 4409 close reason; it names the view the session is
// on or moving to (terminal or chat).
func movedReason(side string) string {
	if side == "" {
		return "session moved"
	}
	return "session moved: " + string(wire.View(side))
}

// sideOf is where a hybrid session is or, during a switch, moves to.
func sideOf(info session.Info) string {
	if info.Hybrid == nil {
		return ""
	}
	if info.Hybrid.To != "" {
		return info.Hybrid.To
	}
	return info.Hybrid.Side
}

// Attach owns the only data writer. Its reader is cancelled and joined on all exits.
// A hybrid session attaches only on its terminal view: in the chat or
// during a view change it refuses with 4409, even while a run still exists.
func Attach(ctx context.Context, s *session.Session, c MessageConn, o session.AttachOptions, deflateReplay bool) {
	if h := s.Info().Hybrid; h != nil && (h.Side != string(hybrid.SideTUI) || h.To != "") {
		_ = c.Close(contract.CloseMoved, movedReason(sideOf(s.Info())))
		return
	}
	sub, err := s.AttachClient(o)
	if err != nil {
		// Only a shell without a terminal run refuses: it is on its other side.
		_ = c.Close(contract.CloseMoved, movedReason(sideOf(s.Info())))
		return
	}
	defer s.Detach(sub)
	ctx, cancel := context.WithCancel(ctx)
	c.SetReadLimit(1024 * 1024)
	readerDone := make(chan struct{})
	inputDone := make(chan struct{})
	inputs := make(chan input, terminalInputQueueCapacity)
	go func() {
		defer close(inputDone)
		defer cancel()
		for {
			select {
			case <-ctx.Done():
				return
			case item := <-inputs:
				if item.submit != nil {
					// A received submission finishes even if this connection
					// drops: its status stays queryable, and the PTY write
					// remains bounded by the write deadline and session exit.
					submit := item.submit
					s.Submit(context.WithoutCancel(ctx), sub, submit.ID, submit.Generation, submit.body, submit.submit)
					if ctx.Err() != nil {
						return
					}
				} else if s.WriteInput(ctx, item.data, item.reply) != nil {
					return
				}
			}
		}
	}()
	enqueue := func(item input) bool {
		select {
		case inputs <- item:
			return true
		case <-ctx.Done():
		default:
			cancel()
		}
		return false
	}
	go func() {
		defer close(readerDone)
		defer cancel()
		for {
			kind, p, err := c.Read(ctx)
			if err != nil {
				return
			}
			if kind != MessageBinary || len(p) == 0 {
				continue
			}
			switch p[0] {
			case contract.FrameInput:
				// User input takes control.
				s.Claim(sub)
				if !enqueue(input{data: p[1:]}) {
					return
				}
			case contract.FrameReply:
				// Every attached xterm answers the same device queries; only
				// the responsible connection's answers reach the PTY, and
				// answering never takes control.
				if s.Responds(sub) && !enqueue(input{data: p[1:], reply: true}) {
					return
				}
			case contract.FrameResize:
				var size contract.TerminalResize
				// Observers render the controller's grid and never resize it.
				if json.Unmarshal(p[1:], &size) == nil {
					_ = s.ResizeFrom(sub, size.Cols, size.Rows)
				}
			case contract.FrameReplay:
				s.Replay(sub)
			case contract.FrameSubmit:
				item, ok := parseSubmission(p[1:])
				if !ok {
					if item != nil {
						s.SendSubmissionStatus(sub, session.SubmissionStatus{ID: item.ID, State: session.SubmissionFailed, Error: "invalid"})
					}
					continue
				}
				s.Claim(sub)
				if !enqueue(input{submit: item}) {
					return
				}
			case contract.FrameQuery:
				var query contract.SubmissionQuery
				if json.Unmarshal(p[1:], &query) != nil || len(query.IDs) > 64 {
					continue
				}
				ids := query.IDs[:0]
				for _, id := range query.IDs {
					if clientPattern.MatchString(id) {
						ids = append(ids, id)
					}
				}
				s.QuerySubmissions(sub, ids)
			case contract.FrameClaim:
				s.Claim(sub)
			}
		}
	}()
	defer func() {
		cancel()
		_ = c.CloseNow()
		<-readerDone
		<-inputDone
	}()
	writeMessages(ctx, c, sub, deflateReplay)
}

// Output pacing. An idle connection sends output at once, so a keystroke echo
// never waits for later bytes. While a burst flows, such as a TUI repainting
// its whole transcript after a resize, frames are spaced about one display
// frame apart: output is combined until the burst pauses for outputQuietGap,
// the frame deadline passes, the frame reaches maxOutputFrame, or a control
// message must keep its place. The browser then parses and paints a redraw in
// a few large frames instead of hundreds of fragments.
//
// A replay is exempt: it is one bounded transfer the client waits for, so its
// frames go out as fast as the connection takes them, and the first live
// output after it is not delayed either. When the client asked for it and the
// WebSocket itself is not compressed, the snapshot frame is DEFLATEd here, which
// is what shortens a restore over a slow or high-latency link.
const (
	maxOutputFrame      = 1 + 64*1024
	outputFrameInterval = 16 * time.Millisecond
	outputQuietGap      = 4 * time.Millisecond
	replayDeflateMin    = 512
)

var deflaters = sync.Pool{New: func() any {
	w, _ := flate.NewWriter(nil, flate.DefaultCompression)
	return w
}}

// deflateFrame wraps frame in a FrameDeflate frame, or returns it unchanged
// when it is small or does not shrink.
func deflateFrame(frame []byte) []byte {
	if len(frame) < replayDeflateMin {
		return frame
	}
	buf := bytes.NewBuffer(make([]byte, 1, len(frame)/2))
	buf.Bytes()[0] = contract.FrameDeflate
	w := deflaters.Get().(*flate.Writer)
	w.Reset(buf)
	_, err := w.Write(frame)
	if err == nil {
		err = w.Close()
	}
	deflaters.Put(w)
	if err != nil || buf.Len() >= len(frame) {
		return frame
	}
	return buf.Bytes()
}

// writeMessages is the sole terminal data writer, including aggregation.
func writeMessages(ctx context.Context, c MessageConn, sub *session.Subscriber, deflateReplay bool) {
	var pending session.Message
	var hasPending bool
	var combinedBuffer []byte
	var lastOutput time.Time
	// replaying holds from a position frame to the state frame that ends a
	// replay; both are queued under one lock, so nothing else lies between.
	var replaying bool
	timer := time.NewTimer(time.Hour)
	timer.Stop()
	for {
		var m session.Message
		if hasPending {
			m = pending
			pending = session.Message{}
			hasPending = false
		} else {
			select {
			case <-ctx.Done():
				return
			case <-sub.Slow:
				_ = c.Close(StatusSlowClient, "slow client")
				return
			case m = <-sub.Out:
				sub.Release(m)
			}
		}
		if m.Moved != nil {
			_ = c.Close(contract.CloseMoved, movedReason(*m.Moved))
			return
		}
		frame, exit, err := encode(m)
		if err != nil {
			_ = c.Close(StatusInternalError, "invalid session message")
			return
		}
		switch {
		case m.Position != nil:
			replaying = true
			lastOutput = time.Time{}
		case m.Info != nil:
			replaying = false
		}
		output := m.Frame != nil && frame[0] == contract.FrameOutput
		if output {
			if combinedBuffer == nil {
				combinedBuffer = make([]byte, 0, maxOutputFrame)
			}
			var deadline time.Time
			if since := time.Since(lastOutput); !replaying && since < outputFrameInterval {
				deadline = lastOutput.Add(outputFrameInterval)
			}
			var ok bool
			frame, pending, hasPending, ok = collectOutput(ctx, c, sub, append(combinedBuffer[:0], frame...), deadline, timer)
			if !ok {
				return
			}
		}
		if replaying && deflateReplay && m.Frame != nil && frame[0] == contract.FrameSnapshot {
			frame = deflateFrame(frame)
		}
		if write(ctx, c, MessageBinary, frame) != nil {
			_ = c.Close(StatusSlowClient, "write timeout")
			return
		}
		if output && !replaying {
			lastOutput = time.Now()
		}
		if exit {
			_ = c.Close(StatusSessionExited, "session exited")
			return
		}
	}
}

// collectOutput appends further output to combined. Queued output is always
// taken; with a deadline it also waits for more until the burst goes quiet.
// It returns the frame, a message that ended it and must be sent next, and
// false when the connection is finished.
func collectOutput(ctx context.Context, c MessageConn, sub *session.Subscriber, combined []byte, deadline time.Time, timer *time.Timer) (frame []byte, next session.Message, hasNext, ok bool) {
	for len(combined) < maxOutputFrame {
		var m session.Message
		select {
		case m = <-sub.Out:
		case <-ctx.Done():
			return nil, next, false, false
		case <-sub.Slow:
			_ = c.Close(StatusSlowClient, "slow client")
			return nil, next, false, false
		default:
			wait := min(outputQuietGap, time.Until(deadline))
			if deadline.IsZero() || wait <= 0 {
				return combined, next, false, true
			}
			timer.Reset(wait)
			select {
			case m = <-sub.Out:
				timer.Stop()
			case <-timer.C:
				return combined, next, false, true
			case <-ctx.Done():
				timer.Stop()
				return nil, next, false, false
			case <-sub.Slow:
				timer.Stop()
				_ = c.Close(StatusSlowClient, "slow client")
				return nil, next, false, false
			}
		}
		sub.Release(m)
		if !m.Valid() || m.Frame == nil || m.Frame[0] != contract.FrameOutput || len(combined)+len(m.Frame)-1 > maxOutputFrame {
			return combined, m, true, true
		}
		combined = append(combined, m.Frame[1:]...)
	}
	return combined, next, false, true
}
