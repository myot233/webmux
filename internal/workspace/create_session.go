package workspace

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/rainchan/webmux/internal/hybrid"
	"github.com/rainchan/webmux/internal/session"
)

const (
	// createRetention is how long a finished create answers a retry of its
	// request ID with its outcome.
	createRetention = 10 * time.Minute
	// CreateTimeout bounds one create: the checks, the worktree, the
	// session's start, the agent's options and the first prompt.
	CreateTimeout = 3 * time.Minute
)

// Create steps, as CreateError.Step.
const (
	StepPrecheck = "precheck"
	StepWorktree = "worktree"
	StepSession  = "session"
	StepConfig   = "config"
	StepPrompt   = "prompt"
)

// createRequestID is a client request ID, as hybrid chat commands take it.
var createRequestID = regexp.MustCompile(`^[A-Za-z0-9_-]{8,64}$`)

// NewWorktree is a worktree Create makes before the session. Mode is new (a
// new Branch from Base) or existing (the existing Branch). An empty Branch
// of a new worktree is generated, an empty Base is the repository's default
// base, and an empty Path derives the directory from the branch.
type NewWorktree struct {
	RepositoryID string
	Mode         string
	Branch       string
	Base         string
	Path         string
}

// CreateInput describes a session to create. The location is Cwd, Workspace
// (Cwd may name a directory inside it) or NewWorktree; the command is
// ConfigID or Cmd with Env. Side (tui or acp) applies only to a config with
// hybrid mode; empty starts in the terminal. Prompt is the user's first
// message; ExpandPrompt fills its {{dir}} and {{webmux}} once the session
// exists, otherwise it is sent as written. UseInitInstruction puts the
// repository's initialization
// instruction before it: it is sent once the session is up, as the first
// chat prompt on the chat side, otherwise typed into the terminal with
// Enter. Attachments are files staged by an upload before the session
// existed, moved into the session's uploads folder and sent with the first
// chat prompt or appended as paths to the terminal message. AgentConfig
// (option ID to value: a select value, or "true"/"false" for a boolean)
// and PermissionMode (ask, auto or empty for the default) belong to the
// chat side only. A non-empty ClientRequestID dedupes retries. Cols and
// Rows are the terminal's initial size (the TUI side of a hybrid session
// too): both zero for the default, otherwise both set within 1..1000.
type CreateInput struct {
	ClientRequestID string
	Cwd             string
	Workspace       *Reference
	NewWorktree     *NewWorktree
	Cmd             []string
	Env             map[string]string
	ConfigID        *string
	Name            string
	Side            string

	Prompt             string
	UseInitInstruction bool
	ExpandPrompt       bool
	Attachments        []string
	AgentConfig        map[string]string
	PermissionMode     string
	Cols, Rows         int
}

// CreateResult is a created session. Worktree is the worktree the request
// created, nil when it used an existing location; ConfigWarnings describe
// each AgentConfig item the agent did not take.
type CreateResult struct {
	Session        *session.Session
	Worktree       *Worktree
	ConfigWarnings []string
}

// CreateError is a create that stopped at Step. Worktree and SessionID are
// what it had created; nothing is rolled back. Err keeps the error kind of
// the failed step.
type CreateError struct {
	Step      string
	Worktree  *Worktree
	SessionID string
	Err       error
}

func (e *CreateError) Error() string { return e.Err.Error() }
func (e *CreateError) Unwrap() error { return e.Err }

// createFlight is one create by client request ID: duplicates wait for done
// and share its outcome.
type createFlight struct {
	done   chan struct{}
	result CreateResult
	err    error
}

// creates dedupes creates by client request ID, in memory only.
type creates struct {
	mu      sync.Mutex
	flights map[string]*createFlight
	// retention replaces createRetention when set (tests).
	retention time.Duration
}

