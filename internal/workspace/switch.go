package workspace

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/rainchan/webmux/internal/session"
)

// switchTimeout bounds one git switch. The switch does not follow the
// request's cancellation: Git killed midway leaves the index and the files
// half updated, and filters or hooks may legitimately take long.
const switchTimeout = 3 * time.Minute

// Switch target kinds.
const (
	SwitchLocal  = "local"
	SwitchRemote = "remote"
	SwitchNew    = "new"
	SwitchDetach = "detach"
)

// Blocker and warning codes of a switch preview.
const (
	issueChanges     = "changes"
	issueOperation   = "operation"
	issueOccupied    = "occupied"
	issueSame        = "same"
	issueUnverified  = "unverifiable"
	issueExists      = "exists"
	issueUnsupported = "unsupported"
	issueUntracked   = "untracked"
	issueLocked      = "locked"
	issueSubmodules  = "submodules"
	issueSessions    = "sessions"
)

// SwitchTarget is what a worktree switches to. Local takes Branch; Remote
// takes Ref, a short remote-tracking name such as "origin/fix/b", and
// creates the local branch Name (default: Ref without its remote); New takes
// Name and StartPoint; Detach takes Commit.
type SwitchTarget struct {
	Kind       string
	Branch     string
	Ref        string
	Name       string
	StartPoint string
	Commit     string
}

// Issue is one blocker or warning; Path names the worktree occupying a
// branch.
type Issue struct {
	Code    string
	Message string
	Path    string
}

// SwitchTargetInfo is a target resolved to its commit. Branch is the branch
// the worktree ends on, nil for a detached target; Upstream is the
// remote-tracking branch a remote target tracks.
type SwitchTargetInfo struct {
	Kind          string
	Branch        *string
	Commit        string
	Subject       string
	CreatesBranch bool
	Upstream      *string
}

// SwitchPreview describes a switch. With blockers ConfirmationID is empty
// and ExpiresAt nil. Sessions are the worktree's running sessions.
type SwitchPreview struct {
	ConfirmationID string
	ExpiresAt      *time.Time
	Worktree       Worktree
	Target         SwitchTargetInfo
	Sessions       []session.Info
	Blockers       []Issue
	Warnings       []Issue
}

// SwitchRequest executes a switch preview; AcknowledgeSessions confirms
// the running sessions it listed.
type SwitchRequest struct {
	ConfirmationID      string
	AcknowledgeSessions bool
}

// switchPlan is a target resolved to exact objects.
type switchPlan struct {
	target SwitchTarget
	info   SwitchTargetInfo
	// exists reports that the branch a remote or new target creates
	// already exists.
	exists bool
}

// switchState is what a switch preview shows and its execution rechecks.
type switchState struct {
	tree Worktree
	plan switchPlan
	// sessions are the running sessions, sorted by ID.
	sessions []session.Info
	blockers []Issue
	warnings []Issue
}

// PreviewSwitch resolves target and reports what blocks or affects
// switching the worktree to it. Only an unblocked preview is confirmed.
func (s *Service) PreviewSwitch(ctx context.Context, repositoryID, worktreeID string, target SwitchTarget) (SwitchPreview, error) {
	ctx, cancel := context.WithTimeout(ctx, readTimeout)
	defer cancel()
	r, err := s.repository(repositoryID)
	if err != nil {
		return SwitchPreview{}, err
	}
	if err := acquire(ctx, r); err != nil {
		return SwitchPreview{}, err
	}
	defer release(r)
	state, err := s.switchState(ctx, r, worktreeID, target)
	if err != nil {
		return SwitchPreview{}, err
	}
	preview := SwitchPreview{
		Worktree: s.withLastCommit(ctx, r, state.tree), Target: state.plan.info, Sessions: state.sessions,
		Blockers: state.blockers, Warnings: state.warnings,
	}
	if len(state.blockers) != 0 {
		return preview, nil
	}
	id, expires, err := s.confirm(confirmation{purpose: purposeSwitch, repositoryID: repositoryID, worktreeID: worktreeID, switching: &state})
	if err != nil {
		return SwitchPreview{}, err
	}
	preview.ConfirmationID, preview.ExpiresAt = id, &expires
	return preview, nil
}

