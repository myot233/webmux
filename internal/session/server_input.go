package session

import (
	"context"
	"time"
)

// ServerInput is input a client sends over REST rather than a terminal
// connection (POST /api/sessions/{id}/input). Body and Submit are written as
// one acknowledged submission under RequestID, like a browser's draft send;
// each of Keys follows as its own write. Label names the controller, as a
// terminal connection's label does.
type ServerInput struct {
	RequestID string
	Label     string
	Body      []byte
	Submit    []byte
	Keys      [][]byte
}

// serverClientPrefix marks the controller of server-side input; it never
// collides with a browser's client ID.
const serverClientPrefix = "server-"

// SubmitServer writes in to the current run and takes control of the
// terminal for it, as typing in the browser does: the control indicator
// shows in.Label until the write ends, when control is released again like
// a script connection that detaches. A repeated RequestID is never written
// again; the recorded status is returned instead. ErrNoRun means the shell
// has no terminal run.
func (s *Session) SubmitServer(ctx context.Context, in ServerInput) (SubmissionStatus, error) {
	now := time.Now()
	client := serverClientPrefix + in.RequestID
	s.mu.Lock()
	if record := s.submissions.get(in.RequestID, now); record != nil {
		status := record.status
		s.mu.Unlock()
		return status, nil
	}
	r := s.run
	if r == nil {
		s.mu.Unlock()
		return SubmissionStatus{}, ErrNoRun
	}
	record := s.submissions.add(in.RequestID, client, now)
	controller := &Controller{ClientID: client, Label: in.Label, Since: now}
	s.control, s.controlSub = controller, nil
	s.broadcastLocked(s.controlLocked())
	s.mu.Unlock()

	status := SubmissionStatus{ID: in.RequestID, State: SubmissionWritten}
	if len(in.Body) > 0 || len(in.Submit) > 0 {
		written, err := s.writeRunDelay(ctx, r, inputUser, s.options.submitDelay, in.Body, in.Submit)
		if err != nil {
			status.State, status.Error = SubmissionFailed, "write"
			if written > 0 {
				status.State = SubmissionPartial
			}
		}
	}
	for _, key := range in.Keys {
		if status.State != SubmissionWritten {
			break
		}
		if _, err := s.writeRun(ctx, r, inputUser, key); err != nil {
			status.State, status.Error = SubmissionPartial, "write"
		}
	}
	s.mu.Lock()
	record.status = status
	if s.control == controller {
		s.control = nil
		s.broadcastLocked(s.controlLocked())
	}
	s.mu.Unlock()
	return status, nil
}
