package workspace

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/rainchan/webmux/internal/hybrid"
	"github.com/rainchan/webmux/internal/session"
)

// HandoffPlan is a durable resource intention. The coordinator saves it before
// EnsureHandoffWorktree; recovery adopts only this exact branch and path.
type HandoffPlan struct {
	Cwd, RepositoryID, WorktreeID, Path, Branch, Base string
}

type HandoffIdentity struct {
	RuntimeID, ProviderThreadID, ExecutionID string
	RequireNativeResume                      bool
}

func (s *Service) PlanHandoff(ctx context.Context, in CreateInput, reservation string) (HandoffPlan, error) {
	plan, err := s.createPlan(ctx, in)
	if err != nil {
		return HandoffPlan{}, err
	}
	out := HandoffPlan{Cwd: in.Cwd}
	if in.Workspace != nil {
		out.RepositoryID = in.Workspace.RepositoryID
		out.WorktreeID = in.Workspace.WorktreeID
	}
	if in.NewWorktree != nil {
		out.RepositoryID = in.NewWorktree.RepositoryID
		out.Path = plan.path + "-" + reservation
		out.Branch, out.Base = plan.branch, plan.base
		r, err := s.repository(out.RepositoryID)
		if err != nil {
			return out, err
		}
		oid, err := s.git(ctx, r.entry.AnchorPath, "rev-parse", "--verify", "--end-of-options", out.Base+"^{commit}")
		if err != nil {
			return out, err
		}
		out.Base = strings.TrimSpace(string(oid))
		if _, err := os.Lstat(out.Path); !errors.Is(err, os.ErrNotExist) {
			return out, ErrConflict
		}
	} else {
		// Reuse start's connector checks before any durable resource is created.
		cwd := in.Cwd
		if in.Workspace != nil {
			r, err := s.repository(in.Workspace.RepositoryID)
			if err != nil {
				return out, err
			}
			snapshot, err := s.snapshot(ctx, r)
			if err != nil {
				return out, err
			}
			tree, err := findTree(snapshot, in.Workspace.WorktreeID)
			if err != nil {
				return out, err
			}
			if !tree.Available || tree.Kind == "bare" {
				return out, ErrConflict
			}
			if cwd == "" {
				cwd = tree.Path
			}
		}
		if cwd == "" {
			cwd = s.defaultCwd
		}
		if err := s.hybrids.CheckCreate(ctx, hybrid.CreateInput{Cwd: cwd, ConfigID: plan.command.configID, Cmd: plan.command.cmd, Env: plan.command.env, StartSide: hybrid.SideACP}); err != nil {
			return out, err
		}
	}
	return out, nil
}

func (s *Service) EnsureHandoffWorktree(ctx context.Context, p HandoffPlan) (Worktree, error) {
	r, err := s.repository(p.RepositoryID)
	if err != nil {
		return Worktree{}, err
	}
	snapshot, err := s.snapshot(ctx, r)
	if err != nil {
		return Worktree{}, err
	}
	for _, tree := range snapshot.Worktrees {
		if filepath.Clean(tree.Path) != filepath.Clean(p.Path) {
			continue
		}
		if !tree.Available || tree.Branch == nil || *tree.Branch != p.Branch {
			return Worktree{}, ErrConflict
		}
		return tree, nil
	}
	return s.CreateWorktree(ctx, p.RepositoryID, CreateRequest{Path: p.Path, Mode: "new", Branch: p.Branch, Base: p.Base})
}

// EnsureHandoffSession uses the reserved runtime ID for both fresh creation
// and archive recovery. A persisted shell is never replaced by a second one.
func (s *Service) EnsureHandoffSession(ctx context.Context, in CreateInput, identity HandoffIdentity) (*session.Session, error) {
	if s.hybrids == nil {
		return nil, ErrUnavailable
	}
	if rec, err := s.hybrids.Record(identity.RuntimeID); err == nil {
		if in.ConfigID == nil || rec.ConfigID != *in.ConfigID {
			return nil, ErrConflict
		}
		current, err := s.launches.Get(*in.ConfigID)
		if err != nil {
			return nil, err
		}
		if !slices.Equal(rec.Cmd, current.Cmd) {
			return nil, hybrid.ErrProviderConfigChanged
		}
		if rec.State == hybrid.StateActive {
			item, err := s.manager.Get(identity.RuntimeID)
			if err != nil {
				return nil, ErrConflict
			}
			return item, nil
		}
		return s.ResumeHybrid(ctx, identity.RuntimeID, HybridResumeInput{ClientRequestID: "handoff-resume-" + identity.ExecutionID, Side: string(hybrid.SideACP), ContextExecutionID: identity.ExecutionID, RequireNativeResume: identity.RequireNativeResume})
	} else if !errors.Is(err, hybrid.ErrNotFound) {
		return nil, err
	}
	plan, err := s.createPlan(ctx, in)
	if err != nil {
		return nil, err
	}
	item, _, err := s.startSession(ctx, in.Cwd, in.Name, in.Workspace, plan.command, hybrid.SideACP, nil, &identity)
	return item, err
}

func (s *Service) ConfigureHandoff(ctx context.Context, id string, options map[string]string, requestID string) ([]string, error) {
	return s.applyAgentConfig(ctx, id, CreateInput{AgentConfig: options, ClientRequestID: requestID})
}

// HandoffDirectory resolves an existing target without creating resources. A
// source subdirectory is retained only when the caller selected that workspace.
func (s *Service) HandoffDirectory(ctx context.Context, reference Reference, cwd string) (string, error) {
	r, err := s.repository(reference.RepositoryID)
	if err != nil {
		return "", err
	}
	snapshot, err := s.snapshot(ctx, r)
	if err != nil {
		return "", err
	}
	if snapshot.Availability != "available" {
		return "", problemf(ErrUnavailable, "%s", snapshot.Error)
	}
	tree, err := findTree(snapshot, reference.WorktreeID)
	if err != nil {
		return "", err
	}
	if !tree.Available || tree.Kind == "bare" {
		return "", ErrConflict
	}
	if cwd == "" {
		return tree.Path, nil
	}
	path, err := canonical(cwd)
	if err != nil {
		return "", ErrInvalid
	}
	info, err := os.Stat(path)
	if err != nil || !info.IsDir() || !within(tree.Path, path) {
		return "", ErrInvalid
	}
	return path, nil
}
