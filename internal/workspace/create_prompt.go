package workspace

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/rainchan/webmux/internal/hybrid"
	"github.com/rainchan/webmux/internal/launch"
	"github.com/rainchan/webmux/internal/session"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"
)

// resolvedCommand is the command a session start runs.
type resolvedCommand struct {
	cmd       []string
	env       map[string]string
	configID  string // empty for an explicit command
	hybrid    bool
	attention *session.AttentionPolicy
	// cols and rows are the terminal's requested initial size, zero for the
	// default.
	cols, rows int
}

func attentionPolicy(a *launch.AttentionSettings) *session.AttentionPolicy {
	if a == nil {
		return nil
	}
	return &session.AttentionPolicy{
		Mode:          session.AttentionMode(a.Mode),
		ProgramStatus: a.ProgramStatus,
		AgentProtocol: a.AgentProtocol,
		Notify:        a.Notify,
		Bell:          a.Bell,
		MinTurn:       time.Duration(a.MinTurnSeconds) * time.Second,
		DoneDelay:     time.Duration(a.DoneDelaySeconds) * time.Second,
	}
}

// resolveCommand applies a saved launch config and validates the explicit
// environment and side. It runs before any filesystem or Git work.
func (s *Service) resolveCommand(in CreateInput) (resolvedCommand, error) {
	out := resolvedCommand{cmd: in.Cmd, env: in.Env}
	if in.ConfigID != nil {
		if *in.ConfigID == "" {
			return resolvedCommand{}, problemf(ErrInvalid, "configId must not be empty")
		}
		if len(in.Cmd) > 0 || len(in.Env) > 0 {
			return resolvedCommand{}, problemf(ErrInvalid, "configId cannot be combined with cmd or env")
		}
		// A nil store answers launch.ErrUnavailable.
		saved, err := s.launches.Get(*in.ConfigID)
		if err != nil {
			return resolvedCommand{}, err
		}
		out = resolvedCommand{
			cmd: saved.Cmd, env: saved.Env, configID: saved.ID, hybrid: saved.Hybrid,
			attention: attentionPolicy(saved.Attention),
		}
	}
	switch in.Side {
	case "", string(hybrid.SideTUI):
	case string(hybrid.SideACP):
		if !out.hybrid {
			return resolvedCommand{}, problemf(ErrInvalid, "聊天视图只适用于开启了聊天视图的启动配置")
		}
	default:
		return resolvedCommand{}, problemf(ErrInvalid, "view 只能是 terminal 或 chat")
	}
	if err := session.ValidateEnv(out.env); err != nil {
		return resolvedCommand{}, problemf(ErrInvalid, "%v", err)
	}
	return out, nil
}

