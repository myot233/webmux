package workspace

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
)

// maxDeleteBranches bounds the branches one delete preview takes.
const maxDeleteBranches = 50

// Groups of a branch delete preview: every commit of a contained branch
// stays reachable from another ref; an unmerged branch has commits only it
// keeps.
const (
	BranchContained = "contained"
	BranchUnmerged  = "unmerged"
)

// Outcomes of deleting one branch.
const (
	BranchDeleted = "deleted"
	BranchSkipped = "skipped"
	BranchFailed  = "failed"
)

// BranchDeleteItem previews deleting one branch at Commit. Unmerged lists
// the commits that only it keeps, empty for a contained branch. A blocked
// item (checked out, missing) cannot be deleted.
type BranchDeleteItem struct {
	Branch   string
	Commit   string
	Group    string
	Unmerged CommitList
	Blockers []Issue
}

// BranchDeletePreview previews deleting branches. Without a deletable item
// ConfirmationID is empty and ExpiresAt nil.
type BranchDeletePreview struct {
	ConfirmationID string
	ExpiresAt      *time.Time
	Items          []BranchDeleteItem
}

// BranchDeleteRequest executes a delete preview; the unmerged group is
// deleted only with AcknowledgeUnmerged.
type BranchDeleteRequest struct {
	ConfirmationID      string
	AcknowledgeUnmerged bool
}

// BranchDeleteItemResult is the outcome of deleting one branch; a deleted
// branch's Message tells how to restore it at Commit.
type BranchDeleteItemResult struct {
	Branch  string
	Status  string
	Commit  string
	Message string
}

// BranchDeleteResult lists each branch's outcome in preview order and the
// repository afterwards.
type BranchDeleteResult struct {
	Results    []BranchDeleteItemResult
	Repository Repository
}

// branchDeleteTarget is one branch a delete confirmation binds.
type branchDeleteTarget struct {
	branch string
	commit string
	group  string
}

// PreviewBranchDelete groups the local branches by whether deleting them
// loses commits: a commit counts as kept when another local branch, a
// remote-tracking branch, a tag, a WebMux protection ref or any worktree's
// HEAD reaches it. Branches checked out by a worktree are blocked. Only a
// preview with a deletable branch is confirmed.
func (s *Service) PreviewBranchDelete(ctx context.Context, repositoryID string, branches []string) (BranchDeletePreview, error) {
	if len(branches) == 0 || len(branches) > maxDeleteBranches {
		return BranchDeletePreview{}, problemf(ErrInvalid, "一次可删除 1 到 %d 个分支", maxDeleteBranches)
	}
	seen := make(map[string]bool, len(branches))
	for _, name := range branches {
		if seen[name] {
			return BranchDeletePreview{}, problemf(ErrInvalid, "分支重复: %s", name)
		}
		seen[name] = true
	}
	ctx, cancel := context.WithTimeout(ctx, readTimeout)
	defer cancel()
	r, err := s.repository(repositoryID)
	if err != nil {
		return BranchDeletePreview{}, err
	}
	if err := acquire(ctx, r); err != nil {
		return BranchDeletePreview{}, err
	}
	defer release(r)
	items, err := s.branchDeleteItems(ctx, r, branches)
	if err != nil {
		return BranchDeletePreview{}, err
	}
	targets := []branchDeleteTarget{}
	for _, item := range items {
		if len(item.Blockers) == 0 {
			targets = append(targets, branchDeleteTarget{branch: item.Branch, commit: item.Commit, group: item.Group})
		}
	}
	preview := BranchDeletePreview{Items: items}
	if len(targets) == 0 {
		return preview, nil
	}
	id, expires, err := s.confirm(confirmation{purpose: purposeBranchDelete, repositoryID: repositoryID, branchDelete: targets})
	if err != nil {
		return BranchDeletePreview{}, err
	}
	preview.ConfirmationID, preview.ExpiresAt = id, &expires
	return preview, nil
}

// DeleteBranches deletes the previewed branches one by one under the
// repository gate, each only while it is not checked out, still at the
// previewed commit and still in its previewed group; the unmerged group
// only with AcknowledgeUnmerged. A branch's configuration goes with it.
// Remote branches are never touched. Once started, the request's
// cancellation no longer stops it.
func (s *Service) DeleteBranches(ctx context.Context, repositoryID string, req BranchDeleteRequest) (BranchDeleteResult, error) {
	c, ok := s.takeConfirmation(req.ConfirmationID, purposeBranchDelete, repositoryID, "")
	if !ok || c.branchDelete == nil {
		return BranchDeleteResult{}, problemf(ErrConflict, "删除确认已失效，请重新预览")
	}
	r, err := s.repository(repositoryID)
	if err != nil {
		return BranchDeleteResult{}, err
	}
	checkCtx, cancel := context.WithTimeout(ctx, writeTimeout)
	defer cancel()
	if err := acquire(checkCtx, r); err != nil {
		return BranchDeleteResult{}, err
	}
	defer release(r)
	runCtx, runCancel := context.WithTimeout(context.WithoutCancel(ctx), maxBatchWriteTimeout)
	defer runCancel()
	results := make([]BranchDeleteItemResult, len(c.branchDelete))
	var attempted []branchDeleteTarget
	var names []string
	for i, target := range c.branchDelete {
		results[i] = BranchDeleteItemResult{Branch: target.branch, Commit: target.commit}
		if target.group == BranchUnmerged && !req.AcknowledgeUnmerged {
			results[i].Status, results[i].Message = BranchSkipped, "有独有提交，未确认删除"
			continue
		}
		attempted = append(attempted, target)
		names = append(names, target.branch)
	}
	if len(names) != 0 {
		current, err := s.branchDeleteItems(runCtx, r, names)
		if err != nil {
			return BranchDeleteResult{}, err
		}
		next := 0
		for i := range results {
			if results[i].Status != "" {
				continue
			}
			results[i] = s.deleteBranch(runCtx, r.entry.AnchorPath, attempted[next], current[next])
			next++
		}
	}
	refreshCtx, refreshCancel := context.WithTimeout(context.Background(), readTimeout)
	defer refreshCancel()
	return BranchDeleteResult{Results: results, Repository: s.readRepository(refreshCtx, r)}, nil
}

