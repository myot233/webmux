package hybrid

import (
	"time"

	"github.com/rainchan/webmux/internal/settings"
)

// watchSettings follows chatIdleSleepMinutes (D27). A subscription closed
// under it (a slow reader or a failed commit) is renewed; settings that
// cannot be read fall back to the default and are not watched further.
func (s *Service) watchSettings(src *settings.Service) {
	defer close(s.settingsDone)
	for {
		sub, err := src.Subscribe()
		if err != nil {
			s.log.Warn("hybrid idle sleep uses the default: settings unavailable", "error", err)
			s.setIdleMinutes(settings.DefaultChatIdleSleepMinutes)
			return
		}
		s.mu.Lock()
		if s.closed {
			s.mu.Unlock()
			sub.Close()
			return
		}
		s.settingsSub = sub
		s.mu.Unlock()
		for e := range sub.Events {
			values := e.Changes.Values
			if e.Snapshot != nil {
				values = e.Snapshot.Values
			}
			minutes := settings.DefaultChatIdleSleepMinutes
			if v := values.ChatIdleSleepMinutes; v != nil {
				minutes = *v
			}
			s.setIdleMinutes(minutes)
		}
		s.mu.Lock()
		closed := s.closed
		s.settingsSub = nil
		s.mu.Unlock()
		if closed {
			return
		}
	}
}

// setIdleMinutes applies a new idle sleep setting to every session at once.
func (s *Service) setIdleMinutes(minutes int) {
	s.mu.Lock()
	changed := s.idleMinutes != minutes
	s.idleMinutes = minutes
	actors := make([]*actor, 0, len(s.actors))
	for _, a := range s.actors {
		actors = append(actors, a)
	}
	s.mu.Unlock()
	if !changed {
		return
	}
	for _, a := range actors {
		// The actor re-arms its sleep timer after every message.
		a.post(func() {})
	}
}

// sleepAfter is how long an idle chat side runs before it sleeps; 0 never.
func (s *Service) sleepAfter() time.Duration {
	s.mu.Lock()
	defer s.mu.Unlock()
	return time.Duration(s.idleMinutes) * s.timing.minute
}

// touch restarts the idle clock: a command, a turn end or a wake.
func (a *actor) touch() { a.lastActive = time.Now() }

// activity is a chat command arriving: it restarts the idle clock and
// wakes a dormant session.
func (a *actor) activity() {
	a.touch()
	a.wake()
}

// canSleep: the chat side is connected and idle (no turn, request or
// queued prompt) and nothing else is in progress.
func (a *actor) canSleep() bool {
	return a.state == StateActive && !a.closing && !a.starting && !a.opening && a.switching == nil &&
		a.side == SideACP && a.link != nil && a.turn == nil && len(a.requests) == 0 && len(a.chat.Queue.Items) == 0
}

// armSleep keeps the sleep timer due at the idle deadline while the chat
// side can sleep; it runs after every actor message.
func (a *actor) armSleep() {
	after := a.svc.sleepAfter()
	if after <= 0 || !a.canSleep() {
		if a.sleepTimer != nil {
			a.sleepTimer.Stop()
			a.sleepTimer = nil
		}
		return
	}
	at := a.lastActive.Add(after)
	if a.sleepTimer != nil && a.sleepAt.Equal(at) {
		return
	}
	if a.sleepTimer != nil {
		a.sleepTimer.Stop()
	}
	a.sleepAt = at
	a.sleepTimer = time.AfterFunc(time.Until(at), func() { a.post(func() { a.sleepDue(at) }) })
}

// sleepDue puts the session to sleep when the timer for at still holds.
func (a *actor) sleepDue(at time.Time) {
	if a.sleepTimer == nil || !a.sleepAt.Equal(at) {
		return
	}
	a.sleepTimer = nil
	after := a.svc.sleepAfter()
	if after <= 0 || !a.canSleep() || time.Since(a.lastActive) < after {
		return
	}
	a.sleep()
}

// sleep closes the agent session and stops its process: the session stays
// on the chat side, dormant, until a command or chat connection wakes it.
func (a *actor) sleep() {
	link := a.link
	a.link = nil
	a.sleeping = true
	a.chat.Dormant = true
	a.chat.PromptCapabilities = PromptCapabilities{}
	a.dirty = true
	a.svc.log.Info("hybrid session sleeping", "id", a.id)
	go func() {
		err := link.release()
		a.post(func() { a.slept(err) })
	}()
}

// slept finishes a sleep. A process that would not stop interrupts the
// session, keeping its identity in the record: starting another could
// make two writers.
func (a *actor) slept(err error) {
	a.sleeping = false
	a.dirty = true
	if err != nil {
		a.svc.log.Warn("hybrid agent did not stop for sleep", "id", a.id, "error", err)
		a.interrupt("idle", false)
	} else {
		a.record(true, func(r *Record) { r.PID, r.Process = 0, nil })
	}
	if a.wakeAfterSleep {
		a.wakeAfterSleep = false
		a.wake()
	}
	a.afterTransition()
}

// wake reopens a dormant session's agent: with session/resume when the
// agent declared it (the log continues), else session/load in a new
// generation.
func (a *actor) wake() {
	switch {
	case !a.chat.Dormant || a.opening || a.state != StateActive || a.closing || a.switching != nil || a.side != SideACP:
		return
	case a.sleeping:
		a.wakeAfterSleep = true
		return
	}
	a.svc.log.Info("hybrid session waking", "id", a.id)
	a.reopen(nil, false)
}
