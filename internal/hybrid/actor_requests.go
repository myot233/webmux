package hybrid

import (
	"context"
	"fmt"
	"github.com/rainchan/webmux/internal/acp"
	"github.com/rainchan/webmux/internal/session"
	"slices"
	"strconv"
	"strings"
	"time"
)

// handler receives one ACP connection's updates and requests and forwards
// them to the actor.
type handler struct {
	a     *actor
	epoch int
}

func (h *handler) SessionUpdate(n acp.SessionNotification) {
	h.a.post(func() { h.a.onUpdate(h.epoch, n) })
}

func (h *handler) RequestPermission(ctx context.Context, req acp.RequestPermissionRequest) (acp.RequestPermissionResponse, error) {
	tool := toolPatch(req.ToolCall)
	ans, err := h.ask(ctx, RequestPermission, &tool, func(id string) Request { return permissionRequest(id, req) })
	if err != nil {
		return acp.RequestPermissionResponse{}, err
	}
	if ans.Outcome == OutcomeSelected {
		return acp.RequestPermissionResponse{Outcome: acp.PermissionOutcome{Outcome: acp.OutcomeSelected, OptionID: ans.OptionID}}, nil
	}
	return acp.RequestPermissionResponse{Outcome: acp.PermissionOutcome{Outcome: acp.OutcomeCancelled}}, nil
}

func (h *handler) CreateElicitation(ctx context.Context, req acp.CreateElicitationRequest) (acp.CreateElicitationResponse, error) {
	if req.Mode != acp.ElicitationForm {
		// Only form mode is advertised.
		return acp.CreateElicitationResponse{Action: acp.ElicitDecline}, nil
	}
	ans, err := h.ask(ctx, RequestForm, nil, func(id string) Request { return formRequest(id, req) })
	if err != nil {
		return acp.CreateElicitationResponse{}, err
	}
	switch ans.Outcome {
	case OutcomeAccept:
		return acp.CreateElicitationResponse{Action: acp.ElicitAccept, Content: ans.Content}, nil
	case OutcomeDecline:
		return acp.CreateElicitationResponse{Action: acp.ElicitDecline}, nil
	}
	return acp.CreateElicitationResponse{Action: acp.ElicitCancel}, nil
}

// ask opens a request on the actor and waits for its answer. The agent
// withdrawing it (ctx) closes it as cancelled; a stopped actor answers
// cancelled.
func (h *handler) ask(ctx context.Context, kind string, tool *Tool, build func(id string) Request) (Answer, error) {
	p := &pending{epoch: h.epoch, kind: kind, build: build, tool: tool, answer: make(chan Answer, 1)}
	if !h.a.post(func() { h.a.openRequest(p) }) {
		return cancelledAnswer(kind), nil
	}
	select {
	case ans := <-p.answer:
		return ans, nil
	case <-ctx.Done():
		h.a.post(func() { h.a.withdraw(p) })
		return Answer{}, ctx.Err()
	case <-h.a.quit:
		return cancelledAnswer(kind), nil
	}
}

func cancelledAnswer(kind string) Answer {
	if kind == RequestForm {
		return Answer{Outcome: OutcomeCancel}
	}
	return Answer{Outcome: OutcomeCancelled}
}

func (a *actor) onUpdate(epoch int, n acp.SessionNotification) {
	if epoch != a.epoch || a.stopped {
		return
	}
	if a.turn != nil {
		if _, ok := n.Update.(*acp.UserMessageChunk); ok {
			return
		}
		a.work.noteBackground(n.Update)
	}
	a.recordUpdate(n.Update)
	events := a.tr.update(n.Update)
	if hidden := a.cmd.Profile.HiddenCommands; len(hidden) > 0 {
		for i := range events {
			if events[i].Kind == EventCommands {
				events[i].Commands = slices.DeleteFunc(slices.Clone(events[i].Commands), func(c Command) bool {
					return slices.Contains(hidden, c.Name)
				})
			}
		}
	}
	a.emit(events...)
	for _, e := range events {
		if e.Kind == EventInfo && e.Info != nil && e.Info.Title != nil {
			a.titleSession = n.SessionID
		}
	}
}

func (a *actor) openRequest(p *pending) {
	if p.epoch != a.epoch || a.closing || a.stopped {
		p.answer <- cancelledAnswer(p.kind)
		return
	}
	if p.tool != nil && a.turn != nil {
		a.work.tool(*p.tool)
		a.refreshWork()
	}
	a.nextRequest++
	p.req, p.since = p.build("r"+strconv.Itoa(a.nextRequest)), time.Now().UTC()
	if a.autoApprove(p) != "" {
		return
	}
	a.requests = append(a.requests, p)
	a.stopDoneDelayTimer()
	a.syncRequests()
	a.tr.breakMessage()
	req := p.req
	a.emit(Event{Kind: EventRequest, Request: &req})
}

