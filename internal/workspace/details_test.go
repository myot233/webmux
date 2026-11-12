package workspace

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/rainchan/webmux/internal/session"
)

func TestParseStatusPorcelainV2(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		give string
		want worktreeStatus
	}{
		{
			name: "ordinary staged and unstaged",
			give: "1 M. N... 100644 100644 100644 a b staged\x001 .M N... 100644 100644 100644 a a unstaged\x001 MM N... 100644 100644 100644 a b both\x00",
			want: worktreeStatus{dirty: true, changes: WorktreeChanges{Staged: 2, Unstaged: 2}},
		},
		{
			name: "rename keeps the next record aligned",
			give: "2 R. N... 100644 100644 100644 a a R100 new name\x00old name\x00? after rename\x00",
			want: worktreeStatus{dirty: true, untracked: true, changes: WorktreeChanges{Staged: 1, Untracked: 1}},
		},
		{
			name: "conflict",
			give: "u UU N... 100644 100644 100644 100644 a b c conflicted\x00",
			want: worktreeStatus{dirty: true, changes: WorktreeChanges{Conflicted: 1}},
		},
		{
			name: "untracked and ignored",
			give: "? new\nfile\x00? other\x00! .env\x00",
			want: worktreeStatus{untracked: true, ignored: true, changes: WorktreeChanges{Untracked: 2}},
		},
		{
			name: "submodule",
			give: "1 .M S.M. 160000 160000 160000 a a sub module\x00",
			want: worktreeStatus{dirty: true, changes: WorktreeChanges{Unstaged: 1}},
		},
		{
			name: "initial",
			give: "# branch.oid (initial)\x00# branch.head main\x00",
			want: worktreeStatus{oid: "(initial)", head: "main"},
		},
		{
			name: "detached",
			give: "# branch.oid 1234\x00# branch.head (detached)\x00",
			want: worktreeStatus{oid: "1234", head: "(detached)"},
		},
		{
			name: "upstream counted",
			give: "# branch.oid 1234\x00# branch.head main\x00# branch.upstream origin/main\x00# branch.ab +3 -2\x00",
			want: worktreeStatus{oid: "1234", head: "main", upstream: strptr("origin/main"), ahead: new(3), behind: new(2)},
		},
		{
			name: "upstream gone",
			give: "# branch.oid 1234\x00# branch.head main\x00# branch.upstream origin/gone\x00",
			want: worktreeStatus{oid: "1234", head: "main", upstream: strptr("origin/gone"), upstreamGone: true},
		},
		{
			name: "not computed",
			give: "# branch.oid 1234\x00# branch.head main\x00# branch.upstream origin/main\x00# branch.ab +? -?\x00",
			want: worktreeStatus{oid: "1234", head: "main", upstream: strptr("origin/main")},
		},
		{
			name: "unknown headers are ignored",
			give: "# stash 2\x00",
			want: worktreeStatus{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseStatus([]byte(tt.give))
			if err != nil {
				t.Fatal(err)
			}
			if fmt.Sprint(describeStatus(got)) != fmt.Sprint(describeStatus(tt.want)) {
				t.Fatalf("parseStatus(%q) = %s, want %s", tt.give, describeStatus(got), describeStatus(tt.want))
			}
		})
	}
}

// describeStatus prints pointer fields by value for comparison.
func describeStatus(s worktreeStatus) string {
	show := func(p *int) string {
		if p == nil {
			return "nil"
		}
		return fmt.Sprint(*p)
	}
	upstream := "nil"
	if s.upstream != nil {
		upstream = *s.upstream
	}
	return fmt.Sprintf("oid=%s head=%s upstream=%s ahead=%s behind=%s gone=%v changes=%+v dirty=%v untracked=%v ignored=%v",
		s.oid, s.head, upstream, show(s.ahead), show(s.behind), s.upstreamGone, s.changes, s.dirty, s.untracked, s.ignored)
}

