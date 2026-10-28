package hybrid

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/rainchan/webmux/internal/acp"
	"github.com/rainchan/webmux/internal/agentprofile"
	"github.com/rainchan/webmux/internal/attentionkind"
	"github.com/rainchan/webmux/internal/runtime"
	"github.com/rainchan/webmux/internal/session"
)

const (
	// inboxSize absorbs bursts of agent updates while the actor works.
	inboxSize = 64
	// closeTimeout bounds stopping a side's processes on close.
	closeTimeout = 10 * time.Second
	// turnEndWait is how long a close waits for a cancelled turn to end
	// before it stops the agent anyway.
	turnEndWait = 5 * time.Second
	// releaseTimeout bounds session/close before an agent is stopped.
	releaseTimeout = 5 * time.Second
	// errorTextBytes bounds agent error messages shown in turn.end.
	errorTextBytes = 4 << 10
	// stderrTailBytes bounds the stderr tail returned with a failed start.
	stderrTailBytes = 8 << 10
)

// errMissingCapability rejects agents that cannot load or list sessions:
// hybrid mode depends on both.
var errMissingCapability = errors.New("agent 未声明 session/load 或 session/list")

// actor owns one hybrid session. Every mutable field below the marker is
// touched only by the run goroutine; commands, ACP updates, process exits
// and finished subtasks arrive as closures on inbox. Blocking work (process
// start and stop, ACP calls) runs in subtasks that post their result back,
// so the actor always answers promptly.
type actor struct {
	svc      *Service
	cmd      agentprofile.Command
	cwd      string
	configID string
	log      *chatLog
	inbox    chan func()
	quit     chan struct{}
	// id and shell are set before run starts and never change.
	id    string
	shell *session.Session
	// recent dedupes commands by client request ID; it has its own lock.
	recent dedupe

	// probes counts this session's running eligibility probes; a close
	// waits for them so the shell has no process left to remove.
	probes sync.WaitGroup
	// probeCtx ends the probes when the session closes or stops.
	probeCtx    context.Context
	probeCancel context.CancelFunc

	// --- owned by run ---
	side           Side
	state          State
	agentSessionID string
	// resumedSID is the agent session the current terminal run was started
	// to resume. It stands in for the terminal's own report until there is
	// one: OpenCode names its session only after a message.
	resumedSID string
	// agentCwd is the resolved cwd the agent session was opened or found
	// in; reconnects and wakes load it there.
	agentCwd string
	chat     ChatState
	tr       translator
	// epoch numbers ACP connections; updates and requests of an older one
	// are ignored.
	epoch       int
	link        *acpLink
	turn        *turn
	requests    []*pending
	nextTurn    int
	nextRequest int
	nextItem    int
	// sendAfterCancel makes the end of a turn cancelled by send-now send
	// the queue's head instead of pausing.
	sendAfterCancel bool
	// attention is the latched turn-end attention (done or error); pending
	// requests take precedence over it.
	attention *session.Attention
	preview   preview
	// switching is the running switch, from its eligibility check on.
	switching *switchOp
	// opening: a reconnect or wake is opening the agent session again;
	// prompts queue until it is ready. sleeping: the agent process is
	// stopping for idle sleep; a wake waits for it (wakeAfterSleep).
	opening        bool
	sleeping       bool
	wakeAfterSleep bool
	// reconnects counts automatic reconnects since the last turn that
	// ended normally (D24 allows one).
	reconnects int
	// lastActive is the last command, turn end or wake; the chat side
	// sleeps chatIdleSleepMinutes after it.
	lastActive time.Time
	sleepTimer *time.Timer
	sleepAt    time.Time
	// probe is the eligibility check of the terminal side's session.
	probe           probeState
	starting        bool
	closeAfterStart bool
	closing         bool
	stopped         bool
	dirty           bool
}

type turn struct {
	id     string
	cancel context.CancelFunc
	// done closes once the session/prompt call has returned and its result
	// was posted.
	done chan struct{}
}

// pending is an agent request waiting for a user's answer.
type pending struct {
	epoch  int
	kind   string
	build  func(id string) Request
	req    Request
	since  time.Time
	answer chan Answer
}

