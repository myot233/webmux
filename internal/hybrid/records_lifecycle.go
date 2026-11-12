package hybrid

import (
	"fmt"
	"log/slog"
	"slices"
	"syscall"
	"time"

	"github.com/rainchan/webmux/internal/runtime"
	"github.com/rainchan/webmux/internal/statefile"
)

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