func TestParseTrack(t *testing.T) {
	t.Parallel()
	tests := []struct {
		give               string
		wantAhead, wantBeh *int
		wantGone           bool
	}{
		{give: "", wantAhead: new(0), wantBeh: new(0)},
		{give: "ahead 3", wantAhead: new(3), wantBeh: new(0)},
		{give: "behind 2", wantAhead: new(0), wantBeh: new(2)},
		{give: "ahead 3, behind 2", wantAhead: new(3), wantBeh: new(2)},
		{give: "gone", wantGone: true},
		{give: "unexpected words"},
	}
	for _, tt := range tests {
		ahead, behind, gone := parseTrack(tt.give)
		got := describeStatus(worktreeStatus{ahead: ahead, behind: behind, upstreamGone: gone})
		want := describeStatus(worktreeStatus{ahead: tt.wantAhead, behind: tt.wantBeh, upstreamGone: tt.wantGone})
		if got != want {
			t.Errorf("parseTrack(%q) = %s, want %s", tt.give, got, want)
		}
	}
}

func TestRedactURL(t *testing.T) {
	t.Parallel()
	tests := []struct{ give, want string }{
		{"https://user:token123@example.com/app.git", "https://user:***@example.com/app.git"},
		{"https://token123@example.com/app.git", "https://***@example.com/app.git"},
		{"http://token123@example.com:8080/app.git", "http://***@example.com:8080/app.git"},
		{"https://user:p%40ss%3Aword@example.com/app.git", "https://user:***@example.com/app.git"},
		{"ssh://git@example.com/app.git", "ssh://git@example.com/app.git"},
		{"ssh://git:secret99@example.com/app.git", "ssh://git:***@example.com/app.git"},
		{"git@example.com:team/app.git", "git@example.com:team/app.git"},
		{"https://example.com/app.git", "https://example.com/app.git"},
		{"/srv/git/app.git", "/srv/git/app.git"},
		{"https://user:pa ss99@example.com/app.git", "https://example.com/app.git"},
		{"https://us%zzer:secret99@example.com/app.git", "https://example.com/app.git"},
	}
	for _, tt := range tests {
		got := redactURL(tt.give)
		if got != tt.want {
			t.Errorf("redactURL(%q) = %q, want %q", tt.give, got, tt.want)
		}
		for _, secret := range []string{"token123", "p%40ss", "secret99", "pa ss99"} {
			if strings.Contains(got, secret) {
				t.Errorf("redactURL(%q) leaked %q", tt.give, secret)
			}
		}
	}
}

func TestParseRemotesPairsDirectionsAndSorts(t *testing.T) {
	t.Parallel()
	got := parseRemotes("upstream\thttps://token123@example.com/up.git (fetch)\nupstream\tgit@example.com:up.git (push)\n" +
		"origin\thttps://user:secret99@example.com/app.git (fetch)\norigin\thttps://user:secret99@example.com/app.git (push)\norigin\thttps://second.example/app.git (push)\n")
	want := []Remote{
		{Name: "origin", FetchURL: "https://user:***@example.com/app.git", PushURL: "https://user:***@example.com/app.git"},
		{Name: "upstream", FetchURL: "https://***@example.com/up.git", PushURL: "git@example.com:up.git"},
	}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("parseRemotes = %+v, want %+v", got, want)
	}
	if empty := parseRemotes(""); empty == nil || len(empty) != 0 {
		t.Fatalf("no remotes: %#v", empty)
	}
}

func TestRemotesAreRedacted(t *testing.T) {
	t.Parallel()
	isolateGit(t)
	root := initRepo(t)
	gitTest(t, root, "remote", "add", "origin", "https://user:secret99@example.com/app.git")
	gitTest(t, root, "remote", "set-url", "--push", "origin", "https://token123@example.com/app.git")
	s, _ := testService(t, t.TempDir(), root)
	r := registerTest(t, s, root)
	remotes, err := s.Remotes(context.Background(), r.ID)
	if err != nil {
		t.Fatal(err)
	}
	want := []Remote{{Name: "origin", FetchURL: "https://user:***@example.com/app.git", PushURL: "https://***@example.com/app.git"}}
	if fmt.Sprint(remotes.Remotes) != fmt.Sprint(want) {
		t.Fatalf("remotes = %+v", remotes)
	}
	if _, err := s.Remotes(context.Background(), "missing"); errorKind(err) != ErrNotFound {
		t.Fatalf("unknown repository: %v", err)
	}
}