// deleteBranch deletes target after comparing it with current, its state
// now. The caller holds the repository gate.
func (s *Service) deleteBranch(ctx context.Context, anchor string, target branchDeleteTarget, current BranchDeleteItem) BranchDeleteItemResult {
	result := BranchDeleteItemResult{Branch: target.branch, Commit: target.commit, Status: BranchSkipped}
	switch {
	case len(current.Blockers) != 0 || current.Commit != target.commit:
		result.Message = "分支已变化，请重新预览"
		return result
	case target.group == BranchContained && current.Group != BranchContained:
		result.Message = "分支的提交已不再被其他引用包含，请重新预览"
		return result
	}
	if _, stderr, err := s.gitOutput(ctx, anchor, "update-ref", "-d", "refs/heads/"+target.branch, target.commit); err != nil {
		result.Status, result.Message = BranchFailed, "Git 未删除分支: "+firstGitLine(redactLine(stderr))
		if errorKind(err) != ErrInternal {
			result.Message = "Git 未删除分支: " + err.Error()
		}
		return result
	}
	result.Status = BranchDeleted
	result.Message = fmt.Sprintf("已删除；在 Git 垃圾回收清除这些提交之前，可用 git branch %s %s 恢复", target.branch, target.commit)
	if _, stderr, err := s.gitOutput(ctx, anchor, "config", "--remove-section", "branch."+target.branch); err != nil && !strings.Contains(stderr, "no such section") {
		result.Message += "；分支配置未能删除: " + firstGitLine(redactLine(stderr))
	}
	return result
}

// branchDeleteItems reads names and groups them. A name's commits count
// as kept when reachable from refs other than the deletable names, so
// deleting several branches together never drops a commit that only they
// share. The caller holds r's gate.
func (s *Service) branchDeleteItems(ctx context.Context, r *repositoryState, names []string) ([]BranchDeleteItem, error) {
	_, listed, err := s.currentTreesListed(ctx, r, nil)
	if err != nil {
		return nil, err
	}
	anchor := r.entry.AnchorPath
	occupants := map[string]string{}
	var heads []string
	seen := map[string]bool{}
	for _, tree := range listed {
		if tree.Branch != nil {
			if _, ok := occupants[*tree.Branch]; !ok {
				occupants[*tree.Branch] = tree.Path
			}
		}
		if tree.Head != nil && isObjectID(*tree.Head) && !seen[*tree.Head] {
			seen[*tree.Head] = true
			heads = append(heads, *tree.Head)
		}
	}
	items := make([]BranchDeleteItem, len(names))
	var deletable []string
	for i, name := range names {
		items[i] = BranchDeleteItem{Branch: name, Group: BranchContained, Unmerged: CommitList{Items: []Commit{}}, Blockers: []Issue{}}
		b, err := s.readBranch(ctx, anchor, name)
		switch {
		case errorKind(err) == ErrNotFound:
			items[i].Blockers = append(items[i].Blockers, Issue{Code: issueMissing, Message: "分支已不存在"})
			continue
		case err != nil:
			return nil, err
		}
		items[i].Commit = b.commit
		if path, ok := occupants[name]; ok {
			items[i].Blockers = append(items[i].Blockers, Issue{Code: issueOccupied, Message: "分支 " + name + " 已被工作树检出，不能删除", Path: path})
			continue
		}
		deletable = append(deletable, name)
	}
	kept := []string{"--not"}
	for _, name := range deletable {
		kept = append(kept, "--exclude="+name)
	}
	// WebMux's protection refs, such as released worktrees' commits, all
	// live below refs/webmux/.
	kept = append(kept, "--branches", "--remotes", "--tags", "--glob=refs/webmux/*")
	kept = append(kept, heads...)
	limit := s.worktreeGate
	errs := make([]error, len(items))
	var wg sync.WaitGroup
	for i := range items {
		if len(items[i].Blockers) != 0 {
			continue
		}
		wg.Add(1)
		go func(item *BranchDeleteItem, failed *error) {
			defer wg.Done()
			select {
			case limit <- struct{}{}:
			case <-ctx.Done():
				*failed = ctx.Err()
				return
			}
			defer func() { <-limit }()
			list, err := s.listCommits(ctx, anchor, append([]string{item.Commit}, kept...)...)
			if err != nil {
				*failed = err
				return
			}
			item.Unmerged = list
			if list.Total > 0 {
				item.Group = BranchUnmerged
			}
		}(&items[i], &errs[i])
	}
	wg.Wait()
	if err := errors.Join(errs...); err != nil {
		return nil, err
	}
	return items, ctx.Err()
}
