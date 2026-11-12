package workspace

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestNULParsersPreservePathsAndRenameRecords(t *testing.T) {
	// Absent roots keep the parser test free of host symlinks such as macOS /tmp.
	path := "/webmux-absent/path with spaces\nand newline"
	trees, err := parseWorktrees([]byte("worktree "+path+"\x00HEAD abc123\x00branch refs/heads/main\x00future value\x00\x00worktree /webmux-absent/linked\x00HEAD def456\x00detached\x00locked reason with\nnewline\x00\x00"), "repo")
	if err != nil || len(trees) != 2 || trees[0].Path != path || !trees[1].Detached || trees[1].LockReason != "reason with\nnewline" {
		t.Fatalf("NUL parser: %+v %v", trees, err)
	}
	status, err := parseStatus([]byte("2 R. N... 100644 100644 100644 abc abc R100 destination\x00source\x00! .env\x00? file\nname\x00"))
	if err != nil || !status.dirty || !status.untracked || !status.ignored {
		t.Fatalf("status: %+v %v", status, err)
	}
	for _, malformed := range [][]byte{[]byte("2 R. N... 100644 100644 100644 abc abc R100 new\x00"), []byte("? path"), []byte("x\x00"), []byte("1 M\x00")} {
		if _, err := parseStatus(malformed); err == nil {
			t.Fatalf("accepted malformed status %q", malformed)
		}
	}
}
func TestSnapshotsTrackExternalStateAndProtectedContents(t *testing.T) {
	t.Parallel()
	isolateGit(t)
	root := initRepo(t)
	s, _ := testService(t, t.TempDir(), root)
	r := registerTest(t, s, root)
	path := filepath.Join(t.TempDir(), "external\nworktree")
	gitTest(t, root, "worktree", "add", "-b", "external", path, "HEAD")
	gitTest(t, path, "checkout", "--detach")
	putFile(t, filepath.Join(path, "tracked"), "modified\n")
	putFile(t, filepath.Join(path, "unknown\nname"), "untracked")
	putFile(t, filepath.Join(path, ".env"), "secret")
	gitTest(t, root, "worktree", "lock", "--reason", "keep\nreason", path)
	snapshot, err := s.Refresh(context.Background(), r.ID)
	if err != nil {
		t.Fatal(err)
	}
	tree, err := findTree(snapshot, worktreeID(r.ID, path))
	if err != nil {
		t.Fatal(err)
	}
	if !tree.Detached || tree.Branch != nil || tree.Head == nil || !tree.Locked || tree.LockReason != "keep\nreason" || tree.Dirty == nil || !*tree.Dirty || !tree.HasUntracked || !tree.HasIgnored {
		t.Fatalf("external state: %+v", tree)
	}
	preview, err := s.PreviewRemoval(context.Background(), r.ID, tree.ID)
	if err != nil || preview.ForceAllowed {
		t.Fatalf("locked deletion allowed: %+v %v", preview, err)
	}
	gitTest(t, root, "worktree", "unlock", path)
	if err := os.Rename(path, path+"-missing"); err != nil {
		t.Fatal(err)
	}
	snapshot, err = s.Refresh(context.Background(), r.ID)
	if err != nil {
		t.Fatal(err)
	}
	tree, err = findTree(snapshot, tree.ID)
	if err != nil {
		t.Fatal(err)
	}
	if tree.Available || tree.Dirty != nil || tree.Error == "" {
		t.Fatalf("missing tree shown clean: %+v", tree)
	}
}
func TestBareUnbornAndBranchBaselines(t *testing.T) {
	t.Parallel()
	isolateGit(t)
	unborn := t.TempDir()
	gitTest(t, unborn, "init", "-b", "main")
	s, _ := testService(t, t.TempDir(), unborn)
	r := registerTest(t, s, unborn)
	branches, err := s.Branches(context.Background(), r.ID)
	if err != nil || branches.DefaultBase != nil || len(branches.Bases) != 0 {
		t.Fatalf("unborn branches: %+v %v", branches, err)
	}
	if _, err := s.CreateWorktree(context.Background(), r.ID, CreateRequest{Path: filepath.Join(t.TempDir(), "unborn"), Mode: "new", Branch: "topic", Base: "HEAD"}); errorKind(err) != ErrInvalid {
		t.Fatalf("unborn creation: %v", err)
	}
	item, err := s.CreateSession(context.Background(), CreateInput{Cmd: []string{"/bin/sh", "-c", "printf UNBORN"}, Workspace: &Reference{r.ID, r.Worktrees[0].ID}})
	if err != nil {
		t.Fatal(err)
	}
	waitDone(t, item)
	root := initRepo(t)
	bare := filepath.Join(t.TempDir(), "bare.git")
	gitTest(t, root, "clone", "--bare", root, bare)
	br := registerTest(t, s, bare)
	branches, err = s.Branches(context.Background(), br.ID)
	if err != nil || branches.DefaultBase != nil || len(branches.Bases) == 0 || branches.DefaultParent != bare+".worktrees" {
		t.Fatalf("bare branches: %+v %v", branches, err)
	}
	if _, err := s.CreateSession(context.Background(), CreateInput{Cmd: []string{"/bin/sh"}, Workspace: &Reference{br.ID, br.Worktrees[0].ID}}); errorKind(err) != ErrConflict {
		t.Fatalf("bare launch: %v", err)
	}
	tree, err := s.CreateWorktree(context.Background(), br.ID, CreateRequest{Path: filepath.Join(t.TempDir(), "bare linked"), Mode: "new", Branch: "topic", Base: "refs/heads/main"})
	if err != nil || tree.Kind != "linked" {
		t.Fatalf("bare linked create: %+v %v", tree, err)
	}
}
func TestBranchesDefaultBranchUsesMainWorktree(t *testing.T) {
	t.Parallel()
	isolateGit(t)
	root := initRepo(t)
	gitTest(t, root, "branch", "same-head")
	s, _ := testService(t, t.TempDir(), root)
	r := registerTest(t, s, root)
	branches, err := s.Branches(context.Background(), r.ID)
	if err != nil {
		t.Fatal(err)
	}
	if branches.DefaultBranch == nil || *branches.DefaultBranch != "main" {
		t.Fatalf("default branch guessed from shared OID: %+v", branches)
	}
}