// countingGit makes s run Git through a wrapper that logs each subcommand
// (argv[3], after "-C <dir>") and returns a reader of that log.
func countingGit(t *testing.T, s *Service) func() []string {
	t.Helper()
	dir := t.TempDir()
	log := filepath.Join(dir, "calls")
	wrapper := filepath.Join(dir, "git")
	putFile(t, wrapper, fmt.Sprintf("#!/bin/sh\nprintf '%%s\\n' \"$3\" >> %q\nexec %q \"$@\"\n", log, s.gitPath))
	if err := os.Chmod(wrapper, 0o700); err != nil {
		t.Fatal(err)
	}
	s.gitPath = wrapper
	return func() []string {
		t.Helper()
		data, err := os.ReadFile(log)
		if err != nil && !os.IsNotExist(err) {
			t.Fatal(err)
		}
		calls := strings.Fields(string(data))
		if err := os.Remove(log); err != nil && !os.IsNotExist(err) {
			t.Fatal(err)
		}
		return calls
	}
}

func countCalls(calls []string, name string) int {
	n := 0
	for _, call := range calls {
		if call == name {
			n++
		}
	}
	return n
}

// snapshotOf takes one fresh snapshot without the reassociation Refresh adds.
func snapshotOf(t *testing.T, s *Service, id string) Repository {
	t.Helper()
	r, err := s.repository(id)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := s.snapshot(context.Background(), r)
	if err != nil {
		t.Fatal(err)
	}
	return snapshot
}

// assertSnapshotProcesses checks a snapshot's Git processes: rev-parse and
// worktree list for the repository, rev-parse and status per worktree, and
// one batched log.
func assertSnapshotProcesses(t *testing.T, calls []string, trees int) {
	t.Helper()
	if countCalls(calls, "log") != 1 || countCalls(calls, "status") != trees || len(calls) != 2+2*trees+1 {
		t.Fatalf("snapshot of %d worktrees ran %v", trees, calls)
	}
}

