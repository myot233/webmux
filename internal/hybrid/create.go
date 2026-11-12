package hybrid

import (
	"context"
	"errors"
	"fmt"
	"github.com/rainchan/webmux/internal/agentprofile"
	"github.com/rainchan/webmux/internal/session"
	"path/filepath"
	"time"
)

// CreateInput starts a hybrid session from a saved launch config. Cwd is
// resolved again through symlinks, as ACP cwds must be. Env reaches the
// processes only; it is never stored.
type CreateInput struct {
	// RuntimeID and ProviderThreadID are reserved by the durable handoff coordinator.
	// They are never accepted by the ordinary session-creation HTTP API.
	RuntimeID, ProviderThreadID, ContextExecutionID string
	Cwd                                             string
	Name                                            string
	ConfigID                                        string
	Cmd                                             []string
	Env                                             map[string]string
	StartSide                                       Side
	// Cols and Rows are the initial terminal size, both zero for the default.
	Cols, Rows int
	// InitialPrompt resolves the first message for a terminal-side start once
	// the session ID is allocated. When the agent profile supports passing it
	// on the command line, InitialPromptSent is set to true so the caller does
	// not also type the message into the PTY.
	InitialPrompt     func(id string) (string, error)
	InitialPromptSent *bool
	// Attention configures completion and intervention notifications for both
	// sides of the hybrid session. Nil uses the global default policy.
	Attention *session.AttentionPolicy
}

// CheckCreate makes the checks of a Create of in before anything exists:
// the record store, the start side, the launch command's profile, the
// directory, the record limit and, for a chat-first start, the profile's
// connector, looked up as the start would in in.Cwd with in.Env. A missing
// connector is a *ConnectorMissingError. It registers nothing and starts
// only the connector lookup.
func (s *Service) CheckCreate(ctx context.Context, in CreateInput) error {
	cmd, side, cwd, err := s.admit(in)
	if err != nil || side != SideACP || cmd.Profile.Connector == nil {
		return err
	}
	probe, err := s.manager.ProbeShell(session.CreateInput{Cwd: cwd, Cmd: cmd.TUIArgv(), Env: in.Env, ConfigID: in.ConfigID})
	if err != nil {
		return shellError(err)
	}
	_, err = prepareACP(ctx, cmd, probe)
	return err
}

// admit runs the checks that precede every create.
func (s *Service) admit(in CreateInput) (agentprofile.Command, Side, string, error) {
	if err := s.Unavailable(); err != nil {
		return agentprofile.Command{}, "", "", err
	}
	side := in.StartSide
	if side == "" {
		side = SideTUI
	}
	if side != SideTUI && side != SideACP {
		return agentprofile.Command{}, "", "", fmt.Errorf("%w: startSide 只能是 tui 或 acp", ErrInvalid)
	}
	cmd, err := s.agents.Parse(in.Cmd)
	if err != nil {
		var unsupported *agentprofile.UnsupportedError
		if errors.As(err, &unsupported) {
			return agentprofile.Command{}, "", "", fmt.Errorf("%w: 该启动配置不能用于混合会话: %s", ErrInvalid, s.agents.Explain(unsupported.Reason))
		}
		return agentprofile.Command{}, "", "", fmt.Errorf("%w: %w", ErrInvalid, err)
	}
	cwd, err := filepath.EvalSymlinks(in.Cwd)
	if err == nil {
		cwd, err = filepath.Abs(cwd)
	}
	if err != nil {
		return agentprofile.Command{}, "", "", fmt.Errorf("%w: 目录: %w", ErrInvalid, err)
	}
	if !s.records.accepts(s.recoveringNow) {
		return agentprofile.Command{}, "", "", ErrLimit
	}
	s.mu.Lock()
	closed := s.closed
	s.mu.Unlock()
	if closed {
		return agentprofile.Command{}, "", "", fmt.Errorf("%w: 服务已关闭", ErrUnavailable)
	}
	return cmd, side, cwd, nil
}

