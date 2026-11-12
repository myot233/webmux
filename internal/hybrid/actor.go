package hybrid

import (
	"context"
	"errors"
	"github.com/rainchan/webmux/internal/agentprofile"
	"github.com/rainchan/webmux/internal/attentionkind"
	"github.com/rainchan/webmux/internal/conversation"
	"github.com/rainchan/webmux/internal/session"
	"sync"
	"time"
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
	activity activityRecorder
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
	// attentionPolicy configures notification behavior on the chat side.
	attentionPolicy session.AttentionPolicy
	// doneDelayTimer delays latching done attention when DoneDelay > 0.
	doneDelayTimer *time.Timer
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
	id                string
	cancel            context.CancelFunc
	backgroundRefresh bool
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

func newActor(s *Service, cmd agentprofile.Command, cwd, configID string, side Side, policy *session.AttentionPolicy) *actor {
	_, unsupported := cmd.ACPArgv()
	if unsupported == nil {
		unsupported = []string{}
	}
	a := &actor{
		svc: s, cmd: cmd, cwd: cwd, configID: configID,
		inbox: make(chan func(), inboxSize), quit: make(chan struct{}),
		side: side, state: StateActive, agentCwd: cwd,
		attentionPolicy: session.DefaultAttentionPolicy(),
	}
	if policy != nil {
		a.attentionPolicy = *policy
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

func (a *actor) stopDoneDelayTimer() {
	if a.doneDelayTimer != nil {
		a.doneDelayTimer.Stop()
		a.doneDelayTimer = nil
	}
}

// stopTimers stops the probe timers and the running probes.
func (a *actor) stopTimers() {
	a.stopDoneDelayTimer()
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
	if !a.post(func() {
		v, err := fn()
		a.flush()
		reply <- result{v, err}
	}) {
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
		a.stopDoneDelayTimer()
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
	if a.attentionPolicy.Mode == session.AttentionModeOff {
		return nil
	}
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
	a.stopDoneDelayTimer()
	if a.attention != nil {
		a.attention = nil
		a.dirty = true
	}
}

// scheduleOrLatchDone latches done attention when allowed by attentionPolicy
// (Mode == all and elapsed turn duration >= MinTurn), applying DoneDelay when
// configured.
func (a *actor) scheduleOrLatchDone(startedAt *time.Time, now time.Time) {
	a.stopDoneDelayTimer()
	if a.attentionPolicy.Mode == session.AttentionModeUrgent || a.attentionPolicy.Mode == session.AttentionModeOff {
		return
	}
	if a.attentionPolicy.MinTurn > 0 && startedAt != nil && now.Sub(*startedAt) < a.attentionPolicy.MinTurn {
		return
	}
	if a.attentionPolicy.DoneDelay <= 0 {
		a.attention = &session.Attention{Kind: attentionkind.Done, Source: session.SourceAgent, Since: now}
		a.dirty = true
		return
	}
	var timer *time.Timer
	timer = time.AfterFunc(a.attentionPolicy.DoneDelay, func() {
		a.post(func() {
			if a.doneDelayTimer != timer {
				return
			}
			a.doneDelayTimer = nil
			if a.side != SideACP || a.state != StateActive || a.turn != nil || len(a.requests) > 0 || a.attention != nil {
				return
			}
			a.attention = &session.Attention{Kind: attentionkind.Done, Source: session.SourceAgent, Since: time.Now().UTC()}
			a.dirty = true
		})
	})
	a.doneDelayTimer = timer
}

// emit stores events in the log, keeping the replaceable state they carry.
func (a *actor) emit(events ...Event) {
	for _, e := range events {
		if e.Kind == EventResolved && e.Resolved != nil {
			if err := a.writeActivity(conversation.ActivityInput{LocalKey: a.activityKey("permission"), Kind: "permission", Status: e.Resolved.Outcome, Text: e.Resolved.OptionID}); err != nil {
				a.activityFailure()
			}
		}
		if e.Kind == EventMarker && e.Marker != nil {
			if err := a.writeActivity(conversation.ActivityInput{LocalKey: a.activityKey("marker"), Kind: "marker", Status: e.Marker.Kind}); err != nil {
				a.activityFailure()
			}
		}
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