func TestSnapshotReportsUpstreamCommitsAndChanges(t *testing.T) {
	t.Parallel()
	isolateGit(t)
	s, r, root, remote := upstreamFixture(t)
	advanceUpstream(t, root, remote, "upstream change")
	gitTest(t, root, "fetch", "origin")
	gitTest(t, root, "commit", "--allow-empty", "-m", "local change")
	putFile(t, filepath.Join(root, "tracked"), "staged\n")
	gitTest(t, root, "add", "tracked")
	putFile(t, filepath.Join(root, "tracked"), "staged and unstaged\n")
	putFile(t, filepath.Join(root, "new"), "untracked\n")
	topic := linkedTest(t, s, r, "topic")
	detached := filepath.Join(t.TempDir(), "detached")
	gitTest(t, root, "worktree", "add", "--detach", detached, "HEAD~1")
	gitTest(t, root, "update-ref", "refs/remotes/origin/vanished", "HEAD")
	gone := linkedTest(t, s, r, "gone")
	gitTest(t, gone.Path, "branch", "--set-upstream-to", "origin/vanished")
	gitTest(t, root, "update-ref", "-d", "refs/remotes/origin/vanished")

	calls := countingGit(t, s)
	snapshot := snapshotOf(t, s, r.ID)
	assertSnapshotProcesses(t, calls(), 4)

	byPath := map[string]Worktree{}
	for _, tree := range snapshot.Worktrees {
		byPath[tree.Path] = tree
	}
	main := byPath[root]
	if main.Upstream == nil || *main.Upstream != "origin/main" || main.Ahead == nil || *main.Ahead != 1 || main.Behind == nil || *main.Behind != 1 || main.UpstreamGone {
		t.Fatalf("main upstream: %s", describeTree(main))
	}
	if main.Changes == nil || *main.Changes != (WorktreeChanges{Staged: 1, Unstaged: 1, Untracked: 1}) || main.Dirty == nil || !*main.Dirty || !main.HasUntracked {
		t.Fatalf("main changes: %s", describeTree(main))
	}
	if main.Operation != nil {
		t.Fatalf("operation without one in progress: %s", *main.Operation)
	}
	for path, subject := range map[string]string{root: "local change", detached: "initial", topic.Path: "local change"} {
		tree := byPath[path]
		want := gitTest(t, path, "log", "-1", "--format=%H%x00%s%x00%an%x00%cI")
		if tree.LastCommit == nil || strings.Join([]string{tree.LastCommit.Hash, tree.LastCommit.Subject, tree.LastCommit.Author, tree.LastCommit.CommittedAt}, "\x00") != want || tree.LastCommit.Subject != subject {
			t.Fatalf("last commit of %s: %+v, want %q", path, tree.LastCommit, want)
		}
	}
	if topic := byPath[topic.Path]; topic.Upstream != nil || topic.Ahead != nil || topic.UpstreamGone || topic.Changes == nil {
		t.Fatalf("no upstream: %s", describeTree(topic))
	}
	if gone := byPath[gone.Path]; gone.Upstream == nil || *gone.Upstream != "origin/vanished" || !gone.UpstreamGone || gone.Ahead != nil || gone.Behind != nil {
		t.Fatalf("gone upstream: %s", describeTree(gone))
	}
	if d := byPath[detached]; d.Upstream != nil || d.Ahead != nil || d.UpstreamGone {
		t.Fatalf("detached upstream: %s", describeTree(d))
	}
}

func describeTree(tree Worktree) string {
	status := worktreeStatus{upstream: tree.Upstream, ahead: tree.Ahead, behind: tree.Behind, upstreamGone: tree.UpstreamGone}
	if tree.Changes != nil {
		status.changes = *tree.Changes
	}
	return fmt.Sprintf("%s changesKnown=%v error=%q", describeStatus(status), tree.Changes != nil, tree.Error)
}

func TestLastCommitFailureKeepsSnapshotAvailable(t *testing.T) {
	t.Parallel()
	isolateGit(t)
	root := initRepo(t)
	s, _ := testService(t, t.TempDir(), root)
	r := registerTest(t, s, root)
	realGit := s.gitPath
	wrapper := filepath.Join(t.TempDir(), "git")
	putFile(t, wrapper, fmt.Sprintf("#!/bin/sh\nif [ \"$3\" = log ]; then exit 1; fi\nexec %q \"$@\"\n", realGit))
	if err := os.Chmod(wrapper, 0o700); err != nil {
		t.Fatal(err)
	}
	s.gitPath = wrapper
	snapshot := snapshotOf(t, s, r.ID)
	tree := snapshot.Worktrees[0]
	if snapshot.Availability != "available" || !tree.Available || tree.LastCommit != nil || tree.Changes == nil || tree.Error != "" {
		t.Fatalf("log failure: %+v", snapshot)
	}
}

func TestStatusFailureLeavesDetailsUnknown(t *testing.T) {
	t.Parallel()
	isolateGit(t)
	s, r, root, _ := upstreamFixture(t)
	realGit := s.gitPath
	wrapper := filepath.Join(t.TempDir(), "git")
	putFile(t, wrapper, fmt.Sprintf("#!/bin/sh\nif [ \"$3\" = status ]; then exit 1; fi\nexec %q \"$@\"\n", realGit))
	if err := os.Chmod(wrapper, 0o700); err != nil {
		t.Fatal(err)
	}
	s.gitPath = wrapper
	tree := snapshotOf(t, s, r.ID).Worktrees[0]
	if tree.Path != root || tree.Changes != nil || tree.Upstream != nil || tree.Ahead != nil || tree.Dirty != nil || tree.LastCommit == nil {
		t.Fatalf("status failure: %s %+v", describeTree(tree), tree.LastCommit)
	}
}

