package hybrid

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"time"

	"github.com/rainchan/webmux/internal/acp"
	"github.com/rainchan/webmux/internal/agentprofile"
	"github.com/rainchan/webmux/internal/session"
)

// Reasons switching to chat is unavailable, in the order the conditions
// are checked (agent-session-identity).
const (
	ReasonUnknownAgent = "unknown_agent"
	ReasonNotRoot      = "not_root"
	ReasonNoSessionID  = "no_session_id"
	ReasonNotListed    = "not_listed"
	ReasonOccupied     = "occupied"
	// ReasonChecking: the session/list probe has not answered yet.
	ReasonChecking = "checking"
)

const (
	// maxProbes bounds the eligibility probes running at once, server-wide.
	maxProbes = 2
	// probeTimeout bounds one probe, from process start to the last page.
	probeTimeout = 15 * time.Second
	// probePages bounds the session/list pages one probe reads.
	probePages = 20
)

// IneligibleError refuses a switch to chat: Reason is an eligibility code.
type IneligibleError struct {
	Reason string
}

func (e *IneligibleError) Error() string {
	return ErrNotEligible.Error() + ": " + reasonText(e.Reason)
}

func (e *IneligibleError) Unwrap() error { return ErrNotEligible }

// reasonText is the user-facing text of an eligibility reason.
func reasonText(reason string) string {
	switch reason {
	case ReasonUnknownAgent:
		return "未识别到支持的 Agent"
	case ReasonNotRoot:
		return "Agent 不是由本会话直接启动的"
	case ReasonNoSessionID:
		return "请先在 TUI 中发送一条消息"
	case ReasonNotListed:
		return "会话尚未保存或找不到会话"
	case ReasonOccupied:
		return "该会话已在另一个会话中打开"
	case ReasonChecking:
		return "正在核对会话"
	}
	return reason
}

// target is the agent session the terminal has open, and the resolved cwd
// its agent lists it under.
type target struct {
	sid, cwd string
}

// probe outcomes for the current target.
type probeResult int

const (
	probePending probeResult = iota
	probeListed
	probeNotListed
)

// probeState is condition 4 for the terminal's current target: checked
// after the target changes (debounced), again once the terminal is idle
// after output since the last check while it was not listed yet (an agent
// saves a new session with its first message), and right before a switch.
type probeState struct {
	key     target
	result  probeResult
	timer   *time.Timer
	running bool
	again   bool
	// checked is when the last probe of key started.
	checked time.Time
	// rawCwd and cwd cache the symlink resolution of the identity's cwd.
	rawCwd, cwd string
}

func signals(id session.Identity) agentprofile.Signals {
	return agentprofile.Signals{
		WarpAgent: id.WarpAgent, WarpPluginVersion: id.WarpPluginVersion, WarpSessionID: id.WarpSessionID,
		WarpCwd: id.WarpCwd, ProgramApp: id.ProgramApp, ProgramChildID: id.ProgramChildID,
		RootExecutable: id.Executable, RootCwd: id.Cwd,
	}
}

// observeLoop has the actor look at its terminal every timing.observe.
func (a *actor) observeLoop() {
	tick := time.NewTicker(a.svc.timing.observe)
	defer tick.Stop()
	for {
		select {
		case <-tick.C:
			a.post(a.observe)
		case <-a.quit:
			return
		}
	}
}

// observe follows the terminal side's agent: its session ID (an ID change
// is shown and recorded) and the switch eligibility.
func (a *actor) observe() {
	if a.side != SideTUI || a.state != StateActive || a.closing || a.starting || a.switching != nil {
		return
	}
	id := a.shell.Identity()
	ident, known := agentprofile.Identify(signals(id))
	if known && ident.Agent == a.cmd.Profile.Name && ident.AgentSessionID != "" && ident.AgentSessionID != a.agentSessionID {
		a.agentSessionChanged(ident.AgentSessionID)
	}
	info := a.shell.Info()
	t, reason := a.assess(id, ident, known)
	if reason == "" {
		switch {
		case t != a.probe.key:
			if a.probe.timer != nil {
				a.probe.timer.Stop()
			}
			a.probe = probeState{key: t, rawCwd: a.probe.rawCwd, cwd: a.probe.cwd, running: a.probe.running}
			a.scheduleProbe()
		case a.probe.result == probeNotListed && a.probe.timer == nil && !a.probe.running &&
			!tuiBusy(info) && info.LastActiveAt.After(a.probe.checked):
			a.scheduleProbe()
		}
		reason = a.listedReason(t)
	}
	a.setEligibility(reason)
}

// agentSessionChanged follows the terminal to another agent session
// (/new, /resume, /fork): the record keeps the previous ID and every
// device is told. The first ID a terminal reports is no change.
func (a *actor) agentSessionChanged(sid string) {
	old := a.agentSessionID
	a.agentSessionID = sid
	if old != "" {
		a.chat.IDChange = &IDChange{From: old, To: sid}
		a.svc.log.Info("hybrid agent session changed", "id", a.id)
	}
	a.record(false, func(r *Record) { r.SetAgentSessionID(sid) })
	a.dirty = true
}

// assess checks conditions 1 to 3: a known agent of this session's
// profile, running as the PTY's root process, with a session ID: the one
// the terminal reports, or else the one its run was started to resume. It
// returns the target or the first failed condition's reason.
func (a *actor) assess(id session.Identity, ident agentprofile.Identity, known bool) (target, string) {
	sid := ident.AgentSessionID
	if sid == "" {
		sid = a.resumedSID
	}
	switch {
	case !known || ident.Agent != a.cmd.Profile.Name:
		return target{}, ReasonUnknownAgent
	case id.PID == 0 || id.Executable != ident.Agent:
		return target{}, ReasonNotRoot
	case sid == "":
		return target{}, ReasonNoSessionID
	}
	raw := ident.Cwd
	if raw == "" {
		raw = a.cwd
	}
	if raw != a.probe.rawCwd {
		cwd, err := filepath.EvalSymlinks(raw)
		if err != nil {
			cwd = raw
		}
		a.probe.rawCwd, a.probe.cwd = raw, cwd
	}
	return target{sid: sid, cwd: a.probe.cwd}, ""
}

