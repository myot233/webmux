// Package hybrid runs hybrid sessions: one agent conversation that moves
// between the agent's terminal UI (a PTY run of a session shell) and an
// ACP chat (a sidecar process of the same shell), under one WebMux session
// ID. Each session is owned by an actor goroutine that drives the shell
// (session.Driver), translates ACP updates into normalized chat events,
// and keeps the session's record in hybrid-sessions.json.
//
// Content safety: nothing here logs prompts, messages, tool output,
// permission options, form content or agent stderr. Agent stderr only
// reaches the user who requested a failed start, in the returned error.
package hybrid

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"
	"sync"
	"time"

	"github.com/rainchan/webmux/internal/agentprofile"
	"github.com/rainchan/webmux/internal/session"
	"github.com/rainchan/webmux/internal/settings"
)

const (
	// defaultScrollbackBytes matches the session default when no limit is
	// configured.
	defaultScrollbackBytes = 2 << 20
	// shutdownTimeout bounds stopping the ACP processes in Close: actors
	// stop them in parallel and kill those still running after it.
	shutdownTimeout = 10 * time.Second
	// shutdownJoin bounds Close as a whole: the kill, the wait for killed
	// processes to go (goneTimeout), and a margin for the actors to end.
	shutdownJoin = shutdownTimeout + goneTimeout + time.Second
)

// Options configure a Service. Only cmd/ builds them.
type Options struct {
	// StateDir holds hybrid-sessions.json.
	StateDir string
	Manager  *session.Manager
	// ScrollbackBytes bounds each session's chat event log, like a
	// terminal's scrollback (config scrollback_bytes).
	ScrollbackBytes int
	// Version is the WebMux version sent as ACP clientInfo.
	Version string
	// Logger receives lifecycle events, method names and error codes only.
	Logger *slog.Logger
	// Settings supplies chatIdleSleepMinutes; changes apply at once. Nil
	// or unavailable settings use the default of 30 minutes.
	Settings *settings.Service
	// Uploads validates that path is one of session id's uploaded files and
	// returns it cleaned (artifacts.Service.UploadPath). Nil refuses every
	// attachment.
	Uploads func(id, path string) (string, error)
}

// timing holds the service's waits; tests shorten them.
type timing struct {
	// interrupt is how long a switch waits for the terminal UI to go idle
	// after the interrupt key before it asks the user (D21).
	interrupt time.Duration
	// probeDelay debounces the eligibility probe after the terminal's
	// agent session or cwd changes.
	probeDelay time.Duration
	// observe is how often a terminal side's identity is read.
	observe time.Duration
	// minute is the unit of chatIdleSleepMinutes.
	minute time.Duration
}

var defaultTiming = timing{interrupt: 10 * time.Second, probeDelay: time.Second, observe: 500 * time.Millisecond, minute: time.Minute}

// Service owns the hybrid sessions of one server.
type Service struct {
	manager  *session.Manager
	records  *records
	capacity int
	version  string
	log      *slog.Logger
	timing   timing
	uploads  func(id, path string) (string, error)
	// probes holds a token per running eligibility probe (maxProbes).
	probes chan struct{}
	// settingsDone closes when the settings watcher ends (nil without one).
	settingsDone chan struct{}
	// resumes dedupes resume requests by client request ID.
	resumes dedupe

	mu          sync.Mutex
	actors      map[string]*actor
	closed      bool
	idleMinutes int
	settingsSub *settings.Subscription
	// recovering holds the record IDs a resume or removal is working on.
	recovering map[string]bool
}