func TestBranchesOrderedByLatestCommit(t *testing.T) {
	t.Parallel()
	isolateGit(t)
	root := initRepo(t)
	commitAt := func(branch, date string) {
		gitTest(t, root, "checkout", "-b", branch, "main")
		gitTestEnv(t, root, []string{"GIT_COMMITTER_DATE=" + date, "GIT_AUTHOR_DATE=" + date}, "commit", "--allow-empty", "-m", branch)
	}
	commitAt("a-oldest", "2000-01-01T00:00:00Z")
	commitAt("z-newest", "2020-01-01T00:00:00Z")
	commitAt("m-middle", "2010-01-01T00:00:00Z")
	gitTest(t, root, "checkout", "main")
	s, _ := testService(t, t.TempDir(), root)
	r := registerTest(t, s, root)
	branches, err := s.Branches(context.Background(), r.ID)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, branch := range branches.Branches {
		names = append(names, branch.Name)
	}
	if got, want := strings.Join(names, ","), "main,z-newest,m-middle,a-oldest"; got != want {
		t.Fatalf("branch order = %s, want %s", got, want)
	}
}

func TestBranchesDescribeCommitsIncludingDetachedDefault(t *testing.T) {
	t.Parallel()
	isolateGit(t)
	root := initRepo(t)
	gitTest(t, root, "update-ref", "refs/remotes/origin/main", "HEAD")
	gitTest(t, root, "commit", "--allow-empty", "-m", "修复终端重连\t与输入", "-m", "不展示提交正文")
	gitTest(t, root, "checkout", "--detach")
	gitTest(t, root, "commit", "--allow-empty", "-m", "独立提交 <title> & 引号")
	head := gitTest(t, root, "rev-parse", "HEAD")
	s, _ := testService(t, t.TempDir(), root)
	r := registerTest(t, s, root)
	branches, err := s.Branches(context.Background(), r.ID)
	if err != nil {
		t.Fatal(err)
	}
	if branches.DefaultBase == nil || *branches.DefaultBase != head || branches.DefaultBaseSubject != "独立提交 <title> & 引号" || branches.DefaultBranch != nil {
		t.Fatalf("detached default: %+v", branches)
	}
	subjects := map[string]string{}
	for _, base := range branches.Bases {
		subjects[base.Name] = base.Subject
	}
	if subjects["refs/heads/main"] != "修复终端重连\t与输入" || subjects["refs/remotes/origin/main"] != "initial" {
		t.Fatalf("commit subjects: %+v", subjects)
	}
}

