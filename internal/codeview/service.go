package codeview

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"github.com/rainchan/webmux/internal/artifacts"
	"github.com/rainchan/webmux/internal/session"
	"github.com/rainchan/webmux/internal/workspace"
	"golang.org/x/sys/unix"
)

type binding struct {
	Context
	identity       os.FileInfo
	gitDirs        []string
	gitIdentity    os.FileInfo
	commonIdentity os.FileInfo
}
type snapshot struct {
	session           string
	context           string
	file              *File
	diff              *Diff
	size              int
	created, timeUsed time.Time
}

// The roots a session can browse: its workspace (launch directory or the
// Git worktree containing it) and its interaction artifacts folder.
const (
	RootWorkspace = "workspace"
	RootArtifacts = "artifacts"
)

type rootKey struct{ session, root string }

type Service struct {
	manager   *session.Manager
	artifacts *artifacts.Service
	mu        sync.Mutex
	roots     map[rootKey]*binding
	snapshots map[string]*snapshot
	bytes     int
	slots     chan struct{}
	transfers chan struct{}
	done      chan struct{}
	once      sync.Once
}

// Limit concurrent Git/blob materialization to two jobs and raw downloads to
// four transfers, bounding CPU, open descriptors and per-request memory.
const (
	materializationConcurrency = 2
	transferConcurrency        = 4
)

// New browses sessions of m. A nil folders leaves the artifacts root
// unavailable.
func New(m *session.Manager, folders *artifacts.Service) *Service {
	s := &Service{manager: m, artifacts: folders, roots: map[rootKey]*binding{}, snapshots: map[string]*snapshot{}, slots: make(chan struct{}, materializationConcurrency), transfers: make(chan struct{}, transferConcurrency), done: make(chan struct{})}
	go s.sweep()
	return s
}
func (s *Service) Close() {
	s.once.Do(func() {
		close(s.done)
		s.mu.Lock()
		clear(s.roots)
		clear(s.snapshots)
		s.bytes = 0
		s.mu.Unlock()
	})
}
func (s *Service) sweep() {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-s.done:
			return
		case <-s.manager.Done():
			s.Close()
			return
		case <-ticker.C:
			s.mu.Lock()
			for key := range s.roots {
				if _, err := s.manager.Get(key.session); err != nil {
					delete(s.roots, key)
				}
			}
			for id, v := range s.snapshots {
				if _, err := s.manager.Get(v.session); err != nil || time.Since(v.created) > Lifetime {
					s.bytes -= v.size
					delete(s.snapshots, id)
				}
			}
			s.mu.Unlock()
		}
	}
}

// fallbackIDs keeps ids unique if the system random source ever fails.
var fallbackIDs atomic.Uint64

func random() string {
	var b [18]byte
	if _, err := rand.Read(b[:]); err != nil {
		n := fallbackIDs.Add(1)
		return "fallback-" + strconv.FormatInt(time.Now().UnixNano(), 36) + "-" + strconv.FormatUint(n, 36)
	}
	return base64.RawURLEncoding.EncodeToString(b[:])
}
func digest(b []byte) string { v := sha256.Sum256(b); return hex.EncodeToString(v[:]) }
func nodeID(p string) string { return base64.RawURLEncoding.EncodeToString([]byte(p)) }
func nodePath(id string) (string, error) {
	b, err := base64.RawURLEncoding.DecodeString(id)
	p := string(b)
	if err != nil || strings.ContainsRune(p, 0) || filepath.IsAbs(p) {
		return "", problemf(ErrInvalid, "无效文件标识")
	}
	if p != "" {
		for _, part := range strings.Split(p, "/") {
			if part == "" || part == "." || part == ".." || strings.EqualFold(part, ".git") {
				return "", problemf(ErrInvalid, "路径越界或 Git 内部路径")
			}
		}
	}
	return p, nil
}
func (s *Service) acquire(ctx context.Context) (func(), error) {
	select {
	case s.slots <- struct{}{}:
		return func() { <-s.slots }, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-s.done:
		return nil, problemf(ErrUnavailable, "代码服务已关闭")
	}
}
func git(ctx context.Context, root string, args ...string) ([]byte, error) {
	return workspace.ReadGit(ctx, root, MaxBytes, args...)
}

