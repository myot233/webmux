package hybrid

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/rainchan/webmux/internal/acp"
	"github.com/rainchan/webmux/internal/agentprofile"
	"github.com/rainchan/webmux/internal/runtime"
	"github.com/rainchan/webmux/internal/session"
)

// maxCandidates bounds the agent sessions a SessionMissingError offers.
const maxCandidates = 20

// Candidate is an agent session offered when a record's agent session can
// no longer be found: another session its agent lists in the same
// directory.
type Candidate struct {
	AgentSessionID string
	Title          string
	UpdatedAt      time.Time
}

// SessionMissingError refuses a resume whose agent session is no longer
// listed in the recorded directory. Candidates are the directory's
// sessions, most recently updated first.
type SessionMissingError struct {
	Candidates []Candidate
}

func (e *SessionMissingError) Error() string { return ErrSessionMissing.Error() }

func (e *SessionMissingError) Unwrap() error { return ErrSessionMissing }

// ResumeInput resumes a closed or interrupted session on Side (empty: the
// recorded side). AgentSessionID replaces the recorded agent session with
// one a SessionMissingError offered. Env is the launch config's current
// environment, nil once the config was deleted; like at creation it only
// reaches the processes.
type ResumeInput struct {
	ClientRequestID string
	Side            Side
	AgentSessionID  string
	Env             map[string]string
}

// Records lists the hybrid session records, most recently updated first;
// recoverable keeps the closed and interrupted ones.
func (s *Service) Records(recoverable bool) ([]Record, error) {
	if err := s.records.unavailable(); err != nil {
		return nil, err
	}
	all := s.records.all()
	out := all[:0]
	for _, rec := range all {
		if !recoverable || rec.State != StateActive {
			out = append(out, rec)
		}
	}
	slices.SortStableFunc(out, func(a, b Record) int { return b.UpdatedAt.Compare(a.UpdatedAt) })
	return out, nil
}

// Record returns record id.
func (s *Service) Record(id string) (Record, error) {
	if err := s.records.unavailable(); err != nil {
		return Record{}, err
	}
	rec, ok := s.records.get(id)
	if !ok {
		return Record{}, ErrNotFound
	}
	return rec, nil
}

// Recoverable returns record id when it can be resumed or removed: closed
// or interrupted. An active session fails with ErrSwitching during a
// switch and ErrConflict otherwise.
func (s *Service) Recoverable(id string) (Record, error) {
	rec, err := s.Record(id)
	if err != nil || rec.State != StateActive {
		return rec, err
	}
	if shell, err := s.manager.Get(id); err == nil {
		if h := shell.Info().Hybrid; h != nil && h.Side == string(SideSwitching) {
			return Record{}, ErrSwitching
		}
	}
	return Record{}, fmt.Errorf("%w: 会话仍在运行，请先关闭", ErrConflict)
}

// claim marks record id as being resumed or removed; another resume or
// removal of it meanwhile fails with ErrSwitching.
func (s *Service) claim(id string) (func(), error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	switch {
	case s.closed:
		return nil, fmt.Errorf("%w: 服务已关闭", ErrUnavailable)
	case s.recovering[id]:
		return nil, ErrSwitching
	}
	s.recovering[id] = true
	return func() {
		s.mu.Lock()
		delete(s.recovering, id)
		s.mu.Unlock()
	}, nil
}

// RemoveRecord deletes closed or interrupted record id together with what
// is left of its session, an exited shell kept for its last output. The
// agent's own session store is left alone.
func (s *Service) RemoveRecord(ctx context.Context, id string) error {
	release, err := s.claim(id)
	if err != nil {
		return err
	}
	defer release()
	if _, err := s.Recoverable(id); err != nil {
		return err
	}
	if err := s.clear(ctx, id); err != nil {
		return err
	}
	if err := s.records.remove(id); err != nil {
		return err
	}
	s.log.Info("hybrid record removed", "id", id)
	return nil
}

// clear ends what is left of an interrupted session before its record is
// resumed or removed: the actor (its timers, probes and chat log) and the
// exited shell. A session still closing or holding a live process refuses
// with ErrConflict.
func (s *Service) clear(ctx context.Context, id string) error {
	if a, err := s.actor(id); err == nil {
		if _, err := call(ctx, a, a.retire); err != nil && !errors.Is(err, ErrNotFound) {
			return err
		}
		a.probes.Wait()
	}
	switch err := s.manager.Remove(id); {
	case err == nil, errors.Is(err, session.ErrNotFound):
		return nil
	case errors.Is(err, session.ErrRunning):
		return fmt.Errorf("%w: 会话的进程仍在运行", ErrConflict)
	default:
		return err
	}
}

