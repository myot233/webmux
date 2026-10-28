package hybrid

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/rainchan/webmux/internal/acp"
	"github.com/rainchan/webmux/internal/agentprofile"
	"github.com/rainchan/webmux/internal/attentionkind"
	"github.com/rainchan/webmux/internal/session"
)

// Answers to an interrupt timeout (D21).
const (
	ChoiceWait   = "wait"
	ChoiceForce  = "force"
	ChoiceCancel = "cancel"
)

const (
	// idlePoll is how often a switch looks at the terminal UI's status
	// while waiting for it to go idle.
	idlePoll = 50 * time.Millisecond
	// keyTimeout bounds writing the interrupt key.
	keyTimeout = 5 * time.Second
)

// SwitchInput asks to move a session to side To, or answers a switch
// waiting in PhaseAwaitingChoice with Choice.
type SwitchInput struct {
	ClientRequestID string
	To              Side
	Choice          string
}

// SwitchResult is a finished switch request: the side the session is on.
// Draft (to the terminal only) is the merged text of the prompts that were
// still queued, returned to the requesting device alone; Dropped lists the
// launch arguments the resume command left out; Unsupported the ones the
// chat side cannot apply.
type SwitchResult struct {
	Side        Side
	Draft       string
	Dropped     []string
	Unsupported []string
}

// switchOp is one running switch. done receives its outcome once.
type switchOp struct {
	from, to Side
	done     chan switchOutcome
	// choice receives the user's answer while the switch awaits one.
	choice chan string
	// queued is what the queue held when it became the draft, restored if
	// the switch rolls back to the chat.
	queued []QueueItem
}

type switchOutcome struct {
	result SwitchResult
	err    error
}

// Switch moves session id to in.To and returns once it got there, rolled
// back, or was cancelled; a retry with the same client request ID gets the
// same outcome. Moving to the chat requires eligibility (an
// *IneligibleError otherwise) and may pause in PhaseAwaitingChoice, which
// another Switch with Choice answers. During a switch every other switch,
// command and close fails with ErrSwitching. A failed switch returns to
// the original side and fails with the reason; a failed rollback leaves
// the session interrupted.
func (s *Service) Switch(ctx context.Context, id string, in SwitchInput) (SwitchResult, error) {
	a, err := s.actor(id)
	if err != nil {
		return SwitchResult{}, err
	}
	if in.Choice != "" {
		switch in.Choice {
		case ChoiceWait, ChoiceForce, ChoiceCancel:
		default:
			return SwitchResult{}, fmt.Errorf("%w: choice 只能是 wait、force 或 cancel", ErrInvalid)
		}
		return once(ctx, &a.recent, in.ClientRequestID, "switch.choice", func(ctx context.Context) (SwitchResult, error) {
			return call(ctx, a, func() (SwitchResult, error) { return a.choose(in.Choice) })
		})
	}
	if in.To != SideTUI && in.To != SideACP {
		return SwitchResult{}, fmt.Errorf("%w: to 只能是 tui 或 acp", ErrInvalid)
	}
	return once(ctx, &a.recent, in.ClientRequestID, "switch", func(ctx context.Context) (SwitchResult, error) {
		done, err := call(ctx, a, func() (<-chan switchOutcome, error) { return a.beginSwitch(in.To) })
		if err != nil {
			return SwitchResult{}, err
		}
		// A switch may wait for the user's choice for as long as it takes:
		// no command timeout applies.
		select {
		case out := <-done:
			return out.result, out.err
		case <-a.quit:
			select {
			case out := <-done:
				return out.result, out.err
			default:
				return SwitchResult{}, errStopped
			}
		}
	})
}

// beginSwitch starts a switch to side to and returns where its outcome
// arrives. Switching to the side the session is on succeeds at once.
func (a *actor) beginSwitch(to Side) (<-chan switchOutcome, error) {
	switch {
	case a.state != StateActive || a.closing:
		return nil, fmt.Errorf("%w: 会话已结束", ErrConflict)
	case a.switching != nil:
		return nil, ErrSwitching
	case a.starting || a.opening || a.sleeping:
		return nil, fmt.Errorf("%w: agent 正在启动或休眠，请稍后重试", ErrConflict)
	}
	done := make(chan switchOutcome, 1)
	if to == a.side {
		done <- switchOutcome{result: a.resultOn(to)}
		return done, nil
	}
	op := &switchOp{from: a.side, to: to, done: done}
	if to == SideACP {
		// Conditions 1-3 now, 4 (session/list) in a fresh probe, 5 after it.
		id := a.shell.Identity()
		ident, known := agentprofile.Identify(signals(id))
		t, reason := a.assess(id, ident, known)
		if reason != "" {
			a.setEligibility(reason)
			return nil, &IneligibleError{Reason: reason}
		}
		a.switching = op
		a.chat.SwitchError = ""
		a.dirty = true
		a.runProbe(t, func(listed bool, err error) { a.checked(op, t, listed, err) })
		return done, nil
	}
	a.switching = op
	a.chat.SwitchError = ""
	a.toTUI(op)
	return done, nil
}

