package workspace

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"
)

// unsetEnv removes variables for one serial test; t.Setenv restores them.
func unsetEnv(t *testing.T, names ...string) {
	t.Helper()
	for _, name := range names {
		t.Setenv(name, "")
		if err := os.Unsetenv(name); err != nil {
			t.Fatal(err)
		}
	}
}

// fakeSSH makes root's ssh remotes fail like ssh does, chosen by host:
// auth.invalid refuses the key, hostkey.invalid has an untrusted host key,
// down.invalid does not resolve and slow.invalid never answers.
func fakeSSH(t *testing.T, root string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "ssh")
	putFile(t, path, `#!/bin/sh
case "$*" in
*auth.invalid*) echo "git@auth.invalid: Permission denied (publickey)." >&2 ;;
*hostkey.invalid*) echo "Host key verification failed." >&2 ;;
*down.invalid*) echo "ssh: Could not resolve hostname down.invalid: Name or service not known" >&2 ;;
*slow.invalid*) sleep 30 ;;
esac
exit 255
`)
	if err := os.Chmod(path, 0o700); err != nil {
		t.Fatal(err)
	}
	gitTest(t, root, "config", "core.sshCommand", path)
}

// gatedFetch makes s run Git through a wrapper that logs each subcommand
// and holds every fetch until release is called. calls reads the log.
func gatedFetch(t *testing.T, s *Service) (calls func() []string, release func()) {
	t.Helper()
	dir := t.TempDir()
	log, gate := filepath.Join(dir, "calls"), filepath.Join(dir, "gate")
	wrapper := filepath.Join(dir, "git")
	putFile(t, wrapper, fmt.Sprintf("#!/bin/sh\nprintf '%%s\\n' \"$3\" >> %q\nif [ \"$3\" = fetch ]; then\n  while [ ! -e %q ]; do sleep 0.02; done\nfi\nexec %q \"$@\"\n", log, gate, s.gitPath))
	if err := os.Chmod(wrapper, 0o700); err != nil {
		t.Fatal(err)
	}
	s.gitPath = wrapper
	var once sync.Once
	release = func() {
		once.Do(func() { putFile(t, gate, "") })
	}
	t.Cleanup(release)
	calls = func() []string {
		t.Helper()
		data, err := os.ReadFile(log)
		if err != nil && !os.IsNotExist(err) {
			t.Fatal(err)
		}
		return strings.Fields(string(data))
	}
	return calls, release
}

