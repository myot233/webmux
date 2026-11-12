package workspace

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/rainchan/webmux/internal/pathutil"
)

const (
	maxWorkspaceGitOutput = 32 << 20
	maxWorkspaceGitError  = 64 << 10
)

// gitEnv keeps every workspace Git call noninteractive and its output in
// the fixed English the callers parse.
var gitEnv = []string{"GIT_TERMINAL_PROMPT=0", "GCM_INTERACTIVE=never", "LC_ALL=C"}

func (s *Service) git(ctx context.Context, cwd string, args ...string) ([]byte, error) {
	out, _, err := s.gitOutput(ctx, cwd, args...)
	return out, err
}

// gitOutput is git that also returns what Git wrote to standard error, for
// callers that explain a failure themselves; the error is git's.
func (s *Service) gitOutput(ctx context.Context, cwd string, args ...string) ([]byte, string, error) {
	if s.gitPath == "" {
		return nil, "", problemf(ErrUnavailable, "%s", s.Capability().GitError)
	}
	stdout, run := s.runGit(ctx, cwd, nil, args...)
	if err := run.err; err != nil {
		if stdout.exceeded || run.stderrExceeded {
			return nil, run.stderr, problemf(ErrTooLarge, "Git 输出超过限制，实际状态可能已变化；请刷新")
		}
		if run.ctxErr != nil {
			return nil, run.stderr, problemf(ErrTimeout, "Git 操作取消或超时，实际状态可能已变化: %v", run.ctxErr)
		}
		var launch *exec.Error
		if errors.As(err, &launch) || errors.Is(err, os.ErrNotExist) || errors.Is(err, os.ErrPermission) {
			return nil, run.stderr, problemf(ErrUnavailable, "Git 无法执行: %v", err)
		}
		return nil, run.stderr, problemf(ErrInternal, "Git %s 失败: %s (%v)", args[0], strings.TrimSpace(run.stderr), err)
	}
	return stdout.Bytes(), run.stderr, nil
}

// runGit runs `git -C cwd args...` with gitEnv plus extra and bounded
// output; callers map the exit themselves. s.gitPath must be set.
func (s *Service) runGit(ctx context.Context, cwd string, extra []string, args ...string) (*boundedOutput, gitExit) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	stdout := &boundedOutput{limit: maxWorkspaceGitOutput, cancel: cancel}
	run := runGitProcess(ctx, gitProcess{
		path:        s.gitPath,
		args:        append([]string{"-C", cwd}, args...),
		env:         append(append([]string{}, gitEnv...), extra...),
		stdout:      stdout,
		stderrLimit: maxWorkspaceGitError,
	})
	return stdout, run
}
func canonical(path string) (string, error) {
	path, err := pathutil.Expand(path)
	if err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(path)
}

