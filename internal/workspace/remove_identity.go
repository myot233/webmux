package workspace

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/rainchan/webmux/internal/session"
)

type identity struct {
	root            os.FileInfo
	admin           os.FileInfo
	common          os.FileInfo
	gitFile         os.FileInfo
	adminPath       string
	gitContents     string
	prunableMissing bool
	// Holding the objects prevents inode reuse from making a removed/recreated
	// checkout compare equal to an earlier confirmation.
	handles []*os.File
}

func (id *identity) hold(path string) (os.FileInfo, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	id.handles = append(id.handles, file)
	return file.Stat()
}
func (id *identity) close() {
	for _, file := range id.handles {
		_ = file.Close()
	}
	id.handles = nil
}

// blockerKind tells removal and release which blockers they share.
type blockerKind int

const (
	blockUnsupported blockerKind = iota
	blockDirty
	blockUntracked
	blockSessions
)

type blocker struct {
	kind    blockerKind
	message string
}

type removalState struct {
	tree     Worktree
	identity identity
	sessions []session.Info
	blockers []blocker
	// warnings do not block removal or release; the preview shows them.
	warnings  []string
	supported bool
}

// messages lists the blocker messages except those of kinds skip; never nil.
func (st removalState) messages(skip ...blockerKind) []string {
	out := []string{}
	for _, b := range st.blockers {
		if !slices.Contains(skip, b.kind) {
			out = append(out, b.message)
		}
	}
	return out
}

func (s *Service) identify(ctx context.Context, r *repositoryState, tree Worktree) (id identity, err error) {
	defer func() {
		if err != nil {
			id.close()
		}
	}()
	id.root, err = id.hold(tree.Path)
	if err != nil {
		return id, err
	}
	path, err := canonical(tree.Path)
	if err != nil || path != tree.Path {
		return id, fmt.Errorf("工作树规范路径身份已改变: %v", err)
	}
	data, err := s.git(ctx, tree.Path, "rev-parse", "--absolute-git-dir")
	if err != nil {
		return id, err
	}
	id.adminPath, err = canonical(outputLine(data))
	if err != nil {
		return id, err
	}
	if !within(r.entry.CommonDir, id.adminPath) || id.adminPath == r.entry.CommonDir {
		return id, errors.New("linked worktree 管理目录身份无效")
	}
	id.admin, err = id.hold(id.adminPath)
	if err != nil {
		return id, err
	}
	id.common, err = id.hold(r.entry.CommonDir)
	if err != nil {
		return id, err
	}
	file := filepath.Join(tree.Path, ".git")
	id.gitFile, err = os.Lstat(file)
	if err != nil {
		return id, err
	}
	if !id.gitFile.Mode().IsRegular() {
		return id, errors.New("linked worktree 的 .git 不是普通文件")
	}
	held, err := id.hold(file)
	if err != nil {
		return id, err
	}
	if !os.SameFile(id.gitFile, held) {
		return id, errors.New(".git 文件身份在读取期间改变")
	}
	contents, err := os.ReadFile(file)
	if err != nil {
		return id, err
	}
	id.gitContents = string(contents)
	return id, nil
}
func findWorktreeAdmin(commonDir, targetPath string) (string, string, error) {
	worktreesDir := filepath.Join(commonDir, "worktrees")
	entries, err := os.ReadDir(worktreesDir)
	if err != nil {
		return "", "", err
	}
	var matchedPath, matchedContents string
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		candidateDir := filepath.Join(worktreesDir, entry.Name())
		data, readErr := os.ReadFile(filepath.Join(candidateDir, "gitdir"))
		if readErr != nil {
			continue
		}
		raw := strings.TrimRight(string(data), "\r\n")
		if raw == "" {
			continue
		}
		if !filepath.IsAbs(raw) {
			raw = filepath.Join(candidateDir, raw)
		}
		wtPath := filepath.Dir(filepath.Clean(raw))
		if resolved, resolveErr := canonicalTarget(wtPath); resolveErr == nil {
			wtPath = resolved
		}
		if wtPath != targetPath {
			continue
		}
		adminPath, canonErr := canonical(candidateDir)
		if canonErr != nil {
			return "", "", canonErr
		}
		if !within(commonDir, adminPath) || adminPath == commonDir {
			return "", "", errors.New("linked worktree 管理目录身份无效")
		}
		if matchedPath != "" {
			return "", "", errors.New("存在多个匹配的 worktree 管理目录")
		}
		matchedPath = adminPath
		matchedContents = string(data)
	}
	if matchedPath == "" {
		return "", "", errors.New("未找到对应的 worktree 管理目录")
	}
	return matchedPath, matchedContents, nil
}

