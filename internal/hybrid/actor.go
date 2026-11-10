package hybrid

import (
	"context"

	"errors"
	"fmt"
	"slices"
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
// and finished subtasks arrive as closures on inbox. Process and ACP work
// runs in subtasks that post their result back. Immediate record commits stay
// on the actor to preserve completed operations' recovery contract, but never
// hold the record state lock during filesystem I/O.
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
	// in; reconnects load it there.
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
	// title is the last title the agent reported through session_info_update;
	// titleSession is the agent session it belongs to. It names the shell
	// while the chat side shows no terminal run.
	title        string
	titleSession string
	// recordTitle is the last title sent to the persistent record. Terminal
	// titles arrive through the shell, chat titles through its overlay.
	recordTitle string
	// recordWorkspace is the last repository association sent to the
	// persistent record.
	recordWorkspace *WorkspaceRef
	// promptTitle: the agent session was created by this session and its
	// first prompt has not been sent, so that prompt may name the chat until
	// the agent reports a title.
	promptTitle bool
	preview     preview
	// work tracks tool calls and the plan to derive the one-line work
	// detail shown on the session card; workLine is its latest text, empty
	// outside a turn.
	work     workTracker
	workLine string
	// bgBase is the agent's process tree when the turn began; bgCancel ends
	// the watch for background work left after a turn.
	bgBase   map[int]bool
	bgCancel context.CancelFunc
	// switching is the running switch, from its eligibility check on.
	switching *switchOp
	// opening: a reconnect is opening the agent session again; prompts
	// queue until it is ready.
	opening bool
	// reconnects counts automatic reconnects since the last turn that
	// ended normally (D24 allows one).
	reconnects int
	// probe is the eligibility check of the terminal side's session.
	probe probeState
	// connector is the eligibility check of the profile's connector.
	connector connectorState
	// locks follows the writer locks of a profile that names sessions by
	// them (Codex).
	locks lockTracker
	// registry follows process registration records and their best-effort
	// external occupancy checks, independently of writer-lock tracking.
	registry     processRegistryTracker
	externalSID  string
	externalHeld bool
	externalRead time.Time
	// runStart is when the actor last started a terminal run.
	runStart time.Time
	// agentPathNoted: the notice that the connector uses its own agent was
	// shown.
	agentPathNoted  bool
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
	tool   *Tool
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
		side: side, state: StateActive, agentCwd: cwd,
	}
	a.probeCtx, a.probeCancel = context.WithCancel(context.Background())
	a.chat = ChatState{
		Side: side, Agent: cmd.Profile.Name, Requests: []Request{}, Config: []ConfigOption{},
		Commands: []Command{}, Queue: Queue{Items: []QueueItem{}}, Unsupported: unsupported,
		PermissionMode: PermissionAuto, Outdated: []Outdated{},
		ForceSwitchNote: agentprofile.ForceSwitchNote(cmd.Agent),
	}
	a.log = newChatLog(s.capacity, a.chat)
	a.tr.modelChoiceValues = cmd.Profile.ModelChoiceValues
	return a
}

func (a *actor) run() {
	defer close(a.quit)
	defer a.stopTimers()
	defer a.stopBackgroundWatch()
	for !a.stopped {
		(<-a.inbox)()
		a.flush()
	}
}