// New opens the record store, marks sessions left active by an earlier
// server interrupted (ending their leftover processes), and reserves the
// recorded IDs in the manager. A store that cannot be opened leaves the
// service unavailable for hybrid creation; ordinary sessions keep working.
func New(o Options) *Service {
	log := o.Logger
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	capacity := o.ScrollbackBytes
	if capacity <= 0 {
		capacity = defaultScrollbackBytes
	}
	if o.Version == "" {
		o.Version = "dev"
	}
	s := &Service{
		manager: o.Manager, records: openRecords(o.StateDir), capacity: capacity, version: o.Version, log: log,
		uploads: o.Uploads,
		timing:  defaultTiming, probes: make(chan struct{}, maxProbes), actors: make(map[string]*actor),
		recovering:  make(map[string]bool),
		idleMinutes: settings.DefaultChatIdleSleepMinutes,
	}
	if err := s.records.recoverAfterRestart(time.Now()); err != nil {
		log.Warn("hybrid sessions unavailable", "error", err)
	}
	o.Manager.SetIDReserver(s.records.reserved)
	if o.Settings != nil {
		s.settingsDone = make(chan struct{})
		go s.watchSettings(o.Settings)
	}
	return s
}

// Unavailable reports why hybrid sessions cannot be created, or nil.
func (s *Service) Unavailable() error { return s.records.unavailable() }

// uploadCheck validates attachment paths against session id's uploads.
func (s *Service) uploadCheck(id string) func(string) (string, error) {
	return func(path string) (string, error) {
		if s.uploads == nil {
			return "", errors.New("上传不可用")
		}
		return s.uploads(id, path)
	}
}

// Close stops every actor and its ACP process without changing records,
// so a restart finds the sessions active and marks them interrupted, then
// releases the store once the actors ended (at most shutdownJoin).
// Terminal runs are left to Manager.Shutdown.
func (s *Service) Close() {
	s.mu.Lock()
	s.closed = true
	actors := make([]*actor, 0, len(s.actors))
	for _, a := range s.actors {
		actors = append(actors, a)
	}
	sub := s.settingsSub
	s.mu.Unlock()
	if sub != nil {
		sub.Close()
	}
	if s.settingsDone != nil {
		<-s.settingsDone
	}
	ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	join, cancelJoin := context.WithTimeout(context.Background(), shutdownJoin)
	defer cancelJoin()
	for _, a := range actors {
		a.post(func() { a.shutdown(ctx) })
	}
	for _, a := range actors {
		select {
		case <-a.quit:
		case <-join.Done():
			select {
			case <-a.quit:
			default:
				s.log.Warn("hybrid session did not stop in time", "id", a.id)
			}
		}
	}
	s.records.close()
}

// CreateInput starts a hybrid session from a saved launch config. Cwd is
// resolved again through symlinks, as ACP cwds must be. Env reaches the
// processes only; it is never stored.
type CreateInput struct {
	Cwd       string
	Name      string
	ConfigID  string
	Cmd       []string
	Env       map[string]string
	StartSide Side
}