func (s *Service) identifyPrunable(r *repositoryState, tree Worktree) (id identity, err error) {
	defer func() {
		if err != nil {
			id.close()
		}
	}()
	if _, statErr := os.Lstat(tree.Path); !errors.Is(statErr, os.ErrNotExist) {
		return id, fmt.Errorf("待清理工作树路径仍存在或不可验证: %v", statErr)
	}
	resolved, err := canonicalTarget(tree.Path)
	if err != nil || resolved != tree.Path {
		return id, fmt.Errorf("工作树规范路径身份已改变: %v", err)
	}
	id.common, err = id.hold(r.entry.CommonDir)
	if err != nil {
		return id, err
	}
	adminPath, gitdirContents, err := findWorktreeAdmin(r.entry.CommonDir, tree.Path)
	if err != nil {
		return id, err
	}
	id.adminPath = adminPath
	id.admin, err = id.hold(adminPath)
	if err != nil {
		return id, err
	}
	id.gitContents = gitdirContents
	id.prunableMissing = true
	return id, nil
}

func sameIdentity(a, b identity) bool {
	if a.prunableMissing || b.prunableMissing {
		return a.prunableMissing && b.prunableMissing &&
			a.admin != nil && b.admin != nil && a.common != nil && b.common != nil &&
			os.SameFile(a.admin, b.admin) && os.SameFile(a.common, b.common) &&
			a.adminPath == b.adminPath && a.gitContents == b.gitContents
	}
	return a.root != nil && b.root != nil && a.admin != nil && b.admin != nil && a.common != nil && b.common != nil && a.gitFile != nil && b.gitFile != nil &&
		os.SameFile(a.root, b.root) && os.SameFile(a.admin, b.admin) && os.SameFile(a.common, b.common) && os.SameFile(a.gitFile, b.gitFile) && a.adminPath == b.adminPath && a.gitContents == b.gitContents
}
func sameText(a, b *string) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}
func sameTarget(a, b removalState) bool {
	return sameIdentity(a.identity, b.identity) && a.supported == b.supported &&
		a.tree.ID == b.tree.ID && a.tree.Path == b.tree.Path && a.tree.Kind == b.tree.Kind &&
		sameText(a.tree.Branch, b.tree.Branch) && sameText(a.tree.Head, b.tree.Head) && a.tree.Detached == b.tree.Detached &&
		a.tree.Locked == b.tree.Locked && a.tree.LockReason == b.tree.LockReason && a.tree.Prunable == b.tree.Prunable && a.tree.Available == b.tree.Available
}
func (s *Service) removalState(ctx context.Context, r *repositoryState, worktreeID string) (removalState, error) {
	tree, err := s.currentTree(ctx, r, worktreeID)
	if err != nil {
		return removalState{}, err
	}
	return s.removalStateFromTree(ctx, r, tree)
}

func (s *Service) removalStateFromTree(ctx context.Context, r *repositoryState, tree Worktree) (removalState, error) {
	var err error
	state := removalState{tree: tree, sessions: s.targetSessions(tree.Path), blockers: []blocker{}, supported: true}
	block := func(message string) {
		state.supported = false
		state.blockers = append(state.blockers, blocker{kind: blockUnsupported, message: message})
	}
	add := func(kind blockerKind, message string) {
		state.blockers = append(state.blockers, blocker{kind: kind, message: message})
	}
	isMissingPrunable := false
	if tree.Kind == "linked" && !tree.Locked && tree.Prunable && !tree.Available {
		if _, statErr := os.Lstat(tree.Path); errors.Is(statErr, os.ErrNotExist) {
			isMissingPrunable = true
		}
	}
	if tree.Kind != "linked" {
		block("主工作区和 bare 仓库不能在页面删除")
	}
	if tree.Locked {
		block("工作树已锁定，请先在终端解锁: " + tree.LockReason)
	}
	if isMissingPrunable {
		if state.supported {
			state.identity, err = s.identifyPrunable(r, tree)
			if err != nil {
				block("无法验证待清理工作树身份: " + err.Error())
			}
		}
	} else {
		if !tree.Available || tree.Prunable {
			block("工作树目录缺失或不可验证，请在终端处理")
		}
		if tree.Dirty == nil || tree.Error != "" {
			block("工作树状态未知，不能确认安全删除: " + tree.Error)
		}
		if state.supported {
			state.identity, err = s.identify(ctx, r, tree)
			if err != nil {
				block("无法验证工作树身份: " + err.Error())
			}
			if err := s.unsupportedContents(ctx, tree); err != nil {
				block(err.Error())
			}
		}
	}
	if !state.supported {
		state.identity.close()
	}
	if tree.Dirty != nil && *tree.Dirty {
		add(blockDirty, "存在未提交修改")
	}
	if tree.HasUntracked {
		add(blockUntracked, "存在未跟踪文件")
	}
	if tree.HasIgnored {
		state.warnings = append(state.warnings, "存在忽略文件（可能包含 .env 或构建产物），删除后将一并丢失")
	}
	for _, info := range state.sessions {
		if info.Status != session.StatusExited {
			add(blockSessions, "存在运行中的 WebMux 会话")
			break
		}
	}
	if ctx.Err() != nil {
		state.identity.close()
		return state, ctx.Err()
	}
	return state, nil
}
