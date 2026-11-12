package workspace

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/rainchan/webmux/internal/session"
)

func previewPush(t *testing.T, s *Service, r Repository, branch, remote, target string) PushPreview {
	t.Helper()
	preview, err := s.PreviewPush(context.Background(), r.ID, branch, remote, target)
	if err != nil {
		t.Fatalf("push preview %s: %v", branch, err)
	}
	return preview
}

func pushTest(t *testing.T, s *Service, r Repository, preview PushPreview) PushResult {
	t.Helper()
	if preview.ConfirmationID == "" {
		t.Fatalf("push preview blocked: %+v", preview.Blockers)
	}
	result, err := s.Push(context.Background(), r.ID, PushRequest{ConfirmationID: preview.ConfirmationID})
	if err != nil {
		t.Fatal(err)
	}
	return result
}

// localCommit commits an empty change on the branch checked out at dir.
func localCommit(t *testing.T, dir, subject string) string {
	t.Helper()
	gitTest(t, dir, "commit", "--allow-empty", "-m", subject)
	return gitTest(t, dir, "rev-parse", "HEAD")
}

// gitConfig reads one configuration value at dir, "" when unset.
func gitConfig(t *testing.T, dir, key string) string {
	t.Helper()
	cmd := exec.Command("git", "-C", dir, "config", "--get", key)
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := session.RunCommand(cmd); err != nil {
		return ""
	}
	return strings.TrimSpace(out.String())
}

