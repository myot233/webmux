package workspace

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/rainchan/webmux/internal/artifacts"
	"github.com/rainchan/webmux/internal/config"
	"github.com/rainchan/webmux/internal/hybrid"
	"github.com/rainchan/webmux/internal/launch"
	"github.com/rainchan/webmux/internal/pathutil"
	"github.com/rainchan/webmux/internal/session"
	"github.com/rainchan/webmux/internal/statefile"
)

// MaxInitInstruction is the most characters a repository's initialization
// instruction may have.
const MaxInitInstruction = 4000

// MaxRepositories bounds repositories.json, like the other stores.
const MaxRepositories = 500

type registration struct {
	ID          string `json:"id"`
	CommonDir   string `json:"commonDir"`
	AnchorPath  string `json:"anchorPath"`
	DisplayName string `json:"displayName"`
	// InitInstruction prefixes a quick start's first prompt. Files written
	// before it existed lack it: no instruction.
	InitInstruction string `json:"initInstruction,omitempty"`
}
type registry struct {
	Version      int            `json:"version"`
	Repositories []registration `json:"repositories"`
}
type repositoryState struct {
	// entry is immutable; its InitInstruction is always empty; the current
	// instruction is instruction.
	entry registration
	// instruction is guarded by Service.mu.
	instruction string
	gate        chan struct{}
	flightMu    sync.Mutex
	flight      *snapshotFlight
	// fetching is the running FetchRemotes; guarded by fetchMu.
	fetchMu  sync.Mutex
	fetching *fetchFlight
	// swept is set once a sweep of orphan release refs ran; guarded by
	// gate.
	swept bool
}
type snapshotFlight struct {
	done       chan struct{}
	repository Repository
	err        error
	cancel     context.CancelFunc
	waiters    int
}
type Service struct {
	manager   *session.Manager
	launches  *launch.Service
	hybrids   *hybrid.Service
	artifacts *artifacts.Service
	// commands lists the saved quick commands an initialization instruction
	// may reference; nil means none exist.
	commands func() []QuickCommand
	// brief writes the {{webmux}} brief of a session; nil leaves an
	// instruction that uses it unavailable.
	brief        func(sessionID, dir string) string
	defaultCwd   string
	gitPath      string
	fetchTimeout time.Duration
	// fetchRemoteTimeout bounds each remote of FetchRemotes.
	fetchRemoteTimeout time.Duration
	stateDir           string
	// worktreeGate bounds concurrent worktree status reads across every
	// repository in a list, not just within one repository.
	worktreeGate chan struct{}
	mu           sync.Mutex
	// saveMu serializes registry read-modify-write cycles without holding mu
	// across the state file commit.
	saveMu        sync.Mutex
	capability    Capability
	entries       []*repositoryState
	store         *statefile.Handle[registry]
	coord         sync.Mutex
	starts        map[string]int
	reservations  map[string]bool
	changed       chan struct{}
	confirmations map[string]confirmation
	creates       creates
}

// New never makes optional Git or storage availability a server startup failure.
// launches resolves saved launch configs; nil makes them unavailable.
func New(stateDir string, manager *session.Manager, launches *launch.Service, defaultCwd string) *Service {
	if stateDir == "" {
		stateDir = config.DefaultStateDir()
	}
	s := &Service{manager: manager, launches: launches, defaultCwd: defaultCwd, stateDir: stateDir, fetchTimeout: branchFetchTimeout, fetchRemoteTimeout: fetchRemoteTimeout, worktreeGate: make(chan struct{}, 4), starts: make(map[string]int), reservations: make(map[string]bool), changed: make(chan struct{}), confirmations: make(map[string]confirmation)}
	git, err := exec.LookPath("git")
	if err != nil {
		s.capability.GitError = "Git 不可用: " + err.Error()
	} else {
		s.gitPath = git
		s.capability.GitAvailable = true
	}
	if err := s.load(); err != nil {
		s.capability.RegistryError = "仓库清单不可用: " + err.Error()
	} else {
		s.capability.RegistryAvailable = true
	}
	return s
}
func (s *Service) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.store.Close()
	s.capability.RegistryAvailable = false
	s.capability.RegistryError = "仓库服务已关闭"
	s.clearConfirmations()
}
func (s *Service) Capability() Capability { s.mu.Lock(); defer s.mu.Unlock(); return s.capability }