// SwitchWorktree switches the worktree as its preview described, after
// checking again under the repository gate that nothing it bound changed.
// Once Git starts, the request's cancellation no longer stops it. The
// returned Worktree is re-read afterwards, also when Git failed after it
// changed the checkout.
func (s *Service) SwitchWorktree(ctx context.Context, repositoryID, worktreeID string, req SwitchRequest) (Worktree, error) {
	c, ok := s.takeConfirmation(req.ConfirmationID, purposeSwitch, repositoryID, worktreeID)
	if !ok || c.switching == nil {
		return Worktree{}, problemf(ErrConflict, "切换确认已失效，请重新预览")
	}
	confirmed := *c.switching
	r, err := s.repository(repositoryID)
	if err != nil {
		return Worktree{}, err
	}
	checkCtx, cancel := context.WithTimeout(ctx, writeTimeout)
	defer cancel()
	if err := acquire(checkCtx, r); err != nil {
		return Worktree{}, err
	}
	defer release(r)
	current, err := s.switchState(checkCtx, r, worktreeID, confirmed.plan.target)
	if err != nil {
		if errorKind(err) == ErrInvalid {
			return Worktree{}, problemf(ErrConflict, "%v；切换目标已变化，请重新预览", err)
		}
		return Worktree{}, err
	}
	if !sameSwitch(confirmed, current) {
		return Worktree{}, problemf(ErrConflict, "工作树的分支或提交、目标提交或运行会话已变化，请重新预览")
	}
	if len(current.blockers) != 0 {
		return Worktree{}, problemf(ErrConflict, "切换被阻止：%s；请重新预览", issueMessages(current.blockers))
	}
	if len(current.sessions) != 0 && !req.AcknowledgeSessions {
		return Worktree{}, problemf(ErrInvalid, "工作树中有运行会话，请先确认它们会看到切换后的文件")
	}
	if err := checkCtx.Err(); err != nil {
		return Worktree{}, err
	}
	runCtx, runCancel := context.WithTimeout(context.WithoutCancel(ctx), switchTimeout)
	defer runCancel()
	_, stderr, runErr := s.gitOutput(runCtx, current.tree.Path, switchArgs(current.plan)...)
	// The actual state counts, whatever Git or the client did.
	refreshCtx, refreshCancel := context.WithTimeout(context.Background(), readTimeout)
	defer refreshCancel()
	after, refreshErr := s.currentTree(refreshCtx, r, worktreeID)
	if runErr != nil {
		return after, switchFailure(runErr, stderr, current.tree, after, refreshErr)
	}
	if refreshErr != nil {
		return Worktree{}, problemf(ErrConflict, "切换已完成，但刷新失败，请刷新: %v", refreshErr)
	}
	return s.withLastCommit(refreshCtx, r, after), nil
}

// withLastCommit describes tree's HEAD as a snapshot does.
func (s *Service) withLastCommit(ctx context.Context, r *repositoryState, tree Worktree) Worktree {
	trees := []Worktree{tree}
	s.fillLastCommits(ctx, r.entry.AnchorPath, trees)
	return trees[0]
}

// switchState reads the worktree, resolves target and decides its
// blockers and warnings. The caller holds r's gate.
func (s *Service) switchState(ctx context.Context, r *repositoryState, worktreeID string, target SwitchTarget) (switchState, error) {
	selected, listed, err := s.currentTreesListed(ctx, r, []string{worktreeID})
	if err != nil {
		return switchState{}, err
	}
	plan, err := s.resolveSwitch(ctx, r.entry.AnchorPath, target)
	if err != nil {
		return switchState{}, err
	}
	state := switchState{tree: selected[0], plan: plan, sessions: s.runningSessions(selected[0].Path)}
	state.blockers = switchBlockers(state.tree, listed, plan)
	state.warnings = switchWarnings(state.tree, state.sessions)
	return state, ctx.Err()
}