// resultOn is the result of a switch that ended on side.
func (a *actor) resultOn(side Side) SwitchResult {
	r := SwitchResult{Side: side, Dropped: []string{}, Unsupported: []string{}}
	if side == SideACP {
		r.Unsupported = append(r.Unsupported, a.chat.Unsupported...)
	}
	return r
}

// finish ends op with an outcome, published before the requester hears it.
func (a *actor) finish(op *switchOp, result SwitchResult, err error) {
	if a.switching == op {
		a.switching = nil
	}
	a.chat.Phase = ""
	a.dirty = true
	a.flush()
	op.done <- switchOutcome{result: result, err: err}
}

func (a *actor) setPhase(p Phase) {
	a.chat.Phase = p
	a.dirty = true
}

// checked continues a switch to the chat after its probe: the session must
// be listed (4) and not open elsewhere (5), else nothing changed.
func (a *actor) checked(op *switchOp, t target, listed bool, err error) {
	if key := a.probe.key; key == t && err == nil {
		a.probe.result = probeNotListed
		if listed {
			a.probe.result = probeListed
		}
	}
	reason := ""
	switch {
	case a.state != StateActive || a.side != SideTUI:
		a.finish(op, SwitchResult{}, fmt.Errorf("%w: 会话已结束", ErrConflict))
		return
	case a.shell.Info().Status == session.StatusExited:
		// The terminal UI exited during the check; RunExited was held
		// back by the switch (D23).
		a.interrupt("idle", true)
		a.finish(op, SwitchResult{}, fmt.Errorf("%w: 终端已退出", ErrConflict))
		return
	case err != nil:
		a.finish(op, SwitchResult{}, err)
		return
	case !listed:
		reason = ReasonNotListed
	case a.svc.occupied(a.id, t.sid):
		reason = ReasonOccupied
	}
	if reason != "" {
		a.setEligibility(reason)
		a.finish(op, SwitchResult{}, &IneligibleError{Reason: reason})
		return
	}
	busy := tuiBusy(a.shell.Info())
	a.side = SideSwitching
	a.setPhase(PhaseStopping)
	if busy {
		a.setPhase(PhaseInterrupting)
	}
	a.flush()
	// Terminal clients leave now; they may attach again if the user
	// cancels.
	a.shell.MoveClients()
	a.svc.log.Info("hybrid switch started", "id", a.id, "to", string(SideACP), "interrupt", busy)
	go a.runToACP(op, t, busy)
}

// tuiBusy reports whether the terminal UI is in a turn by its signals:
// running, or waiting on the user inside a turn (not a latched done or
// error).
func tuiBusy(info session.Info) bool {
	switch info.Status {
	case session.StatusRunning:
		return true
	case session.StatusWaiting:
		return info.Attention == nil || (info.Attention.Kind != attentionkind.Done && info.Attention.Kind != attentionkind.Error)
	}
	return false
}

// interruptKeyGap separates key presses so the terminal UI reads each one
// separately; the M0 baseline stopped OpenCode with Esc, 150 ms, Esc.
const interruptKeyGap = 150 * time.Millisecond

// interruptKey writes the profile's interrupt key presses to the terminal UI.
func (a *actor) interruptKey() {
	for i, key := range a.cmd.Profile.InterruptKeys {
		if i > 0 {
			time.Sleep(interruptKeyGap)
		}
		ctx, cancel := context.WithTimeout(context.Background(), keyTimeout)
		err := a.shell.WriteContext(ctx, key)
		cancel()
		if err != nil {
			a.svc.log.Info("hybrid interrupt key not written", "id", a.id, "error", err)
			return
		}
	}
}

// waitIdle waits up to d for the terminal UI to leave its turn.
func (a *actor) waitIdle(d time.Duration) bool {
	deadline := time.NewTimer(d)
	defer deadline.Stop()
	tick := time.NewTicker(idlePoll)
	defer tick.Stop()
	for {
		if !tuiBusy(a.shell.Info()) {
			return true
		}
		select {
		case <-tick.C:
		case <-deadline.C:
			return !tuiBusy(a.shell.Info())
		case <-a.quit:
			return false
		}
	}
}