// bind performs filesystem and Git validation; callers must hold a service slot.
func (s *Service) bind(ctx context.Context, id, kind string) (*binding, error) {
	if kind == RootArtifacts {
		return s.bindArtifacts(id)
	}
	item, err := s.manager.Get(id)
	if err != nil {
		return nil, problemf(ErrNotFound, "会话不存在")
	}
	key := rootKey{id, RootWorkspace}
	s.mu.Lock()
	b := s.roots[key]
	s.mu.Unlock()
	if b != nil {
		return b, s.validate(ctx, b)
	}
	cwd, err := filepath.EvalSymlinks(item.Info().Cwd)
	if err != nil {
		return nil, err
	}
	for _, segment := range strings.Split(cwd, string(os.PathSeparator)) {
		if strings.EqualFold(segment, ".git") {
			return nil, problemf(ErrConflict, "会话启动目录位于 Git 内部，代码浏览不可用")
		}
	}
	if raw, e := git(ctx, cwd, "rev-parse", "--is-inside-git-dir"); e == nil && strings.TrimSpace(string(raw)) == "true" {
		return nil, problemf(ErrConflict, "Git 管理目录不可作为浏览根")
	}
	root := cwd
	b = &binding{Context: Context{ID: random(), Kind: "directory", Browse: true, Reason: "普通目录或 Git 不可用；忽略规则与差异不可用"}}
	if out, e := git(ctx, cwd, "rev-parse", "--show-toplevel"); e == nil {
		root = strings.TrimSuffix(string(out), "\n")
		root, e = filepath.EvalSymlinks(root)
		if e != nil {
			return nil, e
		}
		b.Kind = "worktree"
		b.Diff = true
		b.Ignore = true
		b.Reason = ""
		for _, arg := range []string{"--absolute-git-dir", "--git-common-dir"} {
			out, e = git(ctx, root, "rev-parse", "--path-format=absolute", arg)
			if e != nil {
				return nil, e
			}
			p, e := filepath.EvalSymlinks(strings.TrimSuffix(string(out), "\n"))
			if e != nil {
				return nil, e
			}
			b.gitDirs = append(b.gitDirs, p)
		}
		b.commonIdentity, err = os.Stat(b.gitDirs[1])
		if err != nil {
			return nil, err
		}
		b.gitIdentity, err = os.Stat(b.gitDirs[0])
		if err != nil {
			return nil, err
		}
	}
	if linked := item.Info().Workspace; linked != nil && b.Diff {
		expected, e := filepath.EvalSymlinks(linked.WorktreePath)
		if e != nil || expected != root {
			return nil, problemf(ErrConflict, "工作区关联与浏览根不一致")
		}
	}
	b.Root = root
	b.identity, err = os.Stat(root)
	if err != nil {
		return nil, err
	}
	b.Write = writable(root)
	// A canceled Git probe must not be cached as a plain-directory fallback.
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.Lock()
	if old := s.roots[key]; old != nil {
		b = old
	} else {
		s.roots[key] = b
	}
	s.mu.Unlock()
	return b, s.validate(ctx, b)
}