func TestPushPreviewTargets(t *testing.T) {
	t.Parallel()
	isolateGit(t)
	s, r, root, remote := upstreamFixture(t)
	localCommit(t, root, "local one")

	upstream := previewPush(t, s, r, "main", "", "")
	if upstream.Publish || upstream.Remote != "origin" || upstream.Target != "main" || upstream.PushURL != remote ||
		upstream.Commit != gitTest(t, root, "rev-parse", "main") || upstream.Commits.Total != 1 || upstream.Commits.Items[0].Subject != "local one" ||
		upstream.ConfirmationID == "" || upstream.ExpiresAt == nil || len(upstream.Blockers) != 0 || len(upstream.Warnings) != 0 {
		t.Fatalf("upstream preview %+v", upstream)
	}
	for _, give := range [][2]string{{"origin", ""}, {"", "other"}} {
		if _, err := s.PreviewPush(context.Background(), r.ID, "main", give[0], give[1]); errorKind(err) != ErrInvalid {
			t.Errorf("upstream branch with target %v: %v", give, err)
		}
	}

	gitTest(t, root, "branch", "feat/c")
	publish := previewPush(t, s, r, "feat/c", "", "")
	if !publish.Publish || publish.Remote != "origin" || publish.Target != "feat/c" || publish.Commits.Total != 1 || publish.ConfirmationID == "" {
		t.Fatalf("publish preview %+v", publish)
	}
	if named := previewPush(t, s, r, "feat/c", "", "pub/c"); named.Target != "pub/c" {
		t.Fatalf("named target %+v", named)
	}
	for _, target := range []string{"--x", "a..b"} {
		if _, err := s.PreviewPush(context.Background(), r.ID, "feat/c", "", target); errorKind(err) != ErrInvalid {
			t.Errorf("target %q: %v", target, err)
		}
	}
	if _, err := s.PreviewPush(context.Background(), r.ID, "feat/c", "nope", ""); errorKind(err) != ErrInvalid {
		t.Errorf("unknown remote: %v", err)
	}

	// Several remotes: remote.pushDefault, then origin, else a blocker.
	fork := filepath.Join(t.TempDir(), "fork.git")
	gitTest(t, root, "init", "--bare", "-b", "main", fork)
	gitTest(t, root, "remote", "add", "fork", fork)
	if got := previewPush(t, s, r, "feat/c", "", "").Remote; got != "origin" {
		t.Fatalf("default remote %s, want origin", got)
	}
	gitTest(t, root, "config", "remote.pushDefault", "fork")
	if got := previewPush(t, s, r, "feat/c", "", ""); got.Remote != "fork" || got.PushURL != fork {
		t.Fatalf("pushDefault remote %+v", got)
	}
	gitTest(t, root, "config", "--unset", "remote.pushDefault")
	gitTest(t, root, "remote", "rename", "origin", "upstream")
	undecided := previewPush(t, s, r, "feat/c", "", "")
	if !slices.Contains(issueCodes(undecided.Blockers), issueRemote) || undecided.ConfirmationID != "" || undecided.ExpiresAt != nil ||
		!strings.Contains(undecided.Blockers[0].Message, "fork, upstream") {
		t.Fatalf("undecided remote %+v", undecided)
	}
	if chosen := previewPush(t, s, r, "feat/c", "fork", ""); chosen.Remote != "fork" || len(chosen.Blockers) != 0 {
		t.Fatalf("chosen remote %+v", chosen)
	}
	gitTest(t, root, "remote", "rename", "upstream", "origin")

	// A pushRemote other than the upstream's is only warned about.
	gitTest(t, root, "config", "branch.main.pushRemote", "fork")
	if warned := previewPush(t, s, r, "main", "", ""); warned.Remote != "origin" || !slices.Contains(issueCodes(warned.Warnings), issuePushRemote) ||
		!strings.Contains(warned.Warnings[0].Message, "fork") {
		t.Fatalf("pushRemote warning %+v", warned)
	}
	gitTest(t, root, "config", "--unset", "branch.main.pushRemote")

	// Diverged, behind and synced branches have nothing safe to push.
	gitTest(t, root, "push", "origin", "main")
	if synced := previewPush(t, s, r, "main", "", ""); !slices.Contains(issueCodes(synced.Blockers), issueSame) {
		t.Fatalf("synced %+v", synced.Blockers)
	}
	advanceUpstream(t, root, remote, "upstream one")
	gitTest(t, root, "fetch", "origin")
	if behind := previewPush(t, s, r, "main", "", ""); !slices.Contains(issueCodes(behind.Blockers), issueBehind) {
		t.Fatalf("behind %+v", behind.Blockers)
	}
	localCommit(t, root, "local two")
	if diverged := previewPush(t, s, r, "main", "", ""); !slices.Contains(issueCodes(diverged.Blockers), issueDiverged) || diverged.ConfirmationID != "" {
		t.Fatalf("diverged %+v", diverged.Blockers)
	}

	// A deleted upstream is published again, with a warning.
	gitTest(t, root, "push", "origin", "feat/c:gone")
	gitTest(t, root, "branch", "--set-upstream-to", "origin/gone", "feat/c")
	gitTest(t, root, "push", "origin", "--delete", "gone")
	gone := previewPush(t, s, r, "feat/c", "", "")
	if !gone.Publish || gone.Target != "feat/c" || !slices.Contains(issueCodes(gone.Warnings), issueGone) || gone.ConfirmationID == "" {
		t.Fatalf("gone upstream %+v", gone)
	}
}

