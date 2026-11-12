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

// switchFixture is a repository whose main worktree is on main, with a
// local branch feat/a and a linked worktree on work, all at the initial
// commit.
func switchFixture(t *testing.T) (*Service, Repository, string, Worktree) {
	t.Helper()
	isolateGit(t)
	root := initRepo(t)
	gitTest(t, root, "branch", "feat/a")
	s, _ := testService(t, t.TempDir(), root)
	r := registerTest(t, s, root)
	return s, r, root, linkedTest(t, s, r, "work")
}

func previewSwitch(t *testing.T, s *Service, r Repository, tree Worktree, target SwitchTarget) SwitchPreview {
	t.Helper()
	preview, err := s.PreviewSwitch(context.Background(), r.ID, tree.ID, target)
	if err != nil {
		t.Fatalf("preview %+v: %v", target, err)
	}
	return preview
}

func issueCodes(list []Issue) []string {
	codes := make([]string, len(list))
	for i, issue := range list {
		codes[i] = issue.Code
	}
	return codes
}

func currentBranch(t *testing.T, path string) string {
	t.Helper()
	return gitTest(t, path, "branch", "--show-current")
}

// installHook makes every checkout of the repository at root run script.
func installHook(t *testing.T, root, script string) {
	t.Helper()
	hook := filepath.Join(root, ".git", "hooks", "post-checkout")
	putFile(t, hook, "#!/bin/sh\n"+script+"\n")
	if err := os.Chmod(hook, 0o700); err != nil {
		t.Fatal(err)
	}
}

func TestSwitchTargetsResolve(t *testing.T) {
	t.Parallel()
	s, r, root, tree := switchFixture(t)
	head := gitTest(t, root, "rev-parse", "main")
	gitTest(t, root, "update-ref", "refs/remotes/origin/fix/b", head)
	gitTest(t, root, "update-ref", "refs/remotes/origin/taken", head)
	gitTest(t, root, "branch", "taken")
	tests := []struct {
		name        string
		give        SwitchTarget
		wantBranch  string
		wantCreates bool
		wantUp      string
		wantBlocker string
	}{
		{"local", SwitchTarget{Kind: SwitchLocal, Branch: "feat/a"}, "feat/a", false, "", ""},
		{"remote", SwitchTarget{Kind: SwitchRemote, Ref: "origin/fix/b"}, "fix/b", true, "origin/fix/b", ""},
		{"remote renamed", SwitchTarget{Kind: SwitchRemote, Ref: "origin/fix/b", Name: "mine"}, "mine", true, "origin/fix/b", ""},
		{"remote with local branch", SwitchTarget{Kind: SwitchRemote, Ref: "origin/taken"}, "taken", true, "origin/taken", issueExists},
		{"new", SwitchTarget{Kind: SwitchNew, Name: "feat/c", StartPoint: "main"}, "feat/c", true, "", ""},
		{"new from remote", SwitchTarget{Kind: SwitchNew, Name: "feat/d", StartPoint: "origin/fix/b"}, "feat/d", true, "", ""},
		{"new existing", SwitchTarget{Kind: SwitchNew, Name: "feat/a", StartPoint: "main"}, "feat/a", true, "", issueExists},
		{"detach", SwitchTarget{Kind: SwitchDetach, Commit: "main"}, "", false, "", ""},
	}
	for _, tt := range tests {
		preview := previewSwitch(t, s, r, tree, tt.give)
		got := preview.Target
		text := func(p *string) string {
			if p == nil {
				return ""
			}
			return *p
		}
		if got.Kind != tt.give.Kind || text(got.Branch) != tt.wantBranch || (got.Branch == nil) != (tt.wantBranch == "") ||
			got.CreatesBranch != tt.wantCreates || text(got.Upstream) != tt.wantUp || (got.Upstream == nil) != (tt.wantUp == "") ||
			got.Commit != head || got.Subject != "initial" {
			t.Errorf("%s: target %+v", tt.name, got)
		}
		blocked := tt.wantBlocker != ""
		if blocked && !slices.Contains(issueCodes(preview.Blockers), tt.wantBlocker) {
			t.Errorf("%s: blockers %v", tt.name, preview.Blockers)
		}
		if !blocked && len(preview.Blockers) != 0 {
			t.Errorf("%s: unexpected blockers %v", tt.name, preview.Blockers)
		}
		if (preview.ConfirmationID == "") != blocked || (preview.ExpiresAt == nil) != blocked {
			t.Errorf("%s: confirmation %q %v with blockers %v", tt.name, preview.ConfirmationID, preview.ExpiresAt, preview.Blockers)
		}
	}
	for _, give := range []SwitchTarget{
		{Kind: SwitchLocal, Branch: "--orphan"},
		{Kind: SwitchNew, Name: "--orphan", StartPoint: "main"},
		{Kind: SwitchRemote, Ref: "origin/fix/b", Name: "--orphan"},
		{Kind: SwitchRemote, Ref: "--orphan"},
		{Kind: SwitchRemote, Ref: "origin"},
		{Kind: SwitchLocal, Branch: "@{-1}"},
		{Kind: SwitchLocal, Branch: "missing"},
		{Kind: SwitchRemote, Ref: "origin/missing"},
		{Kind: SwitchNew, Name: "feat/e", StartPoint: "nope"},
		{Kind: SwitchNew, Name: "feat/e", StartPoint: "-x"},
		{Kind: SwitchDetach, Commit: "--orphan"},
		{Kind: SwitchDetach, Commit: strings.Repeat("0", 40)},
		{Kind: "orphan", Name: "x"},
	} {
		if _, err := s.PreviewSwitch(context.Background(), r.ID, tree.ID, give); errorKind(err) != ErrInvalid {
			t.Errorf("%+v: %v, want ErrInvalid", give, err)
		}
	}
	if gitTest(t, root, "for-each-ref", "--format=%(refname)", "refs/heads/") != "refs/heads/feat/a\nrefs/heads/main\nrefs/heads/taken\nrefs/heads/work" {
		t.Fatal("a preview created branches")
	}
}