func TestRecentCommitsListsHistoryFromObjectID(t *testing.T) {
	t.Parallel()
	isolateGit(t)
	root := initRepo(t)
	first := gitTest(t, root, "rev-parse", "HEAD")
	gitTest(t, root, "commit", "--allow-empty", "-m", "修复终端重连\t与输入", "-m", "不展示提交正文")
	head := gitTest(t, root, "rev-parse", "HEAD")
	s, _ := testService(t, t.TempDir(), root)
	r := registerTest(t, s, root)

	commits, err := s.RecentCommits(context.Background(), r.ID, []string{head})
	if err != nil || len(commits) != 2 {
		t.Fatalf("commits: %+v %v", commits, err)
	}
	if commits[0].Hash != head || commits[0].Subject != "修复终端重连\t与输入" || commits[0].Author == "" || commits[0].CommittedAt == "" || commits[1].Hash != first {
		t.Fatalf("history order or fields: %+v", commits)
	}
	older, err := s.RecentCommits(context.Background(), r.ID, []string{first})
	if err != nil || len(older) != 1 || older[0].Hash != first {
		t.Fatalf("history from older commit: %+v %v", older, err)
	}
	// A diverged upstream tip is merged into the same history once.
	upstream := gitTest(t, root, "commit-tree", first+"^{tree}", "-p", first, "-m", "上游提交")
	merged, err := s.RecentCommits(context.Background(), r.ID, []string{head, upstream})
	if err != nil || len(merged) != 3 {
		t.Fatalf("merged history: %+v %v", merged, err)
	}
	seen := map[string]bool{}
	for _, commit := range merged {
		seen[commit.Hash] = true
	}
	if !seen[head] || !seen[upstream] || !seen[first] {
		t.Fatalf("merged history misses a tip: %+v", merged)
	}
	for _, bad := range [][]string{nil, {""}, {"main"}, {"--all"}, {head[:12]}, {strings.ToUpper(head)}, {head, "main"}, {head, upstream, first}} {
		if _, err := s.RecentCommits(context.Background(), r.ID, bad); errorKind(err) != ErrInvalid {
			t.Fatalf("from %q accepted: %v", bad, err)
		}
	}
	if _, err := s.RecentCommits(context.Background(), r.ID, []string{head, strings.Repeat("0", 40)}); errorKind(err) != ErrNotFound {
		t.Fatalf("missing object: %v", err)
	}
}

func TestRecentCommitsCapsHistory(t *testing.T) {
	t.Parallel()
	isolateGit(t)
	root := initRepo(t)
	for i := range recentCommitLimit + 5 {
		gitTest(t, root, "commit", "--allow-empty", "-m", fmt.Sprintf("c%d", i))
	}
	s, _ := testService(t, t.TempDir(), root)
	r := registerTest(t, s, root)
	commits, err := s.RecentCommits(context.Background(), r.ID, []string{gitTest(t, root, "rev-parse", "HEAD")})
	if err != nil || len(commits) != recentCommitLimit {
		t.Fatalf("cap: %d %v", len(commits), err)
	}
}

