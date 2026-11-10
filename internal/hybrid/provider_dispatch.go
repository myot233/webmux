package hybrid

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/rainchan/webmux/internal/session"
)

type executionContextKey struct{}

// ExecutionContext carries an internal ownership token through existing config
// and lifecycle service calls. HTTP headers and request JSON cannot set it.
func ExecutionContext(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, executionContextKey{}, id)
}

func (a *actor) checkDispatch() error {
	if a.contextExecutionID != "" && a.contextExecutionID != a.operationExecutionID {
		if a.svc.conversations != nil {
			e, err := a.svc.conversations.Execution(context.Background(), a.contextExecutionID)
			if err == nil && (e.Status == "inline" || (e.Status == "failed" && a.operationExecutionID != "")) {
				a.contextExecutionID = ""
				return nil
			}
		}
		return ErrSwitching
	}
	return nil
}

// QuiesceProvider admits only an idle ACP writer with no observed background
// work or external owner. Admission and closing share the actor's queue, so a
// concurrent prompt cannot slip between the check and retirement.
func (s *Service) QuiesceProvider(ctx context.Context, id string) error {
	a, err := s.actor(id)
	if errors.Is(err, ErrNotFound) {
		rec, err := s.Record(id)
		if err != nil {
			return err
		}
		if rec.State == StateActive || rec.PID != 0 {
			return ErrConflict
		}
		if err := s.manager.Remove(id); err != nil && !errors.Is(err, session.ErrNotFound) {
			return ErrConflict
		}
		return nil
	}
	if err != nil {
		return err
	}
	_, err = call(ctx, a, func() (struct{}, error) {
		if a.state != StateActive {
			return struct{}{}, nil
		}
		if err := a.checkDispatch(); err != nil {
			return struct{}{}, err
		}
		if a.side != SideACP || a.starting || a.opening || a.switching != nil || a.closing {
			return struct{}{}, ErrSwitching
		}
		if a.turn != nil || a.work.running() || a.chat.Turn.Unfinished || len(a.requests) > 0 || len(a.chat.Queue.Items) > 0 {
			return struct{}{}, ErrBusy
		}
		if a.link != nil && a.bgBase != nil && hasNewProcess(a.link.sidecar.PID(), a.bgBase) {
			return struct{}{}, ErrBusy
		}
		if a.externallyHeld(a.agentSessionID, true) {
			return struct{}{}, ErrExternalWriter
		}
		a.beginClose()
		return struct{}{}, nil
	})
	if err != nil {
		return err
	}
	tick := time.NewTicker(10 * time.Millisecond)
	defer tick.Stop()
	for {
		rec, err := s.Record(id)
		if err != nil {
			return err
		}
		_, shellErr := s.manager.Get(id)
		if rec.State == StateClosed && rec.PID == 0 && errors.Is(shellErr, session.ErrNotFound) {
			return nil
		}
		if rec.State == StateClosed && rec.PID != 0 {
			return fmt.Errorf("%w: agent 进程未确认退出", ErrConflict)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-tick.C:
		}
	}
}

func (s *Service) NativeContinuation(ctx context.Context, id string) (bool, error) {
	a, err := s.actor(id)
	if err != nil {
		return false, err
	}
	return call(ctx, a, func() (bool, error) { return a.nativeContinue, nil })
}

func (a *actor) acknowledgeCurrentRun() {
	if a.turn == nil {
		return
	}
	a.turn.consumed = true
}

func (s *Service) ClaimProvider(ctx context.Context, id, owner string) error {
	a, err := s.actor(id)
	if err != nil {
		return err
	}
	_, err = call(ctx, a, func() (struct{}, error) {
		if err := a.checkDispatch(); err != nil {
			return struct{}{}, err
		}
		if a.turn != nil || a.work.running() || a.chat.Turn.Unfinished {
			return struct{}{}, ErrBusy
		}
		a.contextExecutionID = owner
		return struct{}{}, nil
	})
	return err
}