// canonicalTarget resolves existing ancestors too, including symlinked parents.
func canonicalTarget(path string) (string, error) {
	path, err := pathutil.Expand(path)
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err == nil {
		return resolved, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	parent := filepath.Dir(path)
	if parent == path {
		return "", err
	}
	resolved, err = canonicalTarget(parent)
	if err != nil {
		return "", err
	}
	return filepath.Join(resolved, filepath.Base(path)), nil
}
func within(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator)) && !filepath.IsAbs(rel)
}
func overlap(a, b string) bool      { return within(a, b) || within(b, a) }
func outputLine(data []byte) string { return strings.TrimSuffix(string(data), "\n") }
func (s *Service) common(ctx context.Context, path string) (string, error) {
	out, err := s.git(ctx, path, "rev-parse", "--path-format=absolute", "--git-common-dir")
	if err != nil {
		return "", err
	}
	return canonical(outputLine(out))
}
func (s *Service) locate(ctx context.Context, path string) (string, string, error) {
	path, err := canonical(path)
	if err != nil {
		return "", "", problemf(ErrInvalid, "仓库目录不可访问: %v", err)
	}
	common, err := s.common(ctx, path)
	if err != nil {
		if errorKind(err) == ErrInternal {
			err = problemf(ErrInvalid, "无法识别 Git 仓库（请检查目录权限和信任设置）: %v", err)
		}
		return "", "", err
	}
	out, err := s.git(ctx, path, "worktree", "list", "--porcelain", "-z")
	if err != nil {
		return "", "", err
	}
	trees, err := parseWorktrees(out, "")
	if err != nil || len(trees) == 0 {
		return "", "", problemf(ErrUnavailable, "Git 不支持所需 worktree 机器输出: %v", err)
	}
	return common, trees[0].Path, nil
}
func parseWorktrees(data []byte, repository string) ([]Worktree, error) {
	if len(data) == 0 || data[len(data)-1] != 0 {
		return nil, errors.New("worktree 输出不是 NUL 格式")
	}
	out := make([]Worktree, 0)
	var current *Worktree
	for _, raw := range bytes.Split(data, []byte{0}) {
		field := string(raw)
		if field == "" {
			current = nil
			continue
		}
		key, value, _ := strings.Cut(field, " ")
		if key == "worktree" {
			if !filepath.IsAbs(value) {
				return nil, errors.New("工作树路径不是绝对路径（来自 Git）")
			}
			path, err := canonicalTarget(value)
			if err != nil {
				path = filepath.Clean(value)
			}
			kind := "linked"
			if len(out) == 0 {
				kind = "main"
			}
			out = append(out, Worktree{ID: worktreeID(repository, path), RepositoryID: repository, Path: path, Kind: kind})
			current = &out[len(out)-1]
			continue
		}
		if current == nil {
			return nil, errors.New("工作树记录缺少路径（来自 Git）")
		}
		switch key {
		case "HEAD":
			if strings.Trim(value, "0") != "" {
				current.Head = strptr(value)
			}
		case "branch":
			current.Branch = strptr(strings.TrimPrefix(value, "refs/heads/"))
		case "detached":
			current.Detached = true
		case "bare":
			current.Kind = "bare"
		case "locked":
			current.Locked = true
			current.LockReason = value
		case "prunable":
			current.Prunable = true
		}
	}
	return out, nil
}
func (s *Service) snapshot(ctx context.Context, r *repositoryState) (Repository, error) {
	if err := ctx.Err(); err != nil {
		return Repository{}, err
	}
	r.flightMu.Lock()
	flight := r.flight
	if flight == nil {
		readCtx, cancel := context.WithTimeout(context.Background(), readTimeout)
		flight = &snapshotFlight{done: make(chan struct{}), cancel: cancel}
		r.flight = flight
		go func() {
			defer cancel()
			if err := acquire(readCtx, r); err != nil {
				flight.err = err
			} else {
				flight.repository = s.readRepository(readCtx, r)
				s.sweepOnce(readCtx, r)
				release(r)
			}
			r.flightMu.Lock()
			if r.flight == flight {
				r.flight = nil
			}
			close(flight.done)
			r.flightMu.Unlock()
		}()
	}
	flight.waiters++
	r.flightMu.Unlock()
	defer func() {
		r.flightMu.Lock()
		flight.waiters--
		if flight.waiters == 0 {
			flight.cancel()
			if r.flight == flight {
				r.flight = nil
			}
		}
		r.flightMu.Unlock()
	}()
	select {
	case <-ctx.Done():
		return Repository{}, ctx.Err()
	case <-flight.done:
		// A shared read may predate an instruction change.
		repository := flight.repository
		repository.InitInstruction = s.initInstruction(r)
		return repository, flight.err
	}
}
func repositoryResult(e registration) Repository {
	return Repository{ID: e.ID, Name: e.DisplayName, CommonDir: e.CommonDir, AnchorPath: e.AnchorPath, Availability: "unavailable", CheckedAt: time.Now().UTC(), Worktrees: []Worktree{}}
}

// unavailableRepository reports a read that failed before Git produced a
// snapshot, so one repository cannot hide the others in a list.
func (s *Service) unavailableRepository(r *repositoryState, err error) Repository {
	result := repositoryResult(r.entry)
	result.InitInstruction = s.initInstruction(r)
	result.Error = err.Error()
	return result
}