// Create creates a session: the worktree first when asked, then the
// session, its agent options and permission mode, and its first message,
// each step through the same service methods a client could use one by
// one. What can be checked beforehand (location, launch config, connector,
// first message) is checked before anything is created. A duplicate of a
// running or, for createRetention, finished request ID gets its outcome
// without running again. Failures are *CreateError.
func (s *Service) Create(ctx context.Context, in CreateInput) (CreateResult, error) {
	if in.ClientRequestID == "" {
		return s.create(ctx, in)
	}
	if !createRequestID.MatchString(in.ClientRequestID) {
		return CreateResult{}, &CreateError{Step: StepPrecheck, Err: problemf(ErrInvalid, "clientRequestId 必须是 8 到 64 个字母、数字、- 或 _")}
	}
	s.creates.mu.Lock()
	if s.creates.flights == nil {
		s.creates.flights = make(map[string]*createFlight)
	}
	flight, running := s.creates.flights[in.ClientRequestID]
	if !running {
		flight = &createFlight{done: make(chan struct{})}
		s.creates.flights[in.ClientRequestID] = flight
	}
	s.creates.mu.Unlock()
	if running {
		select {
		case <-flight.done:
			return flight.result, flight.err
		case <-ctx.Done():
			return CreateResult{}, &CreateError{Step: StepPrecheck, Err: ctx.Err()}
		}
	}
	flight.result, flight.err = s.create(ctx, in)
	close(flight.done)
	retention := s.creates.retention
	if retention == 0 {
		retention = createRetention
	}
	time.AfterFunc(retention, func() {
		s.creates.mu.Lock()
		defer s.creates.mu.Unlock()
		if s.creates.flights[in.ClientRequestID] == flight {
			delete(s.creates.flights, in.ClientRequestID)
		}
	})
	return flight.result, flight.err
}

func (s *Service) create(ctx context.Context, in CreateInput) (CreateResult, error) {
	fail := func(step string, tree *Worktree, sessionID string, err error) (CreateResult, error) {
		return CreateResult{}, &CreateError{Step: step, Worktree: tree, SessionID: sessionID, Err: err}
	}
	plan, err := s.createPlan(ctx, in)
	if err != nil {
		return fail(StepPrecheck, nil, "", err)
	}
	reference := in.Workspace
	var tree *Worktree
	if nw := in.NewWorktree; nw != nil {
		created, err := s.CreateWorktree(ctx, nw.RepositoryID, CreateRequest{Path: plan.path, Mode: nw.Mode, Branch: plan.branch, Base: plan.base})
		if err != nil {
			return fail(StepWorktree, nil, "", err)
		}
		tree = &created
		reference = &Reference{RepositoryID: nw.RepositoryID, WorktreeID: created.ID}
	}
	var initialPrompt func(id string) (string, error)
	var preparedPrompt string
	var promptPrepared bool
	prepareTerminalPrompt := func(id string) (string, error) {
		if promptPrepared {
			return preparedPrompt, nil
		}
		text, err := s.composeTerminalPrompt(id, plan)
		if err != nil {
			return "", err
		}
		preparedPrompt = text
		promptPrepared = true
		return preparedPrompt, nil
	}
	if plan.side != hybrid.SideACP && (plan.text != "" || plan.instruction != "") {
		initialPrompt = prepareTerminalPrompt
	}
	name := in.Name
	if name == "" && plan.side != hybrid.SideACP {
		name = plan.title
	}
	item, promptSent, err := s.startSession(ctx, in.Cwd, name, reference, plan.command, plan.side, initialPrompt)
	if err != nil {
		return fail(StepSession, tree, "", err)
	}
	id := item.Info().ID
	warnings := []string{}
	if plan.side == hybrid.SideACP && (len(in.AgentConfig) > 0 || in.PermissionMode != "") {
		if warnings, err = s.applyAgentConfig(ctx, id, in); err != nil {
			return fail(StepConfig, tree, id, err)
		}
	}
	if !promptSent && (plan.text != "" || plan.instruction != "") {
		if plan.side != hybrid.SideACP {
			text, err := prepareTerminalPrompt(id)
			if err != nil {
				return fail(StepPrompt, tree, id, err)
			}
			go func() { _ = item.SendWhenReady(context.Background(), text) }()
		} else if err := s.sendFirstMessage(ctx, item, in.ClientRequestID, plan); err != nil {
			return fail(StepPrompt, tree, id, err)
		}
	}
	return CreateResult{Session: item, Worktree: tree, ConfigWarnings: warnings}, nil
}

// createPlan is a checked create: the command, the side, the new
// worktree's branch, base and path, and the first message with its staged
// attachments.
type createPlan struct {
	command resolvedCommand
	side    hybrid.Side
	branch  string
	base    string
	path    string
	// text is the user's message; instruction is the expanded
	// initialization instruction ("" for none), still holding {{dir}}.
	// expand asks for text's own {{dir}} and {{webmux}} to be filled.
	text        string
	instruction string
	expand      bool
	attachments []string
	// title is the first-prompt title of the user's text alone (no
	// instruction, no placeholders), "" for none.
	title string
}

