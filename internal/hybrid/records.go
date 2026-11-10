package hybrid

import (
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"
	"regexp"
	"slices"
	"sync"
	"syscall"
	"time"

	"github.com/rainchan/webmux/internal/pathutil"
	"github.com/rainchan/webmux/internal/runtime"
	"github.com/rainchan/webmux/internal/statefile"
)

const (
	recordsVersion = 1
	// MaxRecords bounds hybrid-sessions.json. A new session drops the least
	// recently updated archived record when the file is full; running
	// records are never dropped.
	MaxRecords = 200
	// maxPreviousIDs keeps the most recent agent session IDs a session
	// switched away from, for recovery candidates.
	maxPreviousIDs = 8
	// persistDelay coalesces bursts of record changes into one write.
	persistDelay = time.Second
	// retryDelay spaces the retries of a coalesced write that failed.
	retryDelay = 30 * time.Second
)

// Side is where a hybrid session runs: the terminal UI or the ACP chat.
type Side string

const (
	SideTUI Side = "tui"
	SideACP Side = "acp"
)

// State is the lifecycle of a hybrid session record.
type State string

const (
	StateActive      State = "active"
	StateClosed      State = "closed"
	StateInterrupted State = "interrupted"
)

// WorkspaceRef is the registered repository worktree a session belonged to
// when its record was last saved.
type WorkspaceRef struct {
	RepositoryID string `json:"repositoryId"`
	WorktreeID   string `json:"worktreeId"`
}

// Release says a record's worktree directory was deleted to free disk
// space while its commit stays reachable through a private ref. ID is
// shared by every record released together; Branch is empty for a
// detached worktree.
type Release struct {
	ID           string    `json:"id"`
	RepositoryID string    `json:"repositoryId"`
	Path         string    `json:"path"`
	Commit       string    `json:"commit"`
	Branch       string    `json:"branch,omitempty"`
	ReleasedAt   time.Time `json:"releasedAt"`
}

// Record links a WebMux session ID to an agent session. It never holds
// environment values, messages, queued text or tokens.
type Record struct {
	ID string `json:"id"`
	// Name is the explicit creation name, empty when the session follows
	// terminal- or agent-reported titles.
	Name string `json:"name"`
	// Title is the latest displayed session title. It is optional on disk so
	// records written before titles were persisted remain readable.
	Title          string   `json:"title,omitempty"`
	ConfigID       string   `json:"configId"`
	Agent          string   `json:"agent"`
	Cwd            string   `json:"cwd"`
	Cmd            []string `json:"cmd"`
	Side           Side     `json:"side"`
	AgentSessionID string   `json:"agentSessionId,omitempty"`
	PreviousIDs    []string `json:"previousIds"`
	// Config holds the last applied value of each agent config option, so
	// a switch or recovery can restore options the agent does not keep.
	Config map[string]string `json:"config"`
	// PermissionMode is PermissionAsk when every permission request goes
	// to the user; empty means the default, PermissionAuto.
	PermissionMode PermissionMode `json:"permissionMode,omitempty"`
	State          State          `json:"state"`
	// Activity is what the session was doing when it was interrupted:
	// "idle", "turn" or "switching".
	Activity string `json:"activity,omitempty"`
	// QueuedCount is how many queued prompts the interruption lost.
	QueuedCount   int        `json:"queuedCount"`
	CreatedAt     time.Time  `json:"createdAt"`
	UpdatedAt     time.Time  `json:"updatedAt"`
	InterruptedAt *time.Time `json:"interruptedAt,omitempty"`
	// PID and Process identify the side's root process, so a restart can
	// end an orphan that survived an unclean shutdown.
	PID     int               `json:"pid,omitempty"`
	Process *runtime.Identity `json:"process,omitempty"`
	// Workspace is the session's latest repository association, nil when it
	// had none; records written before it existed omit it.
	Workspace *WorkspaceRef `json:"workspace,omitempty"`
	// Released is set while the worktree directory is released; resume
	// refuses until the worktree is rebuilt.
	Released *Release `json:"released,omitempty"`
}