func TestPushAndPublish(t *testing.T) {
	t.Parallel()
	isolateGit(t)
	s, r, root, remote := upstreamFixture(t)
	localCommit(t, root, "local one")
	tip := localCommit(t, root, "local two")

	preview := previewPush(t, s, r, "main", "", "")
	if preview.Commits.Total != 2 {
		t.Fatalf("commits %+v", preview.Commits)
	}
	result := pushTest(t, s, r, preview)
	main := result.Repository.Worktrees[0]
	if result.Status != remoteOK || result.Message != "" || main.Ahead == nil || *main.Ahead != 0 || gitTest(t, remote, "rev-parse", "main") != tip {
		t.Fatalf("push result %+v, main %+v", result, main)
	}

	gitTest(t, root, "branch", "feat/c")
	result = pushTest(t, s, r, previewPush(t, s, r, "feat/c", "", ""))
	if result.Status != remoteOK || gitTest(t, remote, "rev-parse", "feat/c") != tip ||
		gitConfig(t, root, "branch.feat/c.remote") != "origin" || gitConfig(t, root, "branch.feat/c.merge") != "refs/heads/feat/c" {
		t.Fatalf("publish %+v", result)
	}
	if synced := previewPush(t, s, r, "feat/c", "", ""); synced.Publish || !slices.Contains(issueCodes(synced.Blockers), issueSame) {
		t.Fatalf("published branch %+v", synced)
	}

	gitTest(t, root, "branch", "feat/d")
	result = pushTest(t, s, r, previewPush(t, s, r, "feat/d", "", "pub/d"))
	if result.Status != remoteOK || gitTest(t, remote, "rev-parse", "pub/d") != tip || gitConfig(t, root, "branch.feat/d.merge") != "refs/heads/pub/d" {
		t.Fatalf("publish under another name %+v", result)
	}
}

func TestPushRechecksAndReportsRejections(t *testing.T) {
	t.Parallel()
	isolateGit(t)
	s, r, root, remote := upstreamFixture(t)
	pushed := gitTest(t, remote, "rev-parse", "main")

	// The branch moved on after the preview: nothing is pushed.
	localCommit(t, root, "local one")
	preview := previewPush(t, s, r, "main", "", "")
	localCommit(t, root, "local two")
	if _, err := s.Push(context.Background(), r.ID, PushRequest{ConfirmationID: preview.ConfirmationID}); errorKind(err) != ErrConflict || !strings.Contains(err.Error(), "请重新预览") {
		t.Fatalf("moved branch: %v", err)
	}
	if gitTest(t, remote, "rev-parse", "main") != pushed {
		t.Fatal("the remote received commits")
	}
	if again := previewPush(t, s, r, "main", "", ""); again.Commits.Total != 2 {
		t.Fatalf("fresh preview %+v", again.Commits)
	}

	// The remote has commits the local view lacks.
	advanceUpstream(t, root, remote, "upstream one")
	local := gitTest(t, root, "rev-parse", "main")
	result := pushTest(t, s, r, previewPush(t, s, r, "main", "", ""))
	if result.Status != remoteRejected || !strings.Contains(result.Message, "先获取远程再快进") || gitTest(t, root, "rev-parse", "main") != local {
		t.Fatalf("non-fast-forward %+v", result)
	}
	gitTest(t, root, "branch", "feat/x")
	result = pushTest(t, s, r, previewPush(t, s, r, "feat/x", "", "main"))
	if result.Status != remoteRejected || !strings.Contains(result.Message, "先获取远程再快进") || gitConfig(t, root, "branch.feat/x.remote") != "" {
		t.Fatalf("rejected publish %+v", result)
	}

	// pre-push hooks run and their refusal is named, from .git/hooks or
	// core.hooksPath.
	gitTest(t, root, "branch", "feat/h")
	installGitHook(t, filepath.Join(root, ".git", "hooks"), "pre-push", "echo 'lint failed: https://user:secret@example.invalid/x' >&2\nexit 1")
	result = pushTest(t, s, r, previewPush(t, s, r, "feat/h", "", ""))
	if result.Status != remoteRejected || !strings.Contains(result.Message, "被 pre-push 钩子拒绝") || !strings.Contains(result.Message, "lint failed") ||
		strings.Contains(result.Message, "secret") {
		t.Fatalf("pre-push refusal %+v", result)
	}
	if err := os.Remove(filepath.Join(root, ".git", "hooks", "pre-push")); err != nil {
		t.Fatal(err)
	}
	hooks := filepath.Join(t.TempDir(), "hooks")
	installGitHook(t, hooks, "pre-push", "echo 'tests failed' >&2\nexit 1")
	gitTest(t, root, "config", "core.hooksPath", hooks)
	result = pushTest(t, s, r, previewPush(t, s, r, "feat/h", "", ""))
	if result.Status != remoteRejected || !strings.Contains(result.Message, "被 pre-push 钩子拒绝") || !strings.Contains(result.Message, "tests failed") {
		t.Fatalf("core.hooksPath refusal %+v", result)
	}
	if out, err := s.git(context.Background(), remote, "rev-parse", "--verify", "--quiet", "refs/heads/feat/h"); err == nil {
		t.Fatalf("hook refusal still pushed: %s", out)
	}
	gitTest(t, root, "config", "--unset", "core.hooksPath")

	// A missing remote is a failure, not an error.
	gitTest(t, root, "remote", "set-url", "origin", filepath.Join(t.TempDir(), "missing.git"))
	if result = pushTest(t, s, r, previewPush(t, s, r, "feat/h", "", "")); result.Status == remoteOK || result.Message == "" {
		t.Fatalf("missing remote %+v", result)
	}
}

