package hybrid

import (
	"errors"
	"fmt"
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
	// MaxRecords bounds hybrid-sessions.json; creating more is refused until
	// the user removes old records.
	MaxRecords = 200
	// maxPreviousIDs keeps the most recent agent session IDs a session
	// switched away from, for recovery candidates.
	maxPreviousIDs = 8
	// persistDelay coalesces bursts of record changes into one write.
	persistDelay = time.Second
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

// Record links a WebMux session ID to an agent session. It never holds
// environment values, messages, queued text or tokens.
type Record struct {
	ID             string   `json:"id"`
	Name           string   `json:"name"`
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
	State  State             `json:"state"`
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
	mu      sync.Mutex
	path    string
	store   *statefile.Handle[storedRecords]
	broken  error
	list    []Record
	pending *time.Timer
	closed  bool
}

func openRecords(stateDir string) *records {
	r := &records{}
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

// count is the number of records, for the limit check before a session
// starts; add enforces the limit again.
func (r *records) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.list)
}

// add stores a new record and writes it immediately; creation must fail
// rather than run a session the store cannot track.
func (r *records) add(rec Record) error {
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
	next := append(slices.Clip(r.list), rec.clone())
	if err := r.commitLocked(next); err != nil {
		return err
	}
	r.list = next
	return nil
}

// update changes a record. Immediate writes the file now (state changes
// a restart must see); otherwise the write is coalesced.
func (r *records) update(id string, immediate bool, change func(*Record)) (Record, error) {
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
		r.scheduleLocked()
	}
	r.list = next
	return rec.clone(), nil
}

func (r *records) remove(id string) error {
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

func (r *records) scheduleLocked() {
	if r.pending != nil {
		return
	}
	r.pending = time.AfterFunc(persistDelay, func() {
		r.mu.Lock()
		defer r.mu.Unlock()
		r.pending = nil
		if r.unavailableLocked() == nil {
			// A failed delayed write freezes the store inside statefile and
			// surfaces through later operations.
			_ = r.commitLocked(r.list)
		}
	})
}

func (r *records) commitLocked(list []Record) error {
	if r.pending != nil {
		r.pending.Stop()
		r.pending = nil
	}
	if err := r.store.Commit(storedRecords{V: recordsVersion, Records: list}); err != nil {
		return fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	return nil
}

// recoverAfterRestart marks every active record interrupted. A leftover
// root process of any record whose PID and start identity both still match
// is ended first, so it cannot keep writing the agent session: interrupted
// and closed records keep the identity of a process that was not confirmed
// gone.
func (r *records) recoverAfterRestart(now time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.unavailableLocked(); err != nil {
		return err
	}
	next := slices.Clone(r.list)
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
		}
		next[i] = rec
		changed = true
	}
	if !changed {
		return nil
	}
	if err := r.commitLocked(next); err != nil {
		return err
	}
	r.list = next
	return nil
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
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return
	}
	if r.pending != nil && r.broken == nil {
		_ = r.commitLocked(r.list)
	}
	r.closed = true
	r.store.Close()
}