// Create registers a hybrid session shell, records it, and starts its first
// side: the terminal UI (default) or the ACP chat (initialize, then
// session/new). A failed start removes the shell and the record; an ACP
// failure carries the agent's stderr tail in its message.
func (s *Service) Create(ctx context.Context, in CreateInput) (*session.Session, error) {
	if err := s.beginOperation(); err != nil {
		return nil, err
	}
	defer s.starting.Done()
	cmd, side, cwd, err := s.admit(in)
	if err != nil {
		return nil, err
	}
	a := newActor(s, cmd, cwd, in.ConfigID, side, in.Attention)
	a.contextExecutionID = in.ContextExecutionID
	cleanupConversation := func(ctx context.Context, id string) error {
		if in.ProviderThreadID != "" && s.conversations != nil {
			return s.conversations.UnbindRuntime(ctx, id)
		}
		return s.forgetConversation(ctx, id)
	}
	// The shell shows the terminal command, with the options the profile
	// injects (OpenCode's --standalone); the record keeps the config's argv.
	shell, err := s.manager.CreateShell(session.ShellInput{
		CreateInput: session.CreateInput{
			Cwd: cwd, Cmd: cmd.TUIArgv(), Name: in.Name, Env: in.Env, ConfigID: in.ConfigID,
			Cols: in.Cols, Rows: in.Rows, Attention: in.Attention,
		},
		ID:      in.RuntimeID,
		Overlay: a.overlay(),
	}, a)
	if err != nil {
		return nil, shellError(err)
	}
	info := shell.Info()
	a.id, a.shell = info.ID, shell
	if side == SideACP {
		// A missing connector refuses the session before it is recorded.
		if _, err := a.prepareACP(ctx); err != nil {
			_ = s.manager.Remove(info.ID)
			return nil, err
		}
	}
	var initialArgv []string
	if side == SideTUI && in.InitialPrompt != nil {
		if probe, ok := cmd.InitialTUIArgv("x"); ok && len(probe) > 0 {
			prompt, err := in.InitialPrompt(info.ID)
			if err != nil {
				_ = s.manager.Remove(info.ID)
				return nil, err
			}
			if argv, ok := cmd.InitialTUIArgv(prompt); ok {
				initialArgv = argv
				if in.InitialPromptSent != nil {
					*in.InitialPromptSent = true
				}
			}
		}
	}
	now := time.Now().UTC()
	rec := Record{
		ID: info.ID, Name: in.Name, Title: info.Title, ConfigID: in.ConfigID, Agent: cmd.Profile.Name, Cwd: cwd,
		Cmd: in.Cmd, Side: side, PreviousIDs: []string{}, Config: map[string]string{},
		State: StateActive, CreatedAt: now, UpdatedAt: now,
	}
	bindConversation := func() error {
		if in.ProviderThreadID != "" {
			if s.conversations == nil {
				return ErrUnavailable
			}
			return s.conversations.BindRuntime(context.WithoutCancel(ctx), rec.ID, in.ProviderThreadID, rec.ConfigID, rec.Agent)
		}
		return s.ensureConversation(context.WithoutCancel(ctx), rec)
	}
	if err := bindConversation(); err != nil {
		_ = s.manager.Remove(info.ID)
		return nil, err
	}
	if s.conversations != nil {
		binding, err := s.conversations.Binding(ctx, info.ID)
		if err != nil {
			_ = s.manager.Remove(info.ID)
			return nil, err
		}
		release, err := s.ClaimConversation(binding.ConversationID, in.ContextExecutionID)
		if err != nil {
			_ = cleanupConversation(context.WithoutCancel(ctx), info.ID)
			_ = s.manager.Remove(info.ID)
			return nil, err
		}
		defer release()
	}
	if err := a.activityThread(); err != nil {
		_ = s.manager.Remove(info.ID)
		return nil, err
	}
	a.recordTitle = rec.Title
	evicted, err := s.records.add(rec, s.recoveringNow)
	if err != nil {
		// The shell has no process yet: removal cannot be refused.
		if cleanupErr := cleanupConversation(context.WithoutCancel(ctx), info.ID); cleanupErr != nil {
			s.log.Warn("conversation cleanup incomplete", "id", info.ID)
		}
		_ = s.manager.Remove(info.ID)
		return nil, err
	}
	s.syncConversation(rec)
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		_ = s.manager.Remove(info.ID)
		if err := s.records.remove(info.ID); err != nil {
			s.log.Warn("hybrid record cleanup failed", "id", info.ID, "error", err)
		}
		if err := cleanupConversation(context.Background(), info.ID); err != nil {
			s.log.Warn("conversation cleanup incomplete", "id", info.ID)
		}
		for _, e := range evicted {
			if err := s.records.restore(e); err != nil {
				s.log.Warn("hybrid evicted record restore failed", "id", e.ID, "error", err)
			}
		}
		return nil, fmt.Errorf("%w: 服务已关闭", ErrUnavailable)
	}
	s.actors[a.id] = a
	s.mu.Unlock()
	go a.run()
	go a.watchShell()
	go a.observeLoop()
	started := make(chan error, 1)
	a.post(func() { a.start(started, initialArgv) })
	// The start runs to completion even if ctx ends, so a cancelled request
	// never leaves a half-started session behind.
	if err := awaitStarted(a, started); err != nil {
		// The failed start removed its own record, so the dropped ones
		// go back rather than being lost for nothing.
		if err := cleanupConversation(context.Background(), info.ID); err != nil {
			s.log.Warn("conversation cleanup incomplete", "id", info.ID)
		}
		for _, e := range evicted {
			if err := s.records.restore(e); err != nil {
				s.log.Warn("hybrid evicted record restore failed", "id", e.ID, "error", err)
			}
		}
		return nil, err
	}
	for _, e := range evicted {
		s.log.Info("hybrid record evicted", "id", e.ID)
		s.evict(ctx, e.ID)
		if err := s.forgetConversation(context.WithoutCancel(ctx), e.ID); err != nil {
			s.log.Warn("evicted conversation cleanup incomplete", "id", e.ID)
		}
		if s.onEvicted != nil {
			s.onEvicted(context.WithoutCancel(ctx), e)
		}
	}
	if err := ctx.Err(); err != nil {
		s.log.Info("hybrid session created after its request ended", "id", a.id)
	}
	return shell, nil
}

// evict ends what an archived record dropped for a new session left behind:
// its actor and its last-output shell. The record is already gone, so a
// failure is logged and never blocks the new session.
func (s *Service) evict(ctx context.Context, id string) {
	if err := s.clear(context.WithoutCancel(ctx), id); err != nil {
		s.log.Warn("hybrid evicted record cleanup failed", "id", id, "error", err)
	}
}

// shellError keeps the session start error kinds as hybrid kinds; a
// resumed ID still in use is a conflict.
func shellError(err error) error {
	switch {
	case errors.Is(err, session.ErrInvalid):
		return fmt.Errorf("%w: 启动会话失败: %w", ErrInvalid, err)
	case errors.Is(err, session.ErrShuttingDown):
		return fmt.Errorf("%w: 启动会话失败: %w", ErrUnavailable, err)
	case errors.Is(err, session.ErrExists):
		return fmt.Errorf("%w: 启动会话失败: %w", ErrConflict, err)
	}
	return fmt.Errorf("启动会话失败: %w", err)
}
