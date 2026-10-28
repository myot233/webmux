package hybrid

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"time"

	"github.com/rainchan/webmux/internal/acp"
	"github.com/rainchan/webmux/internal/session"
)

// goneTimeout bounds waiting for a stopped process tree to be observed gone
// after Stop already killed it.
const goneTimeout = 5 * time.Second

// errStopped: the actor ended while a subtask was opening a connection.
var errStopped = fmt.Errorf("%w: 服务正在关闭", ErrUnavailable)

// acpLink is one ACP agent process and its connection.
type acpLink struct {
	epoch     int
	sidecar   *session.Sidecar
	conn      *acp.Conn
	sessionID string
	caps      acp.SessionCapabilities
	prompt    acp.PromptCapabilities
}

// stop closes the connection and ends the process, and reports whether its
// process tree was observed gone: the next writer may only start then.
func (l *acpLink) stop() error {
	ctx, cancel := context.WithTimeout(context.Background(), closeTimeout)
	defer cancel()
	return l.stopWithin(ctx)
}

// stopWithin is stop that kills the process tree when ctx ends first, then
// waits at most goneTimeout for it to go.
func (l *acpLink) stopWithin(ctx context.Context) error {
	_ = l.conn.Close()
	if err := l.sidecar.Stop(ctx); err == nil {
		return nil
	}
	gone := time.NewTimer(goneTimeout)
	defer gone.Stop()
	select {
	case <-l.sidecar.Done():
		return nil
	case <-gone.C:
		return errors.New("agent 进程未能结束")
	}
}

// release closes the agent session when the agent supports session/close,
// so it flushes and lets go of it, then stops the process.
func (l *acpLink) release() error {
	if l.caps.Close != nil && l.sessionID != "" {
		ctx, cancel := context.WithTimeout(context.Background(), releaseTimeout)
		_ = l.conn.CloseSession(ctx, l.sessionID)
		cancel()
	}
	return l.stop()
}

// failure is a failed step: err for the requesting user (it may carry the
// agent's stderr, never log it), public for every device.
type failure struct {
	err    error
	public string
}

// agentFailure describes a failed ACP call: the requester's error carries
// the agent's stderr tail, the public text does not.
func agentFailure(method string, err error, sidecar *session.Sidecar) failure {
	var rpc *acp.RPCError
	var detail string
	switch {
	case errors.As(err, &rpc):
		detail = fmt.Sprintf("错误码 %d: %s", rpc.Code, limitText(rpc.Message, errorTextBytes))
	case errors.Is(err, acp.ErrUnsupportedVersion):
		detail = "协议版本不受支持"
	case errors.Is(err, acp.ErrDisconnected), errors.Is(err, acp.ErrClosed):
		detail = "agent 已退出"
		if code := sidecar.ExitCode(); code >= 0 {
			detail += "（退出码 " + strconv.Itoa(code) + "）"
		}
	default:
		detail = err.Error()
	}
	public := method + ": " + detail
	full := detail
	if tail := stderrTail(sidecar.Stderr()); tail != "" {
		full += "\n" + tail
	}
	return failure{err: fmt.Errorf("%w: %s: %s", ErrAgent, method, full), public: public}
}

// connect starts the ACP process under epoch and initializes it. The agent
// must support session/load and session/list. A failure stops the process
// before returning.
func (a *actor) connect(epoch int) (*acpLink, acp.InitializeResponse, *failure) {
	argv, _ := a.cmd.ACPArgv()
	sidecar, err := a.shell.StartSidecar(argv, a.cmd.ProtocolOnFD3())
	if err != nil {
		a.svc.log.Warn("hybrid acp start failed", "id", a.id, "error", err)
		return nil, acp.InitializeResponse{}, &failure{err: fmt.Errorf("%w: %v", ErrAgent, err), public: "启动 agent 失败"}
	}
	link := &acpLink{epoch: epoch, sidecar: sidecar}
	link.conn = acp.NewConn(sidecar.Stdout(), sidecar.Stdin(), acp.Options{Handler: &handler{a: a, epoch: epoch}, Logger: a.svc.log})
	go func() {
		<-link.conn.Done()
		a.post(func() { a.linkLost(link) })
	}()
	init, err := link.conn.Initialize(context.Background(), acp.Implementation{Name: "webmux", Title: "WebMux", Version: a.svc.version}, nil)
	if err == nil {
		caps := init.AgentCapabilities
		if !caps.LoadSession || caps.SessionCapabilities.List == nil {
			err = errMissingCapability
		}
	}
	if err != nil {
		return nil, init, a.callFailed(link, acp.MethodInitialize, err)
	}
	link.caps = init.AgentCapabilities.SessionCapabilities
	link.prompt = init.AgentCapabilities.PromptCapabilities
	return link, init, nil
}

// callFailed logs a failed ACP call by method, stops the process and
// describes the failure.
func (a *actor) callFailed(link *acpLink, method string, err error) *failure {
	a.svc.log.Warn("hybrid acp call failed", "id", a.id, "method", method, "error", err)
	_ = link.stop()
	f := agentFailure(method, err, link.sidecar)
	return &f
}