// Create registers a hybrid session shell, records it, and starts its first
// side: the terminal UI (default) or the ACP chat (initialize, then
// session/new). A failed start removes the shell and the record; an ACP
// failure carries the agent's stderr tail in its message.
func (s *Service) Create(ctx context.Context, in CreateInput) (*session.Session, error) {
	if err := s.records.unavailable(); err != nil {
		return nil, err
	}
	side := in.StartSide
	if side == "" {
		side = SideTUI
	}
	if side != SideTUI && side != SideACP {
		return nil, fmt.Errorf("%w: startSide 只能是 tui 或 acp", ErrInvalid)
	}
	cmd, err := agentprofile.Parse(in.Cmd)
	if err != nil {
		var unsupported *agentprofile.UnsupportedError
		if errors.As(err, &unsupported) {
			return nil, fmt.Errorf("%w: 该启动配置不能用于混合会话: %s", ErrInvalid, unsupported.Reason.Message())
		}
		return nil, fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	cwd, err := filepath.EvalSymlinks(in.Cwd)
	if err == nil {
		cwd, err = filepath.Abs(cwd)
	}
	if err != nil {
		return nil, fmt.Errorf("%w: 目录: %v", ErrInvalid, err)
	}
	if s.records.count() >= MaxRecords {
		return nil, ErrLimit
	}
	s.mu.Lock()
	closed := s.closed
	s.mu.Unlock()
	if closed {
		return nil, fmt.Errorf("%w: 服务已关闭", ErrUnavailable)
	}
	a := newActor(s, cmd, cwd, in.ConfigID, side)
	// The shell shows the terminal command, with the options the profile
	// injects (OpenCode's --standalone); the record keeps the config's argv.
	shell, err := s.manager.CreateShell(session.ShellInput{
		CreateInput: session.CreateInput{Cwd: cwd, Cmd: cmd.TUIArgv(), Name: in.Name, Env: in.Env, ConfigID: in.ConfigID},
		Overlay:     a.overlay(),
	}, a)
	if err != nil {
		return nil, shellError(err)
	}
	info := shell.Info()
	a.id, a.shell = info.ID, shell
	now := time.Now().UTC()
	rec := Record{
		ID: info.ID, Name: in.Name, ConfigID: in.ConfigID, Agent: cmd.Profile.Name, Cwd: cwd,
		Cmd: in.Cmd, Side: side, PreviousIDs: []string{}, Config: map[string]string{},
		State: StateActive, CreatedAt: now, UpdatedAt: now,
	}
	if rec.Name == "" {
		rec.Name = info.Title
	}
	if err := s.records.add(rec); err != nil {
		// The shell has no process yet: removal cannot be refused.
		_ = s.manager.Remove(info.ID)
		return nil, err
	}
	s.mu.Lock()
	s.actors[a.id] = a
	s.mu.Unlock()
	go a.run()
	go a.watchShell()
	go a.observeLoop()
	started := make(chan error, 1)
	a.post(func() { a.start(started) })
	// The start runs to completion even if ctx ends, so a cancelled request
	// never leaves a half-started session behind.
	if err := awaitStarted(a, started); err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		s.log.Info("hybrid session created after its request ended", "id", a.id)
	}
	return shell, nil
}

// shellError keeps the session start error kinds as hybrid kinds; a
// resumed ID still in use is a conflict.
func shellError(err error) error {
	switch {
	case errors.Is(err, session.ErrInvalid):
		return fmt.Errorf("%w: 启动会话失败: %v", ErrInvalid, err)
	case errors.Is(err, session.ErrShuttingDown):
		return fmt.Errorf("%w: 启动会话失败: %v", ErrUnavailable, err)
	case errors.Is(err, session.ErrExists):
		return fmt.Errorf("%w: 启动会话失败: %v", ErrConflict, err)
	}
	return fmt.Errorf("启动会话失败: %w", err)
}

func (s *Service) unregister(a *actor) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.actors[a.id] == a {
		delete(s.actors, a.id)
	}
}

func (s *Service) actor(id string) (*actor, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	a, ok := s.actors[id]
	if !ok {
		return nil, ErrNotFound
	}
	return a, nil
}

// Subscribe opens a gap-free view of session id's chat: a snapshot, the
// events after resume (or all cached events, with Reset when resume is no
// longer in the log), then live frames. A subscriber that falls behind is
// dropped with ErrSlow. Opening the chat wakes a dormant session.
func (s *Service) Subscribe(id string, resume *ResumePoint) (*Subscription, error) {
	a, err := s.actor(id)
	if err != nil {
		return nil, err
	}
	sub, err := a.log.subscribe(resume)
	if err == nil {
		a.post(a.wake)
	}
	return sub, err
}

// PromptResult reports what a prompt did: Queued with the queue item's
// ItemID, or sent as turn TurnID for prompt PromptID (steering sends
// neither).
type PromptResult struct {
	Queued   bool
	ItemID   string
	TurnID   string
	PromptID string
}

// PromptInput is a chat prompt. Mode is ModeAuto (default) or ModeSteer.
type PromptInput struct {
	ClientRequestID string
	Text            string
	Mode            string
	Attachments     []Attachment
}

// QueueEdit changes a queued prompt: new Text and/or Move MoveUp.
type QueueEdit struct {
	ClientRequestID string
	Text            *string
	Move            string
}