// startSession starts a session in cwd or the selected worktree. Launch
// config errors keep their launch error kinds; a config with hybrid mode
// starts a hybrid session, whose errors keep their hybrid kinds.
func (s *Service) startSession(ctx context.Context, cwd, name string, reference *Reference, command resolvedCommand, side hybrid.Side, initialPrompt func(id string) (string, error), identity *HandoffIdentity) (*session.Session, bool, error) {
	requestCtx := ctx
	ctx, cancel := context.WithTimeout(ctx, readTimeout)
	defer cancel()
	var selected *Worktree
	if reference != nil {
		r, err := s.repository(reference.RepositoryID)
		if err != nil {
			return nil, false, err
		}
		// Do not hold a repository operation gate while registering an in-flight
		// launch: removal holds that gate and waits for earlier launches to finish.
		snapshot, err := s.snapshot(ctx, r)
		if err != nil {
			return nil, false, err
		}
		if snapshot.Availability != "available" {
			return nil, false, problemf(ErrUnavailable, "%s", snapshot.Error)
		}
		tree, err := findTree(snapshot, reference.WorktreeID)
		if err != nil {
			return nil, false, err
		}
		if !tree.Available || tree.Kind == "bare" {
			return nil, false, problemf(ErrConflict, "所选工作树不可作为启动目录: %s", tree.Error)
		}
		selected = &tree
		if cwd == "" {
			cwd = tree.Path
		}
	}
	if cwd == "" {
		cwd = s.defaultCwd
	}
	path, err := canonical(cwd)
	if err != nil {
		return nil, false, problemf(ErrInvalid, "directory: %v", err)
	}
	stat, err := os.Stat(path)
	if err != nil {
		return nil, false, problemf(ErrInvalid, "directory: %v", err)
	}
	if !stat.IsDir() {
		return nil, false, problemf(ErrInvalid, "path is not a directory")
	}
	if selected != nil && !within(selected.Path, path) {
		return nil, false, problemf(ErrInvalid, "cwd 与所选工作树不一致")
	}
	if err := s.beginStart(path); err != nil {
		return nil, false, err
	}
	defer s.endStart(path)
	association := s.association(ctx, path)
	if selected != nil && (association == nil || association.RepositoryID != reference.RepositoryID || association.WorktreeID != reference.WorktreeID) {
		return nil, false, problemf(ErrConflict, "所选工作树身份已变化或 cwd 属于嵌套仓库，请刷新")
	}
	if err := requestCtx.Err(); err != nil {
		return nil, false, err
	}
	var item *session.Session
	var promptSent bool
	if command.hybrid {
		var runtimeID, threadID, executionID string
		if identity != nil {
			runtimeID, threadID, executionID = identity.RuntimeID, identity.ProviderThreadID, identity.ExecutionID
		}
		item, err = s.hybrids.Create(requestCtx, hybrid.CreateInput{
			RuntimeID: runtimeID, ProviderThreadID: threadID, ContextExecutionID: executionID,
			Cwd: path, Name: name, ConfigID: command.configID, Cmd: command.cmd, Env: command.env, StartSide: side,
			Cols: command.cols, Rows: command.rows, Attention: command.attention,
			InitialPrompt: initialPrompt, InitialPromptSent: &promptSent,
		})
		if err != nil {
			return nil, false, err
		}
	} else {
		item, err = s.manager.Create(session.CreateInput{
			Cwd: path, Cmd: command.cmd, Name: name, Env: command.env, ConfigID: command.configID,
			Cols: command.cols, Rows: command.rows, Attention: command.attention,
		})
		if err != nil {
			return nil, false, sessionCreateError(err)
		}
	}
	s.commitAssociation(item.Info(), association, false)
	return item, promptSent, nil
}

// composeTerminalPrompt adopts staged attachments into session id and builds
// the terminal start prompt with adopted attachment paths appended.
func (s *Service) composeTerminalPrompt(id string, plan createPlan) (string, error) {
	attachments, err := s.adoptUploads(id, plan.attachments)
	if err != nil {
		return "", err
	}
	text, err := s.composePrompt(id, plan)
	if err != nil {
		return "", err
	}
	if len(attachments) > 0 {
		paths := make([]string, len(attachments))
		for i, at := range attachments {
			paths[i] = at.Path
		}
		if text != "" {
			text += "\n\n"
		}
		text += "附件路径：\n" + strings.Join(paths, "\n")
	}
	return text, nil
}

// sendFirstMessage sends the plan's first message to a chat session just
// started with its attachments.
func (s *Service) sendFirstMessage(ctx context.Context, item *session.Session, requestID string, plan createPlan) (hybrid.PromptResult, error) {
	id := item.Info().ID
	attachments, err := s.adoptUploads(id, plan.attachments)
	if err != nil {
		return hybrid.PromptResult{}, err
	}
	text, err := s.composePrompt(id, plan)
	if err != nil {
		return hybrid.PromptResult{}, err
	}
	if requestID == "" {
		requestID = "first-" + id
	}
	return s.hybrids.Prompt(ctx, id, hybrid.PromptInput{ClientRequestID: requestID, Text: text, TitleSource: plan.title, Attachments: attachments})
}