// listedReason is conditions 4 and 5 for the current target.
func (a *actor) listedReason(t target) string {
	switch {
	case a.probe.key != t || a.probe.result == probePending:
		return ReasonChecking
	case a.probe.result == probeNotListed:
		return ReasonNotListed
	case a.svc.occupied(a.id, t.sid):
		return ReasonOccupied
	}
	return ""
}

func (a *actor) setEligibility(reason string) {
	e := Eligibility{Available: reason == "", Reason: reason}
	if a.chat.Eligibility != e {
		a.chat.Eligibility = e
		a.dirty = true
	}
}

// scheduleProbe checks the current target after the debounce delay.
func (a *actor) scheduleProbe() {
	if a.probe.timer != nil {
		a.probe.timer.Stop()
	}
	key := a.probe.key
	a.probe.timer = time.AfterFunc(a.svc.timing.probeDelay, func() { a.post(func() { a.probeDue(key) }) })
}

// probeDue starts the debounced probe of key unless the target moved on;
// with one running it runs again afterwards.
func (a *actor) probeDue(key target) {
	if key != a.probe.key || a.side != SideTUI || a.closing || a.stopped {
		return
	}
	a.probe.timer = nil
	a.probe.checked = time.Now()
	if a.probe.running {
		a.probe.again = true
		return
	}
	a.probe.running = true
	a.runProbe(key, func(listed bool, err error) { a.probed(key, listed, err) })
}

// probed records a background probe's answer.
func (a *actor) probed(key target, listed bool, err error) {
	a.probe.running = false
	if key == a.probe.key && !errors.Is(err, context.Canceled) {
		a.probe.result = probeNotListed
		if listed {
			a.probe.result = probeListed
		}
	}
	if a.probe.again {
		a.probe.again = false
		a.probeDue(a.probe.key)
	}
	a.observe()
}

// runProbe checks t in a subtask and posts done with the answer.
func (a *actor) runProbe(t target, done func(listed bool, err error)) {
	a.probes.Add(1)
	go func() {
		defer a.probes.Done()
		listed, err := a.listed(a.probeCtx, t)
		a.post(func() { done(listed, err) })
	}()
}

// listed reports whether t.sid is among the sessions its agent lists in
// t.cwd.
func (a *actor) listed(ctx context.Context, t target) (bool, error) {
	sessions, err := a.sessions(ctx, t.cwd, t.sid)
	if err != nil {
		return false, err
	}
	return slices.ContainsFunc(sessions, func(s acp.SessionInfo) bool { return s.SessionID == t.sid }), nil
}

// sessions runs a temporary ACP process that only initializes and lists the
// sessions in cwd, page by page until one is session until or the pages
// run out (at most probePages). It never loads or resumes a session, so it
// is never a second writer. At most maxProbes run at once, server-wide.
func (a *actor) sessions(ctx context.Context, cwd, until string) ([]acp.SessionInfo, error) {
	select {
	case a.svc.probes <- struct{}{}:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	defer func() { <-a.svc.probes }()
	ctx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()
	argv, _ := a.cmd.ACPArgv()
	sidecar, err := a.shell.StartSidecar(argv, a.cmd.ProtocolOnFD3())
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrAgent, err)
	}
	link := &acpLink{sidecar: sidecar}
	link.conn = acp.NewConn(sidecar.Stdout(), sidecar.Stdin(), acp.Options{Logger: a.svc.log})
	defer func() { _ = link.stop() }()
	init, err := link.conn.Initialize(ctx, acp.Implementation{Name: "webmux", Title: "WebMux", Version: a.svc.version}, nil)
	if err == nil && init.AgentCapabilities.SessionCapabilities.List == nil {
		err = errMissingCapability
	}
	if err != nil {
		f := agentFailure(acp.MethodInitialize, err, sidecar)
		a.svc.log.Warn("hybrid probe failed", "id", a.id, "method", acp.MethodInitialize, "error", err)
		return nil, f.err
	}
	var out []acp.SessionInfo
	cursor := ""
	for range probePages {
		resp, err := link.conn.ListSessions(ctx, acp.ListSessionsRequest{Cwd: cwd, Cursor: cursor})
		if err != nil {
			a.svc.log.Warn("hybrid probe failed", "id", a.id, "method", acp.MethodSessionList, "error", err)
			return nil, agentFailure(acp.MethodSessionList, err, sidecar).err
		}
		out = append(out, resp.Sessions...)
		if resp.NextCursor == "" || slices.ContainsFunc(resp.Sessions, func(s acp.SessionInfo) bool { return s.SessionID == until }) {
			break
		}
		cursor = resp.NextCursor
	}
	return out, nil
}

// occupied reports whether another session has agent session sid open:
// an active hybrid session's record, or any session's terminal signals,
// ordinary sessions included (D26).
func (s *Service) occupied(self, sid string) bool {
	if s.records.agentInUse(self, sid) {
		return true
	}
	for _, info := range s.manager.List() {
		if info.ID == self {
			continue
		}
		id, err := s.manager.Identity(info.ID)
		if err != nil {
			continue
		}
		if ident, ok := agentprofile.Identify(signals(id)); ok && ident.AgentSessionID == sid {
			return true
		}
	}
	return false
}