func TestRecentCommitsKeepsTipsOutsideCap(t *testing.T) {
	t.Parallel()
	isolateGit(t)
	root := initRepo(t)
	stale := gitTest(t, root, "rev-parse", "HEAD")
	oldDate := []string{"GIT_COMMITTER_DATE=2020-01-01T00:00:00Z", "GIT_AUTHOR_DATE=2020-01-01T00:00:00Z"}
	diverged := gitTestEnv(t, root, oldDate, "commit-tree", stale+"^{tree}", "-p", stale, "-m", "上游旧提交")
	for i := range recentCommitLimit + 5 {
		gitTest(t, root, "commit", "--allow-empty", "-m", fmt.Sprintf("c%d", i))
	}
	head := gitTest(t, root, "rev-parse", "HEAD")
	s, _ := testService(t, t.TempDir(), root)
	r := registerTest(t, s, root)

	for name, upstream := range map[string]string{"ahead": stale, "diverged": diverged} {
		commits, err := s.RecentCommits(context.Background(), r.ID, []string{head, upstream})
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		seen := map[string]bool{}
		for _, commit := range commits {
			seen[commit.Hash] = true
		}
		if !seen[head] || !seen[upstream] {
			t.Fatalf("%s: missing tip in %d rows", name, len(commits))
		}
		if name == "diverged" && len(commits) != recentCommitLimit+1 {
			t.Fatalf("%s: outside-cap tip not added exactly once: %d", name, len(commits))
		}
	}
}
func TestCreationContractsAndNoEnvironmentCopy(t *testing.T) {
	t.Parallel()
	isolateGit(t)
	root := initRepo(t)
	putFile(t, filepath.Join(root, ".env"), "do not copy")
	s, _ := testService(t, t.TempDir(), root)
	r := registerTest(t, s, root)
	gitTest(t, root, "branch", "existing")
	gitTest(t, root, "update-ref", "refs/remotes/origin/local-only", "HEAD")
	branches, err := s.Branches(context.Background(), r.ID)
	if err != nil {
		t.Fatal(err)
	}
	if branches.DefaultBase == nil || branches.DefaultParent != root+".worktrees" {
		t.Fatalf("defaults: %+v", branches)
	}
	remote := false
	for _, base := range branches.Bases {
		if base.Name == "refs/remotes/origin/local-only" {
			remote = true
		}
	}
	if !remote {
		t.Fatal("local remote-tracking base missing")
	}
	existingPath := filepath.Join(t.TempDir(), "existing tree")
	existing, err := s.CreateWorktree(context.Background(), r.ID, CreateRequest{Path: existingPath, Mode: "existing", Branch: "existing"})
	if err != nil || existing.Branch == nil || *existing.Branch != "existing" {
		t.Fatalf("existing branch: %+v %v", existing, err)
	}
	newPath := filepath.Join(t.TempDir(), "new\ntree")
	created, err := s.CreateWorktree(context.Background(), r.ID, CreateRequest{Path: newPath, Mode: "new", Branch: "feature/foo", Base: "refs/remotes/origin/local-only"})
	if err != nil {
		t.Fatal(err)
	}
	if created.Path != newPath {
		t.Fatalf("path changed: %q", created.Path)
	}
	if _, err := os.Stat(filepath.Join(newPath, ".env")); !os.IsNotExist(err) {
		t.Fatal("ignored environment file copied")
	}
	occupied := t.TempDir()
	putFile(t, filepath.Join(occupied, "sentinel"), "retain")
	for _, req := range []CreateRequest{
		{Path: filepath.Join(t.TempDir(), "a"), Mode: "new", Branch: "--force", Base: "HEAD"},
		{Path: filepath.Join(t.TempDir(), "b"), Mode: "new", Branch: "valid", Base: "--help"},
		{Path: filepath.Join(t.TempDir(), "c"), Mode: "new", Branch: "feature/foo", Base: "HEAD"},
		{Path: occupied, Mode: "new", Branch: "occupied", Base: "HEAD"},
		{Path: filepath.Join(root, "nested"), Mode: "new", Branch: "nested", Base: "HEAD"},
		{Path: filepath.Join(root, ".git", "nested"), Mode: "new", Branch: "nestedgit", Base: "HEAD"},
		{Path: filepath.Join(t.TempDir(), "d"), Mode: "existing", Branch: "existing"},
		{Path: filepath.Join(t.TempDir(), "e"), Mode: "existing", Branch: "remote-only"},
	} {
		if _, err := s.CreateWorktree(context.Background(), r.ID, req); err == nil {
			t.Fatalf("accepted unsafe creation: %+v", req)
		}
	}
	data, _ := os.ReadFile(filepath.Join(occupied, "sentinel"))
	if string(data) != "retain" {
		t.Fatal("existing directory overwritten")
	}
	if _, err := s.CreateSession(context.Background(), CreateInput{Cmd: []string{"/missing/agent"}, Workspace: &Reference{r.ID, created.ID}}); errorKind(err) != ErrInvalid {
		t.Fatalf("agent failure: %v", err)
	}
	item, err := s.CreateSession(context.Background(), CreateInput{Cmd: []string{"/bin/sh", "-c", "printf RETRY"}, Workspace: &Reference{r.ID, created.ID}})
	if err != nil {
		t.Fatal(err)
	}
	waitDone(t, item)
	if item.Info().Workspace == nil || item.Info().Workspace.WorktreeID != created.ID {
		t.Fatal("retry did not reuse created worktree")
	}
}
func TestFailedGitCreationExposesResidueAndCancellationReapsCommand(t *testing.T) {
	isolateGit(t)
	root := initRepo(t)
	s, _ := testService(t, t.TempDir(), root)
	r := registerTest(t, s, root)
	// Git's real post-checkout hook fails after creating the branch/worktree.
	hook := filepath.Join(root, ".git", "hooks", "post-checkout")
	putFile(t, hook, "#!/bin/sh\nexit 42\n")
	if err := os.Chmod(hook, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "partial")
	if _, err := s.CreateWorktree(context.Background(), r.ID, CreateRequest{Path: path, Mode: "new", Branch: "partial", Base: "HEAD"}); err == nil {
		t.Fatal("hook failure reported success")
	}
	snapshot, err := s.Refresh(context.Background(), r.ID)
	if err != nil {
		t.Fatal(err)
	}
	tree, err := findTree(snapshot, worktreeID(r.ID, path))
	if err != nil {
		t.Fatal("partial worktree hidden", err)
	}
	gitTest(t, root, "show-ref", "--verify", "refs/heads/partial")
	item, err := s.CreateSession(context.Background(), CreateInput{Cmd: []string{"/bin/sh", "-c", "printf RESIDUE"}, Workspace: &Reference{r.ID, tree.ID}})
	if err != nil {
		t.Fatal(err)
	}
	waitDone(t, item)
	marker := filepath.Join(t.TempDir(), "hook-ready")
	t.Setenv("WEBMUX_TEST_HOOK_READY", marker)
	putFile(t, hook, "#!/bin/sh\nprintf READY > \"$WEBMUX_TEST_HOOK_READY\"\nexec sleep 30\n")
	if err := os.Chmod(hook, 0700); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	finished := make(chan error, 1)
	cancelledPath := filepath.Join(t.TempDir(), "cancelled")
	go func() {
		_, err := s.CreateWorktree(ctx, r.ID, CreateRequest{Path: cancelledPath, Mode: "new", Branch: "cancelled", Base: "HEAD"})
		finished <- err
	}()
	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, err := os.Stat(marker); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("Git hook did not reach cancellation point")
		}
		time.Sleep(10 * time.Millisecond)
	}
	start := time.Now()
	cancel()
	select {
	case err := <-finished:
		if errorKind(err) != ErrTimeout || time.Since(start) > 5*time.Second {
			t.Fatalf("Git cancellation not bounded: %v %v", time.Since(start), err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("cancelled Git process was not reaped")
	}
	branches, err := s.Branches(context.Background(), r.ID)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, branch := range branches.Branches {
		if branch.Name == "cancelled" {
			found = true
		}
	}
	if !found {
		t.Fatal("cancelled operation branch residue was rolled back or hidden")
	}
}
func TestGitTrustAndPermissionErrorsNeverBecomeClean(t *testing.T) {
	isolateGit(t)
	root := initRepo(t)
	s, _ := testService(t, t.TempDir(), root)
	r := registerTest(t, s, root)
	// Git itself supplies the dubious-ownership check even under root CI.
	t.Setenv("GIT_TEST_ASSUME_DIFFERENT_OWNER", "1")
	snapshot, err := s.Refresh(context.Background(), r.ID)
	if err != nil || snapshot.Availability != "unavailable" || !strings.Contains(snapshot.Error, "dubious ownership") {
		t.Fatalf("trust failure: %+v %v", snapshot, err)
	}
	if strings.Contains(gitLocalConfig(t, root), "safe.directory") {
		t.Fatal("service modified repository trust")
	}
	item, err := s.CreateSession(context.Background(), CreateInput{Cwd: root, Cmd: []string{"/bin/sh", "-c", "printf TRUST_DIRECT"}})
	if err != nil {
		t.Fatal(err)
	}
	waitDone(t, item)
	if item.Info().Workspace != nil {
		t.Fatal("failed Git detection invented association")
	}
}
func gitLocalConfig(t *testing.T, root string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, ".git", "config"))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
func TestBranchesReportUpstreamTracking(t *testing.T) {
	t.Parallel()
	isolateGit(t)
	s, r, root, remote := upstreamFixture(t)
	gitTest(t, root, "branch", "ahead", "origin/main")
	gitTest(t, root, "branch", "behind", "origin/main")
	gitTest(t, root, "branch", "gone", "origin/main")
	gitTest(t, root, "branch", "plain")
	advanceUpstream(t, root, remote, "upstream change")
	gitTest(t, root, "fetch", "origin")
	gitTest(t, root, "update-ref", "refs/heads/ahead", gitTest(t, root, "commit-tree", "-p", "origin/main", "-m", "ahead", "origin/main^{tree}"))
	gitTest(t, root, "branch", "behind", "--set-upstream-to", "origin/main")
	gitTest(t, root, "update-ref", "refs/heads/behind", "origin/main~1")
	gitTest(t, root, "update-ref", "refs/remotes/origin/vanished", "HEAD")
	gitTest(t, root, "branch", "gone", "--set-upstream-to", "origin/vanished")
	gitTest(t, root, "update-ref", "-d", "refs/remotes/origin/vanished")
	gitTest(t, root, "branch", "ahead", "--set-upstream-to", "origin/main")

	branches, err := s.Branches(context.Background(), r.ID)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, b := range branches.Branches {
		got[b.Name] = describeStatus(worktreeStatus{upstream: b.Upstream, ahead: b.Ahead, behind: b.Behind, upstreamGone: b.UpstreamGone})
		if b.CommittedAt == "" || b.Subject == "" {
			t.Fatalf("branch %s lacks commit details: %+v", b.Name, b)
		}
	}
	want := map[string]worktreeStatus{
		"ahead":  {upstream: strptr("origin/main"), ahead: new(1), behind: new(0)},
		"behind": {upstream: strptr("origin/main"), ahead: new(0), behind: new(1)},
		"gone":   {upstream: strptr("origin/vanished"), upstreamGone: true},
		"plain":  {},
	}
	for name, status := range want {
		if got[name] != describeStatus(status) {
			t.Errorf("branch %s: %s, want %s", name, got[name], describeStatus(status))
		}
	}
	kinds := map[string]string{}
	for _, base := range branches.Bases {
		kinds[base.Name] = base.Kind
		if _, err := time.Parse(time.RFC3339, base.CommittedAt); err != nil {
			t.Errorf("base %s committedAt %q: %v", base.Name, base.CommittedAt, err)
		}
	}
	if kinds["refs/heads/main"] != "local" || kinds["refs/remotes/origin/main"] != "remote" {
		t.Fatalf("base kinds: %v", kinds)
	}
}