func (r Record) clone() Record {
	r.Cmd = slices.Clone(r.Cmd)
	r.PreviousIDs = slices.Clone(r.PreviousIDs)
	if r.PreviousIDs == nil {
		r.PreviousIDs = []string{}
	}
	config := make(map[string]string, len(r.Config))
	for k, v := range r.Config {
		config[k] = v
	}
	r.Config = config
	if r.InterruptedAt != nil {
		at := *r.InterruptedAt
		r.InterruptedAt = &at
	}
	if r.Process != nil {
		p := *r.Process
		r.Process = &p
	}
	if r.Workspace != nil {
		w := *r.Workspace
		r.Workspace = &w
	}
	if r.Released != nil {
		rel := *r.Released
		r.Released = &rel
	}
	return r
}

// SetAgentSessionID records a new agent session ID, keeping the replaced
// one among the bounded previous IDs.
func (r *Record) SetAgentSessionID(id string) {
	if id == "" || id == r.AgentSessionID {
		return
	}
	if r.AgentSessionID != "" {
		r.PreviousIDs = append(slices.DeleteFunc(r.PreviousIDs, func(p string) bool { return p == r.AgentSessionID || p == id }), r.AgentSessionID)
		if extra := len(r.PreviousIDs) - maxPreviousIDs; extra > 0 {
			r.PreviousIDs = r.PreviousIDs[extra:]
		}
	}
	r.AgentSessionID = id
}

type storedRecords struct {
	V       int      `json:"v"`
	Records []Record `json:"records"`
}

var idPattern = regexp.MustCompile(`^[a-z2-7]{8}$`)

func validateRecords(f storedRecords) error {
	if f.V != recordsVersion || f.Records == nil {
		return errors.New("文件损坏或版本不受支持")
	}
	if len(f.Records) > MaxRecords {
		return fmt.Errorf("记录超过 %d 条", MaxRecords)
	}
	seen := map[string]bool{}
	for i, r := range f.Records {
		switch {
		case !idPattern.MatchString(r.ID) || seen[r.ID]:
			return fmt.Errorf("第 %d 条记录: ID 无效或重复", i+1)
		case r.Side != SideTUI && r.Side != SideACP:
			return fmt.Errorf("第 %d 条记录: 所在端无效", i+1)
		case r.State != StateActive && r.State != StateClosed && r.State != StateInterrupted:
			return fmt.Errorf("第 %d 条记录: 状态无效", i+1)
		case r.Agent == "" || len(r.Cmd) == 0 || r.Cwd == "" || r.CreatedAt.IsZero():
			return fmt.Errorf("第 %d 条记录: 缺少必需字段", i+1)
		}
		seen[r.ID] = true
	}
	return nil
}

// records owns <state_dir>/hybrid-sessions.json. A lock conflict, corrupt
// or unreadable file leaves it unavailable: hybrid creation and recovery
// answer ErrUnavailable while ordinary sessions keep working, and the file
// is left untouched.
type records struct {
	// writeMu serializes read-modify-write transactions while mu is released
	// for filesystem operations, so readers never wait for durable storage.
	writeMu     sync.Mutex
	mu          sync.Mutex
	path        string
	store       *statefile.Handle[storedRecords]
	commitStore func(storedRecords) error
	broken      error
	list        []Record
	dirty       bool
	pending     *time.Timer
	closed      bool
	// log reports coalesced writes that failed; never nil after open.
	log *slog.Logger
}

func openRecords(stateDir string) *records {
	r := &records{log: slog.New(slog.DiscardHandler)}
	dir, err := pathutil.Expand(stateDir)
	if err == nil {
		dir, err = filepath.Abs(dir)
	}
	if err != nil {
		r.path = filepath.Join(stateDir, statefile.Describe(statefile.Hybrid).Path)
		r.broken = fmt.Errorf("状态目录无效: %w", err)
		return r
	}
	var f storedRecords
	r.store, f, err = statefile.Open[storedRecords](dir, statefile.Hybrid, validateRecords)
	r.commitStore = r.store.Commit
	r.path = filepath.Join(dir, statefile.Describe(statefile.Hybrid).Path)
	switch {
	case statefile.IsKind(err, statefile.Missing):
		r.list = []Record{}
	case err != nil:
		r.broken = err
	default:
		r.list = f.Records
	}
	return r
}

