package workspace

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/rainchan/webmux/internal/hybrid"
	"github.com/rainchan/webmux/internal/session"
)

// purpose keeps the confirmations of different operations apart: one
// cannot execute another.
type purpose string

const (
	purposeRemove       purpose = "remove"
	purposeRelease      purpose = "release"
	purposeBatchRemove  purpose = "batch-remove"
	purposeSwitch       purpose = "switch"
	purposeFastForward  purpose = "fast-forward"
	purposePush         purpose = "push"
	purposeBranchDelete purpose = "branch-delete"
)

type batchTarget struct {
	state  removalState
	commit string
}

type confirmation struct {
	purpose      purpose
	repositoryID string
	worktreeID   string
	expires      time.Time
	state        removalState
	batch        []batchTarget
	// records are the archived record IDs a release preview showed.
	records []string
	// commit is the commit the preview resolved, empty when HEAD does not
	// resolve.
	commit string
	// switching is what a switch preview bound.
	switching *switchState
	// fastForward, push and branchDelete are what the branch operation
	// previews bound.
	fastForward  *fastForwardState
	push         *pushState
	branchDelete []branchDeleteTarget
	timer        *time.Timer
}

func (c *confirmation) closeHandles() {
	c.state.identity.close()
	for i := range c.batch {
		c.batch[i].state.identity.close()
	}
}

func (s *Service) PreviewRemoval(ctx context.Context, repositoryID, worktreeID string) (Preview, error) {
	ctx, cancel := context.WithTimeout(ctx, readTimeout)
	defer cancel()
	r, err := s.repository(repositoryID)
	if err != nil {
		return Preview{}, err
	}
	if err := acquire(ctx, r); err != nil {
		return Preview{}, err
	}
	defer release(r)
	state, err := s.removalState(ctx, r, worktreeID)
	if err != nil {
		return Preview{}, err
	}
	commit := s.headCommit(ctx, r, state.tree)
	records := s.targetRecords(state.tree.Path)
	check := s.checkPushed(ctx, r, commit)
	c := confirmation{purpose: purposeRemove, repositoryID: repositoryID, worktreeID: worktreeID, state: state, commit: commit}
	id, expires, err := s.confirm(c)
	if err != nil {
		return Preview{}, err
	}
	return Preview{
		ConfirmationID: id, ExpiresAt: expires, Worktree: state.tree, Sessions: state.sessions, Archived: records, Commit: commit, Push: check,
		Blockers: state.messages(), Warnings: append([]string{}, state.warnings...), ForceAllowed: state.supported,
	}, nil
}

// confirm stores c under a new confirmation ID until it expires; it owns
// c's identity handles from now on, also on failure.
func (s *Service) confirm(c confirmation) (string, time.Time, error) {
	id, err := randomID()
	if err != nil {
		c.closeHandles()
		return "", time.Time{}, err
	}
	now := time.Now()
	c.expires = now.Add(confirmationLifetime)
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.requireRegistryLocked(); err != nil {
		c.closeHandles()
		return "", time.Time{}, err
	}
	s.coord.Lock()
	defer s.coord.Unlock()
	for key, old := range s.confirmations {
		if !now.Before(old.expires) {
			old.timer.Stop()
			old.closeHandles()
			delete(s.confirmations, key)
		}
	}
	c.timer = time.AfterFunc(confirmationLifetime, func() {
		s.coord.Lock()
		if expired, exists := s.confirmations[id]; exists {
			delete(s.confirmations, id)
			expired.closeHandles()
		}
		s.coord.Unlock()
	})
	s.confirmations[id] = c
	return id, c.expires, nil
}

// takeConfirmation removes confirmation id and returns it when it is
// still valid for purpose p on the worktree. The caller closes the
// returned confirmation's identity handles.
func (s *Service) takeConfirmation(id string, p purpose, repositoryID, worktreeID string) (confirmation, bool) {
	s.coord.Lock()
	c, ok := s.confirmations[id]
	if ok && c.purpose == p {
		delete(s.confirmations, id)
	}
	s.coord.Unlock()
	if !ok || c.purpose != p {
		return confirmation{}, false
	}
	c.timer.Stop()
	if !time.Now().Before(c.expires) || c.repositoryID != repositoryID || c.worktreeID != worktreeID {
		c.closeHandles()
		return confirmation{}, false
	}
	return c, true
}