func TestPushHoldsNoGateAndKeepsToSafeArguments(t *testing.T) {
	t.Parallel()
	isolateGit(t)
	s, r, root, remote := upstreamFixture(t)
	tip := localCommit(t, root, "local one")
	dir := t.TempDir()
	started, proceed, argv := filepath.Join(dir, "started"), filepath.Join(dir, "proceed"), filepath.Join(dir, "argv")
	wrapper := filepath.Join(dir, "git")
	putFile(t, wrapper, fmt.Sprintf("#!/bin/sh\nif [ \"$3\" = push ]; then\n(shift 2; printf '%%s\\n' \"$@\") >> %q\n: > %q\nwhile [ ! -e %q ]; do sleep 0.02; done\nfi\nexec %q \"$@\"\n",
		argv, started, proceed, s.gitPath))
	if err := os.Chmod(wrapper, 0o700); err != nil {
		t.Fatal(err)
	}
	s.gitPath = wrapper
	preview := previewPush(t, s, r, "main", "", "")
	done := make(chan PushResult, 1)
	go func() {
		result, err := s.Push(context.Background(), r.ID, PushRequest{ConfirmationID: preview.ConfirmationID})
		if err != nil {
			t.Error(err)
		}
		done <- result
	}()
	deadline := time.Now().Add(10 * time.Second)
	for {
		if _, err := os.Stat(started); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("push did not start")
		}
		time.Sleep(10 * time.Millisecond)
	}
	// Snapshots and worktree creation go on while the push runs.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := s.Refresh(ctx, r.ID); err != nil {
		t.Fatalf("snapshot during push: %v", err)
	}
	if _, err := s.CreateWorktree(ctx, r.ID, CreateRequest{Path: filepath.Join(t.TempDir(), "during"), Mode: "new", Branch: "during", Base: "HEAD"}); err != nil {
		t.Fatalf("worktree creation during push: %v", err)
	}
	putFile(t, proceed, "")
	if result := <-done; result.Status != remoteOK || gitTest(t, remote, "rev-parse", "main") != tip {
		t.Fatalf("push %+v", result)
	}

	data, err := os.ReadFile(argv)
	if err != nil {
		t.Fatal(err)
	}
	args := strings.Fields(string(data))
	forbidden := []string{"--force", "--force-with-lease", "--force-if-includes", "--delete", "--mirror", "--all", "--tags", "--follow-tags", "--no-verify", "-f", "-d"}
	for _, arg := range args {
		if slices.Contains(forbidden, arg) || strings.HasPrefix(arg, "--force") || strings.HasPrefix(arg, "+") {
			t.Fatalf("push argv %q contains %q", args, arg)
		}
	}
	if want := []string{"push", "--porcelain", "--end-of-options", "origin", tip + ":refs/heads/main"}; !slices.Equal(args, want) {
		t.Fatalf("push argv %q, want %q", args, want)
	}
}
