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

// fastForwardTimeout bounds the merge of a checked-out branch. Like a
// switch it does not follow the request's cancellation: Git killed midway
// leaves the files half updated.
const fastForwardTimeout = 3 * time.Minute

// FastForwardPreview describes moving Branch from From to its upstream's
// tip To. Worktree is the worktree that checks the branch out, nil for
// none, and Sessions are its running sessions. With blockers
// ConfirmationID is empty and ExpiresAt nil; To is empty without an
// existing upstream.
type FastForwardPreview struct {
	ConfirmationID string
	ExpiresAt      *time.Time
	Branch         string
	From           string
	To             string
	Upstream       string
	Commits        CommitList
	Worktree       *Worktree
	Sessions       []session.Info
	Blockers       []Issue
	Warnings       []Issue
}

// FastForwardRequest executes a fast-forward preview; AcknowledgeSessions
// confirms the running sessions it listed.
type FastForwardRequest struct {
	ConfirmationID      string
	AcknowledgeSessions bool
}

// FastForwardResult is the repository after a fast-forward.
type FastForwardResult struct {
	Repository Repository
}

// fastForwardState is what a fast-forward preview shows and its execution
// rechecks.
type fastForwardState struct {
	branch localBranch
	// tree checks the branch out; nil for none.
	tree *Worktree
	// sessions are tree's running sessions, sorted by ID.
	sessions []session.Info
	commits  CommitList
	blockers []Issue
	warnings []Issue
}

// PreviewFastForward reports what moving branch to its upstream's tip
// would bring and what blocks it, from the remote-tracking refs at hand:
// it never fetches. Only an unblocked preview is confirmed.
func (s *Service) PreviewFastForward(ctx context.Context, repositoryID, branch string) (FastForwardPreview, error) {
	ctx, cancel := context.WithTimeout(ctx, readTimeout)
	defer cancel()
	r, err := s.repository(repositoryID)
	if err != nil {
		return FastForwardPreview{}, err
	}
	if err := acquire(ctx, r); err != nil {
		return FastForwardPreview{}, err
	}
	defer release(r)
	state, err := s.fastForwardState(ctx, r, branch)
	if err != nil {
		return FastForwardPreview{}, err
	}
	preview := FastForwardPreview{
		Branch: branch, From: state.branch.commit, To: state.branch.upstreamCommit, Upstream: state.branch.upstreamShort,
		Commits: state.commits, Sessions: state.sessions, Blockers: state.blockers, Warnings: state.warnings,
	}
	if state.tree != nil {
		tree := s.withLastCommit(ctx, r, *state.tree)
		preview.Worktree = &tree
	}
	if len(state.blockers) != 0 {
		return preview, nil
	}
	id, expires, err := s.confirm(confirmation{purpose: purposeFastForward, repositoryID: repositoryID, fastForward: &state})
	if err != nil {
		return FastForwardPreview{}, err
	}
	preview.ConfirmationID, preview.ExpiresAt = id, &expires
	return preview, nil
}