// interruptOutcome is how interrupting the terminal UI ended.
type interruptOutcome int

const (
	interruptIdle interruptOutcome = iota
	interruptCancelled
	interruptGone
)

// interruptTUI sends the interrupt key and waits for the terminal UI to go
// idle. After timing.interrupt it asks the user (D21): wait another
// period, force the switch, or cancel it. The terminal going idle while
// the question is open continues the switch.
func (a *actor) interruptTUI(op *switchOp) interruptOutcome {
	a.interruptKey()
	for {
		if a.waitIdle(a.svc.timing.interrupt) {
			return interruptIdle
		}
		choice := make(chan string, 1)
		if !a.sync(func() { op.choice = choice; a.setPhase(PhaseAwaitingChoice) }) {
			return interruptGone
		}
		c := a.awaitChoice(op, choice)
		if !a.sync(func() {
			op.choice = nil
			if c == ChoiceWait {
				a.setPhase(PhaseInterrupting)
			}
		}) {
			return interruptGone
		}
		switch c {
		case ChoiceWait:
			continue
		case ChoiceCancel:
			return interruptCancelled
		case "":
			return interruptGone
		}
		return interruptIdle
	}
}

// awaitChoice waits for the user's choice; "idle" when the terminal UI
// went idle first, "" when the actor ended. The terminal going idle wins
// only while the question is still open: deciding so closes it on the
// actor, so a choice is either acted on or refused, never lost.
func (a *actor) awaitChoice(op *switchOp, choice chan string) string {
	tick := time.NewTicker(idlePoll)
	defer tick.Stop()
	for {
		select {
		case c := <-choice:
			return c
		case <-tick.C:
			if tuiBusy(a.shell.Info()) {
				continue
			}
			idle := false
			if !a.sync(func() {
				if op.choice == choice {
					op.choice, idle = nil, true
				}
			}) {
				return ""
			}
			if idle {
				return "idle"
			}
			// choose answered first and closed the question.
			return <-choice
		case <-a.quit:
			return ""
		}
	}
}

// choose answers a switch waiting in PhaseAwaitingChoice.
func (a *actor) choose(choice string) (SwitchResult, error) {
	op := a.switching
	if op == nil || op.choice == nil {
		return SwitchResult{}, fmt.Errorf("%w: 没有等待选择的切换", ErrConflict)
	}
	op.choice <- choice
	op.choice = nil
	if choice == ChoiceWait {
		// The question is closed now; no device may answer it again.
		a.setPhase(PhaseInterrupting)
	}
	if choice == ChoiceCancel {
		return a.resultOn(SideTUI), nil
	}
	return SwitchResult{Side: SideSwitching, Dropped: []string{}, Unsupported: []string{}}, nil
}

// runToACP is the subtask of a switch to the chat: interrupt the turn if
// one runs, end the terminal UI and wait for its processes to go, start
// the ACP process, load the session into a new log generation and restore
// options (D25). Each step's failure rolls back to the terminal.
func (a *actor) runToACP(op *switchOp, t target, busy bool) {
	if busy {
		switch a.interruptTUI(op) {
		case interruptCancelled:
			a.sync(func() { a.switchCancelled(op) })
			return
		case interruptGone:
			return
		}
	}
	var pid int
	if !a.sync(func() { a.setPhase(PhaseStopping); pid = a.shell.Identity().PID }) {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), closeTimeout)
	err := a.shell.EndRun(ctx)
	cancel()
	if err != nil && !errors.Is(err, session.ErrNoRun) {
		a.svc.log.Warn("hybrid terminal did not stop for a switch", "id", a.id, "error", err)
		a.rollbackToTUI(op, t.sid, failure{err: fmt.Errorf("%w: 终端未能结束", ErrConflict), public: "终端未能结束"})
		return
	}
	var epoch int
	if !a.sync(func() { a.setPhase(PhaseStarting); a.epoch++; epoch = a.epoch; a.processGone(pid) }) {
		return
	}
	res := a.openACP(epoch, t.sid, t.cwd, false, func() bool {
		return a.sync(func() { a.setPhase(PhaseLoading); a.restartLog() })
	})
	if res.fail != nil {
		a.rollbackToTUI(op, t.sid, *res.fail)
		return
	}
	if !a.sync(func() { a.switchedToACP(op, t, res) }) {
		_ = res.link.stop()
	}
}

