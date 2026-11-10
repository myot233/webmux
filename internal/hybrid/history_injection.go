package hybrid

import (
	"context"
	"io"
	"time"

	"github.com/rainchan/webmux/internal/agentprofile"
	"github.com/rainchan/webmux/internal/conversation"
	"github.com/rainchan/webmux/internal/nativerpc"
)

// NativeHistoryAvailable uses an already observed executable version. Preview
// never starts a provider or version probe merely to advertise this capability.
func (s *Service) NativeHistoryAvailable(ctx context.Context, configID string) bool {
	s.mu.Lock()
	var actors []*actor
	for _, a := range s.actors {
		if a.configID == configID {
			actors = append(actors, a)
		}
	}
	s.mu.Unlock()
	for _, a := range actors {
		available, err := call(ctx, a, func() (bool, error) {
			injector, ok := a.cmd.Agent.(agentprofile.HistoryInjector)
			return ok && a.link != nil && injector.SupportsHistoryVersion(a.link.tools.agentVersion), nil
		})
		if err == nil && available {
			return true
		}
	}
	return false
}

type historyAdmission struct {
	link     *acpLink
	injector agentprofile.HistoryInjector
	epoch    int
	sid, cwd string
}

// InjectHistory temporarily replaces an idle ACP writer with a native helper.
// The helper belongs to the same shell, environment and process cleanup tree.
// It never runs a turn. Acknowledged history is committed before ACP reopens.
func (s *Service) InjectHistory(ctx context.Context, id, executionID string, history agentprofile.HistoricalContext) (bool, error) {
	if s.conversations == nil {
		return false, ErrUnavailable
	}
	e, err := s.conversations.Execution(ctx, executionID)
	if err != nil || e.RuntimeID != id || e.Status != "pending" {
		return false, ErrConflict
	}
	a, err := s.actor(id)
	if err != nil {
		return false, err
	}
	admit, err := call(ctx, a, func() (historyAdmission, error) {
		if err := a.ready(); err != nil {
			return historyAdmission{}, err
		}
		injector, supported := a.cmd.Agent.(agentprofile.HistoryInjector)
		if !supported || a.link == nil || !injector.SupportsHistoryVersion(a.link.tools.agentVersion) {
			return historyAdmission{}, nil
		}
		if !a.canSendNow() || a.chat.Turn.Unfinished || a.work.running() || len(a.requests) > 0 {
			return historyAdmission{}, ErrBusy
		}
		if a.externallyHeld(a.agentSessionID, true) {
			return historyAdmission{}, ErrExternalWriter
		}
		intent, err := s.conversations.Injection(ctx, e.DeliveryID)
		if err != nil || intent.NativeThreadID != a.agentSessionID || e.TargetProviderThreadID != a.activity.thread || intent.State != "pending" || intent.CallStarted {
			return historyAdmission{}, ErrConflict
		}
		if len(intent.Items) != len(history.Items) {
			return historyAdmission{}, ErrConflict
		}
		for n, item := range history.Items {
			want := intent.Items[n]
			if item.ID != want.ID || item.Role != want.Role || conversation.TextHash(item.Text) != want.TextHash {
				return historyAdmission{}, ErrConflict
			}
		}
		old := a.link
		a.link = nil
		a.opening = true
		a.epoch++
		a.dirty = true
		return historyAdmission{link: old, injector: injector, epoch: a.epoch, sid: a.agentSessionID, cwd: a.agentCwd}, nil
	})
	if err != nil || admit.link == nil {
		return false, err
	}
	accepted, injectErr := a.injectNative(ctx, e, admit, history)
	var reopened openResult
	if injectErr == nil {
		reopened = a.openACP(admit.epoch, admit.sid, admit.cwd, true, nil)
		if reopened.fail != nil {
			injectErr = reopened.fail.err
		}
	}
	if !a.sync(func() {
		a.opening = false
		a.dirty = true
		if injectErr != nil {
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
		return accepted, errStopped
	}
	return accepted, injectErr
}

func (a *actor) injectNative(ctx context.Context, e conversation.Execution, admit historyAdmission, history agentprofile.HistoricalContext) (accepted bool, outcome error) {
	if err := admit.link.release(); err != nil {
		return false, err
	}
	if !a.sync(func() { a.record(true, func(r *Record) { r.PID, r.Process = 0, nil }) }) {
		return false, errStopped
	}
	if a.externalWriterNow(admit.sid) {
		return false, ErrExternalWriter
	}
	path := admit.link.tools.env[a.cmd.Profile.AgentPathEnv]
	if path == "" {
		path = a.shell.Getenv(a.cmd.Profile.AgentPathEnv)
	}
	if path == "" {
		return false, ErrUnavailable
	}
	argv := a.cmd.NativeProtocolArgv(path, "app-server", "--listen", "stdio://")
	helper, err := a.shell.StartSidecar(argv, a.cmd.ProtocolOnFD3(), admit.link.tools.env)
	if err != nil {
		return false, err
	}
	pid := helper.PID()
	process := processIdentity(pid)
	a.sync(func() { a.record(true, func(r *Record) { r.PID, r.Process = pid, process }) })
	defer func() {
		stop, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := helper.Stop(stop); err != nil {
			outcome = ErrConflict
			return
		}
		a.sync(func() { a.processGone(pid) })
	}()
	out, ok := helper.Stdout().(io.ReadCloser)
	if !ok {
		return false, ErrUnavailable
	}
	conn := nativerpc.New(out, helper.Stdin())
	defer conn.Close()
	if err := conn.Call(ctx, "initialize", map[string]any{"clientInfo": map[string]string{"name": "webmux_handoff", "title": "WebMux", "version": a.svc.version}, "capabilities": map[string]bool{"experimentalApi": false}}, &struct{}{}); err != nil {
		return false, err
	}
	if err := conn.Notify(ctx, "initialized"); err != nil {
		return false, err
	}
	accepted, err = admit.injector.InjectHistory(ctx, agentprofile.HistoryThread{ID: admit.sid, Version: admit.link.tools.agentVersion, RPC: conn, BeforeMutation: func() error { return a.svc.conversations.ClaimInjection(ctx, e.DeliveryID) }}, history)
	state := "unsupported"
	if accepted {
		state = "injected"
	} else if err != nil {
		state = "unknown"
	}
	// A handshake/resume failure before the mutation gate is definitively
	// unsent. Once claimed, every error other than MethodNotFound is unknown.
	i, readErr := a.svc.conversations.Injection(context.WithoutCancel(ctx), e.DeliveryID)
	if readErr != nil {
		return false, readErr
	}
	if i.CallStarted {
		if saveErr := a.svc.conversations.FinishInjection(context.WithoutCancel(ctx), e.DeliveryID, state); saveErr != nil {
			return accepted, saveErr
		}
	}
	return accepted, err
}