// UseHybrid installs, once at assembly before serving, the service that
// starts sessions of hybrid launch configs. Without it they answer
// ErrUnavailable.
func (s *Service) UseHybrid(h *hybrid.Service) { s.hybrids = h }

// UseArtifacts installs, once at assembly before serving, the folders quick
// starts stage uploads in. Without it a quick start takes no attachments.
func (s *Service) UseArtifacts(a *artifacts.Service) { s.artifacts = a }

// UseQuickCommands installs, once at assembly before serving, the source of
// the saved quick commands that initialization instructions reference by
// name.
func (s *Service) UseQuickCommands(list func() []QuickCommand) { s.commands = list }

// UseBrief installs, once at assembly before serving, the writer of the
// brief {{webmux}} expands to for a session and its artifacts folder.
func (s *Service) UseBrief(brief func(sessionID, dir string) string) { s.brief = brief }

func (s *Service) load() error {
	expanded, err := pathutil.Expand(s.stateDir)
	if err != nil {
		return err
	}
	s.stateDir = expanded
	var stored registry
	s.store, stored, err = statefile.Open[registry](s.stateDir, statefile.Repositories, validateRegistry)
	if statefile.IsKind(err, statefile.Missing) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, entry := range stored.Repositories {
		s.entries = append(s.entries, newRepository(entry))
	}
	return nil
}
func validateRegistry(stored registry) error {
	if stored.Version != 1 || stored.Repositories == nil {
		return errors.New("不支持或损坏的仓库清单")
	}
	if len(stored.Repositories) > MaxRepositories {
		return fmt.Errorf("仓库清单超过 %d 个仓库", MaxRepositories)
	}
	ids, paths := map[string]bool{}, map[string]bool{}
	for _, entry := range stored.Repositories {
		if entry.ID == "" || !filepath.IsAbs(entry.CommonDir) || !filepath.IsAbs(entry.AnchorPath) || ids[entry.ID] || paths[entry.CommonDir] {
			return errors.New("仓库清单身份无效或重复")
		}
		ids[entry.ID], paths[entry.CommonDir] = true, true
	}
	return nil
}
func newRepository(entry registration) *repositoryState {
	r := &repositoryState{entry: entry, instruction: entry.InitInstruction, gate: make(chan struct{}, 1)}
	r.entry.InitInstruction = ""
	r.gate <- struct{}{}
	return r
}

// initInstruction is r's current initialization instruction.
func (s *Service) initInstruction(r *repositoryState) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return r.instruction
}

func (s *Service) save(entries []*repositoryState) error {
	return s.saveWith(entries, nil, "")
}

// saveWith writes entries, with instruction in place of changed's.
func (s *Service) saveWith(entries []*repositoryState, changed *repositoryState, instruction string) error {
	stored := registry{Version: 1, Repositories: make([]registration, 0, len(entries))}
	s.mu.Lock()
	for _, entry := range entries {
		item := entry.entry
		item.InitInstruction = entry.instruction
		if entry == changed {
			item.InitInstruction = instruction
		}
		stored.Repositories = append(stored.Repositories, item)
	}
	s.mu.Unlock()
	err := s.store.Commit(stored)
	if statefile.IsKind(err, statefile.CommitUncertain) {
		// Rename already published different bytes. Keep the last confirmed
		// memory state, but never allow it to overwrite the uncertain disk state.
		message := "清单已替换但目录同步失败，持久结果不确定；仓库管理已停用，请重启重新读取: " + err.Error()
		s.mu.Lock()
		s.capability.RegistryAvailable = false
		s.capability.RegistryError = message
		s.mu.Unlock()
		return errors.New(message)
	}
	return err
}