func waitFor(t *testing.T, what string, done func() bool) {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for !done() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func fetchTest(t *testing.T, s *Service, id string) FetchRemotesResult {
	t.Helper()
	result, err := s.FetchRemotes(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func TestRemoteEnvKeepsUserSSHChoice(t *testing.T) {
	isolateGit(t)
	unsetEnv(t, "GIT_SSH_COMMAND", "GIT_SSH")
	s, _, root, _ := upstreamFixture(t)
	ctx := context.Background()
	askpass := "SSH_ASKPASS_REQUIRE=never"
	if got := s.remoteEnv(ctx, root); !slices.Equal(got, []string{askpass, batchSSHCommand}) {
		t.Fatalf("nothing set: %v", got)
	}
	for _, name := range []string{"GIT_SSH_COMMAND", "GIT_SSH"} {
		t.Setenv(name, "/usr/local/bin/wrapped-ssh")
		if got := s.remoteEnv(ctx, root); !slices.Equal(got, []string{askpass}) {
			t.Fatalf("%s set: %v", name, got)
		}
		unsetEnv(t, name)
	}
	gitTest(t, root, "config", "core.sshCommand", "ssh -i ~/.ssh/deploy")
	if got := s.remoteEnv(ctx, root); !slices.Equal(got, []string{askpass}) {
		t.Fatalf("core.sshCommand set: %v", got)
	}
}

func TestRemoteGitEnvironmentAndResult(t *testing.T) {
	isolateGit(t)
	unsetEnv(t, "GIT_SSH_COMMAND", "GIT_SSH", "SSH_ASKPASS_REQUIRE")
	s, _, root, _ := upstreamFixture(t)
	dir := t.TempDir()
	log := filepath.Join(dir, "env")
	wrapper := filepath.Join(dir, "git")
	putFile(t, wrapper, fmt.Sprintf("#!/bin/sh\nprintf '%%s %%s|%%s\\n' \"$3\" \"${GIT_SSH_COMMAND-unset}\" \"${SSH_ASKPASS_REQUIRE-unset}\" >> %q\nexec %q \"$@\"\n", log, s.gitPath))
	if err := os.Chmod(wrapper, 0o700); err != nil {
		t.Fatal(err)
	}
	s.gitPath = wrapper
	ctx := context.Background()
	out, failure := s.remoteGit(ctx, root, "ls-remote", "--heads", "origin")
	if failure != nil || !strings.Contains(string(out), "\trefs/heads/main\n") {
		t.Fatalf("ls-remote origin: %q %+v", out, failure)
	}
	if _, err := s.git(ctx, root, "rev-parse", "HEAD"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	want := "config unset|unset\nls-remote ssh -o BatchMode=yes|never\nrev-parse unset|unset\n"
	if string(data) != want {
		t.Fatalf("environments:\n%s\nwant:\n%s", data, want)
	}
	missing := filepath.Join(t.TempDir(), "missing.git")
	if out, failure := s.remoteGit(ctx, root, "ls-remote", missing); out != nil || failure == nil || failure.Status != remoteRejected ||
		!strings.HasPrefix(failure.Message, "远端拒绝了请求：") {
		t.Fatalf("missing remote: %q %+v", out, failure)
	}
}

func TestClassifyRemoteFailures(t *testing.T) {
	t.Parallel()
	const token = "tok-9f8e7d"
	exit := errors.New("exit status 128")
	tests := []struct {
		name       string
		give       gitExit
		wantStatus string
		want       string
	}{
		{"https auth", gitExit{err: exit, stderr: "remote: Invalid username or password.\nfatal: Authentication failed for 'https://u:" + token + "@example.com/app.git/'\n"},
			remoteAuth, "认证失败，请在终端完成认证后重试：remote: Invalid username or password."},
		{"https auth url", gitExit{err: exit, stderr: "fatal: Authentication failed for 'https://u:" + token + "@example.com/app.git/'\n"},
			remoteAuth, "认证失败，请在终端完成认证后重试：fatal: Authentication failed for 'https://u:***@example.com/app.git/'"},
		{"no username", gitExit{err: exit, stderr: "fatal: could not read Username for 'https://example.com': terminal prompts disabled\n"},
			remoteAuth, "认证失败，请在终端完成认证后重试：fatal: could not read Username for 'https://example.com': terminal prompts disabled"},
		{"publickey", gitExit{err: exit, stderr: "git@example.com: Permission denied (publickey).\nfatal: Could not read from remote repository.\n"},
			remoteAuth, "认证失败，请在终端完成认证后重试：git@example.com: Permission denied (publickey)."},
		{"host key", gitExit{err: exit, stderr: "Host key verification failed.\nfatal: Could not read from remote repository.\n"},
			remoteAuth, "主机密钥未受信任，请先在终端连接一次并确认指纹：Host key verification failed."},
		{"resolve", gitExit{err: exit, stderr: "fatal: unable to access 'https://" + token + "@example.com/x.git/': Could not resolve host: example.com\n"},
			remoteNetwork, "网络不可达，请检查网络与远程地址：fatal: unable to access 'https://***@example.com/x.git/': Could not resolve host: example.com"},
		{"refused", gitExit{err: exit, stderr: "ssh: connect to host example.com port 22: Connection refused\n"},
			remoteNetwork, "网络不可达，请检查网络与远程地址：ssh: connect to host example.com port 22: Connection refused"},
		{"unreachable", gitExit{err: exit, stderr: "ssh: connect to host example.com port 22: Network is unreachable\n"},
			remoteNetwork, "网络不可达，请检查网络与远程地址：ssh: connect to host example.com port 22: Network is unreachable"},
		{"timeout", gitExit{err: exit, ctxErr: context.DeadlineExceeded, stderr: ""},
			remoteTimeout, "远程操作超时，请稍后重试"},
		{"push rejected", gitExit{err: exit, stderr: "To https://u:" + token + "@example.com/app.git\n ! [rejected]        main -> main (fetch first)\n"},
			remoteRejected, "远端拒绝了请求：To https://u:***@example.com/app.git"},
		{"not a repository", gitExit{err: exit, stderr: "fatal: '/srv/x' does not appear to be a git repository\nfatal: Could not read from remote repository.\n"},
			remoteRejected, "远端拒绝了请求：fatal: '/srv/x' does not appear to be a git repository"},
		{"other", gitExit{err: exit, stderr: "\n  \nerror: cannot lock ref 'refs/remotes/origin/main'\n"},
			remoteError, "Git 远程操作失败：error: cannot lock ref 'refs/remotes/origin/main'"},
		{"silent", gitExit{err: exit}, remoteError, "Git 远程操作失败"},
		{"progress", gitExit{err: exit, stderr: "Receiving objects:  10%\rReceiving objects:  20%\rfatal: early EOF\n"},
			remoteError, "Git 远程操作失败：fatal: early EOF"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := classifyRemote(tt.give, false)
			if got.Status != tt.wantStatus || got.Message != tt.want {
				t.Fatalf("classifyRemote = %+v, want %s %q", got, tt.wantStatus, tt.want)
			}
			if strings.Contains(got.Message, token) {
				t.Fatalf("token leaked: %q", got.Message)
			}
		})
	}
	long := classifyRemote(gitExit{err: exit, stderr: "fatal: https://" + token + "@example.com/" + strings.Repeat("长", 400) + "\n"}, false)
	if n := utf8.RuneCountInString(long.Message); n != maxRemoteMessage || strings.Contains(long.Message, token) || !strings.HasSuffix(long.Message, "…") {
		t.Fatalf("long message (%d characters): %q", n, long.Message)
	}
	if exceeded := classifyRemote(gitExit{err: exit, ctxErr: context.Canceled, stderr: "fatal: " + token}, true); exceeded.Status != remoteError || strings.Contains(exceeded.Message, token) {
		t.Fatalf("output limit: %+v", exceeded)
	}
}

func TestFetchRemotesPrunesAndAdvancesLastFetchedAt(t *testing.T) {
	t.Parallel()
	isolateGit(t)
	s, r, root, remote := upstreamFixture(t)
	gitTest(t, root, "push", "origin", "main:feat/old")
	gitTest(t, root, "branch", "--track", "feat/old", "origin/feat/old")
	gitTest(t, remote, "branch", "-D", "feat/old")
	remoteHead := advanceUpstream(t, root, remote, "remote latest")
	before := time.Now().Add(-time.Second)

	result := fetchTest(t, s, r.ID)
	if fmt.Sprint(result.Results) != fmt.Sprint([]FetchResult{{Remote: "origin", Status: remoteOK}}) {
		t.Fatalf("results: %+v", result.Results)
	}
	if refs := gitTest(t, root, "for-each-ref", "refs/remotes/origin/feat/old"); refs != "" {
		t.Fatalf("deleted remote branch kept: %q", refs)
	}
	if tracking := gitTest(t, root, "rev-parse", "refs/remotes/origin/main"); tracking != remoteHead {
		t.Fatalf("origin/main = %s, want %s", tracking, remoteHead)
	}
	fetched := result.Repository.LastFetchedAt
	if result.Repository.Availability != "available" || fetched == nil || fetched.Before(before) {
		t.Fatalf("repository after fetch: %+v", result.Repository)
	}
	main := result.Repository.Worktrees[0]
	if main.Kind != "main" || main.Behind == nil || *main.Behind != 1 {
		t.Fatalf("main worktree after fetch: %s", describeTree(main))
	}
	branches, err := s.Branches(context.Background(), r.ID)
	if err != nil {
		t.Fatal(err)
	}
	gone := false
	for _, b := range branches.Branches {
		gone = gone || (b.Name == "feat/old" && b.UpstreamGone)
	}
	if !gone {
		t.Fatalf("feat/old upstream not gone: %+v", branches.Branches)
	}

	old := time.Now().Add(-time.Hour)
	if err := os.Chtimes(filepath.Join(root, ".git", "FETCH_HEAD"), old, old); err != nil {
		t.Fatal(err)
	}
	again := fetchTest(t, s, r.ID).Repository.LastFetchedAt
	if again == nil || !again.After(old.Add(time.Minute)) {
		t.Fatalf("lastFetchedAt did not advance: %v", again)
	}
}

func TestFetchRemotesReportsEachRemote(t *testing.T) {
	t.Parallel()
	isolateGit(t)
	s, r, root, remote := upstreamFixture(t)
	fakeSSH(t, root)
	gitTest(t, root, "remote", "add", "down", "ssh://git@down.invalid/app.git")
	gitTest(t, root, "remote", "add", "fork", "ssh://git@auth.invalid/app.git")
	gitTest(t, root, "remote", "add", "mirror", "ssh://git@hostkey.invalid/app.git")
	remoteHead := advanceUpstream(t, root, remote, "remote latest")

	result := fetchTest(t, s, r.ID)
	want := []FetchResult{
		{Remote: "down", Status: remoteNetwork, Message: "网络不可达，请检查网络与远程地址：ssh: Could not resolve hostname down.invalid: Name or service not known"},
		{Remote: "fork", Status: remoteAuth, Message: "认证失败，请在终端完成认证后重试：git@auth.invalid: Permission denied (publickey)."},
		{Remote: "mirror", Status: remoteAuth, Message: "主机密钥未受信任，请先在终端连接一次并确认指纹：Host key verification failed."},
		{Remote: "origin", Status: remoteOK},
	}
	if fmt.Sprint(result.Results) != fmt.Sprint(want) {
		t.Fatalf("results:\n%+v\nwant:\n%+v", result.Results, want)
	}
	if tracking := gitTest(t, root, "rev-parse", "refs/remotes/origin/main"); tracking != remoteHead {
		t.Fatalf("origin not fetched after failures: %s", tracking)
	}
	if result.Repository.LastFetchedAt == nil {
		t.Fatalf("snapshot not refreshed: %+v", result.Repository)
	}
}

func TestFetchRemotesTimesOutOneRemote(t *testing.T) {
	t.Parallel()
	isolateGit(t)
	s, r, root, _ := upstreamFixture(t)
	fakeSSH(t, root)
	gitTest(t, root, "remote", "add", "slow", "ssh://git@slow.invalid/app.git")
	s.fetchRemoteTimeout = 2 * time.Second
	started := time.Now()
	result := fetchTest(t, s, r.ID)
	want := []FetchResult{{Remote: "origin", Status: remoteOK}, {Remote: "slow", Status: remoteTimeout, Message: "远程操作超时，请稍后重试"}}
	if fmt.Sprint(result.Results) != fmt.Sprint(want) {
		t.Fatalf("results: %+v", result.Results)
	}
	if elapsed := time.Since(started); elapsed > 15*time.Second {
		t.Fatalf("slow remote held the fetch for %v", elapsed)
	}
}

func TestFetchRemotesWithoutRemotes(t *testing.T) {
	t.Parallel()
	isolateGit(t)
	root := initRepo(t)
	s, _ := testService(t, t.TempDir(), root)
	r := registerTest(t, s, root)
	if _, err := s.FetchRemotes(context.Background(), r.ID); errorKind(err) != ErrConflict {
		t.Fatalf("no remotes: %v", err)
	}
	if _, err := s.FetchRemotes(context.Background(), "missing"); errorKind(err) != ErrNotFound {
		t.Fatalf("unknown repository: %v", err)
	}
}

// TestFetchRemotesSharesOneFlight runs two fetches while the first one's Git
// fetch is held: they share it, and a worktree is created meanwhile.
func TestFetchRemotesSharesOneFlight(t *testing.T) {
	t.Parallel()
	isolateGit(t)
	s, r, root, remote := upstreamFixture(t)
	remoteHead := advanceUpstream(t, root, remote, "remote latest")
	calls, release := gatedFetch(t, s)
	type outcome struct {
		result FetchRemotesResult
		err    error
	}
	fetch := func() chan outcome {
		done := make(chan outcome, 1)
		go func() {
			result, err := s.FetchRemotes(context.Background(), r.ID)
			done <- outcome{result, err}
		}()
		return done
	}
	first := fetch()
	waitFor(t, "the first fetch", func() bool { return countCalls(calls(), "fetch") == 1 })
	second := fetch()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	tree, err := s.CreateWorktree(ctx, r.ID, CreateRequest{Path: filepath.Join(t.TempDir(), "during fetch"), Mode: "new", Branch: "during", Base: "HEAD"})
	if err != nil || tree.Branch == nil || *tree.Branch != "during" {
		t.Fatalf("create during fetch: %+v %v", tree, err)
	}
	select {
	case got := <-first:
		t.Fatalf("fetch finished while held: %+v", got)
	default:
	}

	release()
	a, b := <-first, <-second
	if a.err != nil || b.err != nil {
		t.Fatalf("errors: %v %v", a.err, b.err)
	}
	if fmt.Sprint(a.result) != fmt.Sprint(b.result) || fmt.Sprint(a.result.Results) != fmt.Sprint([]FetchResult{{Remote: "origin", Status: remoteOK}}) {
		t.Fatalf("results differ:\n%+v\n%+v", a.result, b.result)
	}
	if n := countCalls(calls(), "fetch"); n != 1 {
		t.Fatalf("git fetch ran %d times", n)
	}
	if tracking := gitTest(t, root, "rev-parse", "refs/remotes/origin/main"); tracking != remoteHead {
		t.Fatalf("origin/main = %s", tracking)
	}
}

func TestFetchRemotesOutlivesItsCaller(t *testing.T) {
	t.Parallel()
	isolateGit(t)
	s, r, root, remote := upstreamFixture(t)
	remoteHead := advanceUpstream(t, root, remote, "remote latest")
	calls, release := gatedFetch(t, s)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := s.FetchRemotes(ctx, r.ID)
		done <- err
	}()
	waitFor(t, "the fetch", func() bool { return countCalls(calls(), "fetch") == 1 })
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled caller: %v", err)
	}
	release()
	state, err := s.repository(r.ID)
	if err != nil {
		t.Fatal(err)
	}
	waitFor(t, "the fetch to finish", func() bool {
		state.fetchMu.Lock()
		defer state.fetchMu.Unlock()
		return state.fetching == nil
	})
	if tracking := gitTest(t, root, "rev-parse", "refs/remotes/origin/main"); tracking != remoteHead {
		t.Fatalf("fetch did not complete: origin/main = %s", tracking)
	}
	if n := countCalls(calls(), "fetch"); n != 1 {
		t.Fatalf("git fetch ran %d times", n)
	}
}

// TestFetchFailureLogsNoURL fails a fetch whose URL holds a token: neither
// the result nor the log shows it, and the log has no URL at all.
func TestFetchFailureLogsNoURL(t *testing.T) {
	isolateGit(t)
	for _, name := range []string{"NO_PROXY", "no_proxy"} {
		t.Setenv(name, "*")
	}
	var logs bytes.Buffer
	original := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, nil)))
	defer slog.SetDefault(original)
	const token = "tok-5c4b3a"
	s, r, root, _ := upstreamFixture(t)
	fakeSSH(t, root)
	gitTest(t, root, "remote", "add", "leaky", "https://user:"+token+"@127.0.0.1:1/app.git")
	gitTest(t, root, "remote", "add", "private", "ssh://deploy:"+token+"@auth.invalid/app.git")
	s.fetchRemoteTimeout = 20 * time.Second
	result := fetchTest(t, s, r.ID)
	if len(result.Results) != 3 || result.Results[0].Remote != "leaky" || result.Results[0].Status == remoteOK || result.Results[2].Status != remoteAuth {
		t.Fatalf("results: %+v", result.Results)
	}
	for _, item := range result.Results {
		if strings.Contains(item.Message, token) {
			t.Fatalf("token in result: %+v", item)
		}
	}
	text := logs.String()
	for _, want := range []string{"repository=" + r.ID, "remote=leaky", "remote=private", "status=" + result.Results[0].Status, "status=auth"} {
		if !strings.Contains(text, want) {
			t.Fatalf("log lacks %q:\n%s", want, text)
		}
	}
	for _, leak := range []string{token, "127.0.0.1", "auth.invalid", "://"} {
		if strings.Contains(text, leak) {
			t.Fatalf("log contains %q:\n%s", leak, text)
		}
	}
}