// retire ends the actor of an interrupted session so its record can be
// resumed or removed. A session that runs, switches or closes refuses.
func (a *actor) retire() (struct{}, error) {
	switch {
	case a.stopped:
		return struct{}{}, nil
	case a.switching != nil:
		return struct{}{}, ErrSwitching
	case a.state == StateActive || a.closing || a.starting || a.opening || a.sleeping:
		return struct{}{}, fmt.Errorf("%w: 会话仍在运行或正在关闭", ErrConflict)
	}
	a.stopTimers()
	a.withdrawAll()
	if link := a.link; link != nil {
		a.link = nil
		_ = link.stop()
	}
	a.end()
	return struct{}{}, nil
}

// Resume brings closed or interrupted session id back under its ID on
// in.Side (default: the recorded side) and returns its shell once that side
// runs. The agent session (the recorded one, or in.AgentSessionID) must
// still be listed in the recorded directory, else a *SessionMissingError
// offers the others, and open in no other session. Then the terminal UI
// resumes it, or a new ACP process loads it and gets the recorded option
// values back (D25); without an agent session the side starts a new
// conversation. The command is the record's snapshot; the record becomes
// active and a queue the interruption lost shows as ChatState.QueueLost.
// A failed resume leaves the record as it was. A retry with the same client
// request ID gets the first outcome.
func (s *Service) Resume(ctx context.Context, id string, in ResumeInput) (*session.Session, error) {
	if in.Side != "" && in.Side != SideTUI && in.Side != SideACP {
		return nil, fmt.Errorf("%w: side 只能是 tui 或 acp", ErrInvalid)
	}
	if err := s.records.unavailable(); err != nil {
		return nil, err
	}
	return once(ctx, &s.resumes, in.ClientRequestID, "resume "+id, func(context.Context) (*session.Session, error) {
		return s.resume(id, in)
	})
}

func (s *Service) resume(id string, in ResumeInput) (*session.Session, error) {
	release, err := s.claim(id)
	if err != nil {
		return nil, err
	}
	defer release()
	rec, err := s.Recoverable(id)
	if err != nil {
		return nil, err
	}
	side := in.Side
	if side == "" {
		side = rec.Side
	}
	cmd, err := agentprofile.Parse(rec.Cmd)
	if err != nil {
		return nil, fmt.Errorf("%w: 记录中的命令不能用于混合会话: %v", ErrInvalid, err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), closeTimeout)
	err = s.clear(ctx, id)
	cancel()
	if err != nil {
		return nil, err
	}
	sid := rec.AgentSessionID
	if in.AgentSessionID != "" {
		sid = in.AgentSessionID
	}
	a := newActor(s, cmd, rec.Cwd, rec.ConfigID, side)
	a.agentSessionID = sid
	a.chat.QueueLost = rec.QueuedCount
	if rec.PermissionMode == PermissionAsk {
		a.chat.PermissionMode = PermissionAsk
	}
	shell, err := s.manager.CreateShell(session.ShellInput{
		CreateInput: session.CreateInput{Cwd: rec.Cwd, Cmd: cmd.TUIArgv(), Name: rec.Name, Env: in.Env, ConfigID: rec.ConfigID},
		ID:          id,
		Overlay:     a.overlay(),
	}, a)
	if err != nil {
		return nil, shellError(err)
	}
	a.id, a.shell = id, shell
	s.mu.Lock()
	s.actors[id] = a
	s.mu.Unlock()
	go a.run()
	go a.watchShell()
	go a.observeLoop()
	started := make(chan error, 1)
	a.post(func() { a.resume(started, sid) })
	if err := awaitStarted(a, started); err != nil {
		return nil, err
	}
	return shell, nil
}

// resumeResult is a finished resume start.
type resumeResult struct {
	link     *acpLink
	init     acp.InitializeResponse
	config   []acp.ConfigOption
	warnings []string
	// loaded: the ACP process loaded the existing agent session.
	loaded  bool
	pid     int
	process *runtime.Identity
	// err is user-facing and may carry the agent's stderr: never log it.
	err error
}

// resume starts the resumed session's side on agent session sid in a
// subtask and reports the outcome on started once the actor applied it.
// When the actor ended first, the subtask stops the ACP process it started
// and reports errStopped.
func (a *actor) resume(started chan<- error, sid string) {
	a.starting = true
	a.epoch++
	epoch, side := a.epoch, a.side
	go func() {
		res := a.resumeSide(epoch, side, sid)
		if !a.sync(func() { a.resumed(started, res) }) {
			if res.link != nil {
				_ = res.link.stop()
			}
			started <- errStopped
		}
	}()
}