// validInstruction normalizes an initialization instruction: blank is none,
// more than MaxInitInstruction characters is ErrInvalid.
func validInstruction(text string) (string, error) {
	if strings.TrimSpace(text) == "" {
		return "", nil
	}
	if n := utf8.RuneCountInString(text); n > MaxInitInstruction {
		return "", problemf(ErrInvalid, "初始化指令最多 %d 个字符，当前 %d 个", MaxInitInstruction, n)
	}
	return text, nil
}

// SetInitInstruction replaces repository id's initialization instruction;
// blank clears it. The registry is saved first: a failed save leaves the
// instruction as it was.
func (s *Service) SetInitInstruction(ctx context.Context, id, text string) (Repository, error) {
	instruction, err := validInstruction(text)
	if err != nil {
		return Repository{}, err
	}
	r, err := s.repository(id)
	if err != nil {
		return Repository{}, err
	}
	s.saveMu.Lock()
	s.mu.Lock()
	if err := s.requireRegistryLocked(); err != nil {
		s.mu.Unlock()
		s.saveMu.Unlock()
		return Repository{}, err
	}
	entries := append([]*repositoryState(nil), s.entries...)
	s.mu.Unlock()
	found := false
	for _, item := range entries {
		found = found || item == r
	}
	if !found {
		s.saveMu.Unlock()
		return Repository{}, problemf(ErrNotFound, "仓库不存在")
	}
	err = s.saveWith(entries, r, instruction)
	if err == nil {
		s.mu.Lock()
		r.instruction = instruction
		s.mu.Unlock()
	}
	s.saveMu.Unlock()
	if err != nil {
		return Repository{}, fmt.Errorf("保存仓库清单: %w", err)
	}
	ctx, cancel := context.WithTimeout(ctx, readTimeout)
	defer cancel()
	snapshot, err := s.snapshot(ctx, r)
	if err != nil {
		// The instruction is saved; a failed read still answers the entry.
		snapshot = s.unavailableRepository(r, err)
	}
	snapshot.InitInstruction = instruction
	return snapshot, nil
}
func (s *Service) requireRegistryLocked() error {
	if !s.capability.RegistryAvailable {
		return problemf(ErrUnavailable, "%s", s.capability.RegistryError)
	}
	return nil
}
func (s *Service) repository(id string) (*repositoryState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.requireRegistryLocked(); err != nil {
		return nil, err
	}
	for _, r := range s.entries {
		if r.entry.ID == id {
			return r, nil
		}
	}
	return nil, problemf(ErrNotFound, "仓库不存在，请刷新")
}
func (s *Service) registered(r *repositoryState) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, candidate := range s.entries {
		if candidate == r {
			return true
		}
	}
	return false
}

