package hybrid

import (
	"context"
	"errors"
	"fmt"
	"github.com/rainchan/webmux/internal/acp"
	"github.com/rainchan/webmux/internal/attentionkind"
	"github.com/rainchan/webmux/internal/runtime"
	"github.com/rainchan/webmux/internal/session"
	"strconv"
	"strings"
	"time"
)

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
		a.recordTUIGap()
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

// send starts a turn with text and attachments; the caller checked that the
// session can take one (ready, linked, no turn running). Sending clears the
// latched attention; authRequired stays until a turn ends normally. Images
// without image capability and files without embeddedContext degrade to
// paths and are reported as notices.
func (a *actor) send(text string, attachments []Attachment, queueItem, titleSource string) (PromptResult, error) {
	return a.sendTurn(text, attachments, queueItem, titleSource, false)
}

func (a *actor) sendTurn(text string, attachments []Attachment, queueItem, titleSource string, backgroundRefresh bool) (PromptResult, error) {
	blocks, notes, err := promptBlocks(text, attachments, a.chat.PromptCapabilities, a.svc.uploadCheck(a.id))
	if err != nil {
		return PromptResult{}, err
	}
	if !backgroundRefresh {
		if err := a.beginActivityRun(text, attachments); err != nil {
			return PromptResult{}, err
		}
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
	t := &turn{id: result.TurnID, cancel: cancel, backgroundRefresh: backgroundRefresh, done: make(chan struct{})}
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
	// a backgrounded command still runs, so the turn is not "done". A refresh
	// turn must not trigger another refresh when the old tool completes.
	unfinishedWork := !t.backgroundRefresh && a.work.unfinished()
	a.work.endTurn()
	t.cancel()
	stop, text := stopReason(resp, err)
	a.finishActivityRun(stop)
	var rpc *acp.RPCError
	code := 0
	if errors.As(err, &rpc) {
		code = rpc.Code
	}
	a.svc.log.Info("hybrid turn ended", "id", a.id, "stopReason", stop, "code", code)
	a.tr.breakMessage()
	endedAt := time.Now().UTC()
	unfinished := stop == acp.StopEndTurn && unfinishedWork
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
		if unfinishedWork {
			a.watchBackground()
		} else {
			a.scheduleOrLatchDone(a.chat.Turn.StartedAt, now)
		}
	case acp.StopRefusal, StopError, StopAuthRequired:
		a.stopDoneDelayTimer()
		if a.attentionPolicy.Mode != session.AttentionModeOff {
			a.attention = &session.Attention{Kind: attentionkind.Error, Source: session.SourceAgent, Since: now}
		}
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