// openResult is an ACP connection opened on an existing agent session.
type openResult struct {
	link    *acpLink
	init    acp.InitializeResponse
	config  []acp.ConfigOption
	resumed bool
	// warnings are reapplied options the agent refused, for a notice.
	warnings []string
	fail     *failure
}

// openACP starts the ACP process under epoch and opens agent session sid
// in cwd: with session/resume when allowed, the profile permits it and the
// agent declared it (no replay), else with session/load after beforeLoad
// (which starts a new log generation; false means the actor ended). Options
// the profile lists as not kept across loads get their recorded values
// back (D25). It runs in a subtask; a failure stops the process.
func (a *actor) openACP(epoch int, sid, cwd string, resume bool, beforeLoad func() bool) openResult {
	want := a.reapplyValues()
	link, init, fail := a.connect(epoch)
	if fail != nil {
		return openResult{fail: fail}
	}
	res := openResult{link: link, init: init}
	ctx := context.Background()
	method := acp.MethodSessionLoad
	var err error
	if resume && a.cmd.Profile.Resume && link.caps.Resume != nil {
		method = acp.MethodSessionResume
		var r acp.ResumeSessionResponse
		r, err = link.conn.ResumeSession(ctx, acp.ResumeSessionRequest{SessionID: sid, Cwd: cwd})
		res.config, res.resumed = r.ConfigOptions, true
	} else {
		if beforeLoad != nil && !beforeLoad() {
			_ = link.stop()
			return openResult{fail: &failure{err: errStopped, public: errStopped.Error()}}
		}
		var r acp.LoadSessionResponse
		r, err = link.conn.LoadSession(ctx, acp.LoadSessionRequest{SessionID: sid, Cwd: cwd})
		res.config = r.ConfigOptions
	}
	if err != nil {
		return openResult{fail: a.callFailed(link, method, err)}
	}
	link.sessionID = sid
	res.config, res.warnings = a.reapply(link, want, res.config)
	return res
}

// reapplyValues reads the recorded values of the options the profile says
// a load does not keep. It is read before the new connection reports any
// option, since reported options replace the recorded values.
func (a *actor) reapplyValues() map[string]string {
	rec, ok := a.svc.records.get(a.id)
	want := map[string]string{}
	if !ok {
		return want
	}
	for _, id := range a.cmd.Profile.ReapplyOnLoad {
		if v, ok := rec.Config[id]; ok && v != "" {
			want[id] = v
		}
	}
	return want
}

// reapply sets each wanted option the agent answered with another value
// (or not at all) and returns the resulting options; refusals become
// warnings, not failures.
func (a *actor) reapply(link *acpLink, want map[string]string, options []acp.ConfigOption) ([]acp.ConfigOption, []string) {
	var warnings []string
	for _, id := range a.cmd.Profile.ReapplyOnLoad {
		value, ok := want[id]
		if !ok {
			continue
		}
		i := slices.IndexFunc(options, func(o acp.ConfigOption) bool { return o.ID == id })
		var v acp.ConfigValue
		switch {
		case i >= 0 && options[i].Type == acp.ConfigBoolean:
			b, err := strconv.ParseBool(value)
			if err != nil || (options[i].Current.Bool != nil && *options[i].Current.Bool == b) {
				continue
			}
			v.Bool = &b
		case i >= 0 && options[i].Current.ID == value:
			continue
		default:
			v.ID = value
		}
		ctx, cancel := context.WithTimeout(context.Background(), acp.RequestTimeout)
		resp, err := link.conn.SetConfigOption(ctx, acp.SetConfigOptionRequest{SessionID: link.sessionID, ConfigID: id, Value: v})
		cancel()
		if err != nil {
			a.svc.log.Warn("hybrid config reapply failed", "id", a.id, "option", id, "error", err)
			warnings = append(warnings, id)
			continue
		}
		switch {
		case resp.ConfigOptions != nil:
			options = resp.ConfigOptions
		case i >= 0:
			options = slices.Clone(options)
			options[i].Current = v
		}
	}
	return options, warnings
}

// installLink makes an opened connection the session's: agent version,
// steering and the agent's options.
func (a *actor) installLink(link *acpLink, init acp.InitializeResponse, config []acp.ConfigOption, warnings []string) {
	a.link = link
	a.agentSessionID = link.sessionID
	if info := init.AgentInfo; info != nil {
		a.chat.AgentVersion = info.Version
		a.chat.VerifiedBelow = ""
		if a.cmd.Profile.BelowVerified(info.Version) {
			a.chat.VerifiedBelow = a.cmd.Profile.VerifiedVersion
		}
	}
	a.chat.Steering = a.cmd.Profile.Steering && steeringSupported(init.Meta)
	a.chat.PromptCapabilities = PromptCapabilities{
		Image:           link.prompt.Image,
		EmbeddedContext: link.prompt.EmbeddedContext,
	}
	a.dirty = true
	if config != nil {
		a.emit(Event{Kind: EventConfig, Config: configOptions(config)})
	}
	for _, id := range warnings {
		a.emit(Event{Kind: EventNotice, Notice: &Notice{Level: acp.NoticeWarning, Text: "未能恢复配置项 " + id}})
	}
}