func (s *Service) readRepository(ctx context.Context, r *repositoryState) Repository {
	e := r.entry
	result := repositoryResult(e)
	result.InitInstruction = s.initInstruction(r)
	common, err := s.common(ctx, e.AnchorPath)
	if err == nil && common != e.CommonDir {
		err = errors.New("仓库 common directory 身份已改变，请移除并重新注册")
	}
	if err != nil {
		result.Error = err.Error()
		return result
	}
	data, err := s.git(ctx, e.AnchorPath, "worktree", "list", "--porcelain", "-z")
	if err != nil {
		result.Error = err.Error()
		return result
	}
	trees, err := parseWorktrees(data, e.ID)
	if err != nil {
		result.Error = err.Error()
		return result
	}
	// Worktrees have independent indexes, so their status checks run in
	// parallel; the service-wide pool avoids a process burst on large lists.
	limit := s.worktreeGate
	var wg sync.WaitGroup
	for i := range trees {
		wg.Add(1)
		go func(tree *Worktree) {
			defer wg.Done()
			select {
			case limit <- struct{}{}:
			case <-ctx.Done():
				tree.Error = ctx.Err().Error()
				return
			}
			defer func() { <-limit }()
			s.populateWorktreeStatus(ctx, e.CommonDir, tree)
		}(&trees[i])
	}
	wg.Wait()
	s.fillLastCommits(ctx, e.AnchorPath, trees)
	sort.SliceStable(trees, func(i, j int) bool {
		if trees[i].Kind != "linked" {
			return true
		}
		if trees[j].Kind != "linked" {
			return false
		}
		return trees[i].Path < trees[j].Path
	})
	result.Availability, result.Worktrees = "available", trees
	result.LastFetchedAt = lastFetched(e.CommonDir)
	return result
}

func (s *Service) populateWorktreeStatus(ctx context.Context, commonDir string, tree *Worktree) {
	stat, statErr := os.Stat(tree.Path)
	if statErr != nil || !stat.IsDir() {
		tree.Error = fmt.Sprintf("工作树目录不可访问: %v", statErr)
		return
	}
	if tree.Kind == "bare" {
		tree.Available = true
		return
	}
	actual, checkErr := s.common(ctx, tree.Path)
	if checkErr != nil || actual != commonDir {
		tree.Error = fmt.Sprintf("无法验证工作树身份: %v", checkErr)
		return
	}
	tree.Available = true
	tree.Operation = detectOperation(tree.Path)
	data, statusErr := s.git(ctx, tree.Path, "status", "--porcelain=v2", "-z", "--branch", "--untracked-files=normal", "--ignored=matching")
	if statusErr != nil {
		tree.Error = statusErr.Error()
		return
	}
	status, parseErr := parseStatus(data)
	if parseErr != nil {
		tree.Error = parseErr.Error()
		return
	}
	dirty := status.dirty
	tree.Dirty, tree.HasUntracked, tree.HasIgnored = &dirty, status.untracked, status.ignored
	tree.Upstream, tree.Ahead, tree.Behind, tree.UpstreamGone = status.upstream, status.ahead, status.behind, status.upstreamGone
	tree.Changes = &status.changes
}

// fillLastCommits describes every worktree's HEAD with one batched log;
// a failed read only leaves LastCommit empty.
func (s *Service) fillLastCommits(ctx context.Context, anchor string, trees []Worktree) {
	eligible := func(tree Worktree) bool {
		return tree.Available && tree.Kind != "bare" && tree.Head != nil && isObjectID(*tree.Head)
	}
	var heads []string
	seen := map[string]bool{}
	for _, tree := range trees {
		if eligible(tree) && !seen[*tree.Head] {
			seen[*tree.Head] = true
			heads = append(heads, *tree.Head)
		}
	}
	if len(heads) == 0 {
		return
	}
	commits, err := s.describeCommits(ctx, anchor, heads)
	if err != nil {
		return
	}
	byHash := make(map[string]Commit, len(commits))
	for _, commit := range commits {
		byHash[commit.Hash] = commit
	}
	for i := range trees {
		if !eligible(trees[i]) {
			continue
		}
		if commit, ok := byHash[*trees[i].Head]; ok {
			trees[i].LastCommit = &commit
		}
	}
}