func TestOperationInProgress(t *testing.T) {
	t.Parallel()
	isolateGit(t)
	root := initRepo(t)
	s, _ := testService(t, t.TempDir(), root)
	r := registerTest(t, s, root)
	linked := linkedTest(t, s, r, "operations")
	calls := countingGit(t, s)
	for _, tree := range []string{root, linked.Path} {
		dir, err := worktreeGitDir(tree)
		if err != nil {
			t.Fatal(err)
		}
		for _, tt := range []struct{ marker, want string }{
			{"MERGE_HEAD", "merge"}, {"rebase-merge", "rebase"}, {"rebase-apply", "rebase"},
			{"CHERRY_PICK_HEAD", "cherry-pick"}, {"REVERT_HEAD", "revert"}, {"BISECT_LOG", "bisect"},
		} {
			marker := filepath.Join(dir, tt.marker)
			if strings.HasPrefix(tt.marker, "rebase-") {
				if err := os.Mkdir(marker, 0o700); err != nil {
					t.Fatal(err)
				}
			} else {
				putFile(t, marker, "")
			}
			calls()
			snapshot := snapshotOf(t, s, r.ID)
			assertSnapshotProcesses(t, calls(), 2)
			for _, got := range snapshot.Worktrees {
				want := (*string)(nil)
				if got.Path == tree {
					want = &tt.want
				}
				if fmt.Sprint(deref(got.Operation)) != fmt.Sprint(deref(want)) {
					t.Fatalf("%s %s: operation %v at %s", tree, tt.marker, deref(got.Operation), got.Path)
				}
			}
			if err := os.RemoveAll(marker); err != nil {
				t.Fatal(err)
			}
		}
	}
}

func deref(s *string) string {
	if s == nil {
		return "<nil>"
	}
	return *s
}

func TestRealMergeConflictIsReported(t *testing.T) {
	t.Parallel()
	isolateGit(t)
	root := initRepo(t)
	s, _ := testService(t, t.TempDir(), root)
	r := registerTest(t, s, root)
	linked := linkedTest(t, s, r, "conflict")
	putFile(t, filepath.Join(linked.Path, "tracked"), "linked\n")
	gitTest(t, linked.Path, "commit", "-am", "linked side")
	putFile(t, filepath.Join(root, "tracked"), "main\n")
	gitTest(t, root, "commit", "-am", "main side")
	if _, err := s.git(context.Background(), linked.Path, "merge", "main"); err == nil {
		t.Fatal("merge did not conflict")
	}
	tree, err := findTree(snapshotOf(t, s, r.ID), linked.ID)
	if err != nil {
		t.Fatal(err)
	}
	if deref(tree.Operation) != "merge" || tree.Changes == nil || tree.Changes.Conflicted != 1 || tree.Dirty == nil || !*tree.Dirty {
		t.Fatalf("merge conflict: %s operation=%s", describeTree(tree), deref(tree.Operation))
	}
}

