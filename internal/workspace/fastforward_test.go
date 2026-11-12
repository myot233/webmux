package workspace

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/rainchan/webmux/internal/session"
)

// behindFixture is upstreamFixture with origin/main one commit ahead of
// main, fetched, and a local branch fix/b tracking origin/main at main's
// old commit; it returns the new upstream commit.
func behindFixture(t *testing.T) (*Service, Repository, string, string, string) {
	t.Helper()
	isolateGit(t)
	s, r, root, remote := upstreamFixture(t)
	gitTest(t, root, "branch", "--track", "fix/b", "origin/main")
	up := advanceUpstream(t, root, remote, "upstream one")
	gitTest(t, root, "fetch", "origin")
	return s, r, root, remote, up
}

func previewFastForward(t *testing.T, s *Service, r Repository, branch string) FastForwardPreview {
	t.Helper()
	preview, err := s.PreviewFastForward(context.Background(), r.ID, branch)
	if err != nil {
		t.Fatalf("fast-forward preview %s: %v", branch, err)
	}
	return preview
}

func installGitHook(t *testing.T, dir, name, script string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	hook := filepath.Join(dir, name)
	putFile(t, hook, "#!/bin/sh\n"+script+"\n")
	if err := os.Chmod(hook, 0o700); err != nil {
		t.Fatal(err)
	}
}

func TestFastForwardCheckedOutAndUncheckedBranches(t *testing.T) {
	t.Parallel()
	s, r, root, _, up := behindFixture(t)
	tree := linkedTest(t, s, r, "work")
	old := gitTest(t, root, "rev-parse", "main")

	// Not checked out: only the ref moves.
	preview := previewFastForward(t, s, r, "fix/b")
	if preview.Worktree != nil || preview.From != old || preview.To != up || preview.Upstream != "origin/main" ||
		preview.Commits.Total != 1 || len(preview.Commits.Items) != 1 || preview.Commits.Items[0].Subject != "upstream one" ||
		preview.ConfirmationID == "" || preview.ExpiresAt == nil || len(preview.Blockers) != 0 || len(preview.Sessions) != 0 {
		t.Fatalf("unchecked preview %+v", preview)
	}
	if _, err := s.FastForward(context.Background(), r.ID, FastForwardRequest{ConfirmationID: preview.ConfirmationID}); err != nil {
		t.Fatal(err)
	}
	if got := gitTest(t, root, "rev-parse", "fix/b"); got != up {
		t.Fatalf("fix/b at %s, want %s", got, up)
	}
	for _, dir := range []string{root, tree.Path} {
		if _, err := os.Lstat(filepath.Join(dir, "remote")); !os.IsNotExist(err) {
			t.Fatalf("a worktree's files changed: %v", err)
		}
	}
	if gitTest(t, root, "rev-parse", "main") != old || gitTest(t, root, "status", "--porcelain") != "" {
		t.Fatal("the main worktree changed")
	}

	// Checked out: merged in its worktree.
	preview = previewFastForward(t, s, r, "main")
	if preview.Worktree == nil || preview.Worktree.Kind != "main" || preview.Worktree.LastCommit == nil || preview.ConfirmationID == "" {
		t.Fatalf("checked-out preview %+v", preview)
	}
	result, err := s.FastForward(context.Background(), r.ID, FastForwardRequest{ConfirmationID: preview.ConfirmationID})
	if err != nil {
		t.Fatal(err)
	}
	if gitTest(t, root, "rev-parse", "main") != up {
		t.Fatal("main did not move")
	}
	if data, err := os.ReadFile(filepath.Join(root, "remote")); err != nil || string(data) != "upstream one\n" {
		t.Fatalf("upstream file: %q %v", data, err)
	}
	main := result.Repository.Worktrees[0]
	if result.Repository.ID != r.ID || main.Kind != "main" || deref(main.Head) != up || main.Behind == nil || *main.Behind != 0 {
		t.Fatalf("refreshed repository %+v", main)
	}
}