// RemoveWorktree removes a linked worktree after its confirmation. The
// closed and interrupted hybrid records under the removed directory stay,
// marked released at the commit the confirmation resolved, so they can be
// rebuilt and resumed; without such a commit they stay as they were.
func (s *Service) RemoveWorktree(ctx context.Context, repositoryID, worktreeID string, req RemoveRequest) error {
	return s.removeWorktree(ctx, repositoryID, worktreeID, req)
}

// targetRecords lists the closed and interrupted hybrid records whose
// directory lies in path, most recently updated first. Released records
// are skipped: their worktree is gone, so whatever now occupies path is not
// theirs, and removing or releasing it must leave them restorable.
func (s *Service) targetRecords(path string) []hybrid.Record {
	result := make([]hybrid.Record, 0)
	if s.hybrids == nil {
		return result
	}
	list, err := s.hybrids.Records(true)
	if err != nil {
		return result
	}
	for _, rec := range list {
		if rec.Released != nil {
			continue
		}
		cwd, err := canonicalTarget(rec.Cwd)
		if err != nil {
			cwd = filepath.Clean(rec.Cwd)
		}
		if within(path, cwd) || within(path, rec.Cwd) {
			result = append(result, rec)
		}
	}
	return result
}

// retainRecords marks the closed and interrupted hybrid records whose
// directory lies in removed worktree tree as released at commit, including
// those of the sessions the removal closed (closedHybrids), whose records
// are awaited first. Without a commit nothing can be pinned, so the
// records stay unchanged and can only be viewed or deleted. The caller
// holds r's gate. The worktree is gone whatever the client does now, so a
// cancelled request still retains. A failure is logged: the records then
// stay as they were.
func (s *Service) retainRecords(ctx context.Context, r *repositoryState, repositoryID string, tree Worktree, commit string, closedHybrids []string) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), writeTimeout)
	defer cancel()
	s.awaitClosed(ctx, closedHybrids)
	records := s.targetRecords(tree.Path)
	if len(records) == 0 {
		return
	}
	if commit == "" {
		slog.Info("archived records kept unreleased: HEAD did not resolve", "records", len(records))
		return
	}
	id, err := s.pinRecords(ctx, r, repositoryID, tree, commit, recordIDs(records))
	if err != nil {
		slog.Warn("archived records could not be marked released after removal", "records", len(records), "error", err)
		return
	}
	slog.Info("worktree removed, archived records released", "release", id, "records", len(records))
}

// closeRecordWait bounds the wait for a closed session's record.
const closeRecordWait = 5 * time.Second

// awaitClosed waits, within closeRecordWait, until each hybrid session in
// ids has recorded its close; a record that never shows is left out.
func (s *Service) awaitClosed(ctx context.Context, ids []string) {
	deadline := time.Now().Add(closeRecordWait)
	for _, id := range ids {
		for time.Now().Before(deadline) && ctx.Err() == nil {
			if rec, err := s.hybrids.Record(id); err == nil && rec.State != hybrid.StateActive {
				break
			}
			time.Sleep(20 * time.Millisecond)
		}
	}
}

// removeWorktree performs the removal and retains the archived records.
func (s *Service) removeWorktree(ctx context.Context, repositoryID, worktreeID string, req RemoveRequest) error {
	ctx, cancel := context.WithTimeout(ctx, writeTimeout)
	defer cancel()
	if req.Mode != "safe" && req.Mode != "force" {
		return problemf(ErrInvalid, "删除模式必须为 safe 或 force")
	}
	confirmation, ok := s.takeConfirmation(req.ConfirmationID, purposeRemove, repositoryID, worktreeID)
	if !ok {
		return problemf(ErrConflict, "删除确认已失效，请重新预览")
	}
	defer confirmation.closeHandles()
	if req.Mode == "force" && (req.ConfirmPath != confirmation.state.tree.Path || !req.CloseSessions) {
		return problemf(ErrInvalid, "强制删除必须输入完整目标路径并明确授权关闭会话")
	}
	r, err := s.repository(repositoryID)
	if err != nil {
		return err
	}
	if err := acquire(ctx, r); err != nil {
		return err
	}
	defer release(r)
	path := confirmation.state.tree.Path
	if err := s.reserve(ctx, path); err != nil {
		return err
	}
	defer s.unreserve(path)
	// Every execution path releases the reservation and re-reads real Git state,
	// including cancellation after Git or session cleanup partially took effect.
	defer func() {
		refreshCtx, refreshCancel := context.WithTimeout(context.Background(), readTimeout)
		defer refreshCancel()
		s.readRepository(refreshCtx, r)
	}()
	return s.executeLockedRemoval(ctx, r, repositoryID, confirmation.state, confirmation.commit, req.Mode)
}