// resolveSwitch validates target and resolves it to a commit. A malformed
// or unresolvable target is ErrInvalid.
func (s *Service) resolveSwitch(ctx context.Context, anchor string, target SwitchTarget) (switchPlan, error) {
	plan := switchPlan{target: target, info: SwitchTargetInfo{Kind: target.Kind}}
	var expression, created string
	switch target.Kind {
	case SwitchLocal:
		if err := s.validBranch(ctx, anchor, target.Branch); err != nil {
			return switchPlan{}, err
		}
		expression, plan.info.Branch = "refs/heads/"+target.Branch, strptr(target.Branch)
	case SwitchRemote:
		remote, rest, _ := strings.Cut(target.Ref, "/")
		if !plainRevision(target.Ref) || remote == "" || rest == "" {
			return switchPlan{}, problemf(ErrInvalid, "远程分支名无效")
		}
		if _, err := s.git(ctx, anchor, "check-ref-format", "refs/remotes/"+target.Ref); err != nil {
			if errorKind(err) != ErrInternal {
				return switchPlan{}, err
			}
			return switchPlan{}, problemf(ErrInvalid, "远程分支名无效: %s", target.Ref)
		}
		created = rest
		if target.Name != "" {
			created = target.Name
		}
		expression, plan.info.Upstream = "refs/remotes/"+target.Ref, strptr(target.Ref)
	case SwitchNew:
		if !plainRevision(target.StartPoint) {
			return switchPlan{}, problemf(ErrInvalid, "起点无效")
		}
		created, expression = target.Name, target.StartPoint
	case SwitchDetach:
		if !plainRevision(target.Commit) {
			return switchPlan{}, problemf(ErrInvalid, "提交无效")
		}
		expression = target.Commit
	default:
		return switchPlan{}, problemf(ErrInvalid, "切换目标类型必须为 local、remote、new 或 detach")
	}
	if target.Kind == SwitchRemote || target.Kind == SwitchNew {
		if err := s.validBranch(ctx, anchor, created); err != nil {
			return switchPlan{}, err
		}
		exists, err := s.resolveCommit(ctx, anchor, "refs/heads/"+created)
		if err != nil {
			return switchPlan{}, err
		}
		plan.exists = exists != ""
		plan.info.Branch, plan.info.CreatesBranch = strptr(created), true
	}
	commit, err := s.resolveCommit(ctx, anchor, expression)
	if err != nil {
		return switchPlan{}, err
	}
	if commit == "" {
		return switchPlan{}, problemf(ErrInvalid, "无法解析为提交: %s", strings.TrimPrefix(expression, "refs/heads/"))
	}
	plan.info.Commit = commit
	commits, err := s.describeCommits(ctx, anchor, []string{commit})
	if err != nil {
		return switchPlan{}, err
	}
	if len(commits) == 1 {
		plan.info.Subject = commits[0].Subject
	}
	return plan, nil
}

// validBranch accepts an explicit branch name: no option prefix, valid for
// Git and not checkout shorthand such as @{-1}.
func (s *Service) validBranch(ctx context.Context, anchor, name string) error {
	if name == "" || strings.HasPrefix(name, "-") || strings.ContainsRune(name, '\x00') {
		return problemf(ErrInvalid, "分支名无效")
	}
	validated, err := s.git(ctx, anchor, "check-ref-format", "--branch", name)
	if err != nil {
		if errorKind(err) != ErrInternal {
			return err
		}
		return problemf(ErrInvalid, "分支名无效: %s", name)
	}
	if outputLine(validated) != name {
		return problemf(ErrInvalid, "请使用明确分支名，不使用 checkout 简写")
	}
	return nil
}

// plainRevision rejects what Git could read as an option or that cannot
// be one argument.
func plainRevision(value string) bool {
	return value != "" && !strings.HasPrefix(value, "-") && !strings.ContainsAny(value, "\x00\n")
}

// resolveCommit is the full OID of the commit expression names, "" when it
// names none. expression never starts with "-".
func (s *Service) resolveCommit(ctx context.Context, anchor, expression string) (string, error) {
	data, err := s.git(ctx, anchor, "rev-parse", "--verify", "--quiet", expression+"^{commit}")
	if err != nil {
		if errorKind(err) != ErrInternal {
			return "", err
		}
		return "", nil
	}
	oid := outputLine(data)
	if !isObjectID(oid) {
		return "", nil
	}
	return oid, nil
}

