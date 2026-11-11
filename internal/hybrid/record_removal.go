package hybrid

import (
	"context"
	"errors"
	"fmt"
	"github.com/rainchan/webmux/internal/session"
	"time"
)

// RemoveRecord deletes closed or interrupted record id together with what
// is left of its session, an exited shell kept for its last output. The
// agent's own session store is left alone.
func (s *Service) RemoveRecord(ctx context.Context, id string) error {
	if _, err := s.Record(id); err != nil {
		return err
	}
	release, err := s.claimRecordConversation(ctx, id)
	if err != nil {
		return err
	}
	defer release()
	return s.removeRecord(ctx, id)
}

// forgetPoll is how often Forget checks that a session it closed has
// recorded the close.
const forgetPoll = 50 * time.Millisecond

// Forget deletes session id's record like RemoveRecord, closing an active
// session first and waiting, within ctx, until the close is recorded.
func (s *Service) Forget(ctx context.Context, id string) error {
	rec, err := s.Record(id)
	if err != nil {
		return err
	}
	release, err := s.claimRecordConversation(ctx, id)
	if err != nil {
		return err
	}
	defer release()
	if a, err := s.actor(id); err == nil && rec.State == StateActive {
		if err := a.Close(); err != nil {
			return err
		}
		tick := time.NewTicker(forgetPoll)
		defer tick.Stop()
		for {
			rec, err := s.Record(id)
			if err != nil {
				return err
			}
			if rec.State != StateActive {
				break
			}
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-tick.C:
			}
		}
	}
	return s.removeRecord(ctx, id)
}

// clear ends what is left of an interrupted session before its record is
// resumed or removed: the actor (its timers, probes and chat log) and the
// exited shell. A session still closing or holding a live process refuses
// with ErrConflict.
func (s *Service) clear(ctx context.Context, id string) error {
	if a, err := s.actor(id); err == nil {
		if _, err := call(ctx, a, a.retire); err != nil && !errors.Is(err, ErrNotFound) {
			return err
		}
		a.probes.Wait()
	}
	switch err := s.manager.Remove(id); {
	case err == nil, errors.Is(err, session.ErrNotFound):
		return nil
	case errors.Is(err, session.ErrRunning):
		return fmt.Errorf("%w: 会话的进程仍在运行", ErrConflict)
	default:
		return err
	}
}

// retire ends the actor of an interrupted session so its record can be
// resumed or removed. A session that runs, switches or closes refuses.
func (a *actor) retire() (struct{}, error) {
	switch {
	case a.stopped:
		return struct{}{}, nil
	case a.switching != nil:
		return struct{}{}, ErrSwitching
	case a.state == StateActive || a.closing || a.starting || a.opening:
		return struct{}{}, fmt.Errorf("%w: 会话仍在运行或正在关闭", ErrConflict)
	}
	a.stopTimers()
	a.withdrawAll()
	if link := a.link; link != nil {
		a.link = nil
		_ = link.stop()
	}
	a.end()
	return struct{}{}, nil
}