// composePrompt is the first prompt of session id: the expanded
// instruction, whose {{dir}} becomes the session's artifacts folder, with the
// user's text at its {{user}} or else after a blank line. With ExpandPrompt
// the user's text has its own {{dir}} and {{webmux}} filled first, so what
// fillUser inserts is never read as a reference.
func (s *Service) composePrompt(id string, plan createPlan) (string, error) {
	userText := plan.text
	if plan.expand {
		var err error
		if userText, err = s.fillSessionPlaceholders(id, userText); err != nil {
			return "", err
		}
	}
	instruction := plan.instruction
	if instruction == "" {
		if len(userText) > hybrid.MaxPromptBytes {
			return "", problemf(ErrInvalid, "展开后的消息超过 %d 字节", hybrid.MaxPromptBytes)
		}
		return userText, nil
	}
	// A start with only the instruction has no message to place.
	noMessage := plan.text == ""
	instruction, err := s.fillSessionPlaceholders(id, instruction)
	if err != nil {
		return "", err
	}
	text := fillUser(instruction, userText)
	if noMessage {
		text = strings.TrimSpace(strings.ReplaceAll(instruction, userReference, ""))
	}
	if len(text) > hybrid.MaxPromptBytes {
		return "", problemf(ErrInvalid, "消息与初始化指令合计超过 %d 字节", hybrid.MaxPromptBytes)
	}
	return text, nil
}

// fillSessionPlaceholders replaces {{dir}} in text with session id's
// artifacts folder (created if missing) and {{webmux}} with its WebMux
// brief. The brief goes in last, after {{dir}}: nothing in it is read as a
// reference. Other {{name}} text stays.
func (s *Service) fillSessionPlaceholders(id, text string) (string, error) {
	if strings.Contains(text, artifactsDirPlaceholder) {
		if s.artifacts == nil {
			return "", problemf(ErrUnavailable, "临时文件夹不可用")
		}
		dir, err := s.artifacts.Ensure(id)
		if err != nil {
			return "", problemf(ErrUnavailable, "临时文件夹不可用：%v", err)
		}
		text = strings.ReplaceAll(text, artifactsDirPlaceholder, dir)
	}
	if strings.Contains(text, webmuxPlaceholder) {
		if s.artifacts == nil || s.brief == nil {
			return "", problemf(ErrUnavailable, "WebMux 工具说明不可用")
		}
		dir, err := s.artifacts.Ensure(id)
		if err != nil {
			return "", problemf(ErrUnavailable, "WebMux 工具说明不可用：%v", err)
		}
		text = strings.ReplaceAll(text, webmuxPlaceholder, s.brief(id, dir))
	}
	return text, nil
}

// expandRepositoryInstruction is a repository's initialization instruction
// with its {{name}} references replaced by quick command text ("" for none).
// A reserved or unknown reference is ErrInvalid; {{user}} and {{dir}} stay.
func (s *Service) expandRepositoryInstruction(instruction string) (string, error) {
	if instruction == "" {
		return "", nil
	}
	var commands []QuickCommand
	if s.commands != nil {
		commands = s.commands()
	}
	expanded, missing := expandInstruction(instruction, commands)
	if len(missing) > 0 {
		return "", problemf(ErrInvalid, "初始化指令引用了不存在的快捷指令：%s", strings.Join(missing, "、"))
	}
	return expanded, nil
}

// adoptUploads moves the staged attachments into session id's uploads,
// where its prompts take files from.
func (s *Service) adoptUploads(id string, staged []string) ([]hybrid.Attachment, error) {
	if len(staged) == 0 {
		return nil, nil
	}
	paths, err := s.artifacts.Adopt(id, staged)
	if err != nil {
		return nil, problemf(ErrUnavailable, "附件无法交给会话：%v", err)
	}
	out := make([]hybrid.Attachment, 0, len(paths))
	for _, p := range paths {
		out = append(out, hybrid.Attachment{Path: p})
	}
	return out, nil
}

