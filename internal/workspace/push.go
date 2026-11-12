package workspace

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

// pushTimeout bounds one push, its pre-push hook included. The push does
// not follow the request's cancellation: the remote may have taken it.
const pushTimeout = 3 * time.Minute

// Fixed explanations of push outcomes that remoteGit cannot classify.
const (
	pushBehindText   = "远端有新的提交，请先获取远程再快进"
	pushRejectedText = "远端拒绝了推送"
	pushHookText     = "被 pre-push 钩子拒绝"
)

// PushPreview describes pushing Commit, the branch's tip, to Target on
// Remote. Publish reports a branch without an existing upstream, which
// becomes its upstream after a successful push. PushURL is redacted. With
// blockers ConfirmationID is empty and ExpiresAt nil.
type PushPreview struct {
	ConfirmationID string
	ExpiresAt      *time.Time
	Branch         string
	Commit         string
	Remote         string
	PushURL        string
	Target         string
	Publish        bool
	Commits        CommitList
	Blockers       []Issue
	Warnings       []Issue
}

// PushRequest executes a push preview.
type PushRequest struct {
	ConfirmationID string
}

// PushResult is the outcome of a push: Status is one of the remote call
// outcomes and Message, empty for ok, explains a failure with credentials
// redacted. Repository is read after the push.
type PushResult struct {
	Status     string
	Message    string
	Repository Repository
}

// pushState is what a push preview shows and its execution rechecks.
type pushState struct {
	branch localBranch
	// askRemote and askTarget are what the preview was asked for.
	askRemote string
	askTarget string
	remote    string
	target    string
	pushURL   string
	publish   bool
	commits   CommitList
	blockers  []Issue
	warnings  []Issue
}

// PreviewPush resolves where branch would be pushed and what it would
// send. A branch with an existing upstream pushes there, and remote and
// target must be empty; otherwise it is published to remote (default:
// remote.pushDefault, origin, the only remote) as target (default: its own
// name). Only an unblocked preview is confirmed.
func (s *Service) PreviewPush(ctx context.Context, repositoryID, branch, remote, target string) (PushPreview, error) {
	ctx, cancel := context.WithTimeout(ctx, readTimeout)
	defer cancel()
	r, err := s.repository(repositoryID)
	if err != nil {
		return PushPreview{}, err
	}
	if err := acquire(ctx, r); err != nil {
		return PushPreview{}, err
	}
	defer release(r)
	state, err := s.pushState(ctx, r, branch, remote, target)
	if err != nil {
		return PushPreview{}, err
	}
	preview := PushPreview{
		Branch: branch, Commit: state.branch.commit, Remote: state.remote, PushURL: state.pushURL, Target: state.target,
		Publish: state.publish, Commits: state.commits, Blockers: state.blockers, Warnings: state.warnings,
	}
	if len(state.blockers) != 0 {
		return preview, nil
	}
	id, expires, err := s.confirm(confirmation{purpose: purposePush, repositoryID: repositoryID, push: &state})
	if err != nil {
		return PushPreview{}, err
	}
	preview.ConfirmationID, preview.ExpiresAt = id, &expires
	return preview, nil
}

// Push pushes the previewed commit, not whatever the branch points at
// now: after checking under the repository gate that the branch and its
// upstream did not move, it releases the gate and pushes by object ID, so
// snapshots and other worktree operations go on meanwhile. A remote's
// refusal is a result, not an error. A published branch gets its upstream
// set afterwards, unless its configuration changed meanwhile.
func (s *Service) Push(ctx context.Context, repositoryID string, req PushRequest) (PushResult, error) {
	c, ok := s.takeConfirmation(req.ConfirmationID, purposePush, repositoryID, "")
	if !ok || c.push == nil {
		return PushResult{}, problemf(ErrConflict, "推送确认已失效，请重新预览")
	}
	r, err := s.repository(repositoryID)
	if err != nil {
		return PushResult{}, err
	}
	current, err := s.recheckPush(ctx, r, *c.push)
	if err != nil {
		return PushResult{}, err
	}
	anchor := r.entry.AnchorPath
	runCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), pushTimeout)
	defer cancel()
	ref := "refs/heads/" + current.target
	stdout, stderr, failure := s.remoteGitOutput(runCtx, anchor, pushArgs(current.remote, current.branch.commit, current.target)...)
	result := PushResult{Status: remoteOK}
	if outcome := s.pushOutcome(stdout, stderr, failure, ref, anchor); outcome != nil {
		result.Status, result.Message = outcome.Status, outcome.Message
		// URLs and Git output stay out of the log: they can hold credentials.
		slog.Warn("branch push failed", "repository", r.entry.ID, "remote", current.remote, "status", outcome.Status)
	} else if current.publish {
		if err := s.setUpstream(r, current); err != nil {
			result.Message = "推送成功，但设置上游失败: " + err.Error()
		}
	}
	result.Repository = s.fetchedRepository(r)
	return result, nil
}

