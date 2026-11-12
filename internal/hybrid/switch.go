package hybrid

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

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
// waiting in PhaseAwaitingChoice with Choice. AgentSessionID picks one of
// the candidates a SessionAmbiguousError offered, moving to the chat only.
type SwitchInput struct {
	ClientRequestID string
	To              Side
	Choice          string
	AgentSessionID  string
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
	// pick is the user's choice among ambiguous candidates.
	pick string
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
	if in.AgentSessionID != "" && in.To != SideACP {
		return SwitchResult{}, fmt.Errorf("%w: agentSessionId 只能用于切到聊天", ErrInvalid)
	}
	return once(ctx, &a.recent, in.ClientRequestID, "switch", func(ctx context.Context) (SwitchResult, error) {
		done, err := call(ctx, a, func() (<-chan switchOutcome, error) { return a.beginSwitch(in.To, in.AgentSessionID) })
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
// arrives. Switching to the side the session is on succeeds at once. pick
// chooses among the candidates a SessionAmbiguousError offered.
func (a *actor) beginSwitch(to Side, pick string) (<-chan switchOutcome, error) {
	if err := a.checkDispatch(true); err != nil {
		return nil, err
	}
	switch {
	case a.state != StateActive || a.closing:
		return nil, fmt.Errorf("%w: 会话已结束", ErrConflict)
	case a.switching != nil:
		return nil, ErrSwitching
	case a.starting || a.opening:
		return nil, fmt.Errorf("%w: agent 正在启动或重连，请稍后重试", ErrConflict)
	}
	done := make(chan switchOutcome, 1)
	if to == a.side {
		done <- switchOutcome{result: a.resultOn(to)}
		return done, nil
	}
	op := &switchOp{from: a.side, to: to, done: done, pick: pick}
	if to == SideACP {
		// Conditions 1-4 now, 5 (session/list, with the connector checked
		// again) in a fresh probe, 6 after it.
		id, ident, known := a.identify()
		t, reason := a.assess(id, ident, known)
		switch reason {
		case "":
		case ReasonConnectorMissing:
			a.setEligibility(reason)
			return nil, &ConnectorMissingError{Connector: a.cmd.Profile.Connector}
		default:
			a.setEligibility(reason)
			return nil, &IneligibleError{Reason: reason}
		}
		a.switching = op
		a.chat.SwitchError = ""
		a.dirty = true
		a.runProbe(t, func(ans probeAnswer) { a.checked(op, t, ans) })
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

// checked continues a switch to the chat after its probe: the connector
// must be installed, the session listed (5) and not open elsewhere (6),
// else nothing changed. Recently updated sessions standing in for the
// terminal's must come down to one, or the user's pick among them.
func (a *actor) checked(op *switchOp, t target, ans probeAnswer) {
	if key := a.probe.key; key == t && ans.err == nil {
		a.probe.result = probeNotListed
		if ans.listed {
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
	case errors.Is(ans.err, ErrConnectorMissing):
		a.setConnector(ans.err)
		a.setEligibility(ReasonConnectorMissing)
		a.finish(op, SwitchResult{}, ans.err)
		return
	case ans.err != nil:
		a.finish(op, SwitchResult{}, ans.err)
		return
	case !ans.listed && (t.sid == "" || a.tracksLocks() || a.tracksRegistry()):
		reason = ReasonNoSessionID
	case !ans.listed:
		reason = ReasonNotListed
	case t.sid == "":
		sid, err := pickCandidate(ans.candidates, op.pick)
		if err != nil {
			a.finish(op, SwitchResult{}, err)
			return
		}
		t.sid = sid
	}
	if reason == "" && a.svc.occupied(a.id, t.sid) {
		reason = ReasonOccupied
	}
	if reason == "" && a.externallyHeld(t.sid, true) {
		a.setEligibility(ReasonOccupied)
		a.finish(op, SwitchResult{}, ErrExternalWriter)
		return
	}
	if reason != "" {
		a.setEligibility(reason)
		a.finish(op, SwitchResult{}, &IneligibleError{Reason: reason})
		return
	}
	busy := tuiBusy(a.shell.Info()) && time.Since(a.runStart) >= a.cmd.Profile.StartupQuiet
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

// pickCandidate is the one candidate, else the user's pick among them, else
// a SessionAmbiguousError offering them.
func pickCandidate(candidates []Candidate, pick string) (string, error) {
	if len(candidates) == 1 {
		return candidates[0].AgentSessionID, nil
	}
	if pick != "" && slices.ContainsFunc(candidates, func(c Candidate) bool { return c.AgentSessionID == pick }) {
		return pick, nil
	}
	return "", &SessionAmbiguousError{Candidates: candidates}
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
	for i, key := range a.cmd.Agent.InterruptKeys() {
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
	// The switch goes on; the result names where it is heading.
	return SwitchResult{Side: op.to, Dropped: []string{}, Unsupported: []string{}}, nil
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
	a.installLink(res.link, res.init, res.config, res.notes)
	a.emit(Event{Kind: EventMarker, Marker: &Marker{Kind: MarkerSwitched, Side: SideACP}})
	pid := res.link.sidecar.PID()
	process := processIdentity(pid)
	a.record(true, func(r *Record) {
		r.Side, r.PID, r.Process = SideACP, pid, process
		r.SetAgentSessionID(t.sid)
	})
	a.probe = probeState{}
	a.reconnects = 0
	a.svc.log.Info("hybrid switch finished", "id", a.id, "side", string(SideACP))
	a.finish(op, a.resultOn(SideACP), nil)
	a.checkLink()
}