// createPlan makes the checks a create can make before it creates
// anything: the request's shape, the launch config and side, the
// repository's availability and initialization instruction, the first
// message's size and attachments, and for a new worktree on the chat side
// the hybrid store and the agent's connector.
func (s *Service) createPlan(ctx context.Context, in CreateInput) (createPlan, error) {
	plan := createPlan{text: strings.TrimSpace(in.Prompt), expand: in.ExpandPrompt}
	plan.title = hybrid.PromptTitle(plan.text)
	if in.ExpandPrompt {
		plan.title = hybrid.PromptTitle(strings.NewReplacer(artifactsDirPlaceholder, "", webmuxPlaceholder, "").Replace(plan.text))
	}
	if in.NewWorktree != nil && (in.Cwd != "" || in.Workspace != nil) {
		return plan, problemf(ErrInvalid, "cwd、workspace 与 newWorktree 只能选择一个")
	}
	if err := session.ValidateInitialSize(in.Cols, in.Rows); err != nil {
		return plan, problemf(ErrInvalid, "%v", err)
	}
	command, err := s.resolveCommand(in)
	if err != nil {
		return plan, err
	}
	command.cols, command.rows = in.Cols, in.Rows
	plan.command = command
	plan.side = hybrid.Side(in.Side)
	if plan.side == "" {
		plan.side = hybrid.SideTUI
	}
	if plan.side != hybrid.SideACP && (len(in.AgentConfig) > 0 || in.PermissionMode != "") {
		return plan, problemf(ErrInvalid, "agentConfig 与 permissionMode 只适用于聊天视图")
	}
	if len(in.Attachments) > 0 && plan.text == "" {
		return plan, problemf(ErrInvalid, "附件需要随消息发送，请输入消息")
	}
	if in.PermissionMode != "" && in.PermissionMode != string(hybrid.PermissionAsk) && in.PermissionMode != string(hybrid.PermissionAuto) {
		return plan, problemf(ErrInvalid, "permissionMode 只能是 ask 或 auto")
	}
	if in.UseInitInstruction && in.Workspace == nil && in.NewWorktree == nil {
		return plan, problemf(ErrInvalid, "初始化指令属于仓库，请选择仓库中的位置")
	}
	if len(in.Prompt) > hybrid.MaxPromptBytes {
		return plan, problemf(ErrInvalid, "用户指令超过 %d 字节", hybrid.MaxPromptBytes)
	}
	if len(in.Attachments) > hybrid.MaxAttachments {
		return plan, problemf(ErrInvalid, "每条消息最多 %d 个附件", hybrid.MaxAttachments)
	}
	if len(in.Attachments) > 0 && s.artifacts == nil {
		return plan, problemf(ErrUnavailable, "临时文件夹不可用")
	}
	if in.ExpandPrompt {
		if strings.Contains(plan.text, artifactsDirPlaceholder) && s.artifacts == nil {
			return plan, problemf(ErrUnavailable, "临时文件夹不可用")
		}
		if strings.Contains(plan.text, webmuxPlaceholder) && (s.artifacts == nil || s.brief == nil) {
			return plan, problemf(ErrUnavailable, "WebMux 工具说明不可用")
		}
	}
	for _, p := range in.Attachments {
		clean, err := s.artifacts.StagedPath(p)
		if err != nil || slices.Contains(plan.attachments, clean) {
			return plan, problemf(ErrInvalid, "附件不是为新会话上传的文件")
		}
		plan.attachments = append(plan.attachments, clean)
	}
	if command.hybrid {
		if s.hybrids == nil {
			return plan, problemf(ErrUnavailable, "混合会话不可用")
		}
		if err := s.hybrids.Unavailable(); err != nil {
			return plan, err
		}
	}
	repositoryID := ""
	switch {
	case in.NewWorktree != nil:
		repositoryID = in.NewWorktree.RepositoryID
	case in.Workspace != nil:
		repositoryID = in.Workspace.RepositoryID
	}
	if repositoryID == "" {
		return plan, nil
	}
	r, err := s.repository(repositoryID)
	if err != nil {
		return plan, err
	}
	readCtx, cancel := context.WithTimeout(ctx, readTimeout)
	defer cancel()
	snapshot, err := s.snapshot(readCtx, r)
	if err != nil {
		return plan, err
	}
	if snapshot.Availability != "available" {
		return plan, problemf(ErrUnavailable, "%s", snapshot.Error)
	}
	if in.UseInitInstruction {
		if plan.instruction, err = s.expandRepositoryInstruction(snapshot.InitInstruction); err != nil {
			return plan, err
		}
		if strings.Contains(plan.instruction, artifactsDirPlaceholder) && s.artifacts == nil {
			return plan, problemf(ErrUnavailable, "临时文件夹不可用")
		}
		if strings.Contains(plan.instruction, webmuxPlaceholder) && (s.artifacts == nil || s.brief == nil) {
			return plan, problemf(ErrUnavailable, "WebMux 工具说明不可用")
		}
		if plan.instruction != "" && promptSize(plan.instruction, plan.text) > hybrid.MaxPromptBytes {
			return plan, problemf(ErrInvalid, "消息与初始化指令合计超过 %d 字节", hybrid.MaxPromptBytes)
		}
	}
	if in.NewWorktree == nil {
		return plan, nil
	}
	if err := s.planWorktree(readCtx, r, in.NewWorktree, &plan); err != nil {
		return plan, err
	}
	if plan.side == hybrid.SideACP {
		if err := s.hybrids.CheckCreate(ctx, hybrid.CreateInput{
			Cwd: r.entry.AnchorPath, ConfigID: command.configID, Cmd: command.cmd, Env: command.env, StartSide: plan.side,
		}); err != nil {
			return plan, err
		}
	}
	return plan, nil
}