// describeCommits reads exactly the given commits, without walking history.
func (s *Service) describeCommits(ctx context.Context, cwd string, oids []string) ([]Commit, error) {
	args := append([]string{"log", "--no-walk=unsorted", "--no-show-signature", "--format=%H%x00%s%x00%an%x00%cI%x00"}, oids...)
	data, err := s.git(ctx, cwd, append(args, "--")...)
	if err != nil {
		return nil, err
	}
	return parseCommitLog(data), nil
}

// verifyIdentity checks that the repository is still registered and its
// anchor still belongs to the registered common directory.
func (s *Service) verifyIdentity(ctx context.Context, r *repositoryState) error {
	if !s.registered(r) {
		return problemf(ErrNotFound, "仓库已移除")
	}
	common, err := s.common(ctx, r.entry.AnchorPath)
	if err == nil && common != r.entry.CommonDir {
		err = errors.New("仓库 common directory 身份已改变，请移除并重新注册")
	}
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return problemf(ErrUnavailable, "%s", err.Error())
	}
	return nil
}

func (s *Service) currentTree(ctx context.Context, r *repositoryState, id string) (Worktree, error) {
	trees, err := s.currentTrees(ctx, r, []string{id})
	if err != nil {
		return Worktree{}, err
	}
	return trees[0], nil
}

func (s *Service) currentTrees(ctx context.Context, r *repositoryState, ids []string) ([]Worktree, error) {
	selected, _, err := s.currentTreesListed(ctx, r, ids)
	return selected, err
}

// currentTreesListed is currentTrees that also returns every listed
// worktree, without its status.
func (s *Service) currentTreesListed(ctx context.Context, r *repositoryState, ids []string) ([]Worktree, []Worktree, error) {
	if err := s.verifyIdentity(ctx, r); err != nil {
		return nil, nil, err
	}
	e := r.entry
	data, err := s.git(ctx, e.AnchorPath, "worktree", "list", "--porcelain", "-z")
	if err != nil {
		if ctx.Err() != nil {
			return nil, nil, ctx.Err()
		}
		return nil, nil, problemf(ErrUnavailable, "%s", err.Error())
	}
	trees, err := parseWorktrees(data, e.ID)
	if err != nil {
		if ctx.Err() != nil {
			return nil, nil, ctx.Err()
		}
		return nil, nil, problemf(ErrUnavailable, "%s", err.Error())
	}
	byID := make(map[string]Worktree, len(trees))
	for _, tree := range trees {
		byID[tree.ID] = tree
	}
	selected := make([]Worktree, len(ids))
	for i, id := range ids {
		tree, ok := byID[id]
		if !ok {
			return nil, nil, problemf(ErrNotFound, "工作树不存在，请刷新")
		}
		selected[i] = tree
	}
	limit := s.worktreeGate
	var wg sync.WaitGroup
	for i := range selected {
		wg.Add(1)
		go func(tree *Worktree) {
			defer wg.Done()
			select {
			case limit <- struct{}{}:
			case <-ctx.Done():
				tree.Error = ctx.Err().Error()
				return
			}
			defer func() { <-limit }()
			s.populateWorktreeStatus(ctx, e.CommonDir, tree)
		}(&selected[i])
	}
	wg.Wait()
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	return selected, trees, nil
}
func (s *Service) current(ctx context.Context, r *repositoryState) (Repository, error) {
	if !s.registered(r) {
		return Repository{}, problemf(ErrNotFound, "仓库已移除")
	}
	repository := s.readRepository(ctx, r)
	if repository.Availability != "available" {
		if ctx.Err() != nil {
			return repository, ctx.Err()
		}
		return repository, problemf(ErrUnavailable, "%s", repository.Error)
	}
	return repository, nil
}
func findTree(repository Repository, id string) (Worktree, error) {
	for _, tree := range repository.Worktrees {
		if tree.ID == id {
			return tree, nil
		}
	}
	return Worktree{}, problemf(ErrNotFound, "工作树不存在，请刷新")
}