// Register adds the repository at path. A repository already registered is
// returned as it is. Its initialization instruction is set afterwards with
// SetInitInstruction.
func (s *Service) Register(ctx context.Context, path string) (Repository, bool, error) {
	ctx, cancel := context.WithTimeout(ctx, readTimeout)
	defer cancel()
	if err := s.requireRegistry(); err != nil {
		return Repository{}, false, err
	}
	if path == "" {
		return Repository{}, false, problemf(ErrInvalid, "请选择仓库目录")
	}
	common, anchor, err := s.locate(ctx, path)
	if err != nil {
		return Repository{}, false, err
	}
	s.saveMu.Lock()
	s.mu.Lock()
	if err := s.requireRegistryLocked(); err != nil {
		s.mu.Unlock()
		s.saveMu.Unlock()
		return Repository{}, false, err
	}
	for _, r := range s.entries {
		if r.entry.CommonDir == common {
			s.mu.Unlock()
			s.saveMu.Unlock()
			snapshot, err := s.snapshot(ctx, r)
			s.reassociate(ctx)
			return snapshot, false, err
		}
	}
	if len(s.entries) >= MaxRepositories {
		s.mu.Unlock()
		s.saveMu.Unlock()
		return Repository{}, false, problemf(ErrInvalid, "最多登记 %d 个仓库，请先移除不再使用的仓库", MaxRepositories)
	}
	id, err := randomID()
	if err != nil {
		s.mu.Unlock()
		s.saveMu.Unlock()
		return Repository{}, false, err
	}
	r := newRepository(registration{ID: id, CommonDir: common, AnchorPath: anchor, DisplayName: filepath.Base(anchor)})
	next := append(append([]*repositoryState(nil), s.entries...), r)
	s.mu.Unlock()
	err = s.save(next)
	if err == nil {
		s.mu.Lock()
		s.entries = next
		s.mu.Unlock()
	}
	s.saveMu.Unlock()
	if err != nil {
		return Repository{}, false, fmt.Errorf("保存仓库清单: %w", err)
	}
	snapshot, err := s.snapshot(ctx, r)
	s.reassociate(ctx)
	return snapshot, true, err
}
func (s *Service) requireRegistry() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.requireRegistryLocked()
}
func (s *Service) Unregister(id string) error {
	r, err := s.repository(id)
	if err != nil {
		return err
	}
	s.saveMu.Lock()
	defer s.saveMu.Unlock()
	s.mu.Lock()
	if err := s.requireRegistryLocked(); err != nil {
		s.mu.Unlock()
		return err
	}
	next := make([]*repositoryState, 0, len(s.entries))
	found := false
	for _, item := range s.entries {
		if item == r {
			found = true
		} else {
			next = append(next, item)
		}
	}
	if !found {
		s.mu.Unlock()
		return problemf(ErrNotFound, "仓库不存在")
	}
	s.mu.Unlock()
	if err := s.save(next); err != nil {
		return fmt.Errorf("保存仓库清单: %w", err)
	}
	s.mu.Lock()
	s.entries = next
	s.mu.Unlock()
	for _, info := range s.manager.List() {
		if info.Workspace != nil && info.Workspace.RepositoryID == id {
			s.manager.SetWorkspace(info.ID, info.Cwd, nil)
		}
	}
	return nil
}
func (s *Service) List(ctx context.Context) ([]Repository, error) {
	s.mu.Lock()
	if err := s.requireRegistryLocked(); err != nil {
		s.mu.Unlock()
		return nil, err
	}
	entries := append([]*repositoryState(nil), s.entries...)
	s.mu.Unlock()
	// Repositories are independent, so a slow one must not serialize the rest.
	// Each snapshot keeps its own gate and shared-flight semantics.
	snapshots := make([]Repository, len(entries))
	available := make([]bool, len(entries))
	failures := make([]error, len(entries))
	var wg sync.WaitGroup
	for i, r := range entries {
		wg.Add(1)
		go func(i int, r *repositoryState) {
			defer wg.Done()
			snapshot, err := s.snapshot(ctx, r)
			if err != nil {
				failures[i] = err
				return
			}
			if s.registered(r) {
				snapshots[i], available[i] = snapshot, true
			}
		}(i, r)
	}
	wg.Wait()
	out := make([]Repository, 0, len(entries))
	for i := range entries {
		if available[i] {
			out = append(out, snapshots[i])
		} else if failures[i] != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			// A single unreadable repository keeps the rest of the list and
			// its sessions reachable instead of failing the whole response.
			if s.registered(entries[i]) {
				out = append(out, s.unavailableRepository(entries[i], failures[i]))
			}
		}
	}
	s.reassociate(ctx)
	return out, nil
}
func (s *Service) Refresh(ctx context.Context, id string) (Repository, error) {
	r, err := s.repository(id)
	if err != nil {
		return Repository{}, err
	}
	snapshot, err := s.snapshot(ctx, r)
	if err == nil {
		s.reassociate(ctx)
	}
	return snapshot, err
}
func acquire(ctx context.Context, r *repositoryState) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-r.gate:
		return nil
	}
}
func release(r *repositoryState) { r.gate <- struct{}{} }