// ConfigInput sets agent option ConfigID: Value for a select option,
// Enabled for a boolean one.
type ConfigInput struct {
	ClientRequestID string
	ConfigID        string
	Value           *string
	Enabled         *bool
}

// PermissionModeInput sets how the chat side answers permission requests.
type PermissionModeInput struct {
	ClientRequestID string
	Mode            PermissionMode
}

// Every chat command below carries the client's request ID: a retry with
// the same ID within 30 minutes (and among the session's last 256) gets the
// first outcome, success or error, without running again. Commands fail
// with ErrSwitching during a switch and ErrWrongSide outside the chat side.
// Each command restarts the idle clock and wakes a dormant session; a
// prompt sent meanwhile waits in the queue until the agent is back.

// command runs fn on session id's actor once per client request ID.
func command[T any](ctx context.Context, s *Service, id, requestID, name string, fn func(a *actor) (T, error)) (T, error) {
	a, err := s.actor(id)
	if err != nil {
		var zero T
		return zero, err
	}
	return once(ctx, &a.recent, requestID, name, func(ctx context.Context) (T, error) {
		return call(ctx, a, func() (T, error) { a.activity(); return fn(a) })
	})
}

// agentCommand is command for commands that end in an ACP call: prepare
// runs on the actor, the call outside it.
func agentCommand(ctx context.Context, s *Service, id, requestID, name string, prepare func(a *actor) (agentCall, error)) error {
	a, err := s.actor(id)
	if err != nil {
		return err
	}
	_, err = once(ctx, &a.recent, requestID, name, func(ctx context.Context) (struct{}, error) {
		run, err := call(ctx, a, func() (agentCall, error) { a.activity(); return prepare(a) })
		if err != nil {
			return struct{}{}, err
		}
		return struct{}{}, run(ctx)
	})
	return err
}

// Prompt sends a prompt. ModeAuto starts a turn when the session is idle
// and its queue empty and running, and queues the text otherwise; queued
// prompts go out one by one as turns end normally. ModeSteer injects the
// text into the running turn and fails with ErrSteeringUnavailable when
// the agent cannot. The text is at most MaxPromptBytes and may be blank
// when the prompt has attachments.
func (s *Service) Prompt(ctx context.Context, id string, in PromptInput) (PromptResult, error) {
	a, err := s.actor(id)
	if err != nil {
		return PromptResult{}, err
	}
	// Resolve attachments inside the deduped command: a retried request ID
	// must return the first outcome without reading the files again.
	switch in.Mode {
	case "", ModeAuto:
		return once(ctx, &a.recent, in.ClientRequestID, "prompt", func(ctx context.Context) (PromptResult, error) {
			attachments, err := resolveAttachments(s.uploadCheck(id), in.Attachments)
			if err != nil {
				return PromptResult{}, err
			}
			if err := validText(in.Text, len(attachments)); err != nil {
				return PromptResult{}, err
			}
			return call(ctx, a, func() (PromptResult, error) {
				a.activity()
				return a.prompt(in.Text, attachments)
			})
		})
	case ModeSteer:
		_, err = once(ctx, &a.recent, in.ClientRequestID, "steer", func(ctx context.Context) (struct{}, error) {
			attachments, err := resolveAttachments(s.uploadCheck(id), in.Attachments)
			if err != nil {
				return struct{}{}, err
			}
			if err := validText(in.Text, len(attachments)); err != nil {
				return struct{}{}, err
			}
			run, err := call(ctx, a, func() (agentCall, error) {
				a.activity()
				return a.steer(in.Text, attachments)
			})
			if err != nil {
				return struct{}{}, err
			}
			return struct{}{}, run(ctx)
		})
		return PromptResult{}, err
	}
	return PromptResult{}, fmt.Errorf("%w: mode 只能是 auto 或 steer", ErrInvalid)
}