// withdraw closes a request the agent stopped waiting for.
func (a *actor) withdraw(p *pending) {
	if a.removeRequest(p) {
		a.emit(Event{Kind: EventResolved, Resolved: &RequestOutcome{RequestID: p.req.RequestID, Outcome: cancelledAnswer(p.kind).Outcome}})
	}
}

// withdrawAll answers every pending request cancelled, as ACP requires
// when a turn is cancelled.
func (a *actor) withdrawAll() {
	for len(a.requests) > 0 {
		p := a.requests[0]
		ans := cancelledAnswer(p.kind)
		p.answer <- ans
		a.withdraw(p)
	}
}

func (a *actor) removeRequest(p *pending) bool {
	for i, q := range a.requests {
		if q == p {
			a.requests = append(a.requests[:i:i], a.requests[i+1:]...)
			a.syncRequests()
			return true
		}
	}
	return false
}

func (a *actor) syncRequests() {
	reqs := make([]Request, len(a.requests))
	for i, p := range a.requests {
		reqs[i] = p.req
	}
	a.chat.Requests = reqs
	a.dirty = true
}

// answer resolves a pending request with a user's answer.
func (a *actor) answer(requestID string, ans Answer) error {
	var p *pending
	for _, q := range a.requests {
		if q.req.RequestID == requestID {
			p = q
		}
	}
	if p == nil {
		if n, err := strconv.Atoi(strings.TrimPrefix(requestID, "r")); err == nil && strings.HasPrefix(requestID, "r") && n > 0 && n <= a.nextRequest {
			return ErrAnswered
		}
		return ErrNotFound
	}
	switch {
	case p.kind == RequestPermission && ans.Outcome == OutcomeSelected:
		known := false
		for _, o := range p.req.Options {
			known = known || o.OptionID == ans.OptionID
		}
		if !known {
			return fmt.Errorf("%w: 未知的选项", ErrInvalid)
		}
	case p.kind == RequestPermission && ans.Outcome == OutcomeCancelled:
		ans.OptionID = ""
	case p.kind == RequestForm && ans.Outcome == OutcomeAccept:
		if !p.req.Form.Supported {
			return fmt.Errorf("%w: 该表单需要在终端中完成", ErrInvalid)
		}
		ans.OptionID = ""
	case p.kind == RequestForm && (ans.Outcome == OutcomeDecline || ans.Outcome == OutcomeCancel):
		ans.OptionID, ans.Content = "", nil
	default:
		return fmt.Errorf("%w: 答复与请求种类不符", ErrInvalid)
	}
	a.removeRequest(p)
	p.answer <- ans
	a.clearAttention()
	a.emit(Event{Kind: EventResolved, Resolved: &RequestOutcome{RequestID: requestID, Outcome: ans.Outcome, OptionID: ans.OptionID}})
	return nil
}

// setPermissionMode changes how permission requests are answered; every
// device sees it in the chat state and the record keeps it. Switching to
// PermissionAuto approves the requests already waiting. It applies on
// either side but not during a switch.
func (a *actor) setPermissionMode(mode PermissionMode) error {
	switch {
	case a.state != StateActive || a.closing:
		return fmt.Errorf("%w: 会话已结束", ErrConflict)
	case a.switching != nil:
		return ErrSwitching
	}
	if a.chat.PermissionMode == mode {
		return nil
	}
	a.chat.PermissionMode = mode
	a.dirty = true
	recorded := mode
	if mode == PermissionAuto {
		recorded = ""
	}
	a.record(false, func(r *Record) { r.PermissionMode = recorded })
	for _, p := range slices.Clone(a.requests) {
		if option := a.autoApprove(p); option != "" {
			a.removeRequest(p)
			a.emit(Event{Kind: EventResolved, Resolved: &RequestOutcome{RequestID: p.req.RequestID, Outcome: OutcomeSelected, OptionID: option}})
		}
	}
	return nil
}

// autoApprove answers permission request p in PermissionAuto with its
// first allow_once option, else its first allow_always one, without a
// timeline entry, and returns the option ID. It returns "", leaving the
// request to the user, for forms, in PermissionAsk and when no option
// allows.
func (a *actor) autoApprove(p *pending) string {
	if p.kind != RequestPermission || a.chat.PermissionMode != PermissionAuto {
		return ""
	}
	option := ""
	for _, kind := range []string{acp.PermissionAllowOnce, acp.PermissionAllowAlways} {
		if i := slices.IndexFunc(p.req.Options, func(o PermissionOption) bool { return o.Kind == kind }); i >= 0 {
			option = p.req.Options[i].OptionID
			break
		}
	}
	if option == "" {
		return ""
	}
	p.answer <- Answer{Outcome: OutcomeSelected, OptionID: option}
	return option
}

