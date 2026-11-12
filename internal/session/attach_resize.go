package session

import (
	"context"
	"fmt"
	"time"
)

// Attach registers an anonymous subscriber with a full replay.
func (s *Session) Attach() (*Subscriber, error) {
	return s.AttachClient(AttachOptions{})
}

// AttachClient atomically captures history and registers the live subscriber:
// no replay gap. The first client of a session without a controller takes
// control; everyone else observes until they send user input or claim it.
// A shell without a run has no terminal to attach to: ErrNoRun.
func (s *Session) AttachClient(o AttachOptions) (*Subscriber, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.run == nil {
		return nil, ErrNoRun
	}
	if o.Client == "" {
		o.Client = randomClientID()
	}
	s.nextSeq++
	sub := &Subscriber{Out: make(chan Message, terminalOutputQueueCapacity), Slow: make(chan struct{}), client: o.Client, label: o.Label, seq: s.nextSeq}
	if s.control == nil {
		s.setControlLocked(sub)
	} else if s.control.ClientID == sub.client {
		// A reconnect supersedes the connection it replaces.
		s.controlSub = sub
		if s.control.Label != sub.label {
			s.control.Label = sub.label
			s.broadcastLocked(s.controlLocked())
		}
	}
	s.subs[sub] = struct{}{}
	s.replayLocked(sub, o.Resume, nil)
	return sub, nil
}

// Position precedes replayed bytes. "append" continues a terminal at Start
// with the bytes up to End sent as ordinary output; "reset" is followed by a
// snapshot covering Start to End. A truncated reset discards incomplete history
// and starts empty at End, initialized only with synthetic persistent-mode bytes.
// Those synthetic bytes do not correspond to stream offsets.
type Position struct {
	Generation string
	Mode       string
	Start      uint64
	End        uint64
	Truncated  bool
}

// replayLocked queues the grid, position, stream and state for sub. Frames in
// held follow the state; the exit frame stays last because the connection
// closes once it is sent.
func (s *Session) replayLocked(sub *Subscriber, resume *ResumePoint, held []Message) {
	// The grid comes first: an observer must size its terminal before it
	// parses cursor-addressed history, or that history lands on a wrong grid.
	s.sendLocked(sub, s.controlLocked())
	ring := s.run.ring
	start, end := ring.Range()
	position := Position{Generation: s.info.Generation, Mode: "reset", Start: start, End: end, Truncated: start > 0}
	if resume != nil && resume.Generation == s.info.Generation {
		if tail, ok := ring.sinceWithPrefix([]byte{0}, resume.Offset); ok {
			position.Mode, position.Start = "append", resume.Offset
			s.sendLocked(sub, positionMessage(position))
			if len(tail) > 1 {
				s.sendLocked(sub, frameMessage(tail))
			}
		}
	}
	if position.Mode == "reset" {
		if position.Truncated {
			// A raw suffix cannot restore cursor, modes or parser state. It
			// may even start inside a control sequence or UTF-8 character.
			// Later live bytes may still finish a sequence begun before
			// this cut; a redraw is best-effort, not VT state serialization.
			position.Start = end
			s.sendLocked(sub, positionMessage(position))
			s.sendLocked(sub, frameMessage(s.preview.RecoveryFrame()))
		} else {
			s.sendLocked(sub, positionMessage(position))
			s.sendLocked(sub, frameMessage(ring.snapshotWithPrefix([]byte{1})))
		}
	}
	s.sendLocked(sub, s.stateLocked())
	for _, m := range held {
		s.sendLocked(sub, m)
	}
	if s.info.ExitCode != nil {
		s.sendLocked(sub, exitMessage(*s.info.ExitCode))
	}
	if position.Mode == "reset" && position.Truncated {
		// Subscription and replay are already queued under mu, so any
		// response follows them without a replay/live-output gap. A brief
		// PTY width change requests a redraw even from programs that ignore
		// same-size SIGWINCH, without changing the logical grid or controller.
		s.repaintLocked()
	}
}