// bindArtifacts binds session id's artifacts folder: a plain directory
// without Git, ignore rules or diffs. A folder replaced since it was bound
// gets a new binding, so requests against the old context conflict.
func (s *Service) bindArtifacts(id string) (*binding, error) {
	if _, err := s.manager.Get(id); err != nil {
		return nil, problemf(ErrNotFound, "会话不存在")
	}
	if s.artifacts == nil {
		return nil, problemf(ErrUnavailable, "临时文件夹不可用")
	}
	dir, err := s.artifacts.Existing(id)
	switch {
	case errors.Is(err, artifacts.ErrNotFound):
		return nil, problemf(ErrNotFound, "临时文件夹尚未创建")
	case err != nil:
		return nil, problemf(ErrUnavailable, "临时文件夹不可用")
	}
	key := rootKey{id, RootArtifacts}
	s.mu.Lock()
	defer s.mu.Unlock()
	if b := s.roots[key]; b != nil {
		if s.validate(context.Background(), b) == nil {
			return b, nil
		}
		delete(s.roots, key)
	}
	identity, err := os.Lstat(dir)
	if err != nil || !identity.IsDir() {
		return nil, problemf(ErrUnavailable, "临时文件夹不可用")
	}
	b := &binding{Context: Context{ID: random(), Root: dir, Kind: RootArtifacts, Browse: true, Write: writable(dir)}, identity: identity}
	s.roots[key] = b
	return b, nil
}
func (s *Service) validate(ctx context.Context, b *binding) error {
	stat, err := os.Lstat(b.Root)
	if err != nil || !stat.IsDir() || !os.SameFile(b.identity, stat) {
		return problemf(ErrConflict, "浏览根身份已失效")
	}
	if b.Diff {
		top, e := git(ctx, b.Root, "rev-parse", "--show-toplevel")
		if e != nil {
			return e
		}
		actual, e := filepath.EvalSymlinks(strings.TrimSuffix(string(top), "\n"))
		if e != nil || actual != b.Root {
			return problemf(ErrConflict, "工作树根来源已改变")
		}
		common, e := git(ctx, b.Root, "rev-parse", "--path-format=absolute", "--git-common-dir")
		if e != nil {
			return e
		}
		commonPath, e := filepath.EvalSymlinks(strings.TrimSuffix(string(common), "\n"))
		if e != nil || commonPath != b.gitDirs[1] {
			return problemf(ErrConflict, "仓库 common directory 已改变")
		}
		commonInfo, e := os.Stat(commonPath)
		if e != nil || !os.SameFile(commonInfo, b.commonIdentity) {
			return problemf(ErrConflict, "仓库 common directory 已替换")
		}
		out, e := git(ctx, b.Root, "rev-parse", "--absolute-git-dir")
		if e != nil {
			return e
		}
		p, e := filepath.EvalSymlinks(strings.TrimSuffix(string(out), "\n"))
		if e != nil || p != b.gitDirs[0] {
			return problemf(ErrConflict, "Git 来源身份已改变")
		}
		stat, e = os.Stat(p)
		if e != nil || !os.SameFile(stat, b.gitIdentity) {
			return problemf(ErrConflict, "Git 管理目录已替换")
		}
	}
	return nil
}

// Context returns the browsing context of session id's root: RootWorkspace
// (also for an empty root) or RootArtifacts.
func (s *Service) Context(ctx context.Context, id, root string) (Context, error) {
	switch root {
	case "":
		root = RootWorkspace
	case RootWorkspace, RootArtifacts:
	default:
		return Context{}, problemf(ErrInvalid, "未知的浏览根")
	}
	return withBinding(ctx, s, id, root, func(b *binding) (Context, error) { return b.Context, nil })
}

// withBinding holds a service slot while fn uses session id's validated
// binding of root. Failing to acquire the slot or bind returns T's zero value.
func withBinding[T any](ctx context.Context, s *Service, id, root string, fn func(*binding) (T, error)) (T, error) {
	release, err := s.acquire(ctx)
	if err != nil {
		var zero T
		return zero, err
	}
	defer release()
	b, err := s.bind(ctx, id, root)
	if err != nil {
		var zero T
		return zero, err
	}
	return fn(b)
}