// unavailable returns an ErrUnavailable error naming the reason and file,
// or nil when the store works.
func (r *records) unavailable() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.unavailableLocked()
}

func (r *records) unavailableLocked() error {
	if r.broken != nil {
		return fmt.Errorf("%w: %s（%s）", ErrUnavailable, r.broken, r.path)
	}
	if r.closed {
		return fmt.Errorf("%w: 服务已关闭", ErrUnavailable)
	}
	return nil
}

func (r *records) index(id string) int {
	return slices.IndexFunc(r.list, func(rec Record) bool { return rec.ID == id })
}

func (r *records) get(id string) (Record, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if i := r.index(id); i >= 0 {
		return r.list[i].clone(), true
	}
	return Record{}, false
}

func (r *records) all() []Record {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]Record, len(r.list))
	for i, rec := range r.list {
		out[i] = rec.clone()
	}
	return out
}

// reserved reports whether id belongs to a record, so random session IDs
// never collide with a recoverable session.
func (r *records) reserved(id string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.index(id) >= 0
}

// agentInUse reports whether an active record other than self's links agent
// session sid: another hybrid session has it open.
func (r *records) agentInUse(self, sid string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, rec := range r.list {
		if rec.ID != self && rec.State == StateActive && rec.AgentSessionID == sid {
			return true
		}
	}
	return false
}

// accepts reports whether add can store one more record now: below
// MaxRecords, or with an archived record it may drop to make room. skip
// names records an eviction must leave alone, such as one being resumed;
// nil skips none.
func (r *records) accepts(skip func(string) bool) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.list) < MaxRecords || oldestArchived(r.list, skip) >= 0
}

// oldestArchived is the archived record to drop first: the least recently
// updated one, ties by ID. Active and skipped records never qualify; -1
// when every remaining record is active or skipped.
func oldestArchived(list []Record, skip func(string) bool) int {
	oldest := -1
	for i, rec := range list {
		if rec.State == StateActive || (skip != nil && skip(rec.ID)) {
			continue
		}
		if oldest < 0 || rec.UpdatedAt.Before(list[oldest].UpdatedAt) ||
			(rec.UpdatedAt.Equal(list[oldest].UpdatedAt) && rec.ID < list[oldest].ID) {
			oldest = i
		}
	}
	return oldest
}

// add stores a new record and writes it immediately; creation must fail
// rather than run a session the store cannot track. A full store drops its
// least recently updated archived records in the same write (more than one
// when the byte limit, not the count, is what is full) and returns them,
// so the caller can end what they left behind. skip names records an
// eviction must leave alone; nil skips none.
func (r *records) add(rec Record, skip func(string) bool) (evicted []Record, err error) {
	r.writeMu.Lock()
	defer r.writeMu.Unlock()
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.unavailableLocked(); err != nil {
		return nil, err
	}
	if r.index(rec.ID) >= 0 {
		return nil, fmt.Errorf("%w: 记录 %s 已存在", ErrConflict, rec.ID)
	}
	base := slices.Clone(r.list)
	evictOne := func() bool {
		i := oldestArchived(base, skip)
		if i < 0 {
			return false
		}
		evicted = append(evicted, base[i].clone())
		base = slices.Delete(base, i, i+1)
		return true
	}
	for len(base) >= MaxRecords {
		if !evictOne() {
			return nil, ErrLimit
		}
	}
	// The file is also bounded in bytes: long commands can fill it before
	// MaxRecords, and a write the store refuses would make hybrid sessions
	// unavailable although archived records could make room.
	next := append(slices.Clone(base), rec.clone())
	for !r.store.Fits(storedRecords{V: recordsVersion, Records: next}) {
		if !evictOne() {
			return nil, ErrLimit
		}
		next = append(slices.Clone(base), rec.clone())
	}
	if err := r.commitLocked(next); err != nil {
		return nil, err
	}
	r.list = next
	return evicted, nil
}