func newActor(s *Service, cmd agentprofile.Command, cwd, configID string, side Side) *actor {
	_, unsupported := cmd.ACPArgv()
	if unsupported == nil {
		unsupported = []string{}
	}
	a := &actor{
		svc: s, cmd: cmd, cwd: cwd, configID: configID,
		inbox: make(chan func(), inboxSize), quit: make(chan struct{}),
		side: side, state: StateActive, agentCwd: cwd, lastActive: time.Now(),
	}
	a.probeCtx, a.probeCancel = context.WithCancel(context.Background())
	a.chat = ChatState{
		Side: side, Agent: cmd.Profile.Name, Requests: []Request{}, Config: []ConfigOption{},
		Commands: []Command{}, Queue: Queue{Items: []QueueItem{}}, Unsupported: unsupported,
	}
	a.log = newChatLog(s.capacity, a.chat)
	return a
}

func (a *actor) run() {
	defer close(a.quit)
	defer a.stopTimers()
	for !a.stopped {
		(<-a.inbox)()
		a.flush()
		if !a.stopped {
			a.armSleep()
		}
	}
}

// stopTimers stops the sleep and probe timers and the running probes.
func (a *actor) stopTimers() {
	if a.sleepTimer != nil {
		a.sleepTimer.Stop()
		a.sleepTimer = nil
	}
	if a.probe.timer != nil {
		a.probe.timer.Stop()
		a.probe.timer = nil
	}
	a.probeCancel()
}

// post queues fn for the actor; false once the actor has stopped. A queued
// fn is dropped when the actor stops before reaching it, so a caller that
// must know whether fn ran uses sync. The actor never posts to itself.
func (a *actor) post(fn func()) bool {
	select {
	case <-a.quit:
		return false
	default:
	}
	select {
	case a.inbox <- fn:
		return true
	case <-a.quit:
		return false
	}
}

// awaitStarted waits for the start or resume posted to a to report on
// started. Its subtask reports once the actor applied the outcome, or
// itself after stopping what it opened when the actor ended first; an
// actor that ended before taking the start never reports.
func awaitStarted(a *actor, started <-chan error) error {
	select {
	case err := <-started:
		return err
	case <-a.quit:
		select {
		case err := <-started:
			return err
		default:
			return errStopped
		}
	}
}

// call runs fn on the actor and returns its result.
func call[T any](ctx context.Context, a *actor, fn func() (T, error)) (T, error) {
	type result struct {
		v   T
		err error
	}
	var zero T
	reply := make(chan result, 1)
	if !a.post(func() { v, err := fn(); reply <- result{v, err} }) {
		return zero, ErrNotFound
	}
	select {
	case r := <-reply:
		return r.v, r.err
	case <-ctx.Done():
		return zero, ctx.Err()
	case <-a.quit:
		select {
		case r := <-reply:
			return r.v, r.err
		default:
			return zero, ErrNotFound
		}
	}
}

// watchShell ends the actor when its shell goes away without it: removed
// directly, or retired by Manager.Shutdown.
func (a *actor) watchShell() {
	select {
	case <-a.shell.Done():
		a.post(a.shellGone)
	case <-a.quit:
	}
}

// flush publishes the state and overlay after a change. Leaving the chat
// side drops its latched attention: the terminal's signals decide there.
func (a *actor) flush() {
	if !a.dirty || a.stopped {
		return
	}
	a.dirty = false
	if a.side != SideACP {
		a.attention = nil
	}
	if a.side != SideTUI || a.state != StateActive {
		a.chat.Eligibility = Eligibility{}
	}
	a.chat.Side = a.side
	if a.agentSessionID != "" {
		id := a.agentSessionID
		a.chat.AgentSessionID = &id
	}
	a.chat.Recovery = nil
	if a.state != StateActive {
		a.chat.Recovery = &RecoveryTip{State: a.state}
	}
	a.log.setState(a.chat)
	a.shell.SetOverlay(a.overlay())
}