func TestBranchesTimingWithManyTrackedBranches(t *testing.T) {
	if testing.Short() {
		t.Skip("builds a 500-branch fixture")
	}
	t.Parallel()
	isolateGit(t)
	root := initRepo(t)
	const branches = 500
	fastImport(t, root, 2*branches, func(stream *strings.Builder) {
		for i := range branches {
			fmt.Fprintf(stream, "reset refs/heads/b%03d\nfrom :%d\n\nreset refs/remotes/origin/b%03d\nfrom :%d\n\n", i, 2*i+1, i, 2*i+2)
		}
	})
	config, err := os.OpenFile(filepath.Join(root, ".git", "config"), os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	fmt.Fprintf(config, "[remote \"origin\"]\n\turl = /nonexistent\n\tfetch = +refs/heads/*:refs/remotes/origin/*\n")
	for i := range branches {
		fmt.Fprintf(config, "[branch \"b%03d\"]\n\tremote = origin\n\tmerge = refs/heads/b%03d\n", i, i)
	}
	if err := config.Close(); err != nil {
		t.Fatal(err)
	}
	s, _ := testService(t, t.TempDir(), root)
	r := registerTest(t, s, root)
	started := time.Now()
	result, err := s.Branches(context.Background(), r.ID)
	elapsed := time.Since(started)
	if err != nil {
		t.Fatal(err)
	}
	tracked, merged := 0, 0
	for _, b := range result.Branches {
		if b.Behind != nil && *b.Behind == 1 && b.Ahead != nil && *b.Ahead == 0 {
			tracked++
		}
		if b.MergedIntoDefault {
			merged++
		}
	}
	// The time includes the for-each-ref --merged that fills mergedIntoDefault.
	t.Logf("Branches with %d tracked branches (%d merged into the default) took %v", branches, merged, elapsed)
	if tracked != branches {
		t.Fatalf("tracked branches: %d of %d", tracked, branches)
	}
	if elapsed > time.Second {
		t.Errorf("Branches took %v, over the 1s budget", elapsed)
	}
}