// pushArgs is the argv pushing commit, a full OID, to branch target of
// remote. It never forces, deletes, mirrors, pushes tags or skips hooks.
func pushArgs(remote, commit, target string) []string {
	return []string{"push", "--porcelain", "--end-of-options", remote, commit + ":refs/heads/" + target}
}

// recheckPush reads the push again under r's gate and refuses it unless
// it matches confirmed and nothing blocks it.
func (s *Service) recheckPush(ctx context.Context, r *repositoryState, confirmed pushState) (pushState, error) {
	ctx, cancel := context.WithTimeout(ctx, writeTimeout)
	defer cancel()
	if err := acquire(ctx, r); err != nil {
		return pushState{}, err
	}
	defer release(r)
	current, err := s.pushState(ctx, r, confirmed.branch.name, confirmed.askRemote, confirmed.askTarget)
	if err != nil {
		if kind := errorKind(err); kind == ErrInvalid || kind == ErrNotFound {
			return pushState{}, problemf(ErrConflict, "%v；请重新预览", err)
		}
		return pushState{}, err
	}
	if !samePush(confirmed, current) {
		return pushState{}, problemf(ErrConflict, "分支、上游或推送目标已变化，请重新预览")
	}
	if len(current.blockers) != 0 {
		return pushState{}, problemf(ErrConflict, "推送被阻止：%s；请重新预览", issueMessages(current.blockers))
	}
	return current, ctx.Err()
}

// pushState resolves the push target of branch name and decides the
// blockers and warnings. The caller holds r's gate.
func (s *Service) pushState(ctx context.Context, r *repositoryState, name, askRemote, askTarget string) (pushState, error) {
	if err := s.verifyIdentity(ctx, r); err != nil {
		return pushState{}, err
	}
	anchor := r.entry.AnchorPath
	b, err := s.readBranch(ctx, anchor, name)
	if err != nil {
		return pushState{}, err
	}
	state := pushState{branch: b, askRemote: askRemote, askTarget: askTarget, commits: CommitList{Items: []Commit{}}, blockers: []Issue{}, warnings: []Issue{}}
	if b.upstream != "" && b.upstreamCommit != "" {
		if err := s.upstreamPush(ctx, anchor, &state); err != nil {
			return pushState{}, err
		}
	} else if err := s.publishPush(ctx, r, &state); err != nil {
		return pushState{}, err
	}
	if state.remote != "" {
		if data, err := s.git(ctx, anchor, "remote", "get-url", "--push", "--end-of-options", state.remote); err == nil {
			state.pushURL = redactURL(outputLine(data))
		}
	}
	return state, ctx.Err()
}