// LaunchOptions reads the agent options launch config configID offers
// before any chat of it exists: the agent opens a throwaway session in the
// repository's main worktree and reports them (hybrid ProbeAgentOptions).
// A config without the chat view fails with hybrid.ErrNoChatView.
func (s *Service) LaunchOptions(ctx context.Context, configID, repositoryID string) ([]hybrid.ConfigOption, error) {
	command, err := s.resolveCommand(CreateInput{ConfigID: &configID})
	if err != nil {
		return nil, err
	}
	if !command.hybrid {
		return nil, hybrid.ErrNoChatView
	}
	if s.hybrids == nil {
		return nil, problemf(ErrUnavailable, "混合会话不可用")
	}
	r, err := s.repository(repositoryID)
	if err != nil {
		return nil, err
	}
	return s.hybrids.ProbeAgentOptions(ctx, hybrid.CreateInput{
		Cwd: r.entry.AnchorPath, ConfigID: command.configID, Cmd: command.cmd, Env: command.env,
	})
}

// applyAgentConfig sets each requested option that differs from what the
// session started with, then the permission mode. An option the agent does
// not offer or refuses is a warning, also shown in the chat; any other
// failure stops the create.
func (s *Service) applyAgentConfig(ctx context.Context, id string, in CreateInput) ([]string, error) {
	warnings := []string{}
	current, err := s.hybrids.Settings(ctx, id)
	if err != nil {
		return warnings, err
	}
	warn := func(text string) error {
		warnings = append(warnings, text)
		return s.hybrids.Notice(ctx, id, text)
	}
	requestID := in.ClientRequestID
	if requestID == "" {
		requestID = id
	}
	optionIDs := make([]string, 0, len(in.AgentConfig))
	for optionID := range in.AgentConfig {
		optionIDs = append(optionIDs, optionID)
	}
	slices.Sort(optionIDs)
	for _, optionID := range optionIDs {
		want := in.AgentConfig[optionID]
		i := slices.IndexFunc(current.Config, func(o hybrid.ConfigOption) bool { return o.ID == optionID })
		if i < 0 {
			if err := warn(fmt.Sprintf("配置项 %s 不可用，未应用 %s", optionID, want)); err != nil {
				return warnings, err
			}
			continue
		}
		option := current.Config[i]
		set := hybrid.ConfigInput{ClientRequestID: derivedRequestID(requestID, "config", optionID), ConfigID: optionID}
		if option.Enabled != nil {
			enabled, err := strconv.ParseBool(want)
			if err != nil || (want != "true" && want != "false") {
				if err := warn(fmt.Sprintf("配置项 %s 只接受 true 或 false，未应用 %s", option.Name, want)); err != nil {
					return warnings, err
				}
				continue
			}
			if enabled == *option.Enabled {
				continue
			}
			set.Enabled = &enabled
		} else {
			if want == option.Value {
				continue
			}
			set.Value = &want
		}
		err := s.hybrids.SetConfig(ctx, id, set)
		switch {
		case err == nil:
		case errors.Is(err, hybrid.ErrInvalid), errors.Is(err, hybrid.ErrAgentCall):
			if err := warn(fmt.Sprintf("配置项 %s 未能设为 %s：%v", option.Name, want, err)); err != nil {
				return warnings, err
			}
		default:
			return warnings, err
		}
	}
	if mode := hybrid.PermissionMode(in.PermissionMode); mode != "" && mode != current.PermissionMode {
		if err := s.hybrids.SetPermissionMode(ctx, id, hybrid.PermissionModeInput{
			ClientRequestID: derivedRequestID(requestID, "permission", ""), Mode: mode,
		}); err != nil {
			return warnings, err
		}
	}
	return warnings, nil
}

// derivedRequestID is the client request ID of one command a create sends
// for request: stable, so a command is never applied twice.
func derivedRequestID(request, command, item string) string {
	sum := sha256.Sum256([]byte(request + "\x00" + command + "\x00" + item))
	return "qs-" + hex.EncodeToString(sum[:16])
}