// restore puts back a record add evicted when the session that made room
// failed to start. The failed record is gone again, so the store has room.
func (r *records) restore(rec Record) error {
	r.writeMu.Lock()
	defer r.writeMu.Unlock()
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.unavailableLocked(); err != nil {
		return err
	}
	if r.index(rec.ID) >= 0 {
		return fmt.Errorf("%w: 记录 %s 已存在", ErrConflict, rec.ID)
	}
	if len(r.list) >= MaxRecords {
		return ErrLimit
	}
	next := append(slices.Clone(r.list), rec.clone())
	if err := r.commitLocked(next); err != nil {
		return err
	}
	r.list = next
	return nil
}

// update changes a record. Immediate writes the file now (state changes
// a restart must see); otherwise the write is coalesced.
func (r *records) update(id string, immediate bool, change func(*Record)) (Record, error) {
	r.writeMu.Lock()
	defer r.writeMu.Unlock()
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.unavailableLocked(); err != nil {
		return Record{}, err
	}
	i := r.index(id)
	if i < 0 {
		return Record{}, ErrNotFound
	}
	next := slices.Clone(r.list)
	rec := next[i].clone()
	change(&rec)
	rec.UpdatedAt = time.Now().UTC()
	next[i] = rec
	if immediate {
		if err := r.commitLocked(next); err != nil {
			return Record{}, err
		}
	} else {
		r.list = next
		r.dirty = true
		r.scheduleLocked()
		return rec.clone(), nil
	}
	r.list = next
	return rec.clone(), nil
}

func (r *records) remove(id string) error {
	r.writeMu.Lock()
	defer r.writeMu.Unlock()
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.unavailableLocked(); err != nil {
		return err
	}
	i := r.index(id)
	if i < 0 {
		return ErrNotFound
	}
	next := slices.Delete(slices.Clone(r.list), i, i+1)
	if err := r.commitLocked(next); err != nil {
		return err
	}
	r.list = next
	return nil
}

// retryable reports whether a failed write may succeed later: an I/O or
// permission failure can pass, an oversize or frozen store cannot.
func retryable(err error) bool {
	return statefile.IsKind(err, statefile.IO) || statefile.IsKind(err, statefile.Permission)
}

// setLogger sets where failed coalesced writes are reported.
func (r *records) setLogger(log *slog.Logger) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.log = log
}

func (r *records) scheduleLocked() {
	r.scheduleAfterLocked(persistDelay)
}

func (r *records) scheduleAfterLocked(delay time.Duration) {
	if r.pending != nil || !r.dirty {
		return
	}
	r.pending = time.AfterFunc(delay, func() {
		r.writeMu.Lock()
		defer r.writeMu.Unlock()
		r.mu.Lock()
		defer r.mu.Unlock()
		r.pending = nil
		if r.unavailableLocked() != nil {
			return
		}
		if err := r.commitLocked(r.list); err != nil {
			// Memory is now ahead of the disk. Say so, and keep trying while
			// the failure can pass (a full disk); an uncertain commit
			// already froze the store inside statefile.
			r.log.Error("hybrid records write failed; changes are not persisted", "error", err)
		}
	})
}

// commitLocked commits one write transaction. writeMu excludes other
// mutations; mu is released during filesystem I/O and held on return.
func (r *records) commitLocked(list []Record) error {
	if r.pending != nil {
		r.pending.Stop()
		r.pending = nil
	}
	wasDirty := r.dirty
	r.dirty = true
	r.mu.Unlock()
	err := r.commitStore(storedRecords{V: recordsVersion, Records: list})
	r.mu.Lock()
	if err != nil {
		r.dirty = wasDirty
		if wasDirty && retryable(err) {
			r.scheduleAfterLocked(retryDelay)
		}
		return fmt.Errorf("%w: %w", ErrUnavailable, err)
	}
	r.dirty = false
	return nil
}