// upstreamPush targets the existing upstream of state's branch.
func (s *Service) upstreamPush(ctx context.Context, anchor string, state *pushState) error {
	b := state.branch
	if state.askRemote != "" || state.askTarget != "" {
		return problemf(ErrInvalid, "分支已有上游 %s，只能推送到上游，不能指定远程或目标分支", b.upstreamShort)
	}
	target, onBranch := strings.CutPrefix(b.merge, "refs/heads/")
	if b.remote == "" || b.remote == "." || !onBranch {
		state.blockers = append(state.blockers, Issue{Code: issueUpstream, Message: "上游 " + b.upstreamShort + " 不是远程分支，请在终端推送"})
		return nil
	}
	state.remote, state.target = b.remote, target
	ahead, behind, err := s.aheadBehind(ctx, anchor, b.commit, b.upstreamCommit)
	if err != nil {
		return err
	}
	switch {
	case ahead > 0 && behind > 0:
		state.blockers = append(state.blockers, Issue{Code: issueDiverged, Message: fmt.Sprintf("已分叉（领先 %d、落后 %d），需要在终端合并或变基后再推送", ahead, behind)})
	case behind > 0:
		state.blockers = append(state.blockers, Issue{Code: issueBehind, Message: fmt.Sprintf("分支落后上游 %d 个提交，请先快进", behind)})
	case ahead == 0:
		state.blockers = append(state.blockers, Issue{Code: issueSame, Message: "分支已与上游同步，没有可推送的提交"})
	}
	if ahead > 0 {
		if state.commits, err = s.listCommits(ctx, anchor, b.commit, "^"+b.upstreamCommit); err != nil {
			return err
		}
	}
	if b.pushRemote != "" && b.pushRemote != b.remote {
		state.warnings = append(state.warnings, Issue{Code: issuePushRemote, Message: fmt.Sprintf("WebMux 推送到上游 %s；如需推送到 %s，请在终端操作", b.remote, b.pushRemote)})
	}
	return nil
}

// publishPush targets the requested or default remote for a branch
// without an existing upstream.
func (s *Service) publishPush(ctx context.Context, r *repositoryState, state *pushState) error {
	anchor := r.entry.AnchorPath
	b := state.branch
	state.publish = true
	if b.upstream != "" {
		state.warnings = append(state.warnings, Issue{Code: issueGone, Message: "上游 " + b.upstreamShort + " 已删除；发布会在远端重新创建该分支"})
	}
	remotes, err := s.remoteNames(ctx, r)
	if err != nil {
		return err
	}
	remotes = slices.DeleteFunc(remotes, func(name string) bool { return !plainRevision(name) })
	if state.askRemote != "" {
		if !slices.Contains(remotes, state.askRemote) {
			return problemf(ErrInvalid, "远程不存在: %s", state.askRemote)
		}
		state.remote = state.askRemote
	} else if state.remote, err = s.defaultPushRemote(ctx, anchor, remotes); err != nil {
		return err
	}
	state.target = b.name
	if state.askTarget != "" {
		if err := s.validBranch(ctx, anchor, state.askTarget); err != nil {
			return err
		}
		state.target = state.askTarget
	}
	switch {
	case len(remotes) == 0:
		state.blockers = append(state.blockers, Issue{Code: issueRemote, Message: "仓库没有配置远程，无法发布"})
	case state.remote == "":
		state.blockers = append(state.blockers, Issue{Code: issueRemote, Message: "无法确定发布到哪个远程，请指定其中之一: " + strings.Join(remotes, ", ")})
	}
	state.commits, err = s.listCommits(ctx, anchor, b.commit, "--not", "--remotes")
	return err
}

// defaultPushRemote picks the remote a branch is published to:
// remote.pushDefault, origin, then the only remote; "" when none decides.
func (s *Service) defaultPushRemote(ctx context.Context, anchor string, remotes []string) (string, error) {
	configured, _, err := s.configValue(ctx, anchor, "remote.pushDefault")
	if err != nil {
		return "", err
	}
	for _, name := range []string{configured, "origin"} {
		if name != "" && slices.Contains(remotes, name) {
			return name, nil
		}
	}
	if len(remotes) == 1 {
		return remotes[0], nil
	}
	return "", nil
}

// configValue reads one Git configuration value; set is false when the
// key is not set.
func (s *Service) configValue(ctx context.Context, cwd, key string) (value string, set bool, err error) {
	if s.gitPath == "" {
		return "", false, problemf(ErrUnavailable, "%s", s.Capability().GitError)
	}
	stdout, run := s.runGit(ctx, cwd, nil, "config", "--get", key)
	switch {
	case run.err == nil:
		return outputLine(stdout.Bytes()), true, nil
	case run.ctxErr != nil:
		return "", false, run.ctxErr
	}
	var exit *exec.ExitError
	if errors.As(run.err, &exit) && exit.ExitCode() == 1 {
		return "", false, nil
	}
	return "", false, problemf(ErrInternal, "Git config 读取失败: %s", strings.TrimSpace(run.stderr))
}

