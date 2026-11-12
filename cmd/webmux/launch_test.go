//go:build linux || darwin

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/rainchan/webmux/internal/contract"
)

var cliFakeAgent struct {
	once sync.Once
	dir  string
	err  error
}

// fakeAgent builds the fake ACP agent as an omp executable once.
func fakeAgent(t *testing.T) string {
	t.Helper()
	if testing.Short() {
		t.Skip("builds and runs the fake agent")
	}
	cliFakeAgent.once.Do(func() {
		cliFakeAgent.dir, cliFakeAgent.err = os.MkdirTemp("/tmp", "wmx-agent-")
		if cliFakeAgent.err != nil {
			return
		}
		build := exec.Command("go", "build", "-o", filepath.Join(cliFakeAgent.dir, "omp"), "../../internal/acp/testdata/fakeagent")
		if out, err := build.CombinedOutput(); err != nil {
			cliFakeAgent.err = fmt.Errorf("go build: %w\n%s", err, out)
		}
	})
	if cliFakeAgent.err != nil {
		t.Fatal(cliFakeAgent.err)
	}
	return filepath.Join(cliFakeAgent.dir, "omp")
}

// gitRepo creates a repository with one commit under dir.
func gitRepo(t *testing.T, dir string) string {
	t.Helper()
	repo := filepath.Join(dir, "app")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"init", "-q", "-b", "main"}, {"commit", "-q", "--allow-empty", "-m", "initial"}} {
		cmd := exec.Command("git", append([]string{"-C", repo}, args...)...)
		cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.invalid", "GIT_COMMITTER_NAME=t",
			"GIT_COMMITTER_EMAIL=t@example.invalid", "GIT_CONFIG_GLOBAL="+os.DevNull, "GIT_CONFIG_NOSYSTEM=1")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v %s", args, err, out)
		}
	}
	return repo
}

func TestRunLaunchesFollowsAndWaits(t *testing.T) {
	t.Parallel()
	f := newFixture(t, "")
	f.start(t, nil, true, "--config", f.config)
	record := f.record(t)

	// --follow streams plain text and returns the program's status.
	stdout, stderr, code := f.cli(t, "run", "--follow", "--strip", "--", "sh", "-c", `printf 'one\r\ntwo\033[31m!\033[0m\n'; echo "$WEBMUX_SOCKET|$WEBMUX_BIN"; exit 7`)
	if code != 7 || !strings.Contains(stdout, "one\ntwo!\n") {
		t.Fatalf("--follow: %d %q %q", code, stdout, stderr)
	}
	exe, _ := os.Executable()
	if !strings.Contains(stdout, record.Socket+"|"+exe) {
		t.Fatalf("session environment lacks WEBMUX_SOCKET/WEBMUX_BIN: %q", stdout)
	}

	// --detach --wait exited --json reports the final state.
	stdout, stderr, code = f.cli(t, "run", "--detach", "--json", "--wait", "exited", "--timeout", "20s", "--label", "run=1", "--", "sh", "-c", "exit 3")
	var detached struct {
		Session contract.SessionInfo `json:"session"`
	}
	if err := json.Unmarshal([]byte(stdout), &detached); err != nil || code != 0 {
		t.Fatalf("--detach --wait: %d %q %q", code, stdout, stderr)
	}
	if detached.Session.Status != "exited" || detached.Session.ExitCode == nil || *detached.Session.ExitCode != 3 ||
		detached.Session.Labels["run"] != "1" || strings.Contains(stdout, `"link"`) {
		t.Fatalf("detached %+v (%s)", detached, stdout)
	}

	// --wait alone detaches too.
	stdout, stderr, code = f.cli(t, "run", "--wait", "exited", "--timeout", "20s", "--", "sh", "-c", "exit 0")
	if code != 0 || strings.TrimSpace(stdout) == "" || strings.Contains(stdout, "\t") {
		t.Fatalf("--wait without --detach: %d %q %q", code, stdout, stderr)
	}
	if _, _, code = f.cli(t, "run", "--wait", "exited", "--follow", "--", "true"); code != contract.ExitUsage {
		t.Fatalf("--wait with --follow: %d", code)
	}

	// --wait times out with 124.
	_, _, code = f.cli(t, "run", "--detach", "--wait", "exited", "--timeout", "300ms", "--", "sleep", "30")
	if code != contract.ExitTimeout {
		t.Fatalf("--wait timeout: %d", code)
	}

	// No terminal: run detaches instead of failing.
	stdout, stderr, code = f.cli(t, "run", "--", "sleep", "30")
	if code != 0 || strings.TrimSpace(stdout) == "" || strings.Contains(stdout, "\t") || !strings.Contains(stderr, "detaching") {
		t.Fatalf("auto-detach: %d %q %q", code, stdout, stderr)
	}

	// --dry-run creates nothing and hides environment values.
	before := len(sessions(t, f))
	stdout, _, code = f.cli(t, "run", "--dry-run", "--env", "SECRET=hunter2", "--", "true")
	if code != 0 || strings.Contains(stdout, "hunter2") || !strings.Contains(stdout, `"SECRET": "<explicit>"`) || len(sessions(t, f)) != before {
		t.Fatalf("--dry-run: %d %q", code, stdout)
	}

	// A session created from inside a session records its parent.
	inner, _ := json.Marshal([]string{"run", "--detach", "--label", "child=1", "--", "sleep", "30"})
	stdout, stderr, code = f.cli(t, "run", "--follow", "--", "sh", "-c", `WEBMUX_TEST_EXEC='`+string(inner)+`' "$WEBMUX_BIN"`)
	if code != 0 {
		t.Fatalf("nested run: %d %q %q", code, stdout, stderr)
	}
	stdout, _, _ = f.cli(t, "ls", "--label", "child=1", "--json")
	var children []contract.SessionInfo
	if err := json.Unmarshal([]byte(stdout), &children); err != nil || len(children) != 1 || children[0].ParentID == "" {
		t.Fatalf("child %q: %v", stdout, err)
	}
	stdout, _, _ = f.cli(t, "ls", "--parent", children[0].ParentID, "--json")
	if !strings.Contains(stdout, children[0].ID) {
		t.Fatalf("ls --parent %q", stdout)
	}

	// The guide reads the running server.
	stdout, _, code = f.cli(t, "guide")
	if code != 0 || !strings.Contains(stdout, "# WebMux") || !strings.Contains(stdout, record.Socket) {
		t.Fatalf("guide: %d %q", code, stdout)
	}
}