// FastForward moves the branch as its preview described, after checking
// again under the repository gate that nothing it bound changed. A
// checked-out branch is merged with --ff-only in its worktree, and the
// request's cancellation no longer stops Git once it starts; any other
// branch only has its ref moved, from the previewed commit.
func (s *Service) FastForward(ctx context.Context, repositoryID string, req FastForwardRequest) (FastForwardResult, error) {
	c, ok := s.takeConfirmation(req.ConfirmationID, purposeFastForward, repositoryID, "")
	if !ok || c.fastForward == nil {
		return FastForwardResult{}, problemf(ErrConflict, "快进确认已失效，请重新预览")
	}
	confirmed := *c.fastForward
	r, err := s.repository(repositoryID)
	if err != nil {
		return FastForwardResult{}, err
	}
	checkCtx, cancel := context.WithTimeout(ctx, writeTimeout)
	defer cancel()
	if err := acquire(checkCtx, r); err != nil {
		return FastForwardResult{}, err
	}
	defer release(r)
	name := confirmed.branch.name
	current, err := s.fastForwardState(checkCtx, r, name)
	if err != nil {
		if kind := errorKind(err); kind == ErrInvalid || kind == ErrNotFound {
			return FastForwardResult{}, problemf(ErrConflict, "%v；请重新预览", err)
		}
		return FastForwardResult{}, err
	}
	if !sameFastForward(confirmed, current) {
		return FastForwardResult{}, problemf(ErrConflict, "分支、上游提交、检出它的工作树或运行会话已变化，请重新预览")
	}
	if len(current.blockers) != 0 {
		return FastForwardResult{}, problemf(ErrConflict, "快进被阻止：%s；请重新预览", issueMessages(current.blockers))
	}
	if len(current.sessions) != 0 && !req.AcknowledgeSessions {
		return FastForwardResult{}, problemf(ErrInvalid, "工作树中有运行会话，请先确认它们会看到快进后的文件")
	}
	if err := checkCtx.Err(); err != nil {
		return FastForwardResult{}, err
	}
	anchor := r.entry.AnchorPath
	from, to := current.branch.commit, current.branch.upstreamCommit
	runCtx, runCancel := context.WithTimeout(context.WithoutCancel(ctx), fastForwardTimeout)
	defer runCancel()
	var stderr string
	var runErr error
	if current.tree != nil {
		_, stderr, runErr = s.gitOutput(runCtx, current.tree.Path, "merge", "--ff-only", "--no-autostash", "--end-of-options", to)
	} else {
		if _, err := s.git(runCtx, anchor, "merge-base", "--is-ancestor", from, to); err != nil {
			if errorKind(err) == ErrInternal {
				return FastForwardResult{}, problemf(ErrConflict, "上游提交不再包含分支的提交，请重新预览")
			}
			return FastForwardResult{}, err
		}
		_, stderr, runErr = s.gitOutput(runCtx, anchor, "update-ref", "-m", "webmux: fast-forward", "refs/heads/"+name, to, from)
	}
	// The actual state counts, whatever Git or the client did.
	refreshCtx, refreshCancel := context.WithTimeout(context.Background(), readTimeout)
	defer refreshCancel()
	if runErr != nil {
		tip, _ := s.resolveCommit(refreshCtx, anchor, "refs/heads/"+name)
		return FastForwardResult{}, fastForwardFailure(runErr, stderr, tip == to)
	}
	return FastForwardResult{Repository: s.readRepository(refreshCtx, r)}, nil
}

// fastForwardState reads branch, its upstream and the worktree checking it
// out, and decides the blockers and warnings. The caller holds r's gate.
func (s *Service) fastForwardState(ctx context.Context, r *repositoryState, name string) (fastForwardState, error) {
	_, listed, err := s.currentTreesListed(ctx, r, nil)
	if err != nil {
		return fastForwardState{}, err
	}
	anchor := r.entry.AnchorPath
	b, err := s.readBranch(ctx, anchor, name)
	if err != nil {
		return fastForwardState{}, err
	}
	state := fastForwardState{branch: b, sessions: []session.Info{}, commits: CommitList{Items: []Commit{}}, blockers: []Issue{}, warnings: []Issue{}}
	switch {
	case b.upstream == "":
		state.blockers = append(state.blockers, Issue{Code: issueUpstream, Message: "分支没有上游，无法快进"})
	case b.upstreamCommit == "":
		state.blockers = append(state.blockers, Issue{Code: issueUpstream, Message: "上游 " + b.upstreamShort + " 已删除，无法快进"})
	default:
		ahead, behind, err := s.aheadBehind(ctx, anchor, b.commit, b.upstreamCommit)
		if err != nil {
			return fastForwardState{}, err
		}
		switch {
		case ahead > 0 && behind > 0:
			state.blockers = append(state.blockers, Issue{Code: issueDiverged, Message: fmt.Sprintf("已分叉（领先 %d、落后 %d），需要在终端合并或变基", ahead, behind)})
		case ahead > 0:
			state.blockers = append(state.blockers, Issue{Code: issueAhead, Message: fmt.Sprintf("分支领先上游 %d 个提交，没有可快进的提交", ahead)})
		case behind == 0:
			state.blockers = append(state.blockers, Issue{Code: issueSame, Message: "分支已与上游同步"})
		}
		if behind > 0 {
			if state.commits, err = s.listCommits(ctx, anchor, b.upstreamCommit, "^"+b.commit); err != nil {
				return fastForwardState{}, err
			}
		}
	}
	for _, tree := range listed {
		if tree.Branch == nil || *tree.Branch != name {
			continue
		}
		if tree.Kind != "bare" && !tree.Prunable {
			s.populateWorktreeStatus(ctx, r.entry.CommonDir, &tree)
		}
		state.tree = &tree
		state.sessions = s.runningSessions(tree.Path)
		state.blockers = append(state.blockers, worktreeBlockers(tree, "快进")...)
		state.warnings = fastForwardWarnings(tree, state.sessions)
		break
	}
	return state, ctx.Err()
}

