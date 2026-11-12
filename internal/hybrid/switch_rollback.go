package hybrid

import (
	"errors"
	"fmt"
	"github.com/rainchan/webmux/internal/acp"
	"github.com/rainchan/webmux/internal/session"
	"strconv"
	"time"
)

// toTUI starts a switch to the terminal: the running turn is cancelled
// (pending requests answered cancelled) and the connection detached, so
// its end is no crash.
func (a *actor) toTUI(op *switchOp) {
	a.stopBackgroundWatch()
	link := a.link
	a.link = nil
	a.chat.PromptCapabilities = PromptCapabilities{}
	a.dirty = true
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
		if link != nil {
			_ = link.stop()
		}
		return
	}
	var gone int
	if link != nil {
		if !a.sync(func() { a.setPhase(PhaseStopping) }) {
			_ = link.stop()
			return
		}
		if err := link.release(); err != nil {
			// The agent may still hold the session: no other writer may
			// start, rollback included.
			a.sync(func() {
				a.switchAbandoned(op, failure{err: fmt.Errorf("%w: %w", ErrAgent, err), public: err.Error()}, SideACP)
			})
			return
		}
		gone = link.sidecar.PID()
	}
	if !a.sync(func() { a.setPhase(PhaseStarting); a.processGone(gone) }) {
		return
	}
	if err := a.prepareTUIResume(sid); err != nil {
		a.svc.log.Warn("hybrid terminal resume preparation failed", "id", a.id, "error", err)
		a.rollbackToACP(op, sid, failure{err: fmt.Errorf("%w: 准备终端会话失败", ErrAgent), public: "准备终端会话失败"})
		return
	}
	argv, dropped := a.cmd.ResumeArgv(sid)
	if err := a.shell.StartRun(argv); err != nil {
		a.svc.log.Warn("hybrid terminal start failed", "id", a.id, "error", err)
		a.rollbackToACP(op, sid, failure{err: fmt.Errorf("%w: 启动终端失败: %w", ErrAgent, err), public: "启动终端失败"})
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
	a.recordTUIGap()
	a.side = SideTUI
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
	a.runStart = time.Now()
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
	err := a.prepareTUIResume(sid)
	if err == nil {
		argv, _ := a.cmd.ResumeArgv(sid)
		err = a.shell.StartRun(argv)
	}
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
		a.installLink(res.link, res.init, res.config, res.notes)
		a.emit(Event{Kind: EventMarker, Marker: &Marker{Kind: MarkerLoaded, Side: SideACP}})
		if len(op.queued) > 0 {
			a.setQueue(op.queued, true)
		}
		pid := res.link.sidecar.PID()
		process := processIdentity(pid)
		a.record(true, func(r *Record) { r.Side, r.PID, r.Process = SideACP, pid, process })
		a.rolledBack(op, cause)
		a.checkLink()
	}) && res.link != nil {
		_ = res.link.stop()
	}
}

// rolledBack fails the switch after the session is back on its side; every
// device sees why.
func (a *actor) rolledBack(op *switchOp, cause failure) {
	a.noteConnector(cause.err)
	a.chat.SwitchError = "切换失败，已回到原端：" + cause.public
	a.emit(Event{Kind: EventNotice, Notice: &Notice{Level: acp.NoticeError, Text: a.chat.SwitchError}})
	a.finish(op, SwitchResult{}, cause.err)
}

// noteConnector applies a failure caused by a missing connector: the chat
// side shows how to install it, the terminal side's eligibility says so.
func (a *actor) noteConnector(err error) {
	var missing *ConnectorMissingError
	if !errors.As(err, &missing) {
		return
	}
	a.setConnector(err)
	if a.side == SideACP {
		a.chat.ConnectorMissing = missing.Connector
	}
	a.dirty = true
}

// switchAbandoned fails a switch that could not return: the session is
// interrupted on side, its shell kept for recovery. Prompts a switch to
// the terminal took from the queue come back, paused, and count as what
// the interruption lost. The side's process may still run: the record
// keeps its identity.
func (a *actor) switchAbandoned(op *switchOp, cause failure, side Side) {
	a.side = side
	a.noteConnector(cause.err)
	if len(op.queued) > 0 {
		a.setQueue(op.queued, true)
	}
	a.chat.SwitchError = "切换失败，且未能回到原端：" + cause.public
	a.emit(Event{Kind: EventNotice, Notice: &Notice{Level: acp.NoticeError, Text: a.chat.SwitchError}})
	a.interrupt("switching", false)
	a.finish(op, SwitchResult{}, cause.err)
}