// cli runs a client command with --config placed before its first option
// or "--", so it never ends up in a session's argv.
func (f *fixture) cli(t *testing.T, args ...string) (string, string, int) {
	t.Helper()
	at := len(args)
	for i, arg := range args {
		if strings.HasPrefix(arg, "-") {
			at = i
			break
		}
	}
	full := append(append(append([]string{}, args[:at]...), "--config", f.config), args[at:]...)
	cmd := f.command(nil, full...)
	var stdout, stderr strings.Builder
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	_ = cmd.Run()
	return stdout.String(), stderr.String(), cmd.ProcessState.ExitCode()
}

func sessions(t *testing.T, f *fixture) []contract.SessionInfo {
	t.Helper()
	var list []contract.SessionInfo
	if err := f.local(t).Do(context.Background(), http.MethodGet, "/api/sessions", nil, &list); err != nil {
		t.Fatal(err)
	}
	return list
}

func TestRunInWorktrees(t *testing.T) {
	t.Parallel()
	f := newFixture(t, "")
	f.start(t, nil, true, "--config", f.config)
	repo := gitRepo(t, f.dir)
	client := f.local(t)
	ctx := context.Background()
	if err := client.Do(ctx, http.MethodPost, "/api/repositories", contract.RegisterRepositoryRequest{Path: repo}, nil); err != nil {
		t.Fatal(err)
	}
	// The current directory associates the session with its worktree.
	stdout, stderr, code := f.cli(t, "run", "--detach", "--json", "--cwd", repo, "--", "sleep", "30")
	if code != 0 || !strings.Contains(stdout, `"repositoryId"`) {
		t.Fatalf("auto association: %d %q %q", code, stdout, stderr)
	}
	// A new worktree is created, used and pinned to the home page.
	stdout, stderr, code = f.cli(t, "run", "--detach", "--json", "--new-worktree", "--branch", "feat/cli", "--repo", repo, "--", "sleep", "30")
	if code != 0 {
		t.Fatalf("--new-worktree: %d %q %q", code, stdout, stderr)
	}
	var created struct {
		Session  contract.SessionInfo `json:"session"`
		Worktree *contract.Worktree   `json:"worktree"`
	}
	_ = json.Unmarshal([]byte(stdout), &created)
	want := filepath.Join(filepath.Dir(repo), "app.worktrees", "cli")
	if created.Session.Cwd != want || created.Session.Workspace == nil || created.Worktree == nil || created.Worktree.Path != want {
		t.Fatalf("session %+v worktree %+v, want cwd %s", created.Session, created.Worktree, want)
	}
	var settings contract.SettingsSnapshot
	if err := client.Do(ctx, http.MethodGet, "/api/settings", nil, &settings); err != nil {
		t.Fatal(err)
	}
	pinned := settings.Values.HomeWorktrees[created.Session.Workspace.RepositoryID]
	if len(pinned) != 2 {
		t.Fatalf("home worktrees %v: want the main worktree and the new one (stderr %q)", pinned, stderr)
	}
	// --worktree selects it by path; --cwd must stay inside.
	stdout, _, code = f.cli(t, "run", "--detach", "--json", "--worktree", want, "--", "true")
	if code != 0 || !strings.Contains(stdout, created.Session.Workspace.WorktreeID) {
		t.Fatalf("--worktree: %d %q", code, stdout)
	}
	if _, _, code = f.cli(t, "run", "--detach", "--worktree", want, "--cwd", f.dir, "--", "true"); code != contract.ExitUsage {
		t.Fatalf("--cwd outside the worktree: %d", code)
	}
	if _, _, code = f.cli(t, "run", "--detach", "--worktree", want, "--new-worktree", "--", "true"); code != contract.ExitUsage {
		t.Fatalf("--worktree with --new-worktree: %d", code)
	}
	if _, _, code = f.cli(t, "run", "--detach", "--new-worktree", "--repo", repo, "--cwd", repo, "--", "true"); code != contract.ExitUsage {
		t.Fatalf("--cwd with --new-worktree: %d", code)
	}
	// The old --new-worktree BRANCH is refused, not run as a command.
	if _, stderr, code = f.cli(t, "run", "--detach", "--new-worktree", "fix/old", "--repo", repo, "--", "true"); code != contract.ExitUsage || !strings.Contains(stderr, "--branch") {
		t.Fatalf("--new-worktree BRANCH: %d %q", code, stderr)
	}
	// An existing branch is a worktree-step conflict; nothing else is made.
	before := len(sessions(t, f))
	if _, stderr, code = f.cli(t, "run", "--detach", "--new-worktree", "--branch", "feat/cli", "--repo", repo, "--", "true"); code != contract.ExitError || !strings.Contains(stderr, "worktree step") {
		t.Fatalf("existing branch: %d %q", code, stderr)
	}
	if after := len(sessions(t, f)); after != before {
		t.Fatalf("a failed worktree step started a session: %d -> %d", before, after)
	}
	// Without --branch the server names it wt/<date>-<time>.
	stdout, stderr, code = f.cli(t, "run", "--detach", "--json", "--new-worktree", "--no-pin", "--repo", repo, "--", "sleep", "30")
	created.Worktree = nil
	if err := json.Unmarshal([]byte(stdout), &created); code != 0 || err != nil || created.Worktree == nil ||
		created.Worktree.Branch == nil || !strings.HasPrefix(*created.Worktree.Branch, "wt/") {
		t.Fatalf("generated branch: %d %q %q", code, stdout, stderr)
	}
	if err := client.Do(ctx, http.MethodGet, "/api/settings", nil, &settings); err != nil {
		t.Fatal(err)
	}
	if got := settings.Values.HomeWorktrees[created.Session.Workspace.RepositoryID]; len(got) != 2 {
		t.Fatalf("--no-pin pinned the worktree: %v", got)
	}
	// The dry run shows the request without creating anything.
	stdout, _, code = f.cli(t, "run", "--dry-run", "--new-worktree", "--branch", "feat/dry", "--repo", repo, "--", "true")
	if code != 0 || !strings.Contains(stdout, `"branch": "feat/dry"`) || !strings.Contains(stdout, `"mode": "new"`) {
		t.Fatalf("--dry-run --new-worktree: %d %q", code, stdout)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(repo), "app.worktrees", "dry")); !os.IsNotExist(err) {
		t.Fatalf("--dry-run created the worktree: %v", err)
	}
	// A given --worktree-path is resolved as the real creation resolves it.
	messy := filepath.Dir(repo) + "/x/../dry2/"
	stdout, _, code = f.cli(t, "run", "--dry-run", "--new-worktree", "--branch", "feat/dry2", "--worktree-path", messy, "--repo", repo, "--", "true")
	if code != 0 || !strings.Contains(stdout, `"path": "`+filepath.Join(filepath.Dir(repo), "dry2")+`"`) {
		t.Fatalf("--dry-run --worktree-path: %d %q", code, stdout)
	}
}