// runningSessions are the sessions in path or below that have not exited,
// sorted by ID.
func (s *Service) runningSessions(path string) []session.Info {
	out := []session.Info{}
	for _, info := range s.targetSessions(path) {
		if info.Status != session.StatusExited {
			out = append(out, info)
		}
	}
	return out
}

// fastForwardWarnings lists what fast-forwarding the checked-out tree
// affects without refusing it. Never nil.
func fastForwardWarnings(tree Worktree, sessions []session.Info) []Issue {
	out := []Issue{}
	if !tree.Available || tree.Kind == "bare" {
		return out
	}
	if tree.HasUntracked {
		out = append(out, Issue{Code: issueUntracked, Message: "存在未跟踪文件；上游跟踪了同名文件时快进会失败，文件不会被覆盖"})
	}
	if _, err := os.Lstat(filepath.Join(tree.Path, ".gitmodules")); err == nil {
		out = append(out, Issue{Code: issueSubmodules, Message: "存在子模块；快进不会更新子模块"})
	}
	if len(sessions) != 0 {
		out = append(out, Issue{Code: issueSessions, Message: fmt.Sprintf("有 %d 个运行中的会话；它们不会被关闭，会看到快进后的文件", len(sessions))})
	}
	return out
}

// sameFastForward compares what a fast-forward confirmation binds: the
// branch and upstream commits, the worktree checking the branch out and
// its running sessions.
func sameFastForward(a, b fastForwardState) bool {
	sameTree := (a.tree == nil) == (b.tree == nil)
	if sameTree && a.tree != nil {
		sameTree = a.tree.ID == b.tree.ID && a.tree.Path == b.tree.Path
	}
	return sameTree && a.branch.name == b.branch.name && a.branch.commit == b.branch.commit &&
		a.branch.upstreamCommit == b.branch.upstreamCommit &&
		slices.EqualFunc(a.sessions, b.sessions, func(x, y session.Info) bool { return x.ID == y.ID })
}

// fastForwardFailure explains a failed merge or ref update; moved reports
// that the branch reached the target anyway.
func fastForwardFailure(err error, stderr string, moved bool) error {
	switch errorKind(err) {
	case ErrTimeout:
		return problemf(ErrTimeout, "快进超过 %v 未完成，Git 已被终止，文件可能只更新了一部分；请在终端检查实际状态", fastForwardTimeout)
	case ErrInternal:
	default:
		return err
	}
	if paths, more := overwrittenPaths(stderr); len(paths) != 0 {
		message := "以下未跟踪文件会被快进覆盖，未快进: " + strings.Join(paths, ", ")
		if more != 0 {
			message += fmt.Sprintf(" 等另外 %d 个", more)
		}
		return problemf(ErrConflict, "%s", message)
	}
	line := firstGitLine(redactLine(stderr))
	if moved {
		return problemf(ErrConflict, "快进已完成，但 Git 钩子或过滤器报告失败，请在终端检查: %s", line)
	}
	return problemf(ErrConflict, "Git 未完成快进，请刷新后在终端检查: %s", line)
}