// EditQueued edits queued prompt itemID; ErrSent once it was sent or
// removed.
func (s *Service) EditQueued(ctx context.Context, id, itemID string, in QueueEdit) error {
	if in.Text != nil {
		if err := validTextLength(*in.Text); err != nil {
			return err
		}
	}
	if in.Move != "" && in.Move != MoveUp {
		return fmt.Errorf("%w: move 只能是 up", ErrInvalid)
	}
	if in.Text == nil && in.Move == "" {
		return fmt.Errorf("%w: 需要 text 或 move", ErrInvalid)
	}
	_, err := command(ctx, s, id, in.ClientRequestID, "queue.edit", func(a *actor) (struct{}, error) {
		return struct{}{}, a.editQueued(itemID, in.Text, in.Move)
	})
	return err
}

// RemoveQueued deletes queued prompt itemID; ErrSent once it was sent or
// removed.
func (s *Service) RemoveQueued(ctx context.Context, id, requestID, itemID string) error {
	_, err := command(ctx, s, id, requestID, "queue.remove", func(a *actor) (struct{}, error) { return struct{}{}, a.removeQueued(itemID) })
	return err
}

// SendQueuedNow sends queued prompt itemID next: the running turn is
// cancelled first (the client confirmed that).
func (s *Service) SendQueuedNow(ctx context.Context, id, requestID, itemID string) error {
	_, err := command(ctx, s, id, requestID, "queue.send-now", func(a *actor) (struct{}, error) { return struct{}{}, a.sendQueuedNow(itemID) })
	return err
}

// ResumeQueue unpauses the queue, sending from its head.
func (s *Service) ResumeQueue(ctx context.Context, id, requestID string) error {
	_, err := command(ctx, s, id, requestID, "queue.resume", func(a *actor) (struct{}, error) { return struct{}{}, a.resumeQueue() })
	return err
}

// ClearQueue removes every queued prompt.
func (s *Service) ClearQueue(ctx context.Context, id, requestID string) error {
	_, err := command(ctx, s, id, requestID, "queue.clear", func(a *actor) (struct{}, error) { return struct{}{}, a.clearQueue() })
	return err
}

// Cancel stops the running turn: pending requests are answered cancelled
// and the agent gets session/cancel; a non-empty queue pauses. Without a
// running turn it does nothing.
func (s *Service) Cancel(ctx context.Context, id, requestID string) error {
	_, err := command(ctx, s, id, requestID, "cancel", func(a *actor) (struct{}, error) { return struct{}{}, a.cancel() })
	return err
}

// Answer resolves pending request requestID. The first answer wins; later
// ones fail with ErrAnswered.
func (s *Service) Answer(ctx context.Context, id, clientRequestID, requestID string, answer Answer) error {
	_, err := command(ctx, s, id, clientRequestID, "answer", func(a *actor) (struct{}, error) { return struct{}{}, a.answer(requestID, answer) })
	return err
}

// SetConfig changes an agent option through session/set_config_option;
// every device sees the agent's new options, and the record keeps the
// value for later loads. A refusal is ErrAgentCall.
func (s *Service) SetConfig(ctx context.Context, id string, in ConfigInput) error {
	return agentCommand(ctx, s, id, in.ClientRequestID, "config", func(a *actor) (agentCall, error) {
		return a.setConfig(in.ConfigID, in.Value, in.Enabled)
	})
}

// SetPermissionMode sets session id's permission mode, PermissionAsk or
// PermissionAuto; every device sees it and the record keeps it. Unlike the
// other chat commands it is accepted on the terminal side too, for the
// chat side's next run; switching to PermissionAuto approves the requests
// already waiting. Form elicitations always wait for the user.
func (s *Service) SetPermissionMode(ctx context.Context, id string, in PermissionModeInput) error {
	if in.Mode != PermissionAsk && in.Mode != PermissionAuto {
		return fmt.Errorf("%w: mode 只能是 ask 或 auto", ErrInvalid)
	}
	_, err := command(ctx, s, id, in.ClientRequestID, "permission-mode", func(a *actor) (struct{}, error) {
		return struct{}{}, a.setPermissionMode(in.Mode)
	})
	return err
}