// worktreeBlockers lists what refuses changing tree's checkout in place;
// action names the change. Never nil.
func worktreeBlockers(tree Worktree, action string) []Issue {
	out := []Issue{}
	if tree.Kind == "bare" || !tree.Available || tree.Prunable {
		return append(out, Issue{Code: issueUnsupported, Message: "bare 仓库、不可用或待清理的工作树不能" + action})
	}
	if tree.Error != "" || tree.Changes == nil {
		return append(out, Issue{Code: issueUnverified, Message: "工作树状态无法验证，不能确认安全" + action + ": " + tree.Error})
	}
	if c := tree.Changes; c.Staged+c.Unstaged+c.Conflicted > 0 {
		out = append(out, Issue{Code: issueChanges, Message: "有未提交的修改，请先提交或贮藏"})
	}
	if tree.Operation != nil {
		out = append(out, Issue{Code: issueOperation, Message: fmt.Sprintf("正在进行 %s，请先在终端完成或中止", *tree.Operation)})
	}
	return out
}

// switchBlockers lists what refuses switching tree as plan says; listed
// are all worktrees of the repository. Never nil.
func switchBlockers(tree Worktree, listed []Worktree, plan switchPlan) []Issue {
	out := worktreeBlockers(tree, "切换")
	if len(out) != 0 && (out[0].Code == issueUnsupported || out[0].Code == issueUnverified) {
		return out
	}
	switch plan.target.Kind {
	case SwitchLocal:
		branch := *plan.info.Branch
		if sameText(tree.Branch, plan.info.Branch) {
			out = append(out, Issue{Code: issueSame, Message: "工作树已在分支 " + branch + " 上"})
			break
		}
		for _, other := range listed {
			if other.ID != tree.ID && sameText(other.Branch, plan.info.Branch) {
				out = append(out, Issue{Code: issueOccupied, Message: "分支 " + branch + " 已被另一个工作树检出", Path: other.Path})
				break
			}
		}
	case SwitchRemote:
		if plan.exists {
			out = append(out, Issue{Code: issueExists, Message: "本地分支 " + *plan.info.Branch + " 已存在，请改为切换到该本地分支"})
		}
	case SwitchNew:
		if plan.exists {
			out = append(out, Issue{Code: issueExists, Message: "分支 " + *plan.info.Branch + " 已存在，请换一个名称或切换到该分支"})
		}
	case SwitchDetach:
		if tree.Detached && tree.Head != nil && *tree.Head == plan.info.Commit {
			out = append(out, Issue{Code: issueSame, Message: "工作树已分离在该提交上"})
		}
	}
	return out
}

// switchWarnings lists what a switch affects without refusing it. Never
// nil.
func switchWarnings(tree Worktree, sessions []session.Info) []Issue {
	out := []Issue{}
	if !tree.Available || tree.Kind == "bare" {
		return out
	}
	if tree.HasUntracked {
		out = append(out, Issue{Code: issueUntracked, Message: "存在未跟踪文件；目标跟踪了同名文件时切换会失败，文件不会被覆盖"})
	}
	if tree.Locked {
		message := "工作树已锁定；锁不限制切换"
		if tree.LockReason != "" {
			message += "，锁定原因: " + tree.LockReason
		}
		out = append(out, Issue{Code: issueLocked, Message: message})
	}
	if _, err := os.Lstat(filepath.Join(tree.Path, ".gitmodules")); err == nil {
		out = append(out, Issue{Code: issueSubmodules, Message: "存在子模块；切换不会更新子模块"})
	}
	if len(sessions) != 0 {
		out = append(out, Issue{Code: issueSessions, Message: fmt.Sprintf("有 %d 个运行中的会话；它们不会被关闭，会看到切换后的文件", len(sessions))})
	}
	return out
}

// sameSwitch compares what a switch confirmation binds: the worktree, its
// branch and HEAD, the target commit and the running sessions.
func sameSwitch(a, b switchState) bool {
	return a.tree.ID == b.tree.ID && a.tree.Path == b.tree.Path && a.tree.Detached == b.tree.Detached &&
		sameText(a.tree.Branch, b.tree.Branch) && sameText(a.tree.Head, b.tree.Head) &&
		a.plan.info.Commit == b.plan.info.Commit &&
		slices.EqualFunc(a.sessions, b.sessions, func(x, y session.Info) bool { return x.ID == y.ID })
}

func issueMessages(list []Issue) string {
	messages := make([]string, len(list))
	for i, issue := range list {
		messages[i] = issue.Message
	}
	return strings.Join(messages, "；")
}