// recoverAfterRestart marks every active record interrupted and returns
// their IDs. A leftover root process of any record whose PID and start
// identity both still match is ended first, so it cannot keep writing the
// agent session: interrupted and closed records keep the identity of a
// process that was not confirmed gone.
func (r *records) recoverAfterRestart(now time.Time) ([]string, error) {
	r.writeMu.Lock()
	defer r.writeMu.Unlock()
	r.mu.Lock()
	if err := r.unavailableLocked(); err != nil {
		r.mu.Unlock()
		return nil, err
	}
	next := slices.Clone(r.list)
	r.mu.Unlock()
	var interrupted []string
	changed := false
	for i, rec := range next {
		if rec.State != StateActive && rec.PID == 0 && rec.Process == nil {
			continue
		}
		if rec.PID > 0 && rec.Process != nil {
			if id, err := runtime.ProcessIdentity(rec.PID); err == nil && id == *rec.Process {
				killGroup(rec.PID)
			}
		}
		rec = rec.clone()
		rec.PID, rec.Process = 0, nil
		if rec.State == StateActive {
			at := now.UTC()
			rec.State, rec.InterruptedAt, rec.UpdatedAt = StateInterrupted, &at, at
			if rec.Activity == "" {
				rec.Activity = "idle"
			}
			interrupted = append(interrupted, rec.ID)
		}
		next[i] = rec
		changed = true
	}
	if !changed {
		return nil, nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.commitLocked(next); err != nil {
		return nil, err
	}
	r.list = next
	return interrupted, nil
}

// killGroup ends a leftover process group: SIGHUP lets agents flush their
// session store, SIGKILL follows if the group is still there.
func killGroup(pid int) {
	if syscall.Kill(-pid, syscall.SIGHUP) != nil {
		return
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if syscall.Kill(-pid, 0) != nil {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	_ = syscall.Kill(-pid, syscall.SIGKILL)
}

// close writes any pending change and releases the file lock.
func (r *records) close() {
	r.writeMu.Lock()
	defer r.writeMu.Unlock()
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return
	}
	if r.pending != nil {
		r.pending.Stop()
		r.pending = nil
	}
	if r.dirty && r.broken == nil {
		if err := r.commitLocked(r.list); err != nil {
			r.log.Error("hybrid records final write failed; changes are not persisted", "error", err)
		}
	}
	r.closed = true
	r.mu.Unlock()
	r.store.Close()
}

// markReleased sets rel on every record in ids and writes the file at once.
// Every record must exist and be closed or interrupted; otherwise nothing
// changes.
func (r *records) markReleased(ids []string, rel Release) error {
	r.writeMu.Lock()
	defer r.writeMu.Unlock()
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.unavailableLocked(); err != nil {
		return err
	}
	next := slices.Clone(r.list)
	for _, id := range ids {
		i := r.index(id)
		if i < 0 {
			return fmt.Errorf("%w: 记录 %s", ErrNotFound, id)
		}
		if next[i].State == StateActive {
			return fmt.Errorf("%w: 会话 %s 仍在运行", ErrConflict, id)
		}
		rec := next[i].clone()
		at := rel
		rec.Released = &at
		next[i] = rec
	}
	if err := r.commitLocked(next); err != nil {
		return err
	}
	r.list = next
	return nil
}

// clearReleased removes the release mark releaseID from every record that
// carries it, writes the file at once and returns their IDs.
func (r *records) clearReleased(releaseID string) ([]string, error) {
	r.writeMu.Lock()
	defer r.writeMu.Unlock()
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.unavailableLocked(); err != nil {
		return nil, err
	}
	next := slices.Clone(r.list)
	var ids []string
	for i, rec := range next {
		if rec.Released == nil || rec.Released.ID != releaseID {
			continue
		}
		rec = rec.clone()
		rec.Released = nil
		next[i] = rec
		ids = append(ids, rec.ID)
	}
	if len(ids) == 0 {
		return nil, nil
	}
	if err := r.commitLocked(next); err != nil {
		return nil, err
	}
	r.list = next
	return ids, nil
}