func TestRunStartsAnAgentOnItsChatViewAndReplies(t *testing.T) {
	t.Parallel()
	agent := fakeAgent(t)
	f := newFixture(t, "")
	f.start(t, nil, true, "--config", f.config)
	repo := gitRepo(t, f.dir)
	client := f.local(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	if err := client.Do(ctx, http.MethodPost, "/api/repositories", contract.RegisterRepositoryRequest{Path: repo}, nil); err != nil {
		t.Fatal(err)
	}
	config := contract.LaunchConfigInput{Name: "Fake", Cmd: []string{agent}, Env: map[string]string{"FAKEAGENT_STORE": t.TempDir(), "FAKEAGENT_TURN_DELAY": "5ms"}, Chat: true}
	if err := client.Do(ctx, http.MethodPost, "/api/launch-configs", config, nil); err != nil {
		t.Fatal(err)
	}
	stdout, stderr, code := f.cli(t, "launch-config", "options", "Fake", "--repo", repo, "--json")
	if code != 0 || !json.Valid([]byte(stdout)) {
		t.Fatalf("options: %d %q %q", code, stdout, stderr)
	}
	attachment := filepath.Join(t.TempDir(), "note.txt")
	if err := os.WriteFile(attachment, []byte("see this"), 0o644); err != nil {
		t.Fatal(err)
	}
	stdout, stderr, code = f.cli(t, "run", "--new-worktree", "--repo", repo, "--launch-config", "Fake", "--view", "chat",
		"--label", "task=t", "--attach-file", attachment, "--prompt", "hello agent", "--reply", "--timeout", "40s")
	if code != 0 || strings.TrimSpace(stdout) == "" {
		t.Fatalf("run --view chat --reply: %d %q %q", code, stdout, stderr)
	}
	stdout, _, _ = f.cli(t, "ls", "--label", "task=t", "--json")
	var list []contract.SessionInfo
	if err := json.Unmarshal([]byte(stdout), &list); err != nil || len(list) != 1 || list[0].View != contract.ViewChat ||
		!slices.Contains(list[0].Views, contract.ViewChat) || list[0].Workspace == nil || strings.Contains(stdout, `"hybrid"`) {
		t.Fatalf("the new session is not on its chat view in a worktree: %q", stdout)
	}
	// --reply needs the chat view and a first prompt; terminal-only
	// configs cannot start on chat.
	if _, _, code := f.cli(t, "run", "--launch-config", "Fake", "--view", "chat", "--reply"); code != contract.ExitUsage {
		t.Fatalf("--reply without --prompt: %d", code)
	}
	if err := client.Do(ctx, http.MethodPost, "/api/launch-configs", contract.LaunchConfigInput{Name: "Plain", Cmd: []string{"sleep", "30"}}, nil); err != nil {
		t.Fatal(err)
	}
	if _, stderr, code := f.cli(t, "run", "--launch-config", "Plain", "--view", "chat"); code != contract.ExitUsage || !strings.Contains(stderr, "chat view") {
		t.Fatalf("--view chat on a terminal config: %d %q", code, stderr)
	}
	if _, stderr, code := f.cli(t, "launch-config", "options", "Plain", "--repo", repo); code != contract.ExitError || !strings.Contains(stderr, "no chat view") {
		t.Fatalf("options of a terminal config: %d %q", code, stderr)
	}
	// The removed quick start command is unknown.
	if _, _, code := f.cli(t, "chat", "new", "--agent", "Fake", "x"); code != contract.ExitUsage {
		t.Fatalf("chat new: %d", code)
	}
}