// Replay discards queued stream frames, which the new snapshot supersedes,
// but keeps submission statuses: they are not part of the stream and a lost
// one would leave a draft locked.
func (s *Session) Replay(sub *Subscriber) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.run == nil {
		// A run that ended for a switch already released its subscribers.
		return
	}
	if _, ok := s.subs[sub]; !ok {
		// MoveClients released this subscriber while the run goes on; its
		// queue still holds Moved, which the connection must receive.
		return
	}
	var statuses []Message
	for {
		select {
		case m := <-sub.Out:
			sub.Release(m)
			if m.Submission != nil {
				statuses = append(statuses, m)
			}
		default:
			s.replayLocked(sub, nil, statuses)
			return
		}
	}
}
func (s *Session) Detach(sub *Subscriber) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.subs, sub)
	// The newest remaining connection of the same client inherits control.
	var heir *Subscriber
	for other := range s.subs {
		if other.client == sub.client && (heir == nil || other.seq > heir.seq) {
			heir = other
		}
	}
	if heir == nil {
		s.submissions.forgetWatcher(sub.client)
	}
	if s.controlSub != sub {
		return
	}
	// Another connection of the same client takes over; otherwise control is
	// not handed to the remaining observers: their layout must not change
	// because another device went away. The next input claims it.
	s.controlSub = heir
	if heir == nil {
		s.control = nil
		s.broadcastLocked(s.controlLocked())
		return
	}
	// The promoted connection's earlier resizes were refused; the control
	// frame makes it send its grid again.
	s.sendLocked(heir, s.controlLocked())
}
func (s *Session) Write(p []byte) error {
	return s.WriteContext(context.Background(), p)
}

// ScriptResize is a resize a script asks for over REST. Label names the
// controller while Take holds control, as ServerInput.Label does.
type ScriptResize struct {
	Cols, Rows int
	Take       bool
	Label      string
}

// ResizeScript resizes the PTY for a script (POST /api/sessions/{id}/resize).
// It is allowed while no client controls the terminal or the controller is
// itself server-side input. A browser or attach controller refuses it with
// ErrNotController unless in.Take is set: the script then takes control for
// the resize and releases it, as SubmitServer does. The check and the resize
// share one critical section, so a takeover cannot interleave between them.
// ErrExited means the run ended; ErrNoRun that the shell has no terminal run.
func (s *Session) ResizeScript(in ScriptResize) error {
	if err := ValidateSize(in.Cols, in.Rows); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalid, err)
	}
	cols, rows := uint16(in.Cols), uint16(in.Rows)
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.run == nil {
		return ErrNoRun
	}
	if s.info.ExitCode != nil {
		return ErrExited
	}
	if s.control == nil || s.controlSub == nil {
		return s.resizeLocked(cols, rows)
	}
	if !in.Take {
		return fmt.Errorf("%w (a browser or attached client controls it; take control for the resize with take, or webmux resize --take)", ErrNotController)
	}
	controller := &Controller{ClientID: serverClientPrefix + randomClientID(), Label: in.Label, Since: time.Now()}
	s.control, s.controlSub = controller, nil
	s.broadcastLocked(s.controlLocked())
	err := s.resizeLocked(cols, rows)
	s.control = nil
	s.broadcastLocked(s.controlLocked())
	return err
}

// ResizeFrom resizes the PTY only if the subscriber is the controlling
// connection, checked in the same critical section as the resize so that a
// takeover cannot interleave between them.
func (s *Session) ResizeFrom(sub *Subscriber, cols, rows uint16) error {
	if err := ValidateSize(int(cols), int(rows)); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalid, err)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.controlSub != sub {
		return ErrNotController
	}
	return s.resizeLocked(cols, rows)
}

func (s *Session) resizeLocked(cols, rows uint16) error {
	if cols == s.info.Cols && rows == s.info.Rows {
		return nil
	}
	// Same-logical-size attach notifications are intentionally no-ops above:
	// restoring immediately would coalesce the recovery width change away.
	err := s.resizePTYLocked(cols, rows)
	if err == nil {
		if s.repaintTimer != nil {
			s.repaintTimer.Stop()
			s.repaintTimer = nil
		}
		s.info.Cols = cols
		s.info.Rows = rows
		s.info.Preview = s.preview.Resize(cols, rows)
		// Observers follow the controller's grid instead of fitting their own.
		s.broadcastLocked(s.controlLocked())
	}
	return err
}

// repaintLocked briefly changes the PTY width, then restores the logical grid.
// The kernel notifies the actual foreground job on each size change. There is
// no synthetic input, control transfer, or transient control-grid broadcast.
func (s *Session) repaintLocked() {
	r := s.run
	if s.repaintTimer != nil || s.info.ExitCode != nil || r == nil || r.terminal == nil || r.cmd == nil || r.cmd.Process == nil || s.info.Cols == 0 || s.info.Rows == 0 {
		return
	}
	cols := s.info.Cols - 1
	if cols == 0 {
		cols = 2
	}
	if err := s.resizePTYLocked(cols, s.info.Rows); err != nil {
		return
	}
	s.info.Preview = s.preview.Resize(cols, s.info.Rows)
	var timer *time.Timer
	timer = time.AfterFunc(100*time.Millisecond, func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		if s.repaintTimer != timer {
			return
		}
		s.repaintTimer = nil
		if s.info.ExitCode != nil || s.run != r {
			return
		}
		if err := s.resizePTYLocked(s.info.Cols, s.info.Rows); err == nil {
			s.info.Preview = s.preview.Resize(s.info.Cols, s.info.Rows)
		}
	})
	s.repaintTimer = timer
}