// restartLog begins a new log generation for a session/load replay.
func (a *actor) restartLog() {
	a.log.restart()
	a.tr = translator{}
	a.chat.Plan, a.chat.Usage = nil, nil
	a.preview = preview{}
	a.dirty = true
}

// checkLink handles a connection that died before the actor installed it.
func (a *actor) checkLink() {
	if link := a.link; link != nil {
		select {
		case <-link.conn.Done():
			a.linkLost(link)
		default:
		}
	}
}

// linkLost handles the end of the current ACP connection outside a
// switch, close or sleep: the agent exited or broke the protocol. The
// running turn ends cancelled and the session reconnects once (D24): with
// session/resume when possible, else session/load in a new generation. A
// second loss before a turn completes normally, or a failed reconnect,
// interrupts the session.
func (a *actor) linkLost(link *acpLink) {
	if a.link != link || a.stopped {
		return
	}
	a.link = nil
	a.chat.PromptCapabilities = PromptCapabilities{}
	a.dirty = true
	a.svc.log.Warn("hybrid acp connection ended", "id", a.id, "error", link.conn.Err())
	a.withdrawAll()
	if a.turn != nil {
		a.turnEnded(a.turn, acp.PromptResponse{}, acp.ErrDisconnected)
	}
	if a.state != StateActive || a.closing {
		go func() { _ = link.stop() }()
		return
	}
	if a.reconnects > 0 {
		a.emit(Event{Kind: EventNotice, Notice: &Notice{Level: acp.NoticeError, Text: "与 agent 的连接再次断开"}})
		a.interrupt("turn", false)
		a.stopLost(link)
		return
	}
	a.reconnects++
	a.emit(Event{Kind: EventNotice, Notice: &Notice{Level: acp.NoticeWarning, Text: "与 agent 的连接已断开，正在重新连接"}})
	a.reopen(link, true)
}

// reopen opens the agent session again on a new process, after old (if
// any) is gone: a reconnect after a crash or a wake from sleep. Prompts
// queue meanwhile.
func (a *actor) reopen(old *acpLink, reconnect bool) {
	a.opening = true
	a.epoch++
	epoch, sid, cwd := a.epoch, a.agentSessionID, a.agentCwd
	a.dirty = true
	go func() {
		var res openResult
		oldGone := true
		if old != nil && old.stop() != nil {
			oldGone = false
			res.fail = &failure{err: fmt.Errorf("%w: 原 agent 进程未能结束", ErrAgent), public: "原 agent 进程未能结束"}
		} else {
			res = a.openACP(epoch, sid, cwd, true, func() bool { return a.sync(a.restartLog) })
		}
		if !a.sync(func() { a.reopened(res, reconnect, oldGone) }) && res.link != nil {
			_ = res.link.stop()
		}
	}()
}

// reopened applies a finished reconnect or wake. A failed reconnect
// interrupts the session, keeping the old process's identity unless it was
// confirmed gone (oldGone); a failed wake leaves it dormant with its queue
// paused, and the next command tries again.
func (a *actor) reopened(res openResult, reconnect, oldGone bool) {
	a.opening = false
	a.dirty = true
	if res.fail != nil {
		a.emit(Event{Kind: EventNotice, Notice: &Notice{Level: acp.NoticeError, Text: res.fail.public}})
		if reconnect {
			a.interrupt("idle", oldGone)
		} else {
			a.setQueue(a.chat.Queue.Items, true)
		}
		a.afterTransition()
		return
	}
	a.installLink(res.link, res.init, res.config, res.warnings)
	a.chat.Dormant = false
	kind := MarkerLoaded
	switch {
	case reconnect:
		kind = MarkerReconnected
	case res.resumed:
		kind = MarkerResumed
	}
	a.emit(Event{Kind: EventMarker, Marker: &Marker{Kind: kind, Side: SideACP}})
	pid := res.link.sidecar.PID()
	process := processIdentity(pid)
	a.record(true, func(r *Record) { r.PID, r.Process = pid, process })
	a.svc.log.Info("hybrid acp reopened", "id", a.id, "reconnect", reconnect, "resumed", res.resumed)
	a.touch()
	a.checkLink()
	a.drain()
	a.afterTransition()
}

// afterTransition runs a close that waited for a start, wake, reconnect or
// sleep to finish.
func (a *actor) afterTransition() {
	if a.closeAfterStart && !a.starting && !a.opening && !a.sleeping {
		a.closeAfterStart = false
		a.beginClose()
	}
}

// sync runs fn on the actor and waits for it; false if the actor ended
// before running it.
func (a *actor) sync(fn func()) bool {
	done := make(chan struct{})
	if !a.post(func() { fn(); close(done) }) {
		return false
	}
	select {
	case <-done:
		return true
	case <-a.quit:
		select {
		case <-done:
			return true
		default:
			return false
		}
	}
}