func TestSwitchBlockersAndWarnings(t *testing.T) {
	t.Parallel()
	s, r, root, tree := switchFixture(t)
	feat := SwitchTarget{Kind: SwitchLocal, Branch: "feat/a"}
	assertBlocked := func(name string, preview SwitchPreview, code string) {
		t.Helper()
		if !slices.Contains(issueCodes(preview.Blockers), code) || preview.ConfirmationID != "" || preview.ExpiresAt != nil {
			t.Fatalf("%s: blockers %v, confirmation %q", name, preview.Blockers, preview.ConfirmationID)
		}
	}
	occupied := previewSwitch(t, s, r, tree, SwitchTarget{Kind: SwitchLocal, Branch: "main"})
	assertBlocked("occupied", occupied, issueOccupied)
	if occupied.Blockers[0].Path != r.Worktrees[0].Path || r.Worktrees[0].Kind != "main" {
		t.Fatalf("occupied path %q, want %q", occupied.Blockers[0].Path, r.Worktrees[0].Path)
	}
	assertBlocked("same", previewSwitch(t, s, r, tree, SwitchTarget{Kind: SwitchLocal, Branch: "work"}), issueSame)

	putFile(t, filepath.Join(tree.Path, "tracked"), "changed\n")
	assertBlocked("changes", previewSwitch(t, s, r, tree, feat), issueChanges)
	gitTest(t, tree.Path, "add", "tracked")
	assertBlocked("staged", previewSwitch(t, s, r, tree, feat), issueChanges)
	gitTest(t, tree.Path, "reset", "--hard")

	dir, err := worktreeGitDir(tree.Path)
	if err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(dir, "BISECT_LOG")
	putFile(t, marker, "")
	assertBlocked("operation", previewSwitch(t, s, r, tree, feat), issueOperation)
	if err := os.Remove(marker); err != nil {
		t.Fatal(err)
	}

	putFile(t, filepath.Join(tree.Path, "notes"), "untracked\n")
	putFile(t, filepath.Join(tree.Path, ".gitmodules"), "")
	gitTest(t, root, "worktree", "lock", "--reason", "on a USB disk", tree.Path)
	item, err := s.CreateSession(context.Background(), CreateInput{Cwd: tree.Path, Cmd: []string{"/bin/sh", "-c", "printf READY; read value"}})
	if err != nil {
		t.Fatal(err)
	}
	waitOutput(t, item, "READY")
	warned := previewSwitch(t, s, r, tree, feat)
	if len(warned.Blockers) != 0 || warned.ConfirmationID == "" {
		t.Fatalf("warnings blocked: %+v", warned.Blockers)
	}
	if got := issueCodes(warned.Warnings); !slices.Equal(got, []string{issueUntracked, issueLocked, issueSubmodules, issueSessions}) {
		t.Fatalf("warnings %v", warned.Warnings)
	}
	if !strings.Contains(warned.Warnings[1].Message, "on a USB disk") {
		t.Fatalf("lock reason missing: %q", warned.Warnings[1].Message)
	}
	if len(warned.Sessions) != 1 || warned.Sessions[0].ID != item.Info().ID {
		t.Fatalf("sessions %+v", warned.Sessions)
	}
}