func TestFastForwardPreviewBlockers(t *testing.T) {
	t.Parallel()
	s, r, root, _, up := behindFixture(t)
	old := gitTest(t, root, "rev-parse", "main")
	gitTest(t, root, "branch", "plain")
	gitTest(t, root, "push", "origin", "main:gone")
	gitTest(t, root, "branch", "--track", "gonebranch", "origin/gone")
	gitTest(t, root, "push", "origin", "--delete", "gone")
	gitTest(t, root, "branch", "--track", "synced", "origin/main")
	ahead := gitTest(t, root, "commit-tree", "-p", up, "-m", "local", up+"^{tree}")
	gitTest(t, root, "branch", "--track", "ahead", "origin/main")
	gitTest(t, root, "update-ref", "refs/heads/ahead", ahead)
	diverged := gitTest(t, root, "commit-tree", "-p", old, "-m", "local", old+"^{tree}")
	gitTest(t, root, "branch", "--track", "diverged", "origin/main")
	gitTest(t, root, "update-ref", "refs/heads/diverged", diverged)

	assertBlocked := func(name string, preview FastForwardPreview, code string) {
		t.Helper()
		if !slices.Contains(issueCodes(preview.Blockers), code) || preview.ConfirmationID != "" || preview.ExpiresAt != nil {
			t.Errorf("%s: blockers %+v, confirmation %q", name, preview.Blockers, preview.ConfirmationID)
		}
	}
	assertBlocked("no upstream", previewFastForward(t, s, r, "plain"), issueUpstream)
	assertBlocked("upstream gone", previewFastForward(t, s, r, "gonebranch"), issueUpstream)
	assertBlocked("synced", previewFastForward(t, s, r, "synced"), issueSame)
	assertBlocked("ahead", previewFastForward(t, s, r, "ahead"), issueAhead)
	divergedPreview := previewFastForward(t, s, r, "diverged")
	assertBlocked("diverged", divergedPreview, issueDiverged)
	if !strings.Contains(divergedPreview.Blockers[0].Message, "已分叉") {
		t.Errorf("diverged message %q", divergedPreview.Blockers[0].Message)
	}

	putFile(t, filepath.Join(root, "tracked"), "changed\n")
	changes := previewFastForward(t, s, r, "main")
	assertBlocked("changes", changes, issueChanges)
	if changes.Blockers[0].Message != "有未提交的修改，请先提交或贮藏" {
		t.Errorf("changes message %q", changes.Blockers[0].Message)
	}
	gitTest(t, root, "checkout", "--", "tracked")
	marker := filepath.Join(root, ".git", "rebase-merge")
	if err := os.Mkdir(marker, 0o700); err != nil {
		t.Fatal(err)
	}
	assertBlocked("rebasing", previewFastForward(t, s, r, "main"), issueOperation)
	if err := os.Remove(marker); err != nil {
		t.Fatal(err)
	}
	if clean := previewFastForward(t, s, r, "main"); len(clean.Blockers) != 0 || clean.ConfirmationID == "" {
		t.Fatalf("clean preview blocked: %+v", clean.Blockers)
	}

	for _, name := range []string{"--orphan", "@{-1}"} {
		if _, err := s.PreviewFastForward(context.Background(), r.ID, name); errorKind(err) != ErrInvalid {
			t.Errorf("%s: %v, want ErrInvalid", name, err)
		}
	}
	if _, err := s.PreviewFastForward(context.Background(), r.ID, "missing"); errorKind(err) != ErrNotFound {
		t.Errorf("missing branch: %v", err)
	}
}

func TestFastForwardPreviewCountsWhenTheSnapshotDoesNot(t *testing.T) {
	t.Parallel()
	s, r, root, remote, _ := behindFixture(t)
	up := advanceUpstream(t, root, remote, "upstream two")
	gitTest(t, root, "fetch", "origin")
	// The snapshot's status reports ahead/behind as not computed.
	wrapper := filepath.Join(t.TempDir(), "git")
	putFile(t, wrapper, fmt.Sprintf("#!/bin/sh\nif [ \"$3\" = status ]; then %q \"$@\" | sed -z 's/^# branch\\.ab .*/# branch.ab +? -?/'; exit 0; fi\nexec %q \"$@\"\n", s.gitPath, s.gitPath))
	if err := os.Chmod(wrapper, 0o700); err != nil {
		t.Fatal(err)
	}
	s.gitPath = wrapper
	if main := snapshotOf(t, s, r.ID).Worktrees[0]; main.Behind != nil || main.Ahead != nil || main.Upstream == nil {
		t.Fatalf("snapshot computed ahead/behind: %+v", main)
	}
	preview := previewFastForward(t, s, r, "main")
	if preview.Worktree == nil || preview.Worktree.Behind != nil || preview.To != up || preview.Commits.Total != 2 ||
		len(preview.Commits.Items) != 2 || preview.Commits.Items[0].Subject != "upstream two" || len(preview.Blockers) != 0 {
		t.Fatalf("preview %+v", preview)
	}
}