// planWorktree settles a new worktree's branch, base and path: a new
// branch without a name gets wt/<date>-<time>, free as a branch and as a
// directory; a missing base is the repository's default base; a missing
// path is the branch's last segment under the repository's worktrees
// folder.
func (s *Service) planWorktree(ctx context.Context, r *repositoryState, nw *NewWorktree, plan *createPlan) error {
	if nw.Mode != "new" && nw.Mode != "existing" {
		return problemf(ErrInvalid, "newWorktree.mode 只能是 new 或 existing")
	}
	plan.branch, plan.base, plan.path = nw.Branch, nw.Base, nw.Path
	if nw.Mode == "existing" && plan.branch == "" {
		return problemf(ErrInvalid, "请选择已有本地分支")
	}
	parent := filepath.Join(filepath.Dir(r.entry.AnchorPath), filepath.Base(r.entry.AnchorPath)+".worktrees")
	if nw.Mode == "new" && (plan.branch == "" || plan.base == "") {
		repository, err := s.current(ctx, r)
		if err != nil {
			return err
		}
		branches, err := s.branches(ctx, r, repository)
		if err != nil {
			return err
		}
		if plan.base == "" {
			plan.base = "HEAD"
			if branches.DefaultBase != nil {
				plan.base = *branches.DefaultBase
			}
		}
		if plan.branch == "" {
			taken := make(map[string]bool, len(branches.Branches))
			for _, b := range branches.Branches {
				taken[b.Name] = true
			}
			plan.branch = generatedBranch(time.Now(), func(branch string) bool {
				if taken[branch] {
					return false
				}
				_, err := os.Lstat(filepath.Join(parent, path.Base(branch)))
				return errors.Is(err, os.ErrNotExist)
			})
		}
	}
	leaf := path.Base(plan.branch)
	if leaf == "." || leaf == "/" || strings.HasPrefix(leaf, "-") {
		return problemf(ErrInvalid, "分支名无效")
	}
	if plan.path == "" {
		plan.path = filepath.Join(parent, leaf)
	}
	return nil
}

// generatedBranch is wt/<date>-<time> at now, with a -2, -3, ... suffix
// until free accepts it.
func generatedBranch(now time.Time, free func(string) bool) string {
	stamp := now.Format("20060102-150405")
	for suffix := 1; suffix <= 1000; suffix++ {
		name := "wt/" + stamp
		if suffix > 1 {
			name = fmt.Sprintf("wt/%s-%d", stamp, suffix)
		}
		if free(name) {
			return name
		}
	}
	return fmt.Sprintf("wt/%s-%d", stamp, now.Nanosecond())
}