// overlay is the shell's hybrid description; on the chat side it also
// supplies the session status, attention and preview: attention makes the
// session waiting, a running turn running, anything else idle.
func (a *actor) overlay() *session.Overlay {
	info := &session.HybridInfo{
		Side: string(a.side), Phase: string(a.chat.Phase), Dormant: a.chat.Dormant,
		State: string(a.state), Agent: a.cmd.Profile.Name,
	}
	if a.agentSessionID != "" {
		id := a.agentSessionID
		info.AgentSessionID = &id
	}
	o := &session.Overlay{Hybrid: info}
	if a.side == SideACP && a.state == StateActive {
		o.Attention = a.currentAttention()
		o.Preview = a.preview.text
		switch {
		case o.Attention != nil:
			o.Status = session.StatusWaiting
		case a.turn != nil:
			o.Status = session.StatusRunning
		default:
			o.Status = session.StatusIdle
		}
	}
	return o
}

// currentAttention is the chat side's attention: the oldest pending
// request (permission, or question for a form), else the latched turn-end
// attention.
func (a *actor) currentAttention() *session.Attention {
	if len(a.requests) > 0 {
		p := a.requests[0]
		kind := attentionkind.Permission
		if p.kind == RequestForm {
			kind = attentionkind.Question
		}
		return &session.Attention{Kind: kind, Source: session.SourceAgent, Since: p.since}
	}
	return a.attention
}

// clearAttention drops the latched turn-end attention: the user sent a
// prompt or answered a request.
func (a *actor) clearAttention() {
	if a.attention != nil {
		a.attention = nil
		a.dirty = true
	}
}

// emit stores events in the log, keeping the replaceable state they carry.
func (a *actor) emit(events ...Event) {
	for _, e := range events {
		switch e.Kind {
		case EventPlan:
			a.chat.Plan = e.Plan
		case EventCommands:
			a.chat.Commands = e.Commands
		case EventConfig:
			a.chat.Config = e.Config
			values := configValues(e.Config)
			a.record(false, func(r *Record) { r.Config = values })
		case EventUsage:
			a.chat.Usage = e.Usage
		case EventMessage:
			if e.Message.Role != RoleAgent || !a.preview.add(e.Message) {
				continue
			}
		default:
			continue
		}
		a.dirty = true
	}
	a.log.append(time.Now().UTC(), events...)
}

// record changes the session's record; immediate writes it now.
func (a *actor) record(immediate bool, change func(*Record)) {
	if _, err := a.svc.records.update(a.id, immediate, change); err != nil {
		a.svc.log.Warn("hybrid record update failed", "id", a.id, "error", err)
	}
}

func processIdentity(pid int) *runtime.Identity {
	if pid <= 0 {
		return nil
	}
	id, err := runtime.ProcessIdentity(pid)
	if err != nil {
		return nil
	}
	return &id
}

// startResult is a finished side start.
type startResult struct {
	link    *acpLink
	pid     int
	process *runtime.Identity
	init    acp.InitializeResponse
	session acp.NewSessionResponse
	// err is user-facing and may carry the agent's stderr: never log it.
	err error
}

// start launches the session's first side in a subtask and reports the
// outcome on started once the actor has applied it. When the actor ended
// first, the subtask stops the ACP process it started and reports
// errStopped.
func (a *actor) start(started chan<- error) {
	a.starting = true
	report := func(res startResult) {
		if !a.sync(func() { a.started(started, res) }) {
			if res.link != nil {
				_ = res.link.stop()
			}
			started <- errStopped
		}
	}
	if a.side == SideTUI {
		shell, argv := a.shell, a.cmd.TUIArgv()
		go func() {
			if err := shell.StartRun(argv); err != nil {
				report(startResult{err: fmt.Errorf("%w: 启动终端失败: %v", ErrInvalid, err)})
				return
			}
			pid := shell.Identity().PID
			report(startResult{pid: pid, process: processIdentity(pid)})
		}()
		return
	}
	a.epoch++
	epoch := a.epoch
	go func() { report(a.spawnACP(epoch)) }()
}

// spawnACP starts the ACP process and opens a new agent session. It runs
// in a subtask; a failure stops the process before returning.
func (a *actor) spawnACP(epoch int) startResult {
	link, init, fail := a.connect(epoch)
	if fail != nil {
		return startResult{err: fail.err}
	}
	res := startResult{link: link, init: init}
	var err error
	res.session, err = link.conn.NewSession(context.Background(), acp.NewSessionRequest{Cwd: a.cwd})
	if err == nil && res.session.SessionID == "" {
		err = errors.New("agent 未返回会话 ID")
	}
	if err != nil {
		return startResult{err: a.callFailed(link, acp.MethodSessionNew, err).err}
	}
	link.sessionID = res.session.SessionID
	res.pid = link.sidecar.PID()
	res.process = processIdentity(res.pid)
	return res
}