// resumeSide verifies agent session sid, when there is one, and starts
// side on it: the terminal UI's resume command, or an ACP process that
// loads it. It runs in a subtask; a failure leaves no process running.
func (a *actor) resumeSide(epoch int, side Side, sid string) resumeResult {
	if sid != "" {
		if err := a.verify(sid); err != nil {
			return resumeResult{err: err}
		}
	}
	if side == SideTUI {
		argv := a.cmd.TUIArgv()
		if sid != "" {
			argv, _ = a.cmd.ResumeArgv(sid)
		}
		if err := a.shell.StartRun(argv); err != nil {
			return resumeResult{err: fmt.Errorf("%w: 启动终端失败: %v", ErrInvalid, err)}
		}
		pid := a.shell.Identity().PID
		return resumeResult{pid: pid, process: processIdentity(pid)}
	}
	if sid == "" {
		r := a.spawnACP(epoch)
		return resumeResult{link: r.link, init: r.init, config: r.session.ConfigOptions, pid: r.pid, process: r.process, err: r.err}
	}
	r := a.openACP(epoch, sid, a.cwd, false, nil)
	if r.fail != nil {
		return resumeResult{err: r.fail.err}
	}
	pid := r.link.sidecar.PID()
	return resumeResult{link: r.link, init: r.init, config: r.config, warnings: r.warnings, loaded: true, pid: pid, process: processIdentity(pid)}
}

// verify checks before a resume that agent session sid is still listed in
// the recorded directory (a *SessionMissingError offering the others when
// not) and open in no other session.
func (a *actor) verify(sid string) error {
	listed, err := a.sessions(a.probeCtx, a.cwd, sid)
	if err != nil {
		return err
	}
	if !slices.ContainsFunc(listed, func(s acp.SessionInfo) bool { return s.SessionID == sid }) {
		return &SessionMissingError{Candidates: candidates(listed)}
	}
	if a.svc.occupied(a.id, sid) {
		return fmt.Errorf("%w: %s", ErrConflict, reasonText(ReasonOccupied))
	}
	return nil
}

// candidates orders listed sessions most recently updated first, at most
// maxCandidates. An update time the agent did not report sorts last.
func candidates(listed []acp.SessionInfo) []Candidate {
	out := make([]Candidate, 0, len(listed))
	for _, s := range listed {
		at, _ := time.Parse(time.RFC3339Nano, s.UpdatedAt)
		out = append(out, Candidate{AgentSessionID: s.SessionID, Title: s.Title, UpdatedAt: at.UTC()})
	}
	slices.SortStableFunc(out, func(a, b Candidate) int { return b.UpdatedAt.Compare(a.UpdatedAt) })
	return out[:min(len(out), maxCandidates)]
}

// resumed applies a finished resume: the session is active again on its
// side and its record says so, or the shell is dropped and the record
// kept as it was. A terminal UI that already exited left the session
// interrupted (D23).
func (a *actor) resumed(started chan<- error, res resumeResult) {
	a.starting = false
	if res.err != nil {
		a.end()
		if err := a.svc.manager.Remove(a.id); err != nil {
			a.svc.log.Warn("hybrid shell removal failed", "id", a.id, "error", err)
		}
		started <- res.err
		return
	}
	a.dirty = true
	a.touch()
	if res.link != nil {
		a.installLink(res.link, res.init, res.config, res.warnings)
		if res.loaded {
			a.emit(Event{Kind: EventMarker, Marker: &Marker{Kind: MarkerLoaded, Side: SideACP}})
		}
	}
	side, sid, active, pid, process := a.side, a.agentSessionID, a.state == StateActive, res.pid, res.process
	if side == SideTUI {
		// resumeSide starts the resume command when there is an ID.
		a.resumedSID = sid
	}
	a.record(true, func(r *Record) {
		r.Side = side
		r.SetAgentSessionID(sid)
		if active {
			r.State, r.Activity, r.InterruptedAt, r.QueuedCount = StateActive, "", nil, 0
			r.PID, r.Process = pid, process
		}
	})
	a.svc.log.Info("hybrid session resumed", "id", a.id, "side", string(side), "agent", a.cmd.Profile.Name)
	// The caller answers with the shell's info: publish the side first.
	a.flush()
	started <- nil
	a.checkLink()
	a.afterTransition()
}
