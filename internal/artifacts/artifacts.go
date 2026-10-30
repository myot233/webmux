// Package artifacts owns each session's interaction artifacts folder: a
// private directory under the system temporary directory where the user
// and the programs in a session exchange files. Uploads land in its
// uploads/ subdirectory; the code view browses it as a second root.
//
// Folders are created on demand and deleted when their session is removed,
// unless a retain predicate keeps them (a recoverable hybrid session). A
// sweep at start-up and after a missed event removes the rest.
package artifacts

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"sync"
	"syscall"

	"github.com/rainchan/webmux/internal/session"
)

// Error kinds; the controller maps them to HTTP statuses.
var (
	ErrNotFound    = errors.New("session artifacts folder not found")
	ErrUnavailable = errors.New("session artifacts folder unavailable")
	ErrInvalid     = errors.New("invalid session artifacts path")
)

// UploadsDir names the subdirectory uploads are stored in.
const UploadsDir = "uploads"

// rootPrefix starts the per-instance root's name under the temp directory.
const rootPrefix = "webmux-artifacts-"

// sessionID matches the manager's generated IDs (eight lowercase base32
// characters); anything else is never a folder name.
var sessionID = regexp.MustCompile(`^[a-z2-7]{8}$`)

// Options configure a Service. Only cmd/ builds them.
type Options struct {
	// TempDir is the system temporary directory; empty uses os.TempDir.
	TempDir string
	// StateDir identifies the instance, so instances sharing TempDir use
	// separate roots.
	StateDir string
	Manager  *session.Manager
	Logger   *slog.Logger
}

// Service resolves, creates and removes the artifacts folders of one server.
type Service struct {
	manager *session.Manager
	log     *slog.Logger
	// root is the canonical per-instance directory; rootErr why it cannot
	// be used.
	root    string
	rootErr error
	// mu serialises creation against removal so a folder is never created
	// for a session that is being removed.
	mu   sync.Mutex
	done chan struct{}
	once sync.Once
	wg   sync.WaitGroup
}

// New resolves the instance root without touching the filesystem beyond
// canonicalising TempDir and StateDir.
func New(o Options) *Service {
	log := o.Logger
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	s := &Service{manager: o.Manager, log: log, done: make(chan struct{})}
	s.root, s.rootErr = instanceRoot(o.TempDir, o.StateDir)
	return s
}

func instanceRoot(tempDir, stateDir string) (string, error) {
	if tempDir == "" {
		tempDir = os.TempDir()
	}
	tmp, err := filepath.EvalSymlinks(tempDir)
	if err != nil {
		return "", fmt.Errorf("%w: 系统临时目录不可用", ErrUnavailable)
	}
	state, err := filepath.Abs(stateDir)
	if err != nil {
		return "", fmt.Errorf("%w: 状态目录无效", ErrUnavailable)
	}
	if real, err := filepath.EvalSymlinks(state); err == nil {
		state = real
	}
	sum := sha256.Sum256([]byte(state))
	name := fmt.Sprintf("%s%d-%s", rootPrefix, os.Geteuid(), hex.EncodeToString(sum[:4]))
	return filepath.Join(tmp, name), nil
}

// Start sweeps folders left by sessions that no longer exist, then deletes
// each removed session's folder as the manager reports it. retain keeps a
// folder whose session left the manager but may come back.
func (s *Service) Start(retain func(id string) bool) {
	s.Sweep(retain)
	s.wg.Add(1)
	go s.watch(retain)
}

// Close stops watching the manager. Folders are kept.
func (s *Service) Close() {
	s.once.Do(func() { close(s.done) })
	s.wg.Wait()
}

func (s *Service) watch(retain func(string) bool) {
	defer s.wg.Done()
	first := true
	for {
		sub := s.manager.Subscribe()
		if !first {
			// Events were dropped while disconnected: reconcile instead.
			s.Sweep(retain)
		}
		first = false
		if !s.follow(sub, retain) {
			s.manager.Unsubscribe(sub)
			return
		}
		s.manager.Unsubscribe(sub)
	}
}

// follow handles events until the subscriber falls behind (true) or the
// service or manager stops (false).
func (s *Service) follow(sub *session.ListSubscriber, retain func(string) bool) bool {
	for {
		select {
		case <-s.done:
			return false
		case <-s.manager.Done():
			return false
		case <-sub.Slow:
			return true
		case e := <-sub.Out:
			if e.Type == "removed" && !retain(e.ID) {
				s.Remove(e.ID)
			}
		}
	}
}

// Path returns session id's folder without creating it. A nil Service has
// no folders: every method reaching here reports ErrUnavailable.
func (s *Service) Path(id string) (string, error) {
	if s == nil {
		return "", fmt.Errorf("%w: 临时文件夹不可用", ErrUnavailable)
	}
	if !sessionID.MatchString(id) {
		return "", fmt.Errorf("%w: 会话不存在", ErrNotFound)
	}
	if s.rootErr != nil {
		return "", s.rootErr
	}
	return filepath.Join(s.root, id), nil
}