// stopTimers stops the probe timers and the running probes.
func (a *actor) stopTimers() {
	if a.probe.timer != nil {
		a.probe.timer.Stop()
		a.probe.timer = nil
	}
	if a.connector.timer != nil {
		a.connector.timer.Stop()
		a.connector.timer = nil
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
	a.saveShellInfo(a.shell.Info())
}

// overlay is the shell's hybrid description; on the chat side it also
// supplies the session status, attention and preview: attention makes the
// session waiting, a running turn running, anything else idle.
func (a *actor) overlay() *session.Overlay {
	side, to := a.side, Side("")
	if a.side == SideSwitching && a.switching != nil {
		side, to = a.switching.from, a.switching.to
	}
	info := &session.HybridInfo{
		Side: string(side), To: string(to), Phase: string(a.chat.Phase),
		State: string(a.state), Agent: a.cmd.Profile.Name,
		AgentTitle: a.cmd.Profile.Title, AgentIcon: a.cmd.Profile.Icon,
	}
	if a.agentSessionID != "" {
		id := a.agentSessionID
		info.AgentSessionID = &id
	}
	o := &session.Overlay{Hybrid: info}
	if a.side == SideACP && a.titleSession == a.agentSessionID {
		// The terminal drives the title on its own side (OSC 0/2), so only
		// the chat side publishes the agent's session title, and only while
		// it belongs to the agent session shown.
		o.Title = a.title
	}
	if a.side == SideACP && a.state == StateActive {
		o.Attention = a.currentAttention()
		o.Preview = a.preview.text
		// The work detail is only meaningful while a turn is in flight.
		if a.turn != nil {
			info.Work = a.workLine
		}
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
		toolsOnly := false
		switch e.Kind {
		case EventPlan:
			a.chat.Plan = e.Plan
			a.work.setPlan(e.Plan)
		case EventCommands:
			a.chat.Commands = e.Commands
		case EventConfig:
			a.chat.Config = e.Config
			values := configValues(e.Config)
			a.record(false, func(r *Record) { r.Config = values })
			a.svc.options.remember(a.configID, e.Config, time.Now())
		case EventUsage:
			a.chat.Usage = e.Usage
		case EventInfo:
			if e.Info == nil || e.Info.Title == nil {
				continue
			}
			a.title = *e.Info.Title
			a.promptTitle = false
		case EventMessage:
			if e.Message.Role != RoleAgent || !a.preview.add(e.Message) {
				continue
			}
		case EventTool:
			// Tool calls only matter to the session card's work
			// line; flush only when it changes.
			if a.turn != nil {
				a.work.tool(*e.Tool)
			}
			toolsOnly = true
		default:
			continue
		}
		if !toolsOnly {
			a.dirty = true
		}
	}
	a.refreshWork()
	a.log.append(time.Now().UTC(), events...)
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
	// config is the agent's options after derived values were set; notes
	// are the warnings about them.
	config []acp.ConfigOption
	notes  []string
	// err is user-facing and may carry the agent's stderr: never log it.
	err error
}

// start launches the session's first side in a subtask and reports the
// outcome on started once the actor has applied it. When the actor ended
// first, the subtask stops the ACP process it started and reports
// errStopped.
func (a *actor) start(started chan<- error, initialArgv []string) {
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
		shell, argv := a.shell, initialArgv
		if len(argv) == 0 {
			argv = a.cmd.TUIArgv()
		}
		a.runStart = time.Now()
		go func() {
			if err := shell.StartRun(argv); err != nil {
				report(startResult{err: fmt.Errorf("%w: 启动终端失败: %w", ErrInvalid, err)})
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

// spawnACP starts the ACP process and opens a new agent session with the
// option values derived from the launch arguments (or recorded, for a
// resumed record). It runs in a subtask; a failure stops the process
// before returning.
func (a *actor) spawnACP(epoch int) startResult {
	want := a.reapplyValues()
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
	res.config, res.notes = a.reapply(link, want, res.session.ConfigOptions)
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
	pid, process := res.pid, res.process
	if res.link == nil {
		// A terminal that already exited left the session interrupted.
		if a.state == StateActive {
			a.record(true, func(r *Record) { r.PID, r.Process = pid, process })
		}
	} else {
		a.promptTitle = true
		a.installLink(res.link, res.init, res.config, res.notes)
		sid := a.agentSessionID
		a.record(true, func(r *Record) {
			r.PID, r.Process = pid, process
			r.SetAgentSessionID(sid)
		})
	}
	a.svc.log.Info("hybrid session started", "id", a.id, "side", string(a.side), "agent", a.cmd.Profile.Name)
	// The creator answers with the shell's info: publish the side first.
	a.flush()
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
	}
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

// send starts a turn with text and attachments; the caller checked that the
// session can take one (ready, linked, no turn running). Sending clears the
// latched attention; authRequired stays until a turn ends normally. Images
// without image capability and files without embeddedContext degrade to
// paths and are reported as notices.
func (a *actor) send(text string, attachments []Attachment, queueItem, titleSource string) (PromptResult, error) {
	blocks, notes, err := promptBlocks(text, attachments, a.chat.PromptCapabilities, a.svc.uploadCheck(a.id))
	if err != nil {
		return PromptResult{}, err
	}
	a.nextTurn++
	if a.promptTitle {
		a.promptTitle = false
		if titleSource == "" {
			titleSource = text
		}
		if title := PromptTitle(titleSource); title != "" && a.title == "" {
			a.title, a.titleSession = title, a.agentSessionID
		}
	}
	n := strconv.Itoa(a.nextTurn)
	result := PromptResult{TurnID: "t" + n, PromptID: "p" + n}
	ctx, cancel := context.WithCancel(context.Background())
	t := &turn{id: result.TurnID, cancel: cancel, done: make(chan struct{})}
	a.turn = t
	a.snapshotProcesses()
	a.work.endTurn()
	startedAt := time.Now().UTC()
	a.chat.Turn = Turn{Running: true, TurnID: t.id, StartedAt: &startedAt}
	a.dirty = true
	a.clearAttention()
	a.emit(Event{Kind: EventTurnStart, TurnStart: &TurnStart{TurnID: result.TurnID, PromptID: result.PromptID, QueueItemID: queueItem}}, a.tr.userMessage(text, attachments))
	a.tr.breakMessage()
	for _, note := range notes {
		a.emit(Event{Kind: EventNotice, Notice: &Notice{Level: acp.NoticeWarning, Text: note}})
	}
	conn, sid := a.link.conn, a.link.sessionID
	go func() {
		defer close(t.done)
		resp, err := conn.Prompt(ctx, acp.PromptRequest{SessionID: sid, Prompt: blocks})
		a.post(func() { a.turnEnded(t, resp, err) })
	}()
	return result, nil
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
	// Read before endTurn clears the tracker. An agent may end the turn while
	// a backgrounded command still runs (agy), so the turn is not "done".
	running := a.work.running()
	a.work.endTurn()
	t.cancel()
	stop, text := stopReason(resp, err)
	var rpc *acp.RPCError
	code := 0
	if errors.As(err, &rpc) {
		code = rpc.Code
	}
	a.svc.log.Info("hybrid turn ended", "id", a.id, "stopReason", stop, "code", code)
	a.tr.breakMessage()
	endedAt := time.Now().UTC()
	unfinished := stop == acp.StopEndTurn && running
	a.chat.Turn = Turn{Unfinished: unfinished, TurnID: t.id, StopReason: stop, StartedAt: a.chat.Turn.StartedAt, EndedAt: &endedAt}
	normal := stop == acp.StopEndTurn || stop == acp.StopMaxTokens || stop == acp.StopMaxTurnRequests
	switch {
	case stop == StopAuthRequired:
		a.chat.AuthRequired = true
	case normal:
		a.chat.AuthRequired = false
		a.reconnects = 0
	}
	a.dirty = true
	a.emit(Event{Kind: EventTurnEnd, TurnEnd: &TurnEnd{TurnID: t.id, StopReason: stop, Error: text}})
	now := time.Now().UTC()
	switch stop {
	case acp.StopEndTurn:
		if running {
			a.watchBackground()
		} else {
			a.attention = &session.Attention{Kind: attentionkind.Done, Source: session.SourceAgent, Since: now}
		}
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
	a.saveShellInfo(a.shell.Info())
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
// closed and removes the shell. A start or reconnect in progress finishes
// first.
func (a *actor) beginClose() {
	if a.closing || a.stopped {
		return
	}
	if a.starting || a.opening {
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