// switchCancelled keeps the terminal side after the user cancelled; a
// terminal UI that exited meanwhile interrupts the session (D23).
func (a *actor) switchCancelled(op *switchOp) {
	a.side = SideTUI
	a.svc.log.Info("hybrid switch cancelled", "id", a.id)
	if a.shell.Info().Status == session.StatusExited {
		a.interrupt("idle", true)
	}
	a.finish(op, a.resultOn(SideTUI), nil)
}

func (a *actor) switchedToACP(op *switchOp, t target, res openResult) {
	a.side = SideACP
	a.agentCwd = t.cwd
	a.chat.Dormant = false
	a.installLink(res.link, res.init, res.config, res.warnings)
	a.emit(Event{Kind: EventMarker, Marker: &Marker{Kind: MarkerSwitched, Side: SideACP}})
	pid := res.link.sidecar.PID()
	process := processIdentity(pid)
	a.record(true, func(r *Record) {
		r.Side, r.PID, r.Process = SideACP, pid, process
		r.SetAgentSessionID(t.sid)
	})
	a.probe = probeState{}
	a.reconnects = 0
	a.touch()
	a.svc.log.Info("hybrid switch finished", "id", a.id, "side", string(SideACP))
	a.finish(op, a.resultOn(SideACP), nil)
	a.checkLink()
}

// toTUI starts a switch to the terminal: the running turn is cancelled
// (pending requests answered cancelled) and the connection detached, so
// its end is no crash.
func (a *actor) toTUI(op *switchOp) {
	link := a.link
	a.link = nil
	var turnDone chan struct{}
	a.side = SideSwitching
	a.setPhase(PhaseStopping)
	if a.turn != nil {
		turnDone = a.turn.done
		a.setPhase(PhaseCancelling)
		a.withdrawAll()
		a.turn.cancel()
	}
	if link == nil {
		// Dormant: the agent process is already gone.
		a.setPhase(PhaseStarting)
	}
	a.svc.log.Info("hybrid switch started", "id", a.id, "to", string(SideTUI))
	go a.runToTUI(op, link, turnDone, a.agentSessionID)
}

// runToTUI is the subtask of a switch to the terminal: wait for the
// cancelled turn, return the queue as the requester's draft, close the
// agent session and wait for its process to go, then start the terminal
// UI on the session. A failed start rolls back to the chat.
func (a *actor) runToTUI(op *switchOp, link *acpLink, turnDone chan struct{}, sid string) {
	if turnDone != nil {
		wait := time.NewTimer(turnEndWait)
		select {
		case <-turnDone:
		case <-wait.C:
		}
		wait.Stop()
	}
	var draft string
	if !a.sync(func() { draft = a.returnQueue(op) }) {
		return
	}
	var gone int
	if link != nil {
		if !a.sync(func() { a.setPhase(PhaseStopping) }) {
			return
		}
		if err := link.release(); err != nil {
			// The agent may still hold the session: no other writer may
			// start, rollback included.
			a.sync(func() {
				a.switchAbandoned(op, failure{err: fmt.Errorf("%w: %v", ErrAgent, err), public: err.Error()}, SideACP)
			})
			return
		}
		gone = link.sidecar.PID()
	}
	if !a.sync(func() { a.setPhase(PhaseStarting); a.processGone(gone) }) {
		return
	}
	argv, dropped := a.cmd.ResumeArgv(sid)
	if err := a.shell.StartRun(argv); err != nil {
		a.svc.log.Warn("hybrid terminal start failed", "id", a.id, "error", err)
		a.rollbackToACP(op, sid, failure{err: fmt.Errorf("%w: 启动终端失败: %v", ErrAgent, err), public: "启动终端失败"})
		return
	}
	if dropped == nil {
		dropped = []string{}
	}
	a.sync(func() { a.switchedToTUI(op, sid, draft, dropped) })
}

// returnQueue empties the queue into the requester's draft (D20). The
// prompts are kept in op until the switch ends: a switch that stays on the
// chat gets them back.
func (a *actor) returnQueue(op *switchOp) string {
	op.queued = a.chat.Queue.Items
	return a.takeQueue()
}