// Ensure creates session id's folder if needed and returns its canonical
// path. The session must exist; an exited session still has a folder until
// it is removed.
func (s *Service) Ensure(id string) (string, error) {
	return s.ensure(id, "")
}

// EnsureUploads is Ensure for the folder's uploads/ subdirectory.
func (s *Service) EnsureUploads(id string) (string, error) {
	return s.ensure(id, UploadsDir)
}

func (s *Service) ensure(id, sub string) (string, error) {
	dir, err := s.Path(id)
	if err != nil {
		return "", err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := s.manager.Get(id); err != nil {
		return "", fmt.Errorf("%w: 会话不存在", ErrNotFound)
	}
	for _, p := range []string{s.root, dir, filepath.Join(dir, sub)} {
		if err := private(p, true); err != nil {
			s.log.Warn("session artifacts folder unavailable", "path", p, "error", err)
			return "", fmt.Errorf("%w: %v", ErrUnavailable, err)
		}
	}
	return filepath.Join(dir, sub), nil
}

// Existing returns session id's folder when it has been created, for
// browsing; it creates nothing.
func (s *Service) Existing(id string) (string, error) {
	dir, err := s.Path(id)
	if err != nil {
		return "", err
	}
	for _, p := range []string{s.root, dir} {
		if err := private(p, false); err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return "", fmt.Errorf("%w: 临时文件夹尚未创建", ErrNotFound)
			}
			return "", fmt.Errorf("%w: %v", ErrUnavailable, err)
		}
	}
	return dir, nil
}

// private makes sure p is a directory only the server's user can enter:
// not a symlink, owned by the effective user, without group or other
// permissions. With create it makes a missing p first (0700). Nothing that
// fails the check is changed or removed.
func private(p string, create bool) error {
	if create {
		if err := os.Mkdir(p, 0o700); err != nil && !errors.Is(err, os.ErrExist) {
			return err
		}
	}
	info, err := os.Lstat(p)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return errors.New("临时文件夹路径不是目录")
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || int(stat.Uid) != os.Geteuid() {
		return errors.New("临时文件夹不属于当前用户")
	}
	if info.Mode().Perm()&0o077 != 0 {
		return errors.New("临时文件夹允许其他用户访问")
	}
	return nil
}

// Remove deletes session id's folder and everything in it. Failures are
// logged; removing the session itself has already succeeded.
func (s *Service) Remove(id string) {
	dir, err := s.Path(id)
	if err != nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.removeLocked(dir)
}

func (s *Service) removeLocked(dir string) {
	if err := private(s.root, false); err != nil {
		return
	}
	info, err := os.Lstat(dir)
	if err != nil || !info.IsDir() {
		return
	}
	if err := os.RemoveAll(dir); err != nil {
		s.log.Warn("session artifacts folder removal failed", "path", dir, "error", err)
	}
}

// Sweep removes every folder whose session is neither in the manager nor
// retained. Only directly contained directories named like a session ID
// are touched.
func (s *Service) Sweep(retain func(id string) bool) {
	if s.rootErr != nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := private(s.root, false); err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			s.log.Warn("session artifacts root unusable; not swept", "path", s.root, "error", err)
		}
		return
	}
	entries, err := os.ReadDir(s.root)
	if err != nil {
		s.log.Warn("session artifacts sweep failed", "path", s.root, "error", err)
		return
	}
	for _, entry := range entries {
		id := entry.Name()
		if !entry.IsDir() || !sessionID.MatchString(id) {
			continue
		}
		if _, err := s.manager.Get(id); err == nil || retain(id) {
			continue
		}
		s.removeLocked(filepath.Join(s.root, id))
	}
}

// UploadPath validates that path names a regular, non-symlink file directly
// inside session id's uploads/ directory and returns it cleaned. Anything
// else, including another session's uploads, cannot be an attachment.
func (s *Service) UploadPath(id, path string) (string, error) {
	invalid := fmt.Errorf("%w: 路径不是本会话的上传文件", ErrInvalid)
	dir, err := s.Path(id)
	if err != nil || path == "" || !filepath.IsAbs(path) {
		return "", invalid
	}
	clean := filepath.Clean(path)
	uploads := filepath.Join(dir, UploadsDir)
	if filepath.Dir(clean) != uploads {
		return "", invalid
	}
	for _, p := range []string{s.root, dir, uploads} {
		if private(p, false) != nil {
			return "", invalid
		}
	}
	info, err := os.Lstat(clean)
	if err != nil || !info.Mode().IsRegular() {
		return "", invalid
	}
	return clean, nil
}
