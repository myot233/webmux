package hybrid

import (
	"context"
	"errors"
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
	ReasonUnknownAgent     = "unknown_agent"
	ReasonConnectorMissing = "connector_missing"
	ReasonNotRoot          = "not_root"
	ReasonNoSessionID      = "no_session_id"
	ReasonNotListed        = "not_listed"
	ReasonOccupied         = "occupied"
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
	// lockReadEvery bounds how often a terminal's open files are read for
	// its writer locks.
	lockReadEvery = 2 * time.Second
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
	case ReasonConnectorMissing:
		return "未安装 ACP 连接器"
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
// its agent lists it under. An empty sid stands for the sessions updated
// since the terminal run started, when a profile's locks cannot tell
// which one the terminal has open.
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

// probeAnswer is a finished probe: whether the target is listed, and for
// an empty sid the candidate sessions.
type probeAnswer struct {
	listed     bool
	candidates []Candidate
	err        error
}

// probeState is condition 5 for the terminal's current target: checked
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
	id, ident, known := a.identify()
	if known && ident.Agent == a.cmd.Profile.Name && ident.AgentSessionID != "" && ident.AgentSessionID != a.agentSessionID {
		a.agentSessionChanged(ident.AgentSessionID)
	}
	info := a.shell.Info()
	a.saveShellInfo(info)
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

// identify reads the terminal's identity and the profile's view of it. A
// lock- or registry-tracking profile gets its session ID from that source.
// When terminal signals don't identify the agent (e.g. qoder/qodercn don't
// emit OSC 7501), fall back to process executable matching.
func (a *actor) identify() (session.Identity, agentprofile.Identity, bool) {
	id := a.shell.Identity()
	ident, known := a.svc.agents.Identify(signals(id))
	if known && ident.Agent == a.cmd.Profile.Name {
		switch {
		case a.tracksLocks():
			ident.AgentSessionID = a.lockedSession(id)
		case a.tracksRegistry():
			ident.AgentSessionID = a.registeredSession(id)
		}
		return id, ident, known
	}
	// Fallback: identify from the PTY root process executable.
	if !known && id.PID != 0 && id.Executable != "" {
		p := a.cmd.Profile
		if p.RunsProcess(id.Executable) ||
			(p.Launcher != "" && id.Executable == p.Launcher &&
				slices.ContainsFunc(a.shell.RunChildren(), p.RunsProcess)) {
			fallback := agentprofile.Identity{Agent: p.Name, Cwd: id.Cwd}
			switch {
			case a.tracksLocks():
				fallback.AgentSessionID = a.lockedSession(id)
			case a.tracksRegistry():
				fallback.AgentSessionID = a.registeredSession(id)
			}
			return id, fallback, true
		}
	}
	return id, ident, known
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

// assess checks conditions 1 to 4: a known agent of this session's
// profile, its connector installed, running as the PTY's root process (or
// under its launcher), with a session ID: the one the terminal reports, or
// else the one its run was started to resume. A lock-tracking profile
// whose locks do not tell gets the recently updated sessions as target
// (empty sid). It returns the target or the first failed condition's
// reason.
func (a *actor) assess(id session.Identity, ident agentprofile.Identity, known bool) (target, string) {
	sid := ident.AgentSessionID
	if sid == "" && !a.tracksRegistry() {
		sid = a.resumedSID
	}
	if !known || ident.Agent != a.cmd.Profile.Name {
		return target{}, ReasonUnknownAgent
	}
	if reason := a.connectorReason(); reason != "" {
		return target{}, reason
	}
	switch {
	case !a.agentIsRoot(id):
		return target{}, ReasonNotRoot
	case sid == "" && !a.identityFallback():
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

// listedReason is conditions 5 and 6 for the current target. A session a
// lock-tracking profile holds but has not saved yet is waiting for its
// first message.
func (a *actor) listedReason(t target) string {
	switch {
	case a.probe.key != t || a.probe.result == probePending:
		return ReasonChecking
	case a.probe.result == probeNotListed && (t.sid == "" || a.tracksLocks() || a.tracksRegistry()):
		return ReasonNoSessionID
	case a.probe.result == probeNotListed:
		return ReasonNotListed
	case t.sid != "" && a.svc.occupied(a.id, t.sid):
		return ReasonOccupied
	case t.sid != "" && a.externallyHeld(t.sid, false):
		return ReasonOccupied
	}
	return ""
}

func (a *actor) setEligibility(reason string) {
	e := Eligibility{Available: reason == "", Reason: reason}
	if reason == ReasonConnectorMissing {
		e.Connector = a.cmd.Profile.Connector
	}
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
	a.runProbe(key, func(ans probeAnswer) { a.probed(key, ans) })
}

// probed records a background probe's answer. A missing connector sends
// the check back to condition 2 and the target is probed again once the
// connector is found.
func (a *actor) probed(key target, ans probeAnswer) {
	a.probe.running = false
	switch {
	case errors.Is(ans.err, ErrConnectorMissing):
		a.setConnector(ans.err)
		a.probe.key = target{}
	case key == a.probe.key && !errors.Is(ans.err, context.Canceled):
		a.probe.result = probeNotListed
		if ans.listed {
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
func (a *actor) runProbe(t target, done func(probeAnswer)) {
	a.probes.Add(1)
	since := a.runStart
	go func() {
		defer a.probes.Done()
		ans := a.listed(a.probeCtx, t, since)
		a.post(func() { done(ans) })
	}()
}

// listed reports whether t.sid is among the sessions its agent lists in
// t.cwd; for an empty sid, the sessions there updated after since are the
// candidates, listed when there is one at least.
func (a *actor) listed(ctx context.Context, t target, since time.Time) probeAnswer {
	sessions, err := a.sessions(ctx, t.cwd, t.sid)
	if err != nil {
		return probeAnswer{err: err}
	}
	if t.sid != "" {
		return probeAnswer{listed: slices.ContainsFunc(sessions, func(s acp.SessionInfo) bool { return s.SessionID == t.sid })}
	}
	var recent []Candidate
	for _, c := range candidates(sessions) {
		if c.UpdatedAt.After(since) {
			recent = append(recent, c)
		}
	}
	return probeAnswer{listed: len(recent) > 0, candidates: recent}
}

// sessions runs a temporary ACP process that only initializes and lists the
// sessions in cwd, page by page until one is session until or the pages
// run out (at most probePages). It never loads or resumes a session, so it
// is never a second writer. A profile connector is checked first. At most
// maxProbes run at once, server-wide.
func (a *actor) sessions(ctx context.Context, cwd, until string) ([]acp.SessionInfo, error) {
	select {
	case a.svc.probes <- struct{}{}:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	defer func() { <-a.svc.probes }()
	ctx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()
	tools, err := a.prepareACP(ctx)
	if err != nil {
		return nil, err
	}
	argv, _ := a.cmd.ACPArgv()
	sidecar, err := a.shell.StartSidecar(argv, a.cmd.ProtocolOnFD3(), tools.env)
	if err != nil {
		return nil, startFailure(a.cmd, err).err
	}
	link := &acpLink{sidecar: sidecar}
	link.conn = acp.NewConn(sidecar.Stdout(), sidecar.Stdin(), acp.Options{Logger: a.svc.log})
	defer func() { _ = link.stop() }()
	init, err := link.conn.Initialize(ctx, acp.Implementation{Name: "webmux", Title: "WebMux", Version: a.svc.version}, nil)
	if err == nil && init.AgentCapabilities.SessionCapabilities.List == nil {
		err = errMissingCapability
	}
	if err != nil {
		a.svc.log.Warn("hybrid probe failed", "id", a.id, "method", acp.MethodInitialize, "error", err)
		_ = link.stop()
		if missing := exitedNotFound(a.cmd, sidecar); missing != nil {
			return nil, missing.err
		}
		return nil, agentFailure(acp.MethodInitialize, err, sidecar).err
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
		if resp.NextCursor == "" || (until != "" && slices.ContainsFunc(resp.Sessions, func(s acp.SessionInfo) bool { return s.SessionID == until })) {
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
		if ident, ok := s.agents.Identify(signals(id)); ok && ident.AgentSessionID == sid {
			return true
		}
	}
	return false
}
