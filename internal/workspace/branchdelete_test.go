package workspace

import (
	"context"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func previewBranchDelete(t *testing.T, s *Service, r Repository, branches ...string) BranchDeletePreview {
	t.Helper()
	preview, err := s.PreviewBranchDelete(context.Background(), r.ID, branches)
	if err != nil {
		t.Fatalf("delete preview %v: %v", branches, err)
	}
	return preview
}

// commitOn adds a commit touching file on branch, creating the branch at
// start when it does not exist, without changing any worktree.
func commitOn(t *testing.T, root, branch, start, file string) string {
	t.Helper()
	if gitTest(t, root, "branch", "--list", branch) == "" {
		gitTest(t, root, "branch", branch, start)
	}
	other := filepath.Join(t.TempDir(), "wt")
	gitTest(t, root, "worktree", "add", "--quiet", other, branch)
	putFile(t, filepath.Join(other, file), branch+" "+file+"\n")
	gitTest(t, other, "add", file)
	gitTest(t, other, "commit", "-m", branch+" "+file)
	head := gitTest(t, other, "rev-parse", "HEAD")
	gitTest(t, root, "worktree", "remove", other)
	return head
}

func groupsOf(items []BranchDeleteItem) map[string]string {
	groups := map[string]string{}
	for _, item := range items {
		groups[item.Branch] = item.Group
		if len(item.Blockers) != 0 {
			groups[item.Branch] = "blocked:" + item.Blockers[0].Code
		}
	}
	return groups
}

func TestBranchDeletePreviewGroups(t *testing.T) {
	t.Parallel()
	isolateGit(t)
	s, r, root, _ := upstreamFixture(t)
	main := gitTest(t, root, "rev-parse", "main")
	gitTest(t, root, "branch", "feat/old")
	commitOn(t, root, "pushed", main, "pushed")
	gitTest(t, root, "push", "origin", "pushed:elsewhere")
	released := commitOn(t, root, "released", main, "released")
	gitTest(t, root, "update-ref", releasedRef("record"), released)
	detached := commitOn(t, root, "detached", main, "detached")
	gitTest(t, root, "worktree", "add", "--detach", filepath.Join(t.TempDir(), "detached"), detached)
	for _, file := range []string{"one", "two", "three"} {
		commitOn(t, root, "feat/sq", main, file)
	}
	gitTest(t, root, "push", "-u", "origin", "feat/sq")
	gitTest(t, root, "merge", "--squash", "feat/sq")
	gitTest(t, root, "commit", "-m", "squashed feat/sq")
	gitTest(t, root, "push", "origin", "--delete", "feat/sq")
	shared := commitOn(t, root, "twin/a", main, "twin")
	gitTest(t, root, "branch", "twin/b", shared)
	work := linkedTest(t, s, r, "work")

	preview := previewBranchDelete(t, s, r, "feat/old", "pushed", "released", "detached", "feat/sq", "twin/a", "twin/b", "work", "main")
	want := map[string]string{
		"feat/old": BranchContained, "pushed": BranchContained, "released": BranchContained, "detached": BranchContained,
		"feat/sq": BranchUnmerged, "twin/a": BranchUnmerged, "twin/b": BranchUnmerged,
		"work": "blocked:" + issueOccupied, "main": "blocked:" + issueOccupied,
	}
	got := groupsOf(preview.Items)
	for name, group := range want {
		if got[name] != group {
			t.Errorf("%s: group %s, want %s", name, got[name], group)
		}
	}
	if preview.ConfirmationID == "" || preview.ExpiresAt == nil {
		t.Fatal("deletable branches without a confirmation")
	}
	for _, item := range preview.Items {
		switch item.Branch {
		case "feat/sq":
			subjects := []string{}
			for _, commit := range item.Unmerged.Items {
				subjects = append(subjects, commit.Subject)
			}
			if item.Unmerged.Total != 3 || !slices.Equal(subjects, []string{"feat/sq three", "feat/sq two", "feat/sq one"}) {
				t.Errorf("squashed branch commits %+v", item.Unmerged)
			}
		case "work":
			if item.Blockers[0].Path != work.Path || item.Commit == "" {
				t.Errorf("occupied item %+v", item)
			}
		case "feat/old":
			if item.Commit != main || item.Unmerged.Total != 0 || item.Unmerged.Items == nil {
				t.Errorf("contained item %+v", item)
			}
		}
	}
	// Alone, a twin is kept by the other.
	if alone := previewBranchDelete(t, s, r, "twin/a"); groupsOf(alone.Items)["twin/a"] != BranchContained {
		t.Errorf("single twin %+v", alone.Items)
	}
	// Only blocked branches: nothing to confirm.
	if blocked := previewBranchDelete(t, s, r, "work"); blocked.ConfirmationID != "" || blocked.ExpiresAt != nil {
		t.Errorf("blocked preview confirmed: %+v", blocked)
	}

	for _, give := range [][]string{{}, {"feat/old", "feat/old"}, {"--x"}, make([]string, maxDeleteBranches+1)} {
		if _, err := s.PreviewBranchDelete(context.Background(), r.ID, give); errorKind(err) != ErrInvalid {
			t.Errorf("%q: %v, want ErrInvalid", give, err)
		}
	}
	if missing := previewBranchDelete(t, s, r, "nope"); groupsOf(missing.Items)["nope"] != "blocked:"+issueMissing {
		t.Errorf("missing branch %+v", missing.Items)
	}
}

func TestBranchesReportMergedIntoDefault(t *testing.T) {
	t.Parallel()
	isolateGit(t)
	root := initRepo(t)
	gitTest(t, root, "branch", "merged")
	commitOn(t, root, "unmerged", "main", "new")
	s, _ := testService(t, t.TempDir(), root)
	r := registerTest(t, s, root)
	branches, err := s.Branches(context.Background(), r.ID)
	if err != nil {
		t.Fatal(err)
	}
	merged := map[string]bool{}
	for _, b := range branches.Branches {
		merged[b.Name] = b.MergedIntoDefault
	}
	if !merged["main"] || !merged["merged"] || merged["unmerged"] {
		t.Fatalf("mergedIntoDefault %v", merged)
	}

	// An unborn main worktree merges nothing.
	gitTest(t, root, "switch", "--orphan", "fresh")
	branches, err = s.Branches(context.Background(), r.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, b := range branches.Branches {
		if b.MergedIntoDefault {
			t.Fatalf("%s merged into an unborn HEAD", b.Name)
		}
	}
}

func TestDeleteBranches(t *testing.T) {
	t.Parallel()
	isolateGit(t)
	s, r, root, remote := upstreamFixture(t)
	main := gitTest(t, root, "rev-parse", "main")
	gitTest(t, root, "branch", "--track", "feat/old", "origin/main")
	gitTest(t, root, "branch", "plain")
	gitTest(t, root, "branch", "moving")
	unique := commitOn(t, root, "feat/sq", main, "sq")

	preview := previewBranchDelete(t, s, r, "feat/old", "plain", "moving", "feat/sq")
	commitOn(t, root, "moving", main, "moved")
	result, err := s.DeleteBranches(context.Background(), r.ID, BranchDeleteRequest{ConfirmationID: preview.ConfirmationID})
	if err != nil {
		t.Fatal(err)
	}
	status := map[string]BranchDeleteItemResult{}
	for _, item := range result.Results {
		status[item.Branch] = item
	}
	if len(result.Results) != 4 || status["feat/old"].Status != BranchDeleted || status["plain"].Status != BranchDeleted ||
		status["moving"].Status != BranchSkipped || !strings.Contains(status["moving"].Message, "请重新预览") ||
		status["feat/sq"].Status != BranchSkipped || status["feat/sq"].Commit != unique {
		t.Fatalf("results %+v", result.Results)
	}
	if old := status["feat/old"]; old.Commit != main || !strings.Contains(old.Message, "git branch feat/old "+main) {
		t.Fatalf("deleted result %+v", old)
	}
	refs := gitTest(t, root, "for-each-ref", "--format=%(refname)", "refs/heads/")
	if strings.Contains(refs, "feat/old") || strings.Contains(refs, "refs/heads/plain") || !strings.Contains(refs, "refs/heads/moving") || !strings.Contains(refs, "feat/sq") {
		t.Fatalf("refs after delete:\n%s", refs)
	}
	if gitConfig(t, root, "branch.feat/old.remote") != "" || gitConfig(t, root, "branch.feat/old.merge") != "" {
		t.Fatal("the branch configuration stayed")
	}
	if gitTest(t, remote, "rev-parse", "main") != main {
		t.Fatal("the remote changed")
	}
	for _, tree := range result.Repository.Worktrees {
		if tree.Kind == "main" && deref(tree.Branch) != "main" {
			t.Fatalf("refreshed repository %+v", result.Repository)
		}
	}

	// The unmerged group needs its own acknowledgement.
	preview = previewBranchDelete(t, s, r, "feat/sq")
	result, err = s.DeleteBranches(context.Background(), r.ID, BranchDeleteRequest{ConfirmationID: preview.ConfirmationID, AcknowledgeUnmerged: true})
	if err != nil || result.Results[0].Status != BranchDeleted {
		t.Fatalf("acknowledged unmerged: %+v %v", result.Results, err)
	}
	if _, err := s.DeleteBranches(context.Background(), r.ID, BranchDeleteRequest{ConfirmationID: preview.ConfirmationID}); errorKind(err) != ErrConflict {
		t.Fatalf("reused confirmation: %v", err)
	}
	// A delete confirmation is for deleting only.
	preview = previewBranchDelete(t, s, r, "moving")
	if _, err := s.FastForward(context.Background(), r.ID, FastForwardRequest{ConfirmationID: preview.ConfirmationID}); errorKind(err) != ErrConflict {
		t.Fatalf("fast-forward with a delete confirmation: %v", err)
	}
}

func TestDeletedBranchRebuildsDetached(t *testing.T) {
	h := newQuickHarness(t, "")
	tree, id := h.releasable("rebuild-deleted")
	h.release(tree)
	preview, err := h.s.PreviewBranchDelete(context.Background(), h.repo.ID, []string{"rebuild-deleted"})
	if err != nil {
		t.Fatal(err)
	}
	// The released ref keeps the commit.
	if len(preview.Items) != 1 || preview.Items[0].Group != BranchContained {
		t.Fatalf("preview %+v", preview.Items)
	}
	result, err := h.s.DeleteBranches(context.Background(), h.repo.ID, BranchDeleteRequest{ConfirmationID: preview.ConfirmationID})
	if err != nil || result.Results[0].Status != BranchDeleted {
		t.Fatalf("delete: %+v %v", result.Results, err)
	}
	rebuild, err := h.s.PreviewRebuild(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	if len(rebuild.Blockers) != 0 || rebuild.Mode != RebuildDetached || rebuild.Reason != ReasonBranchMissing {
		t.Fatalf("rebuild preview %+v", rebuild)
	}
	rebuilt, err := h.s.RebuildWorktree(context.Background(), id, RebuildRequest{ReleaseID: rebuild.ReleaseID, Mode: rebuild.Mode, Commit: rebuild.Commit})
	if err != nil {
		t.Fatal(err)
	}
	if !rebuilt.Detached || deref(rebuilt.Head) != rebuild.Commit {
		t.Fatalf("rebuilt %+v", rebuilt)
	}
}