func TestSwitchBlockersOfUnusualWorktrees(t *testing.T) {
	head := strings.Repeat("a", 40)
	clean := &WorktreeChanges{}
	tests := []struct {
		name string
		give Worktree
		plan switchPlan
		want []string
	}{
		{"bare", Worktree{Kind: "bare", Available: true}, switchPlan{target: SwitchTarget{Kind: SwitchDetach}}, []string{issueUnsupported}},
		{"unavailable", Worktree{Kind: "linked"}, switchPlan{target: SwitchTarget{Kind: SwitchDetach}}, []string{issueUnsupported}},
		{"prunable", Worktree{Kind: "linked", Available: true, Prunable: true, Changes: clean}, switchPlan{target: SwitchTarget{Kind: SwitchDetach}}, []string{issueUnsupported}},
		{"unknown status", Worktree{Kind: "linked", Available: true, Error: "status failed"}, switchPlan{target: SwitchTarget{Kind: SwitchDetach}}, []string{issueUnverified}},
		{"conflicted", Worktree{Kind: "main", Available: true, Changes: &WorktreeChanges{Conflicted: 1}, Operation: strptr("merge")},
			switchPlan{target: SwitchTarget{Kind: SwitchDetach}}, []string{issueChanges, issueOperation}},
		{"detached at target", Worktree{Kind: "main", Available: true, Changes: clean, Detached: true, Head: &head},
			switchPlan{target: SwitchTarget{Kind: SwitchDetach}, info: SwitchTargetInfo{Commit: head}}, []string{issueSame}},
		{"on target branch, detach", Worktree{Kind: "main", Available: true, Changes: clean, Branch: strptr("main"), Head: &head},
			switchPlan{target: SwitchTarget{Kind: SwitchDetach}, info: SwitchTargetInfo{Commit: head}}, []string{}},
	}
	for _, tt := range tests {
		if got := issueCodes(switchBlockers(tt.give, nil, tt.plan)); !slices.Equal(got, tt.want) {
			t.Errorf("%s: blockers %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestSwitchConfirmationIsOnlyForSwitching(t *testing.T) {
	t.Parallel()
	s, r, _, tree := switchFixture(t)
	preview := previewSwitch(t, s, r, tree, SwitchTarget{Kind: SwitchLocal, Branch: "feat/a"})
	if err := s.RemoveWorktree(context.Background(), r.ID, tree.ID, RemoveRequest{ConfirmationID: preview.ConfirmationID, Mode: "safe"}); errorKind(err) != ErrConflict {
		t.Fatalf("removal with a switch confirmation: %v", err)
	}
	if _, err := s.ReleaseWorktree(context.Background(), r.ID, tree.ID, ReleaseRequest{ConfirmationID: preview.ConfirmationID}); errorKind(err) != ErrConflict {
		t.Fatalf("release with a switch confirmation: %v", err)
	}
	assertExists(t, tree.Path)
	removal := previewTest(t, s, r, tree)
	if _, err := s.SwitchWorktree(context.Background(), r.ID, tree.ID, SwitchRequest{ConfirmationID: removal.ConfirmationID}); errorKind(err) != ErrConflict {
		t.Fatalf("switch with a removal confirmation: %v", err)
	}
	switched, err := s.SwitchWorktree(context.Background(), r.ID, tree.ID, SwitchRequest{ConfirmationID: preview.ConfirmationID})
	if err != nil {
		t.Fatal(err)
	}
	if deref(switched.Branch) != "feat/a" || switched.ID != tree.ID || currentBranch(t, tree.Path) != "feat/a" {
		t.Fatalf("switched to %+v", switched)
	}
	if _, err := s.SwitchWorktree(context.Background(), r.ID, tree.ID, SwitchRequest{ConfirmationID: preview.ConfirmationID}); errorKind(err) != ErrConflict {
		t.Fatalf("confirmation reused: %v", err)
	}
}

func TestSwitchRechecksWhatThePreviewBound(t *testing.T) {
	t.Parallel()
	s, r, root, tree := switchFixture(t)
	feat := SwitchTarget{Kind: SwitchLocal, Branch: "feat/a"}

	// A session started after the preview.
	preview := previewSwitch(t, s, r, tree, feat)
	item, err := s.CreateSession(context.Background(), CreateInput{Cwd: tree.Path, Cmd: []string{"/bin/sh", "-c", "printf READY; read value"}})
	if err != nil {
		t.Fatal(err)
	}
	waitOutput(t, item, "READY")
	if _, err := s.SwitchWorktree(context.Background(), r.ID, tree.ID, SwitchRequest{ConfirmationID: preview.ConfirmationID, AcknowledgeSessions: true}); errorKind(err) != ErrConflict || !strings.Contains(err.Error(), "请重新预览") {
		t.Fatalf("new session: %v", err)
	}

	// Sessions the request did not acknowledge.
	preview = previewSwitch(t, s, r, tree, feat)
	if len(preview.Sessions) != 1 {
		t.Fatalf("sessions %+v", preview.Sessions)
	}
	if _, err := s.SwitchWorktree(context.Background(), r.ID, tree.ID, SwitchRequest{ConfirmationID: preview.ConfirmationID}); errorKind(err) != ErrInvalid {
		t.Fatalf("unacknowledged sessions: %v", err)
	}

	// The target branch moved on.
	preview = previewSwitch(t, s, r, tree, feat)
	next := gitTest(t, root, "commit-tree", "-p", "feat/a", "-m", "next", "feat/a^{tree}")
	gitTest(t, root, "update-ref", "refs/heads/feat/a", next)
	if _, err := s.SwitchWorktree(context.Background(), r.ID, tree.ID, SwitchRequest{ConfirmationID: preview.ConfirmationID, AcknowledgeSessions: true}); errorKind(err) != ErrConflict || !strings.Contains(err.Error(), "请重新预览") {
		t.Fatalf("advanced target: %v", err)
	}
	if currentBranch(t, tree.Path) != "work" {
		t.Fatal("a rejected switch changed the branch")
	}

	// A fresh preview shows the new commit and switches, leaving the session running.
	preview = previewSwitch(t, s, r, tree, feat)
	if preview.Target.Commit != next {
		t.Fatalf("target commit %s, want %s", preview.Target.Commit, next)
	}
	switched, err := s.SwitchWorktree(context.Background(), r.ID, tree.ID, SwitchRequest{ConfirmationID: preview.ConfirmationID, AcknowledgeSessions: true})
	if err != nil {
		t.Fatal(err)
	}
	if deref(switched.Branch) != "feat/a" || deref(switched.Head) != next || item.Info().Status == session.StatusExited {
		t.Fatalf("switched %+v, session %s", switched, item.Info().Status)
	}
}

func TestSwitchRefusesToOverwriteUntrackedFiles(t *testing.T) {
	t.Parallel()
	s, r, root, tree := switchFixture(t)
	gitTest(t, root, "switch", "-c", "gen")
	if err := os.Mkdir(filepath.Join(root, "gen"), 0o700); err != nil {
		t.Fatal(err)
	}
	putFile(t, filepath.Join(root, "gen", "out.txt"), "generated\n")
	gitTest(t, root, "add", "gen")
	gitTest(t, root, "commit", "-m", "track gen")
	gitTest(t, root, "switch", "main")
	if err := os.Mkdir(filepath.Join(tree.Path, "gen"), 0o700); err != nil {
		t.Fatal(err)
	}
	mine := filepath.Join(tree.Path, "gen", "out.txt")
	putFile(t, mine, "mine\n")
	preview := previewSwitch(t, s, r, tree, SwitchTarget{Kind: SwitchLocal, Branch: "gen"})
	if len(preview.Blockers) != 0 || !slices.Contains(issueCodes(preview.Warnings), issueUntracked) {
		t.Fatalf("preview %+v %+v", preview.Blockers, preview.Warnings)
	}
	after, err := s.SwitchWorktree(context.Background(), r.ID, tree.ID, SwitchRequest{ConfirmationID: preview.ConfirmationID})
	if errorKind(err) != ErrConflict || !strings.Contains(err.Error(), "gen/out.txt") {
		t.Fatalf("overwrite: %v", err)
	}
	data, readErr := os.ReadFile(mine)
	if readErr != nil || string(data) != "mine\n" || currentBranch(t, tree.Path) != "work" || deref(after.Branch) != "work" {
		t.Fatalf("worktree changed: %q %v, branch %s", data, readErr, deref(after.Branch))
	}
}

func TestSwitchCompletesWhenTheRequestIsCancelled(t *testing.T) {
	t.Parallel()
	s, r, root, tree := switchFixture(t)
	started := filepath.Join(t.TempDir(), "started")
	installHook(t, root, fmt.Sprintf("printf x > %q\nsleep 1", started))
	preview := previewSwitch(t, s, r, tree, SwitchTarget{Kind: SwitchLocal, Branch: "feat/a"})
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
	switched, err := s.SwitchWorktree(ctx, r.ID, tree.ID, SwitchRequest{ConfirmationID: preview.ConfirmationID})
	if err != nil {
		t.Fatal(err)
	}
	if ctx.Err() == nil {
		t.Fatal("the request was not cancelled during the switch")
	}
	if deref(switched.Branch) != "feat/a" || currentBranch(t, tree.Path) != "feat/a" {
		t.Fatalf("switched %+v", switched)
	}
}

func TestSwitchToRemoteBranchSetsUpstream(t *testing.T) {
	t.Parallel()
	isolateGit(t)
	s, r, root, _ := upstreamFixture(t)
	gitTest(t, root, "push", "origin", "main:fix/b")
	gitTest(t, root, "fetch", "origin")
	tree := linkedTest(t, s, r, "work")
	preview := previewSwitch(t, s, r, tree, SwitchTarget{Kind: SwitchRemote, Ref: "origin/fix/b"})
	switched, err := s.SwitchWorktree(context.Background(), r.ID, tree.ID, SwitchRequest{ConfirmationID: preview.ConfirmationID})
	if err != nil {
		t.Fatal(err)
	}
	if merge := gitTest(t, root, "config", "--get", "branch.fix/b.merge"); merge != "refs/heads/fix/b" {
		t.Fatalf("branch.fix/b.merge = %q", merge)
	}
	if remote := gitTest(t, root, "config", "--get", "branch.fix/b.remote"); remote != "origin" {
		t.Fatalf("branch.fix/b.remote = %q", remote)
	}
	if deref(switched.Branch) != "fix/b" || deref(switched.Upstream) != "origin/fix/b" {
		t.Fatalf("switched %+v", switched)
	}
	// A new branch sets no upstream.
	preview = previewSwitch(t, s, r, tree, SwitchTarget{Kind: SwitchNew, Name: "feat/c", StartPoint: "origin/fix/b"})
	switched, err = s.SwitchWorktree(context.Background(), r.ID, tree.ID, SwitchRequest{ConfirmationID: preview.ConfirmationID})
	if err != nil {
		t.Fatal(err)
	}
	if deref(switched.Branch) != "feat/c" || switched.Upstream != nil {
		t.Fatalf("new branch %+v", switched)
	}
}

func TestSwitchReportsFailedHookAfterCheckout(t *testing.T) {
	t.Parallel()
	s, r, root, tree := switchFixture(t)
	installHook(t, root, "echo 'fatal: cannot reach https://user:secret@example.invalid/lfs' >&2\nexit 3")
	preview := previewSwitch(t, s, r, tree, SwitchTarget{Kind: SwitchDetach, Commit: "feat/a"})
	switched, err := s.SwitchWorktree(context.Background(), r.ID, tree.ID, SwitchRequest{ConfirmationID: preview.ConfirmationID})
	if errorKind(err) != ErrConflict || strings.Contains(err.Error(), "secret") || !strings.Contains(err.Error(), "https://user:***@example.invalid/lfs") {
		t.Fatalf("hook failure: %v", err)
	}
	if !switched.Detached || switched.Branch != nil || deref(switched.Head) != preview.Target.Commit {
		t.Fatalf("returned worktree does not show the checkout: %+v", switched)
	}
}

func TestSwitchArgsNeverForce(t *testing.T) {
	oid := strings.Repeat("b", 40)
	tests := []struct {
		give switchPlan
		want []string
	}{
		{switchPlan{target: SwitchTarget{Kind: SwitchLocal, Branch: "feat/a"}, info: SwitchTargetInfo{Branch: strptr("feat/a"), Commit: oid}},
			[]string{"switch", "--no-guess", "--end-of-options", "feat/a"}},
		{switchPlan{target: SwitchTarget{Kind: SwitchRemote, Ref: "origin/fix/b"}, info: SwitchTargetInfo{Branch: strptr("fix/b"), Commit: oid}},
			[]string{"switch", "--no-guess", "--track", "--create", "fix/b", "--end-of-options", "refs/remotes/origin/fix/b"}},
		{switchPlan{target: SwitchTarget{Kind: SwitchNew, Name: "feat/c", StartPoint: "main"}, info: SwitchTargetInfo{Branch: strptr("feat/c"), Commit: oid}},
			[]string{"switch", "--no-guess", "--no-track", "--create", "feat/c", "--end-of-options", oid}},
		{switchPlan{target: SwitchTarget{Kind: SwitchDetach, Commit: "main"}, info: SwitchTargetInfo{Commit: oid}},
			[]string{"switch", "--detach", "--end-of-options", oid}},
	}
	for _, tt := range tests {
		got := switchArgs(tt.give)
		if !slices.Equal(got, tt.want) {
			t.Errorf("%s: %v, want %v", tt.give.target.Kind, got, tt.want)
		}
		for _, forbidden := range []string{"--force", "-f", "--discard-changes", "--merge", "-m", "--orphan", "--ignore-other-worktrees"} {
			if slices.Contains(got, forbidden) {
				t.Errorf("%s uses %s", tt.give.target.Kind, forbidden)
			}
		}
	}
}

func TestOverwrittenPaths(t *testing.T) {
	checkout := "error: The following untracked working tree files would be overwritten by checkout:\n\ta.txt\n\tgen/out.txt\n" +
		"Please move or remove them before you switch branches.\n" +
		"error: Your local changes to the following files would be overwritten by checkout:\n\ttracked\n" +
		"Please commit your changes or stash them before you switch branches.\nAborting\n"
	if paths, more := overwrittenPaths(checkout); !slices.Equal(paths, []string{"a.txt", "gen/out.txt", "tracked"}) || more != 0 {
		t.Errorf("checkout: %v %d", paths, more)
	}
	merge := "error: Untracked working tree file 'x y' would be overwritten by merge.\nfatal: something\n"
	if paths, more := overwrittenPaths(merge); !slices.Equal(paths, []string{"x y"}) || more != 0 {
		t.Errorf("one-line form: %v %d", paths, more)
	}
	var many strings.Builder
	many.WriteString("error: The following untracked working tree files would be overwritten by merge:\n")
	for i := range maxOverwrittenPaths + 7 {
		fmt.Fprintf(&many, "\tf%02d\n", i)
	}
	many.WriteString("Aborting\n")
	if paths, more := overwrittenPaths(many.String()); len(paths) != maxOverwrittenPaths || paths[0] != "f00" || more != 7 {
		t.Errorf("many: %d paths, %d more", len(paths), more)
	}
	if paths, _ := overwrittenPaths("fatal: hook failed\n\tnot a path\n"); len(paths) != 0 {
		t.Errorf("unrelated output: %v", paths)
	}
}

func TestFirstGitLineSkipsProgressAndRedacts(t *testing.T) {
	output := "Switched to branch 'feat/a'\nYour branch is up to date with 'origin/feat/a'.\n\n" +
		"fatal: https://token@example.invalid/x.git: " + strings.Repeat("y", 400) + "\nsecond\n"
	got := firstGitLine(redactLine(output))
	if !strings.HasPrefix(got, "fatal: https://***@example.invalid/x.git: y") || strings.Contains(got, "token") || len(got) > maxGitLine+len("…") {
		t.Errorf("line %q", got)
	}
	if got := firstGitLine("Switched to a new branch 'x'\n"); got != "" {
		t.Errorf("progress only: %q", got)
	}
}
