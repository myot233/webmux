package hybrid

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"time"

	"github.com/rainchan/webmux/internal/session"
)

type executionContextKey struct{}

type writerClaim struct {
	owner string
	refs  int
}

// ClaimConversation holds an admission token, without holding a state mutex
// across process work. Nested calls by the same internal execution share it;
// ordinary resumes each have a distinct owner and cannot race a transition.
func (s *Service) ClaimConversation(id, owner string) (func(), error) {
	if owner == "" {
		owner = rand.Text()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil, ErrUnavailable
	}
	if s.writerClaims == nil {
		s.writerClaims = map[string]writerClaim{}
	}
	claim := s.writerClaims[id]
	if claim.refs > 0 && claim.owner != owner {
		return nil, ErrSwitching
	}
	claim.owner, claim.refs = owner, claim.refs+1
	s.writerClaims[id] = claim
	return func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		claim := s.writerClaims[id]
		claim.refs--
		if claim.refs == 0 {
			delete(s.writerClaims, id)
		} else {
			s.writerClaims[id] = claim
		}
	}, nil
}

// ExecutionContext carries an internal ownership token through existing config
// and lifecycle service calls. HTTP headers and request JSON cannot set it.
func ExecutionContext(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, executionContextKey{}, id)
}

func (a *actor) checkDispatch(allowFailedRecovery bool) error {
	if a.contextExecutionID != "" && a.contextExecutionID != a.operationExecutionID {
		if a.svc.conversations != nil {
			e, err := a.svc.conversations.DispatchExecution(context.Background(), a.contextExecutionID)
			if err == nil && (e.Status == "inline" || e.Status == "cancelled") {
				a.contextExecutionID = ""
				return nil
			}
			if err == nil && e.Status == "failed" && a.operationExecutionID != "" {
				a.contextExecutionID = a.operationExecutionID
				return nil
			}
			// A definite failure permits view-based recovery (for example,
			// terminal authentication), not a new ordinary chat dispatch.
			// Retain the owner until an explicit handoff retry replaces it.
			if err == nil && e.Status == "failed" && allowFailedRecovery {
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
func (s *Service) QuiesceProvider(ctx context.Context, id string, environment ...func() (map[string]string, error)) error {
	a, err := s.actor(id)
	if errors.Is(err, ErrNotFound) {
		rec, err := s.Record(id)
		if err != nil {
			return err
		}
		if rec.State == StateActive || rec.PID != 0 {
			return ErrConflict
		}
		var env func() (map[string]string, error)
		if len(environment) > 0 {
			env = environment[0]
		}
		if held, err := s.archivedProviderHeld(rec, env); err != nil {
			return err
		} else if held {
			return ErrExternalWriter
		}
		if err := s.manager.Remove(id); err != nil && !errors.Is(err, session.ErrNotFound) {
			return ErrConflict
		}
		return nil
	}
	if err != nil {
		return err
	}
	retire := false
	_, err = call(ctx, a, func() (struct{}, error) {
		if a.state != StateActive {
			if a.starting || a.opening || a.switching != nil || a.closing {
				return struct{}{}, ErrSwitching
			}
			record, ok := a.svc.records.get(id)
			if !ok || record.PID != 0 {
				return struct{}{}, ErrConflict
			}
			if a.externallyHeld(a.agentSessionID, true) {
				return struct{}{}, ErrExternalWriter
			}
			retire = true
			return struct{}{}, nil
		}
		if err := a.checkDispatch(false); err != nil {
			return struct{}{}, err
		}
		if a.side != SideACP || a.starting || a.opening || a.switching != nil || a.closing {
			return struct{}{}, ErrSwitching
		}
		if a.turn != nil || a.work.running() || a.chat.Turn.Unfinished || len(a.requests) > 0 || len(a.chat.Queue.Items) > 0 {
			return struct{}{}, ErrBusy
		}
		if a.backgroundProcessRunning() {
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
	if retire {
		// Interrupted actors retain their log and exited shell for inspection.
		// Retire those projections without pretending the record was closed.
		return s.clear(ctx, id)
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
		if err := a.ready(); err != nil {
			return struct{}{}, err
		}
		if !a.canSendNow() || a.work.running() || a.chat.Turn.Unfinished || len(a.requests) > 0 || a.backgroundProcessRunning() {
			return struct{}{}, ErrBusy
		}
		if a.externallyHeld(a.agentSessionID, true) {
			return struct{}{}, ErrExternalWriter
		}
		a.contextExecutionID = owner
		return struct{}{}, nil
	})
	return err
}