// samePush compares what a push confirmation binds: the branch commit,
// the upstream commit, the remote, the target and whether it publishes.
func samePush(a, b pushState) bool {
	return a.branch.name == b.branch.name && a.branch.commit == b.branch.commit && a.branch.upstreamCommit == b.branch.upstreamCommit &&
		a.remote == b.remote && a.target == b.target && a.publish == b.publish
}

// pushOutcome explains a push of ref from its porcelain stdout, its stderr
// and remoteGit's classification; nil means ref was pushed.
func (s *Service) pushOutcome(stdout []byte, stderr string, failure *RemoteFailure, ref, anchor string) *RemoteFailure {
	flag, summary, found := pushStatusLine(string(stdout), ref)
	if found && flag == '!' {
		switch {
		case strings.Contains(summary, "non-fast-forward") || strings.Contains(summary, "fetch first"):
			return &RemoteFailure{Status: remoteRejected, Message: remoteMessage(pushBehindText, "")}
		case strings.HasPrefix(summary, "[remote rejected]"):
			reason := strings.TrimSuffix(strings.TrimPrefix(strings.TrimSpace(strings.TrimPrefix(summary, "[remote rejected]")), "("), ")")
			return &RemoteFailure{Status: remoteRejected, Message: remoteMessage(pushRejectedText, reason)}
		default:
			return &RemoteFailure{Status: remoteRejected, Message: remoteMessage(pushRejectedText, summary)}
		}
	}
	if failure == nil {
		return nil
	}
	switch failure.Status {
	case remoteAuth, remoteNetwork, remoteTimeout:
		return failure
	}
	if !found && s.prePushHook(anchor) {
		return &RemoteFailure{Status: remoteRejected, Message: remoteMessage(pushHookText, firstLine(stderr))}
	}
	return failure
}

// pushStatusLine finds the porcelain status line "FLAG\tFROM:TO\tSUMMARY"
// of ref.
func pushStatusLine(stdout, ref string) (byte, string, bool) {
	for _, line := range strings.Split(stdout, "\n") {
		fields := strings.SplitN(line, "\t", 3)
		if len(fields) != 3 || len(fields[0]) != 1 {
			continue
		}
		if _, to, ok := strings.Cut(fields[1], ":"); ok && to == ref {
			return fields[0][0], fields[2], true
		}
	}
	return 0, "", false
}

// prePushHook reports whether Git would run a pre-push hook, following
// core.hooksPath; a relative path is relative to the anchor worktree.
func (s *Service) prePushHook(anchor string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), readTimeout)
	defer cancel()
	data, err := s.git(ctx, anchor, "rev-parse", "--git-path", "hooks/pre-push")
	if err != nil {
		return false
	}
	path := outputLine(data)
	if !filepath.IsAbs(path) {
		path = filepath.Join(anchor, path)
	}
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular() && info.Mode()&0o111 != 0
}

// setUpstream makes the published target the branch's upstream, unless
// the branch is gone or its upstream configuration changed since the
// preview.
func (s *Service) setUpstream(r *repositoryState, pushed pushState) error {
	ctx, cancel := context.WithTimeout(context.Background(), writeTimeout)
	defer cancel()
	if err := acquire(ctx, r); err != nil {
		return err
	}
	defer release(r)
	anchor, name := r.entry.AnchorPath, pushed.branch.name
	if tip, err := s.resolveCommit(ctx, anchor, "refs/heads/"+name); err != nil || tip == "" {
		return problemf(ErrConflict, "分支已不存在")
	}
	section := "branch." + name
	remote, _, err := s.configValue(ctx, anchor, section+".remote")
	if err != nil {
		return err
	}
	merge, _, err := s.configValue(ctx, anchor, section+".merge")
	if err != nil {
		return err
	}
	if remote != pushed.branch.remote || merge != pushed.branch.merge {
		return problemf(ErrConflict, "分支的上游配置已变化，未修改")
	}
	if _, err := s.git(ctx, anchor, "config", section+".remote", pushed.remote); err != nil {
		return err
	}
	_, err = s.git(ctx, anchor, "config", section+".merge", "refs/heads/"+pushed.target)
	return err
}