func stderrTail(b []byte) string {
	if len(b) > stderrTailBytes {
		b = b[len(b)-stderrTailBytes:]
	}
	return strings.TrimSpace(strings.ToValidUTF8(string(b), ""))
}

func limitText(s string, n int) string {
	if len(s) <= n {
		return s
	}
	for n > 0 && s[n]&0xC0 == 0x80 {
		n--
	}
	return s[:n] + "…"
}

// started applies a finished start: the session runs, or it is discarded
// and the error returned to the creator.
func (a *actor) started(started chan<- error, res startResult) {
	a.starting = false
	if res.err != nil {
		a.discard()
		started <- res.err
		return
	}
	a.dirty = true
	a.touch()
	pid, process := res.pid, res.process
	if res.link == nil {
		// A terminal that already exited left the session interrupted.
		if a.state == StateActive {
			a.record(true, func(r *Record) { r.PID, r.Process = pid, process })
		}
	} else {
		a.installLink(res.link, res.init, res.session.ConfigOptions, nil)
		sid := a.agentSessionID
		a.record(true, func(r *Record) {
			r.PID, r.Process = pid, process
			r.SetAgentSessionID(sid)
		})
	}
	a.svc.log.Info("hybrid session started", "id", a.id, "side", string(a.side), "agent", a.cmd.Profile.Name)
	started <- nil
	a.checkLink()
	a.afterTransition()
}

// discard removes a session whose first start failed: its record and shell
// go, as if it was never created.
func (a *actor) discard() {
	if err := a.svc.records.remove(a.id); err != nil {
		a.svc.log.Warn("hybrid record removal failed", "id", a.id, "error", err)
	}
	a.end()
	if err := a.svc.manager.Remove(a.id); err != nil {
		a.svc.log.Warn("hybrid shell removal failed", "id", a.id, "error", err)
	}
}

// end stops the actor and its chat log.
func (a *actor) end() {
	a.stopped = true
	a.log.end()
	a.svc.unregister(a)
}

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
	ans, err := h.ask(ctx, RequestPermission, func(id string) Request { return permissionRequest(id, req) })
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
	ans, err := h.ask(ctx, RequestForm, func(id string) Request { return formRequest(id, req) })
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
func (h *handler) ask(ctx context.Context, kind string, build func(id string) Request) (Answer, error) {
	p := &pending{epoch: h.epoch, kind: kind, build: build, answer: make(chan Answer, 1)}
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
	a.emit(a.tr.update(n.Update)...)
}