func TestWorktreeGitDirResolvesGitFiles(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	tree := filepath.Join(dir, "tree")
	target := filepath.Join(dir, "admin", "tree")
	for _, path := range []string{tree, target} {
		if err := os.MkdirAll(path, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	putFile(t, filepath.Join(target, "REVERT_HEAD"), "")
	for _, give := range []string{"gitdir: " + target + "\n", "gitdir: ../admin/tree\n", "gitdir: ../admin/tree"} {
		putFile(t, filepath.Join(tree, ".git"), give)
		if got := deref(detectOperation(tree)); got != "revert" {
			t.Fatalf("%q: operation %s", give, got)
		}
	}
	putFile(t, filepath.Join(tree, ".git"), "gitdir: ../moved-away\n")
	if got := detectOperation(tree); got != nil {
		t.Fatalf("missing gitdir: %s", *got)
	}
	putFile(t, filepath.Join(tree, ".git"), "not a git file\n")
	if got := detectOperation(tree); got != nil {
		t.Fatalf("malformed .git: %s", *got)
	}
}

func TestLastFetchedAtReadsLinkedWorktreeFetchHead(t *testing.T) {
	t.Parallel()
	isolateGit(t)
	s, r, root, _ := upstreamFixture(t)
	if got := snapshotOf(t, s, r.ID).LastFetchedAt; got != nil {
		t.Fatalf("never fetched: %v", got)
	}
	linked := linkedTest(t, s, r, "fetcher")
	before := time.Now().Add(-time.Second)
	gitTest(t, linked.Path, "fetch", "origin")
	if _, err := os.Stat(filepath.Join(root, ".git", "FETCH_HEAD")); !os.IsNotExist(err) {
		t.Fatalf("linked fetch wrote the common FETCH_HEAD: %v", err)
	}
	got := snapshotOf(t, s, r.ID).LastFetchedAt
	if got == nil || got.Before(before) || got.Location() != time.UTC {
		t.Fatalf("linked fetch: %v", got)
	}
}

// fastImport writes a linear history of n commits on refs/heads/history and
// lets extra add commands (reset/from lines) after it.
func fastImport(t *testing.T, root string, n int, extra func(*strings.Builder)) {
	t.Helper()
	var stream strings.Builder
	for i := 1; i <= n; i++ {
		fmt.Fprintf(&stream, "commit refs/heads/history\nmark :%d\ncommitter Workspace test <workspace@example.invalid> %d +0000\ndata 8\nc%06d\n", i, 1700000000+i, i)
		if i == 1 {
			stream.WriteString("from refs/heads/main\n")
		}
		stream.WriteString("\n")
	}
	if extra != nil {
		extra(&stream)
	}
	cmd := exec.Command("git", "-C", root, "fast-import", "--quiet")
	cmd.Stdin = strings.NewReader(stream.String())
	var output bytes.Buffer
	cmd.Stdout, cmd.Stderr = &output, &output
	// Registered like gitTestEnv so the session reaper leaves its status.
	if err := session.RunCommand(cmd); err != nil {
		t.Fatalf("fast-import: %v\n%s", err, output.Bytes())
	}
}

// TestStatusAheadBehindTimingWithLargeGap measures `status --branch` with a
// 10k commit gap to the upstream. Exact counts stay on unless this exceeds
// 2s (design risk "--no-ahead-behind").
func TestStatusAheadBehindTimingWithLargeGap(t *testing.T) {
	if testing.Short() {
		t.Skip("builds a 10k commit fixture")
	}
	t.Parallel()
	isolateGit(t)
	root := initRepo(t)
	const gap = 10000
	fastImport(t, root, gap, func(stream *strings.Builder) {
		fmt.Fprintf(stream, "reset refs/remotes/origin/main\nfrom :%d\n\n", gap)
	})
	gitTest(t, root, "config", "branch.main.remote", "origin")
	gitTest(t, root, "config", "branch.main.merge", "refs/heads/main")
	gitTest(t, root, "config", "remote.origin.url", "/nonexistent")
	gitTest(t, root, "config", "remote.origin.fetch", "+refs/heads/*:refs/remotes/origin/*")
	s, _ := testService(t, t.TempDir(), root)
	r := registerTest(t, s, root)
	started := time.Now()
	if _, err := s.git(context.Background(), root, "status", "--porcelain=v2", "-z", "--branch", "--untracked-files=normal", "--ignored=matching"); err != nil {
		t.Fatal(err)
	}
	t.Logf("status --branch with a %d commit gap took %v", gap, time.Since(started))
	tree := snapshotOf(t, s, r.ID).Worktrees[0]
	if tree.Ahead == nil || *tree.Ahead != 0 || tree.Behind == nil || *tree.Behind != gap {
		t.Fatalf("large gap: %s", describeTree(tree))
	}
}
