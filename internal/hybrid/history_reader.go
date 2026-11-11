package hybrid

import (
	"context"
	"time"

	"github.com/rainchan/webmux/internal/agentprofile"

	"github.com/rainchan/webmux/internal/acp"
	"github.com/rainchan/webmux/internal/conversation"
)

func (s *Service) HistoryReaderAvailable(ctx context.Context, scope conversation.ReaderScope) bool {
	if s.historyReader == nil {
		return false
	}
	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	return len(s.historyReader(ctx, scope)) > 0
}

func (a *actor) historyServers(ctx context.Context) ([]acp.MCPServer, string) {
	if a.svc.historyReader == nil || a.svc.conversations == nil {
		return nil, ""
	}
	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	scope, err := a.svc.conversations.ReaderScope(ctx, a.id)
	if err != nil {
		return nil, ""
	}
	// A failed optional lookup never prevents session/new, load or resume.
	servers := a.svc.historyReader(ctx, scope)
	if len(servers) == 0 {
		return nil, ""
	}
	return servers, scope.HandoffID
}

// RefreshHistoryReader reopens an idle reused target through exact native resume
// when its frozen scope changed. No history replay, new thread or user turn is
// allowed as a substitute for refreshing the MCP configuration.
func (s *Service) RefreshHistoryReader(ctx context.Context, id, owner string) error {
	a, err := s.actor(id)
	if err != nil {
		return err
	}
	_, scope := a.historyServers(ctx)
	admit, err := call(ctx, a, func() (historyAdmission, error) {
		if a.link == nil || a.link.historyScope == scope {
			return historyAdmission{}, nil
		}
		if a.contextExecutionID != owner {
			return historyAdmission{}, ErrSwitching
		}
		if err := a.ready(); err != nil {
			return historyAdmission{}, err
		}
		if !a.canSendNow() || a.chat.Turn.Unfinished || a.work.running() || len(a.requests) > 0 {
			return historyAdmission{}, ErrBusy
		}
		if !agentprofile.ResumesSessions(a.cmd.Agent) || a.link.caps.Resume == nil {
			return historyAdmission{}, ErrNativeResumeUnavailable
		}
		if a.externallyHeld(a.agentSessionID, true) {
			return historyAdmission{}, ErrExternalWriter
		}
		old := a.link
		a.link = nil
		a.opening = true
		a.epoch++
		a.dirty = true
		return historyAdmission{link: old, epoch: a.epoch, sid: a.agentSessionID, cwd: a.agentCwd}, nil
	})
	if err != nil || admit.link == nil {
		return err
	}
	outcome := admit.link.release()
	var reopened openResult
	if outcome == nil {
		if !a.sync(func() { a.record(true, func(r *Record) { r.PID, r.Process = 0, nil }) }) {
			return errStopped
		}
		reopened = a.openACPWithRequirement(admit.epoch, admit.sid, admit.cwd, true, nil, true)
		if reopened.fail != nil {
			outcome = reopened.fail.err
		}
	}
	if !a.sync(func() {
		a.opening = false
		a.dirty = true
		if outcome != nil {
			a.interrupt("idle", false)
		} else {
			a.installLink(reopened.link, reopened.init, reopened.config, reopened.notes)
			pid := reopened.link.sidecar.PID()
			process := processIdentity(pid)
			a.record(true, func(r *Record) { r.PID, r.Process = pid, process })
		}
		a.afterTransition()
	}) {
		if reopened.link != nil {
			_ = reopened.link.stop()
		}
		return errStopped
	}
	return outcome
}