func (a *actor) openRequest(p *pending) {
	if p.epoch != a.epoch || a.closing || a.stopped {
		p.answer <- cancelledAnswer(p.kind)
		return
	}
	a.nextRequest++
	p.req, p.since = p.build("r"+strconv.Itoa(a.nextRequest)), time.Now().UTC()
	a.requests = append(a.requests, p)
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

// send starts a turn with text; the caller checked that the session can
// take one (ready, linked, no turn running). Sending clears the latched
// attention; authRequired stays until a turn ends normally.
func (a *actor) send(text string) PromptResult {
	a.nextTurn++
	n := strconv.Itoa(a.nextTurn)
	result := PromptResult{TurnID: "t" + n, PromptID: "p" + n}
	ctx, cancel := context.WithCancel(context.Background())
	t := &turn{id: result.TurnID, cancel: cancel, done: make(chan struct{})}
	a.turn = t
	a.chat.Turn = Turn{Running: true, TurnID: t.id}
	a.dirty = true
	a.clearAttention()
	a.emit(Event{Kind: EventTurnStart, TurnStart: &TurnStart{TurnID: result.TurnID, PromptID: result.PromptID}}, a.tr.userMessage(text))
	a.tr.breakMessage()
	conn, sid := a.link.conn, a.link.sessionID
	go func() {
		defer close(t.done)
		resp, err := conn.Prompt(ctx, acp.PromptRequest{SessionID: sid, Prompt: []acp.ContentBlock{acp.TextBlock(text)}})
		a.post(func() { a.turnEnded(t, resp, err) })
	}()
	return result
}

// turnEnded applies a finished session/prompt. A normal end latches done
// (end_turn) and sends the next queued prompt; cancelled, refusal, error
// and auth_required pause a non-empty queue, refusal, error and
// auth_required latching error attention. A turn cancelled by send-now
// sends the queue's head instead. auth_required sets authRequired until a
// turn ends normally.
func (a *actor) turnEnded(t *turn, resp acp.PromptResponse, err error) {
	if a.turn != t {
		return
	}
	a.turn = nil
	t.cancel()
	stop, text := stopReason(resp, err)
	var rpc *acp.RPCError
	code := 0
	if errors.As(err, &rpc) {
		code = rpc.Code
	}
	a.svc.log.Info("hybrid turn ended", "id", a.id, "stopReason", stop, "code", code)
	a.tr.breakMessage()
	a.chat.Turn = Turn{TurnID: t.id, StopReason: stop}
	normal := stop == acp.StopEndTurn || stop == acp.StopMaxTokens || stop == acp.StopMaxTurnRequests
	switch {
	case stop == StopAuthRequired:
		a.chat.AuthRequired = true
	case normal:
		a.chat.AuthRequired = false
		a.reconnects = 0
	}
	a.dirty = true
	a.touch()
	a.emit(Event{Kind: EventTurnEnd, TurnEnd: &TurnEnd{TurnID: t.id, StopReason: stop, Error: text}})
	now := time.Now().UTC()
	switch stop {
	case acp.StopEndTurn:
		a.attention = &session.Attention{Kind: attentionkind.Done, Source: session.SourceAgent, Since: now}
	case acp.StopRefusal, StopError, StopAuthRequired:
		a.attention = &session.Attention{Kind: attentionkind.Error, Source: session.SourceAgent, Since: now}
	}
	switch {
	case a.sendAfterCancel:
		a.sendAfterCancel = false
		a.setQueue(a.chat.Queue.Items, false)
	case !normal:
		a.setQueue(a.chat.Queue.Items, true)
	}
	a.drain()
}

// stopReason maps a session/prompt outcome to a turn.end reason and error
// text. An authentication error (-32000, or its message on agents that
// use another code) is auth_required; a lost connection ends the turn as
// cancelled (interrupted).
func stopReason(resp acp.PromptResponse, err error) (string, string) {
	var rpc *acp.RPCError
	switch {
	case err == nil:
		switch resp.StopReason {
		case acp.StopEndTurn, acp.StopMaxTokens, acp.StopMaxTurnRequests, acp.StopRefusal, acp.StopCancelled:
			return resp.StopReason, ""
		}
		return StopError, "agent 返回了未知的结束原因"
	case errors.As(err, &rpc):
		msg := limitText(rpc.Message, errorTextBytes)
		if rpc.Code == acp.CodeAuthRequired || strings.Contains(strings.ToLower(rpc.Message), "authentication required") {
			return StopAuthRequired, msg
		}
		return StopError, msg
	case errors.Is(err, acp.ErrClosed), errors.Is(err, acp.ErrDisconnected), errors.Is(err, acp.ErrProtocol):
		return acp.StopCancelled, "与 agent 的连接已断开"
	}
	return StopError, "回合失败"
}

// cancelTurn stops the running turn: requests are answered cancelled and
// cancelling the prompt's context sends session/cancel.
func (a *actor) cancelTurn() {
	if a.turn == nil {
		return
	}
	a.withdrawAll()
	a.turn.cancel()
}

// steeringSupported reads the initialize result's _meta.steering.supported.
func steeringSupported(meta []byte) bool {
	if len(meta) == 0 {
		return false
	}
	var m struct {
		Steering *struct {
			Supported bool `json:"supported"`
		} `json:"steering"`
	}
	if err := json.Unmarshal(meta, &m); err != nil {
		return false
	}
	return m.Steering != nil && m.Steering.Supported
}

// interrupt marks the session interrupted: its side ended by itself, or a
// reconnect or rollback failed. activity is what it was doing: "idle",
// "turn" or "switching". gone says the side's process is confirmed gone;
// otherwise the record keeps its identity, so a restart ends it before
// anything resumes the agent session. The shell stays, showing its last
// output.
func (a *actor) interrupt(activity string, gone bool) {
	if a.turn != nil && activity == "idle" {
		activity = "turn"
	}
	a.state = StateInterrupted
	a.dirty = true
	now := time.Now().UTC()
	lost := len(a.chat.Queue.Items)
	a.record(true, func(r *Record) {
		r.State, r.Activity, r.InterruptedAt, r.QueuedCount = StateInterrupted, activity, &now, lost
		if gone {
			r.PID, r.Process = 0, nil
		}
	})
}

// processGone forgets recorded process pid once it is confirmed gone; a
// record that moved on to another process keeps that one.
func (a *actor) processGone(pid int) {
	if pid <= 0 {
		return
	}
	a.record(false, func(r *Record) {
		if r.PID == pid {
			r.PID, r.Process = 0, nil
		}
	})
}

// stopLost stops a connection the session no longer uses in the
// background and forgets its process in the record once it is gone.
func (a *actor) stopLost(link *acpLink) {
	pid := link.sidecar.PID()
	go func() {
		if link.stop() == nil {
			a.post(func() { a.processGone(pid) })
		}
	}()
}

// Close implements session.Driver: a user closed the session. It is
// refused with ErrSwitching while a switch runs.
func (a *actor) Close() error {
	_, err := call(context.Background(), a, func() (struct{}, error) {
		if a.switching != nil {
			return struct{}{}, ErrSwitching
		}
		a.beginClose()
		return struct{}{}, nil
	})
	return err
}

// RunExited implements session.Driver: the terminal UI exited by itself
// (D23). During a switch the switch notices it instead.
func (a *actor) RunExited(code int) {
	a.post(func() {
		if a.closing || a.stopped || a.side != SideTUI || a.state != StateActive || a.switching != nil {
			return
		}
		a.svc.log.Info("hybrid terminal exited", "id", a.id, "exitCode", code)
		a.interrupt("idle", true)
	})
}

// beginClose ends the session for a user: the running turn is cancelled
// (chat) or interrupted like a switch would (terminal, bounded by the
// interrupt wait), the side's processes stop, then finishClose records
// closed and removes the shell. A start, wake, reconnect or sleep in
// progress finishes first.
func (a *actor) beginClose() {
	if a.closing || a.stopped {
		return
	}
	if a.starting || a.opening || a.sleeping {
		a.closeAfterStart = true
		return
	}
	a.closing = true
	a.stopTimers()
	a.withdrawAll()
	var turnDone chan struct{}
	if a.turn != nil {
		turnDone = a.turn.done
		a.turn.cancel()
	}
	link, shell := a.link, a.shell
	a.link = nil
	terminal := a.side == SideTUI && a.state == StateActive
	go func() {
		a.probes.Wait()
		if turnDone != nil {
			wait := time.NewTimer(turnEndWait)
			select {
			case <-turnDone:
			case <-wait.C:
			}
			wait.Stop()
		}
		gone := true
		if link != nil {
			if err := link.release(); err != nil {
				gone = false
				a.svc.log.Warn("hybrid agent did not stop in time", "id", a.id, "error", err)
			}
		}
		if terminal && tuiBusy(shell.Info()) {
			a.interruptKey()
			a.waitIdle(a.svc.timing.interrupt)
		}
		ctx, cancel := context.WithTimeout(context.Background(), closeTimeout)
		defer cancel()
		if err := shell.EndRun(ctx); err != nil && !errors.Is(err, session.ErrNoRun) {
			gone = false
			a.svc.log.Warn("hybrid terminal did not stop in time", "id", a.id, "error", err)
		}
		a.post(func() { a.finishClose(gone) })
	}()
}

// finishClose records the session closed and removes its shell. gone says
// the side's processes are confirmed gone; otherwise the record keeps the
// process identity for a restart to end.
func (a *actor) finishClose(gone bool) {
	if a.state == StateActive {
		a.state = StateClosed
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
// it; a switch, wake or reconnect in flight stops the process it started
// once it finds the actor gone.
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
