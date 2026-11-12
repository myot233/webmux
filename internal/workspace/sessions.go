package workspace

import (
	"context"
	"errors"
	"path/filepath"
	"slices"
	"sort"

	"github.com/rainchan/webmux/internal/hybrid"
	"github.com/rainchan/webmux/internal/launch"
	"github.com/rainchan/webmux/internal/session"
)

func (s *Service) notifyLocked() { close(s.changed); s.changed = make(chan struct{}) }
func (s *Service) beginStart(path string) error {
	s.coord.Lock()
	defer s.coord.Unlock()
	for reserved := range s.reservations {
		if within(reserved, path) {
			return problemf(ErrConflict, "目标工作树正在删除，请稍后刷新")
		}
	}
	s.starts[path]++
	return nil
}
func (s *Service) endStart(path string) {
	s.coord.Lock()
	defer s.coord.Unlock()
	s.starts[path]--
	if s.starts[path] == 0 {
		delete(s.starts, path)
	}
	s.notifyLocked()
}
func (s *Service) reserve(ctx context.Context, path string) error {
	s.coord.Lock()
	for reserved := range s.reservations {
		if overlap(reserved, path) {
			s.coord.Unlock()
			return problemf(ErrConflict, "重叠工作树已有删除操作")
		}
	}
	s.reservations[path] = true
	for {
		starting := false
		for cwd := range s.starts {
			if within(path, cwd) {
				starting = true
				break
			}
		}
		if !starting {
			s.coord.Unlock()
			return nil
		}
		changed := s.changed
		s.coord.Unlock()
		select {
		case <-ctx.Done():
			s.unreserve(path)
			return ctx.Err()
		case <-changed:
		}
		s.coord.Lock()
	}
}
func (s *Service) unreserve(path string) {
	s.coord.Lock()
	defer s.coord.Unlock()
	delete(s.reservations, path)
	s.notifyLocked()
}
func (s *Service) association(ctx context.Context, cwd string) *session.Workspace {
	if s.gitPath == "" {
		return nil
	}
	common, err := s.common(ctx, cwd)
	if err != nil {
		return nil
	}
	root, err := s.git(ctx, cwd, "rev-parse", "--show-toplevel")
	if err != nil {
		return nil
	}
	path, err := canonical(outputLine(root))
	if err != nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.capability.RegistryAvailable {
		return nil
	}
	for _, r := range s.entries {
		if r.entry.CommonDir == common {
			return &session.Workspace{RepositoryID: r.entry.ID, WorktreeID: worktreeID(r.entry.ID, path), WorktreePath: path}
		}
	}
	return nil
}

// Association commits are ordered with registry removal. Git resolution never
// holds the registry/session locks, and a removed registration cannot reappear.
func (s *Service) commitAssociation(info session.Info, association *session.Workspace, preserve bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	registered := false
	id := ""
	if association != nil {
		id = association.RepositoryID
	} else if preserve && info.Workspace != nil {
		id = info.Workspace.RepositoryID
	}
	for _, r := range s.entries {
		if r.entry.ID == id {
			registered = true
			break
		}
	}
	if !registered {
		association = nil
	} else if association == nil && preserve {
		association = info.Workspace
	}
	s.manager.SetWorkspace(info.ID, info.Cwd, association)
}
func (s *Service) reassociate(ctx context.Context) {
	ctx, cancel := context.WithTimeout(ctx, readTimeout)
	defer cancel()
	for _, info := range s.manager.List() {
		if ctx.Err() != nil {
			return
		}
		association := s.association(ctx, info.Cwd)
		// If the original checkout disappeared, retain its identity instead of
		// silently rebinding the record to a newly created checkout at that path.
		if info.Workspace != nil && association != nil && *association != *info.Workspace {
			association = nil
		}
		s.commitAssociation(info, association, true)
	}
}

// HybridResumeInput resumes a hybrid session record. Side is tui, acp or
// empty for the recorded side; AgentSessionID picks a candidate offered
// after the recorded agent session went missing; ConfirmWithoutEnv accepts
// resuming without the environment of a deleted launch config.
type HybridResumeInput struct {
	ClientRequestID   string
	Side              string
	AgentSessionID    string
	ConfirmWithoutEnv bool
}

// ResumeHybrid resumes a closed or interrupted hybrid session under its
// recorded ID. The command is the record's snapshot; the environment is
// read again from the record's launch config. A deleted config fails with
// hybrid.ErrConfigMissing unless ConfirmWithoutEnv, which resumes without
// its environment. The session joins its directory's repository like a
// new one.
func (s *Service) ResumeHybrid(ctx context.Context, id string, in HybridResumeInput) (*session.Session, error) {
	if s.hybrids == nil {
		return nil, problemf(ErrUnavailable, "混合会话不可用")
	}
	rec, err := s.hybrids.Recoverable(id)
	if err != nil {
		return nil, err
	}
	var env map[string]string
	var attention *session.AttentionPolicy
	saved, err := s.launches.Get(rec.ConfigID)
	switch {
	case err == nil:
		env = saved.Env
		attention = attentionPolicy(saved.Attention)
	case !errors.Is(err, launch.ErrNotFound):
		return nil, err
	case !in.ConfirmWithoutEnv:
		return nil, hybrid.ErrConfigMissing
	}
	if err := s.beginStart(rec.Cwd); err != nil {
		return nil, err
	}
	defer s.endStart(rec.Cwd)
	lookup, cancel := context.WithTimeout(ctx, readTimeout)
	association := s.association(lookup, rec.Cwd)
	cancel()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	item, err := s.hybrids.Resume(ctx, id, hybrid.ResumeInput{
		ClientRequestID: in.ClientRequestID, Side: hybrid.Side(in.Side), AgentSessionID: in.AgentSessionID,
		Env: env, Attention: attention,
	})
	if err != nil {
		return nil, err
	}
	s.commitAssociation(item.Info(), association, false)
	return item, nil
}

// sessionCreateError keeps user-correctable startup failures as 400s while a
// shutdown or reaper failure stays a server-side error.
func sessionCreateError(err error) error {
	switch {
	case errors.Is(err, session.ErrInvalid):
		return problemf(ErrInvalid, "启动会话失败: %v", err)
	case errors.Is(err, session.ErrShuttingDown):
		return problemf(ErrUnavailable, "启动会话失败: %v", err)
	default:
		return problemf(ErrInternal, "启动会话失败: %v", err)
	}
}
func (s *Service) targetSessions(path string) []session.Info {
	result := make([]session.Info, 0)
	for _, info := range s.manager.List() {
		cwd, err := canonicalTarget(info.Cwd)
		if err != nil {
			cwd = filepath.Clean(info.Cwd)
		}
		if within(path, cwd) || within(path, info.Cwd) {
			result = append(result, info)
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result
}

// sameSessions reports whether a and b hold the same sessions in the same
// directories, each exited in both or in neither.
func sameSessions(a, b []session.Info) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].ID != b[i].ID || a[i].Cwd != b[i].Cwd {
			return false
		}
		if (a[i].Status == session.StatusExited) != (b[i].Status == session.StatusExited) {
			return false
		}
	}
	return true
}

// remainingSessions reports whether after holds only sessions of before, in
// the same directories. Sessions may leave: a closed hybrid shell is removed
// once its driver records the close.
func remainingSessions(before, after []session.Info) bool {
	for _, info := range after {
		i := slices.IndexFunc(before, func(old session.Info) bool { return old.ID == info.ID })
		if i < 0 || before[i].Cwd != info.Cwd {
			return false
		}
	}
	return true
}