// switchedToTUI finishes a switch to the terminal; every device is told
// the queued prompts went back to the device that switched.
func (a *actor) switchedToTUI(op *switchOp, sid, draft string, dropped []string) {
	a.side = SideTUI
	a.chat.Dormant = false
	if n := len(op.queued); n > 0 {
		a.emit(Event{Kind: EventNotice, Notice: &Notice{Level: acp.NoticeInfo, Text: strconv.Itoa(n) + " 条排队消息已退回发起切换的设备的输入框"}})
	}
	a.emit(Event{Kind: EventMarker, Marker: &Marker{Kind: MarkerSwitched, Side: SideTUI}})
	a.terminalStarted(sid)
	a.svc.log.Info("hybrid switch finished", "id", a.id, "side", string(SideTUI))
	result := a.resultOn(SideTUI)
	result.Draft, result.Dropped = draft, dropped
	a.finish(op, result, nil)
}

// terminalStarted records a terminal run the actor just started to resume
// agent session sid; one that already exited interrupts the session (D23).
func (a *actor) terminalStarted(sid string) {
	a.resumedSID = sid
	a.probe = probeState{}
	pid := a.shell.Identity().PID
	process := processIdentity(pid)
	a.record(true, func(r *Record) { r.Side, r.PID, r.Process = SideTUI, pid, process })
	if a.shell.Info().Status == session.StatusExited {
		a.interrupt("idle", true)
	}
	a.dirty = true
}

// rollbackToTUI returns a failed switch to the terminal: the terminal UI
// resumes the session (D22).
func (a *actor) rollbackToTUI(op *switchOp, sid string, cause failure) {
	if !a.sync(func() { a.setPhase(PhaseRollingBack) }) {
		return
	}
	argv, _ := a.cmd.ResumeArgv(sid)
	err := a.shell.StartRun(argv)
	a.sync(func() {
		a.side = SideTUI
		if err != nil {
			a.svc.log.Warn("hybrid rollback failed", "id", a.id, "side", string(SideTUI), "error", err)
			a.switchAbandoned(op, cause, SideTUI)
			return
		}
		a.terminalStarted(sid)
		a.rolledBack(op, cause)
	})
}

// rollbackToACP returns a failed switch to the chat: a new ACP process
// loads the session in a new generation and the queue comes back, paused.
func (a *actor) rollbackToACP(op *switchOp, sid string, cause failure) {
	var epoch int
	var cwd string
	if !a.sync(func() { a.setPhase(PhaseRollingBack); a.epoch++; epoch, cwd = a.epoch, a.agentCwd }) {
		return
	}
	res := a.openACP(epoch, sid, cwd, false, func() bool { return a.sync(a.restartLog) })
	if !a.sync(func() {
		a.side = SideACP
		if res.fail != nil {
			a.svc.log.Warn("hybrid rollback failed", "id", a.id, "side", string(SideACP))
			a.switchAbandoned(op, cause, SideACP)
			return
		}
		a.installLink(res.link, res.init, res.config, res.warnings)
		a.emit(Event{Kind: EventMarker, Marker: &Marker{Kind: MarkerLoaded, Side: SideACP}})
		if len(op.queued) > 0 {
			a.setQueue(op.queued, true)
		}
		pid := res.link.sidecar.PID()
		process := processIdentity(pid)
		a.record(true, func(r *Record) { r.Side, r.PID, r.Process = SideACP, pid, process })
		a.touch()
		a.rolledBack(op, cause)
		a.checkLink()
	}) && res.link != nil {
		_ = res.link.stop()
	}
}

// rolledBack fails the switch after the session is back on its side; every
// device sees why.
func (a *actor) rolledBack(op *switchOp, cause failure) {
	a.chat.SwitchError = "切换失败，已回到原端：" + cause.public
	a.emit(Event{Kind: EventNotice, Notice: &Notice{Level: acp.NoticeError, Text: a.chat.SwitchError}})
	a.finish(op, SwitchResult{}, cause.err)
}

// switchAbandoned fails a switch that could not return: the session is
// interrupted on side, its shell kept for recovery. Prompts a switch to
// the terminal took from the queue come back, paused, and count as what
// the interruption lost. The side's process may still run: the record
// keeps its identity.
func (a *actor) switchAbandoned(op *switchOp, cause failure, side Side) {
	a.side = side
	if len(op.queued) > 0 {
		a.setQueue(op.queued, true)
	}
	a.chat.SwitchError = "切换失败，且未能回到原端：" + cause.public
	a.emit(Event{Kind: EventNotice, Notice: &Notice{Level: acp.NoticeError, Text: a.chat.SwitchError}})
	a.interrupt("switching", false)
	a.finish(op, SwitchResult{}, cause.err)
}