// saveShellInfo records the shell's repository association and displayed
// title. The title is skipped while it is still the cwd-derived placeholder
// a fresh or switched terminal shows before it reports an OSC title: that
// would overwrite the recorded title on every terminal resume or switch.
func (a *actor) saveShellInfo(info session.Info) {
	a.saveWorkspace(info.Workspace)
	if info.PlaceholderTitle {
		return
	}
	a.saveTitle(info.Title)
}

// saveWorkspace keeps the record's repository association in sync with the
// running session, so an archived record stays with its worktree.
func (a *actor) saveWorkspace(w *session.Workspace) {
	var ref *WorkspaceRef
	if w != nil {
		ref = &WorkspaceRef{RepositoryID: w.RepositoryID, WorktreeID: w.WorktreeID}
	}
	if ref == nil && a.recordWorkspace == nil || ref != nil && a.recordWorkspace != nil && *ref == *a.recordWorkspace {
		return
	}
	a.recordWorkspace = ref
	a.record(false, func(r *Record) {
		if ref == nil {
			r.Workspace = nil
			return
		}
		value := *ref
		r.Workspace = &value
	})
}

// saveTitle keeps the recoverable card's title in sync with the running
// session. Routine writes are coalesced; closing and interruption flush them.
func (a *actor) saveTitle(title string) {
	if title == a.recordTitle {
		return
	}
	a.recordTitle = title
	a.record(false, func(r *Record) { r.Title = title })
}

// refreshWork recomputes the one-line work detail from the tracked tool
// calls and plan. The detail is only kept while a turn is in flight, so a
// finished or failed turn clears it.
func (a *actor) refreshWork() {
	line := ""
	if a.turn != nil {
		line = a.work.line()
	}
	if line == a.workLine {
		return
	}
	a.workLine = line
	a.dirty = true
}

// record changes the session's record. Immediate writes finish before the
// operation returns, preserving the recovery contract of completed switches.
func (a *actor) record(immediate bool, change func(*Record)) {
	if rec, err := a.svc.records.update(a.id, immediate, change); err != nil {
		a.svc.log.Warn("hybrid record update failed", "id", a.id, "error", err)
	} else {
		a.svc.syncConversation(rec)
	}
}

// finishClose records the session closed and removes its shell. gone says
// the side's processes are confirmed gone; otherwise the record keeps the
// process identity for a restart to end.
func (a *actor) finishClose(gone bool) {
	if a.state == StateActive {
		a.state = StateClosed
		a.saveShellInfo(a.shell.Info())
		a.record(true, func(r *Record) {
			r.State = StateClosed
			if gone {
				r.PID, r.Process = 0, nil
			}
		})
	}
	a.end()
	if err := a.svc.manager.Remove(a.id); err != nil {
		a.svc.log.Warn("hybrid shell removal failed", "id", a.id, "error", err)
	}
	a.svc.log.Info("hybrid session closed", "id", a.id)
}

// shellGone ends the actor after its shell was removed or retired by
// someone else; the record is left for recovery.
func (a *actor) shellGone() {
	if a.stopped {
		return
	}
	a.withdrawAll()
	if a.turn != nil {
		a.turn.cancel()
	}
	if link := a.link; link != nil {
		a.link = nil
		go func() { _ = link.stop() }()
	}
	a.end()
}

// shutdown stops the ACP process for a server shutdown, killing it when
// ctx ends first, and leaves the record active so the next start marks it
// interrupted; the record keeps what the session was doing and how many
// queued prompts the shutdown loses. It blocks the actor, which ends with
// it; a switch or reconnect in flight stops the process it started once it
// finds the actor gone.
func (a *actor) shutdown(ctx context.Context) {
	if a.stopped {
		return
	}
	if a.state == StateActive {
		activity := "idle"
		switch {
		case a.switching != nil:
			activity = "switching"
		case a.turn != nil, a.side == SideTUI && tuiBusy(a.shell.Info()):
			activity = "turn"
		}
		lost := len(a.chat.Queue.Items)
		a.saveShellInfo(a.shell.Info())
		a.record(false, func(r *Record) { r.Activity, r.QueuedCount = activity, lost })
	}
	a.stopTimers()
	a.withdrawAll()
	if a.turn != nil {
		a.turn.cancel()
	}
	if link := a.link; link != nil {
		a.link = nil
		_ = link.stopWithin(ctx)
	}
	a.end()
}