// withRoot is withBinding for requests made against browsing context cid; it
// rejects them once the session's context has been replaced. cid selects
// the root: the artifacts folder's current context, else the workspace.
func withRoot[T any](ctx context.Context, s *Service, id, cid string, fn func(*binding) (T, error)) (T, error) {
	s.mu.Lock()
	root := s.rootOfLocked(id, cid)
	s.mu.Unlock()
	return withBinding(ctx, s, id, root, func(b *binding) (T, error) {
		if b.ID != cid {
			var zero T
			return zero, problemf(ErrConflict, "浏览上下文已改变")
		}
		return fn(b)
	})
}

func (s *Service) rootOfLocked(id, cid string) string {
	if b := s.roots[rootKey{id, RootArtifacts}]; b != nil && b.ID == cid {
		return RootArtifacts
	}
	return RootWorkspace
}

// snapshotRoot is the root snapshot sid was taken from, for re-reading it.
func (s *Service) snapshotRoot(id, sid string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if v := s.snapshots[sid]; v != nil && v.session == id {
		return s.rootOfLocked(id, v.context)
	}
	return RootWorkspace
}

// Open each segment relative to an already opened directory. O_NOFOLLOW and
// O_NONBLOCK prevent symlink races and blocking special-file opens on Unix.
func boundary(fd int) bool {
	var stat unix.Stat_t
	return unix.Fstatat(fd, ".git", &stat, unix.AT_SYMLINK_NOFOLLOW) == nil || (unix.Fstatat(fd, "HEAD", &stat, unix.AT_SYMLINK_NOFOLLOW) == nil && unix.Fstatat(fd, "objects", &stat, unix.AT_SYMLINK_NOFOLLOW) == nil)
}
func displayPath(p string) string {
	if utf8.ValidString(p) {
		return p
	}
	return strconv.QuoteToASCII(p)
}
func (b *binding) open(p string) (*os.File, error) {
	if _, err := nodePath(nodeID(p)); err != nil {
		return nil, err
	}
	full := filepath.Join(b.Root, p)
	for _, g := range b.gitDirs {
		if full == g || strings.HasPrefix(full, g+"/") {
			return nil, problemf(ErrInvalid, "Git 管理路径不可读取")
		}
	}
	fd, err := unix.Open(b.Root, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, err
	}
	f := os.NewFile(uintptr(fd), b.Root)
	stat, err := f.Stat()
	if err != nil || !os.SameFile(stat, b.identity) {
		_ = f.Close()
		return nil, problemf(ErrConflict, "浏览根已替换")
	}
	if p == "" {
		return f, nil
	}
	parts := strings.Split(p, "/")
	for i, part := range parts {
		if i > 0 {
			if boundary(int(f.Fd())) {
				_ = f.Close()
				return nil, problemf(ErrInvalid, "嵌套仓库边界不可遍历")
			}
		}
		flags := unix.O_RDONLY | unix.O_NOFOLLOW | unix.O_NONBLOCK | unix.O_CLOEXEC
		if i < len(parts)-1 {
			flags |= unix.O_DIRECTORY
		}
		next, e := unix.Openat(int(f.Fd()), part, flags, 0)
		_ = f.Close()
		if e != nil {
			if os.IsNotExist(e) {
				return nil, os.ErrNotExist
			}
			return nil, problemf(ErrInvalid, "路径不可安全读取: %v", e)
		}
		f = os.NewFile(uintptr(next), full)
	}
	stat, err = f.Stat()
	if err != nil {
		_ = f.Close()
		return nil, err
	}
	if stat.IsDir() && p != "" {
		if boundary(int(f.Fd())) {
			_ = f.Close()
			return nil, problemf(ErrInvalid, "嵌套仓库边界不可遍历")
		}
	}
	if !stat.IsDir() && !stat.Mode().IsRegular() {
		_ = f.Close()
		return nil, problemf(ErrInvalid, "不支持读取特殊文件或链接")
	}
	return f, nil
}
