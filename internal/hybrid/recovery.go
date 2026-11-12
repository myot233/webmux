package hybrid

import (
	"context"
	"errors"
	"fmt"
	"github.com/rainchan/webmux/internal/acp"
	"github.com/rainchan/webmux/internal/runtime"
	"github.com/rainchan/webmux/internal/session"
	"slices"
	"time"
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

// SessionAmbiguousError refuses a switch to the chat when several agent
// sessions were updated since the terminal run started and nothing tells
// which one it has open. Candidates are those sessions, most recently
// updated first; the switch can be asked again with one of them.
type SessionAmbiguousError struct {
	Candidates []Candidate
}

func (e *SessionAmbiguousError) Error() string { return ErrSessionAmbiguous.Error() }

func (e *SessionAmbiguousError) Unwrap() error { return ErrSessionAmbiguous }

// ResumeInput resumes a closed or interrupted session on Side (empty: the
// recorded side). AgentSessionID replaces the recorded agent session with
// one a SessionMissingError offered. Env is the launch config's current
// environment, nil once the config was deleted; like at creation it only
// reaches the processes.
type ResumeInput struct {
	ContextExecutionID  string
	RequireNativeResume bool
	ClientRequestID     string
	Side                Side
	AgentSessionID      string
	Env                 map[string]string
	Attention           *session.AttentionPolicy
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
// switch and ErrNotArchived otherwise.
func (s *Service) Recoverable(id string) (Record, error) {
	rec, err := s.Record(id)
	if err != nil || rec.State != StateActive {
		return rec, err
	}
	if shell, err := s.manager.Get(id); err == nil {
		if h := shell.Info().Hybrid; h != nil && h.To != "" {
			return Record{}, ErrSwitching
		}
	}
	return Record{}, ErrNotArchived
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
	if err := s.beginOperation(); err != nil {
		return nil, err
	}
	defer s.starting.Done()
	if in.Side != "" && in.Side != SideTUI && in.Side != SideACP {
		return nil, fmt.Errorf("%w: side 只能是 tui 或 acp", ErrInvalid)
	}
	if err := s.Unavailable(); err != nil {
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
	if s.conversations != nil {
		e, leased, leaseErr := s.conversations.RuntimeDispatch(context.Background(), id)
		if leaseErr != nil {
			return nil, leaseErr
		}
		if leased {
			// An unresolved historical call may load only its exact native
			// thread for reconciliation, never the replacement runtime writer.
			if e.Status != "failed" && e.NativeThreadID != "" && e.NativeThreadID != rec.AgentSessionID {
				return nil, ErrSwitching
			}
			if e.RuntimeID != id || (e.Status != "failed" && side != SideACP) || (in.AgentSessionID != "" && in.AgentSessionID != rec.AgentSessionID) || (in.ContextExecutionID != "" && in.ContextExecutionID != e.ID && e.Status != "failed") {
				return nil, ErrSwitching
			}
			if in.ContextExecutionID == "" {
				in.ContextExecutionID = e.ID
			}
		}
		binding, err := s.conversations.Binding(context.Background(), id)
		if err != nil {
			return nil, err
		}
		releaseWriter, err := s.ClaimConversation(binding.ConversationID, in.ContextExecutionID)
		if err != nil {
			return nil, err
		}
		defer releaseWriter()
		c, err := s.conversations.Conversation(context.Background(), binding.ConversationID)
		if err != nil {
			return nil, err
		}
		if in.ContextExecutionID == "" && c.ActiveProviderThreadID != binding.ProviderThreadID {
			return nil, ErrSwitching
		}
		ids, err := s.conversations.RuntimeIDs(context.Background(), c.ID)
		if err != nil {
			return nil, err
		}
		for _, other := range ids {
			if other == id {
				continue
			}
			if record, err := s.Record(other); err == nil && (record.State == StateActive || record.PID != 0) {
				return nil, ErrSwitching
			}
		}
	}
	if rec.Released != nil {
		return nil, fmt.Errorf("%w: 工作树已释放空间，请先重建工作树再恢复", ErrConflict)
	}
	cmd, err := s.agents.Parse(rec.Cmd)
	if err != nil {
		return nil, fmt.Errorf("%w: 记录中的命令不能用于混合会话: %w", ErrInvalid, err)
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
	a := newActor(s, cmd, rec.Cwd, rec.ConfigID, side, in.Attention)
	a.agentSessionID = sid
	a.contextExecutionID = in.ContextExecutionID
	a.requireNativeResume = in.RequireNativeResume
	a.title, a.titleSession = rec.Title, sid
	a.recordTitle = rec.Title
	// recordWorkspace starts nil: the fresh shell has not reported an
	// association yet, so its unset workspace must not clear the saved one.
	a.chat.QueueLost = rec.QueuedCount
	if rec.PermissionMode == PermissionAsk {
		a.chat.PermissionMode = PermissionAsk
	}
	shell, err := s.manager.CreateShell(session.ShellInput{
		CreateInput: session.CreateInput{
			Cwd: rec.Cwd, Cmd: cmd.TUIArgv(), Name: rec.Name, Env: in.Env, ConfigID: rec.ConfigID,
			Attention: in.Attention,
		},
		ID:      id,
		Overlay: a.overlay(),
	}, a)
	if err != nil {
		return nil, shellError(err)
	}
	a.id, a.shell = id, shell
	if err := a.activityThread(); err != nil {
		_ = s.manager.Remove(id)
		return nil, err
	}
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		_ = s.manager.Remove(id)
		return nil, fmt.Errorf("%w: 服务已关闭", ErrUnavailable)
	}
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
	link   *acpLink
	init   acp.InitializeResponse
	config []acp.ConfigOption
	notes  []string
	// loaded: the ACP process loaded the existing agent session.
	loaded  bool
	resumed bool
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
// loads it. The terminal side does not need the profile's connector: a
// missing one only skips the check that the session is still listed. It
// runs in a subtask; a failure leaves no process running.
func (a *actor) resumeSide(epoch int, side Side, sid string) resumeResult {
	if sid != "" {
		err := a.verify(sid)
		if side == SideTUI && errors.Is(err, ErrConnectorMissing) {
			err = nil
			if a.svc.occupied(a.id, sid) {
				err = fmt.Errorf("%w: %s", ErrConflict, reasonText(ReasonOccupied))
			}
		}
		if err != nil {
			return resumeResult{err: err}
		}
	}
	if side == SideTUI {
		argv := a.cmd.TUIArgv()
		if sid != "" {
			if err := a.prepareTUIResume(sid); err != nil {
				a.svc.log.Warn("hybrid terminal resume preparation failed", "id", a.id, "error", err)
				return resumeResult{err: fmt.Errorf("%w: 准备终端会话失败", ErrAgent)}
			}
			argv, _ = a.cmd.ResumeArgv(sid)
		}
		if err := a.shell.StartRun(argv); err != nil {
			return resumeResult{err: fmt.Errorf("%w: 启动终端失败: %w", ErrInvalid, err)}
		}
		pid := a.shell.Identity().PID
		return resumeResult{pid: pid, process: processIdentity(pid)}
	}
	if sid == "" {
		r := a.spawnACP(epoch)
		return resumeResult{link: r.link, init: r.init, config: r.config, notes: r.notes, pid: r.pid, process: r.process, err: r.err}
	}
	r := a.openACP(epoch, sid, a.cwd, a.requireNativeResume, nil)
	if r.fail != nil {
		return resumeResult{err: r.fail.err}
	}
	pid := r.link.sidecar.PID()
	return resumeResult{link: r.link, init: r.init, config: r.config, notes: r.notes, loaded: !r.resumed, resumed: r.resumed, pid: pid, process: processIdentity(pid)}
}

// verify checks before a resume that agent session sid is still listed in
// the recorded directory (a *SessionMissingError offering the others when
// not) and open in no other session.
func (a *actor) verify(sid string) error {
	if a.externalWriterNow(sid) {
		return ErrExternalWriter
	}
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
	a.nativeContinue = res.resumed
	if res.link != nil {
		a.installLink(res.link, res.init, res.config, res.notes)
		if res.loaded {
			a.emit(Event{Kind: EventMarker, Marker: &Marker{Kind: MarkerLoaded, Side: SideACP}})
		}
	}
	side, sid, active, pid, process := a.side, a.agentSessionID, a.state == StateActive, res.pid, res.process
	if side == SideTUI {
		a.recordTUIGap()
		// resumeSide starts the resume command when there is an ID.
		a.resumedSID = sid
		a.runStart = time.Now()
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

// MarkReleased records that the worktree of closed or interrupted records
// ids was released as rel, writing the file at once so a restart sees the
// mark before the directory is deleted.
func (s *Service) MarkReleased(_ context.Context, ids []string, rel Release) error {
	if err := s.records.markReleased(ids, rel); err != nil {
		return err
	}
	s.log.Info("hybrid records released", "release", rel.ID, "records", len(ids))
	return nil
}

// ClearReleased removes release releaseID from every record carrying it
// and returns their IDs; none is not an error.
func (s *Service) ClearReleased(_ context.Context, releaseID string) ([]string, error) {
	return s.records.clearReleased(releaseID)
}

// AgentLabel is the display title and icon of agent name, empty for an
// agent this build does not know.
func (s *Service) AgentLabel(name string) (title, icon string) {
	if a, ok := s.agents.Lookup(name); ok {
		p := a.Profile()
		return p.Title, p.Icon
	}
	return "", ""
}

func (s *Service) claimRecordConversation(ctx context.Context, id string) (func(), error) {
	if s.conversations == nil {
		return func() {}, nil
	}
	b, err := s.conversations.Binding(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("%w: conversation identity unavailable", ErrUnavailable)
	}
	return s.ClaimConversation(b.ConversationID, "")
}

func (s *Service) removeRecord(ctx context.Context, id string) error {
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
	if s.conversations != nil {
		if err := s.conversations.MarkRuntimeForgetting(ctx, id); err != nil {
			return fmt.Errorf("%w: conversation cleanup unavailable", ErrUnavailable)
		}
	}
	if err := s.records.remove(id); err != nil {
		return err
	}
	if err := s.forgetConversation(context.WithoutCancel(ctx), id); err != nil {
		return err
	}
	s.log.Info("hybrid record removed", "id", id)
	return nil
}