func TestFastForwardRechecksWhatThePreviewBound(t *testing.T) {
	t.Parallel()
	s, r, root, remote, _ := behindFixture(t)

	// The upstream moved on.
	preview := previewFastForward(t, s, r, "fix/b")
	up := advanceUpstream(t, root, remote, "upstream two")
	gitTest(t, root, "fetch", "origin")
	if _, err := s.FastForward(context.Background(), r.ID, FastForwardRequest{ConfirmationID: preview.ConfirmationID}); errorKind(err) != ErrConflict || !strings.Contains(err.Error(), "请重新预览") {
		t.Fatalf("moved upstream: %v", err)
	}
	// The branch moved.
	preview = previewFastForward(t, s, r, "fix/b")
	if preview.To != up || preview.Commits.Total != 2 {
		t.Fatalf("fresh preview %+v", preview)
	}
	gitTest(t, root, "update-ref", "refs/heads/fix/b", gitTest(t, root, "rev-parse", "origin/main~1"))
	if _, err := s.FastForward(context.Background(), r.ID, FastForwardRequest{ConfirmationID: preview.ConfirmationID}); errorKind(err) != ErrConflict {
		t.Fatalf("moved branch: %v", err)
	}
	// A confirmation is for fast-forwarding only, and only once.
	preview = previewFastForward(t, s, r, "fix/b")
	if _, err := s.DeleteBranches(context.Background(), r.ID, BranchDeleteRequest{ConfirmationID: preview.ConfirmationID, AcknowledgeUnmerged: true}); errorKind(err) != ErrConflict {
		t.Fatalf("delete with a fast-forward confirmation: %v", err)
	}
	if _, err := s.Push(context.Background(), r.ID, PushRequest{ConfirmationID: preview.ConfirmationID}); errorKind(err) != ErrConflict {
		t.Fatalf("push with a fast-forward confirmation: %v", err)
	}
	if gitTest(t, root, "rev-parse", "--verify", "fix/b") == "" {
		t.Fatal("fix/b deleted")
	}
	if _, err := s.FastForward(context.Background(), r.ID, FastForwardRequest{ConfirmationID: preview.ConfirmationID}); err != nil {
		t.Fatalf("the fast-forward confirmation after other purposes refused it: %v", err)
	}
	if _, err := s.FastForward(context.Background(), r.ID, FastForwardRequest{ConfirmationID: preview.ConfirmationID}); errorKind(err) != ErrConflict {
		t.Fatalf("a consumed confirmation passed: %v", err)
	}

	// Running sessions need an acknowledgement and keep running.
	item, err := s.CreateSession(context.Background(), CreateInput{Cwd: root, Cmd: []string{"/bin/sh", "-c", "printf READY; read value"}})
	if err != nil {
		t.Fatal(err)
	}
	waitOutput(t, item, "READY")
	preview = previewFastForward(t, s, r, "main")
	if len(preview.Sessions) != 1 || preview.Sessions[0].ID != item.Info().ID || !slices.Contains(issueCodes(preview.Warnings), issueSessions) {
		t.Fatalf("sessions %+v warnings %+v", preview.Sessions, preview.Warnings)
	}
	if _, err := s.FastForward(context.Background(), r.ID, FastForwardRequest{ConfirmationID: preview.ConfirmationID}); errorKind(err) != ErrInvalid {
		t.Fatalf("unacknowledged sessions: %v", err)
	}
	preview = previewFastForward(t, s, r, "main")
	if _, err := s.FastForward(context.Background(), r.ID, FastForwardRequest{ConfirmationID: preview.ConfirmationID, AcknowledgeSessions: true}); err != nil {
		t.Fatal(err)
	}
	if gitTest(t, root, "rev-parse", "main") != up || item.Info().Status == session.StatusExited {
		t.Fatalf("main not moved or session stopped: %s", item.Info().Status)
	}
}

func TestFastForwardRefusesToOverwriteUntrackedFiles(t *testing.T) {
	t.Parallel()
	s, r, root, _, _ := behindFixture(t)
	old := gitTest(t, root, "rev-parse", "main")
	mine := filepath.Join(root, "remote")
	putFile(t, mine, "mine\n")
	preview := previewFastForward(t, s, r, "main")
	if len(preview.Blockers) != 0 || !slices.Contains(issueCodes(preview.Warnings), issueUntracked) {
		t.Fatalf("preview %+v %+v", preview.Blockers, preview.Warnings)
	}
	_, err := s.FastForward(context.Background(), r.ID, FastForwardRequest{ConfirmationID: preview.ConfirmationID})
	if errorKind(err) != ErrConflict || !strings.Contains(err.Error(), "remote") || !strings.Contains(err.Error(), "覆盖") {
		t.Fatalf("overwrite: %v", err)
	}
	if data, readErr := os.ReadFile(mine); readErr != nil || string(data) != "mine\n" || gitTest(t, root, "rev-parse", "main") != old {
		t.Fatalf("worktree changed: %q %v", data, readErr)
	}
}

func TestFastForwardCompletesWhenTheRequestIsCancelled(t *testing.T) {
	t.Parallel()
	s, r, root, _, up := behindFixture(t)
	started := filepath.Join(t.TempDir(), "started")
	installGitHook(t, filepath.Join(root, ".git", "hooks"), "post-merge", fmt.Sprintf("printf x > %q\nsleep 1", started))
	preview := previewFastForward(t, s, r, "main")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		deadline := time.Now().Add(10 * time.Second)
		for time.Now().Before(deadline) {
			if _, err := os.Stat(started); err == nil {
				break
			}
			time.Sleep(10 * time.Millisecond)
		}
		cancel()
	}()
	if _, err := s.FastForward(ctx, r.ID, FastForwardRequest{ConfirmationID: preview.ConfirmationID}); err != nil {
		t.Fatal(err)
	}
	if ctx.Err() == nil {
		t.Fatal("the request was not cancelled during the fast-forward")
	}
	if gitTest(t, root, "rev-parse", "main") != up {
		t.Fatal("main did not move")
	}
}