func (s *Service) executeLockedRemoval(ctx context.Context, r *repositoryState, repositoryID string, confirmed removalState, confirmedCommit, mode string) error {
	worktreeID := confirmed.tree.ID
	path := confirmed.tree.Path
	current, err := s.removalState(ctx, r, worktreeID)
	if err != nil {
		return err
	}
	defer current.identity.close()
	if !current.supported {
		return problemf(ErrConflict, "不支持删除此工作树: %v", current.messages())
	}
	if !sameTarget(confirmed, current) || !sameSessions(confirmed.sessions, current.sessions) {
		return problemf(ErrConflict, "工作树身份、分支、锁或会话集合已变化，请重新预览；尚未关闭任何会话")
	}
	if mode == "safe" && len(current.blockers) != 0 {
		return problemf(ErrConflict, "安全删除被阻止: %v", current.messages())
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	closed := make(map[string]bool)
	if mode == "force" {
		active := make([]*session.Session, 0, len(current.sessions))
		for _, info := range current.sessions {
			if info.Status == session.StatusExited {
				continue
			}
			item, err := s.manager.Get(info.ID)
			if err != nil {
				return problemf(ErrConflict, "会话集合已变化，请重新预览")
			}
			active = append(active, item)
		}
		// Manager.Close hands a hybrid shell to its driver, which records the
		// close and removes the shell; Done closes either way.
		for _, item := range active {
			_ = s.manager.Close(item.Info().ID)
		}
		for _, item := range active {
			select {
			case <-item.Done():
				closed[item.Info().ID] = true
			case <-ctx.Done():
				return problemf(ErrTimeout, "关闭会话未在期限内完成，工作树未删除；部分会话可能已关闭")
			}
		}
	}
	final, err := s.removalState(ctx, r, worktreeID)
	if err != nil {
		return err
	}
	defer final.identity.close()
	if !final.supported || !sameTarget(current, final) || !remainingSessions(current.sessions, final.sessions) {
		return problemf(ErrConflict, "关闭期间目标身份或会话集合变化，工作树未删除；部分会话可能已关闭")
	}
	for _, info := range final.sessions {
		if !closed[info.ID] && info.Status != session.StatusExited {
			return problemf(ErrConflict, "仍有运行会话，工作树未删除")
		}
	}
	if mode == "safe" && len(final.blockers) != 0 {
		return problemf(ErrConflict, "安全删除被最新状态阻止: %v", final.messages())
	}
	// The commit the records will be pinned to is the one the user saw.
	if s.headCommit(ctx, r, final.tree) != confirmedCommit {
		return problemf(ErrConflict, "提交已变化，工作树未删除，请重新预览；部分会话可能已关闭")
	}
	if final.identity.prunableMissing {
		if _, statErr := os.Lstat(path); !errors.Is(statErr, os.ErrNotExist) {
			return problemf(ErrConflict, "待清理工作树原路径已重新出现文件或目录，请重新预览")
		}
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	argv := []string{"worktree", "remove"}
	if mode == "force" {
		argv = append(argv, "--force")
	}
	argv = append(argv, "--", path)
	if _, err := s.git(ctx, r.entry.AnchorPath, argv...); err != nil {
		return problemf(errorKind(err), "%v；删除未完整成功，部分会话可能已关闭，请刷新实际状态", err)
	}
	// The records of the removed directory, including those of sessions the
	// removal closed, stay restorable.
	var closedHybrids []string
	for _, info := range current.sessions {
		if closed[info.ID] && info.Hybrid != nil {
			closedHybrids = append(closedHybrids, info.ID)
		}
	}
	s.retainRecords(ctx, r, repositoryID, confirmed.tree, confirmedCommit, closedHybrids)
	return nil
}

func (s *Service) clearConfirmations() {
	s.coord.Lock()
	defer s.coord.Unlock()
	for id, confirmation := range s.confirmations {
		confirmation.timer.Stop()
		confirmation.closeHandles()
		delete(s.confirmations, id)
	}
}
