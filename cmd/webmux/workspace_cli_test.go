//go:build linux || darwin

package main

import (
	"encoding/json"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rainchan/webmux/internal/contract"
)

type cliWorktree struct {
	contract.Worktree
	Home     string `json:"home"`
	Sessions int    `json:"sessions"`
}

type cliRepository struct {
	contract.Repository
	Worktrees []cliWorktree `json:"worktrees"`
}

func TestWorkspaceCommands(t *testing.T) {
	t.Parallel()
	f := newFixture(t, "")
	f.start(t, nil, true, "--config", f.config)
	repo := gitRepo(t, f.dir)

	// repo add is idempotent.
	stdout, stderr, code := f.cli(t, "repo", "add", repo)
	if code != 0 || !strings.Contains(stdout, "\tapp\t"+repo) {
		t.Fatalf("repo add: %d %q %q", code, stdout, stderr)
	}
	repoID, _, _ := strings.Cut(stdout, "\t")
	if _, stderr, code := f.cli(t, "repo", "add", repo); code != 0 || !strings.Contains(stderr, "already registered") {
		t.Fatalf("repo add again: %d %q", code, stderr)
	}
	if _, _, code := f.cli(t, "repo", "add", f.dir); code != contract.ExitError {
		t.Fatalf("repo add outside git: %d", code)
	}
	if out, _, _ := f.cli(t, "repo", "ls"); !strings.Contains(out, repoID) {
		t.Fatalf("repo ls: %q", out)
	}
	if _, _, code := f.cli(t, "repo", "show", "nope"); code != contract.ExitUsage {
		t.Fatalf("unknown repo: %d", code)
	}

	// Settings: set from stdin, warn about missing quick commands, limits.
	set := f.command(nil, "repo", "settings", "--config", f.config, repo, "--init", "-")
	set.Stdin = strings.NewReader("{{webmux}}\n\n{{lint}}\n\n{{user}}\n")
	out, err := set.CombinedOutput()
	if err != nil || !strings.Contains(string(out), "{{lint}}") {
		t.Fatalf("settings --init -: %v %q", err, out)
	}
	stdout, stderr, _ = f.cli(t, "repo", "settings", repo)
	if !strings.HasPrefix(stdout, "{{webmux}}") || !strings.Contains(stderr, "references: {{webmux}}, {{lint}}, {{user}}") {
		t.Fatalf("settings: %q %q", stdout, stderr)
	}
	if _, _, code := f.cli(t, "repo", "settings", repo, "--init", strings.Repeat("x", 4001)); code != contract.ExitUsage {
		t.Fatalf("too long: %d", code)
	}
	if _, _, code := f.cli(t, "repo", "settings", repo, "--init", "a", "--clear-init"); code != contract.ExitUsage {
		t.Fatalf("--init with --clear-init: %d", code)
	}
	if _, _, code := f.cli(t, "repo", "settings", repo, "--clear-init"); code != 0 {
		t.Fatalf("--clear-init: %d", code)
	}
	if out, _, _ := f.cli(t, "repo", "settings", repo, "--json"); strings.TrimSpace(out) != `{
  "initInstruction": ""
}` {
		t.Fatalf("settings after clearing: %q", out)
	}

	// worktree create pins; unpin hides; home --reset restores the default.
	stdout, stderr, code = f.cli(t, "worktree", "create", repo, "--branch", "exp/a")
	want := filepath.Join(f.dir, "app.worktrees", "a")
	if code != 0 || strings.TrimSpace(stdout) != want {
		t.Fatalf("worktree create: %d %q %q", code, stdout, stderr)
	}
	homeOf := func(branch string) string {
		t.Helper()
		out, _, _ := f.cli(t, "worktree", "ls", repo, "--json")
		var repos []cliRepository
		if err := json.Unmarshal([]byte(out), &repos); err != nil || len(repos) != 1 {
			t.Fatalf("worktree ls --json %q: %v", out, err)
		}
		for _, w := range repos[0].Worktrees {
			if w.Branch != nil && *w.Branch == branch {
				return w.Home
			}
		}
		t.Fatalf("no worktree %s in %q", branch, out)
		return ""
	}
	if got := homeOf("exp/a"); got != "pinned" {
		t.Fatalf("new worktree home %q", got)
	}
	if _, _, code := f.cli(t, "worktree", "unpin", repo, "exp/a"); code != 0 || homeOf("exp/a") != "hidden" {
		t.Fatalf("unpin: %d %q", code, homeOf("exp/a"))
	}
	if out, _, _ := f.cli(t, "worktree", "ls", repo, "--hidden"); !strings.Contains(out, "exp/a") || strings.Contains(out, "main ") {
		t.Fatalf("ls --hidden: %q", out)
	}
	if _, _, code := f.cli(t, "worktree", "home", repo, "--set", "exp/a"); code != 0 || homeOf("exp/a") != "pinned" || homeOf("main") != "hidden" {
		t.Fatalf("home --set: %d", code)
	}
	stdout, _, _ = f.cli(t, "worktree", "home", repo, "--reset", "--json")
	var home struct {
		Default   bool                `json:"default"`
		Worktrees []contract.Worktree `json:"worktrees"`
	}
	if err := json.Unmarshal([]byte(stdout), &home); err != nil || !home.Default || len(home.Worktrees) != 1 || home.Worktrees[0].Kind != "main" {
		t.Fatalf("home --reset: %q %v", stdout, err)
	}
	// A worktree with a session is shown for it; unpin explains that.
	if _, _, code := f.cli(t, "run", "--detach", "--cwd", want, "--", "sleep", "30"); code != 0 {
		t.Fatalf("run in the worktree: %d", code)
	}
	if got := homeOf("exp/a"); got != "sessions" {
		t.Fatalf("worktree with a session: %q", got)
	}
	if _, stderr, _ := f.cli(t, "worktree", "unpin", repo, "exp/a"); !strings.Contains(stderr, "stays on the home page") {
		t.Fatalf("unpin with a session: %q", stderr)
	}
	if out, _, code := f.cli(t, "worktree", "show", repo, want, "--json"); code != 0 || !strings.Contains(out, `"home": "sessions"`) || !strings.Contains(out, `"sleep"`) {
		t.Fatalf("worktree show: %d %q", code, out)
	}

	// Branches and commits.
	if out, stderr, code := f.cli(t, "repo", "branches", repo); code != 0 || !strings.Contains(out, "exp/a") || !strings.Contains(out, want) ||
		!strings.Contains(out, "UPSTREAM") || !strings.Contains(out, "AHEAD/BEHIND") || !strings.Contains(stderr, "default branch main") {
		t.Fatalf("repo branches: %d %q %q", code, out, stderr)
	}
	if out, _, code := f.cli(t, "repo", "commits", repo, "--from", "main"); code != 0 || !strings.Contains(out, "\tinitial") {
		t.Fatalf("repo commits: %d %q", code, out)
	}
	if _, _, code := f.cli(t, "repo", "commits", repo, "--from", "nope"); code != contract.ExitUsage {
		t.Fatalf("commits from an unknown ref: %d", code)
	}
	if out, _, code := f.cli(t, "worktree", "show", repo, "main"); code != 0 || !strings.Contains(out, "upstream") || !strings.Contains(out, "none") ||
		!strings.Contains(out, "initial") || !strings.Contains(out, "changes") {
		t.Fatalf("worktree show details: %d %q", code, out)
	}
	if out, _, code := f.cli(t, "repo", "show", repo); code != 0 || !strings.Contains(out, "last fetch") || !strings.Contains(out, "LAST COMMIT") {
		t.Fatalf("repo show details: %d %q", code, out)
	}

	// Remotes are listed with credentials redacted by the server.
	for _, args := range [][]string{
		{"remote", "add", "origin", "https://user:hunter2@example.invalid/app.git"},
		{"remote", "set-url", "--push", "origin", "https://token123@example.invalid/app.git"},
	} {
		if out, err := exec.Command("git", append([]string{"-C", repo}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v %s", args, err, out)
		}
	}
	listed, _, code := f.cli(t, "repo", "remotes", repo)
	if code != 0 || !strings.Contains(listed, "https://user:***@example.invalid/app.git") || !strings.Contains(listed, "https://***@example.invalid/app.git") ||
		strings.Contains(listed, "hunter2") || strings.Contains(listed, "token123") {
		t.Fatalf("repo remotes: %d %q", code, listed)
	}
	var remotes contract.Remotes
	if out, _, code := f.cli(t, "repo", "remotes", repo, "--json"); code != 0 || json.Unmarshal([]byte(out), &remotes) != nil ||
		len(remotes.Remotes) != 1 || remotes.Remotes[0].PushURL != "https://***@example.invalid/app.git" || strings.Contains(out, "hunter2") {
		t.Fatalf("repo remotes --json: %d %q", code, out)
	}
	if _, _, code := f.cli(t, "repo", "remotes", "nope"); code != contract.ExitUsage {
		t.Fatalf("remotes of an unknown repo: %d", code)
	}

	// worktree switch changes the branch in place; running sessions need --yes.
	if out, stderr, code := f.cli(t, "worktree", "switch", repo, "main", "--create", "feat/x"); code != 0 || out != "feat/x\n" {
		t.Fatalf("switch to a new branch: %d %q %q", code, out, stderr)
	}
	if out, stderr, code := f.cli(t, "worktree", "switch", repo, "feat/x", "main"); code != 0 || out != "main\n" {
		t.Fatalf("switch back: %d %q %q", code, out, stderr)
	}
	if _, stderr, code := f.cli(t, "worktree", "switch", repo, "exp/a", "--create", "exp/c"); code != contract.ExitUsage || !strings.Contains(stderr, "--yes") {
		t.Fatalf("switch with a running session: %d %q", code, stderr)
	}

	// worktree rm previews, refuses blockers and needs --yes without a terminal.
	if _, stderr, code := f.cli(t, "worktree", "rm", repo, "exp/a"); code != contract.ExitError || !strings.Contains(stderr, "session") {
		t.Fatalf("rm with a running session: %d %q", code, stderr)
	}
	if _, stderr, code := f.cli(t, "worktree", "rm", repo, "exp/a", "--force"); code != contract.ExitUsage || !strings.Contains(stderr, "--yes") {
		t.Fatalf("rm without --yes: %d %q", code, stderr)
	}
	if _, stderr, code := f.cli(t, "worktree", "rm", repo, "exp/a", "--force", "--yes"); code != 0 {
		t.Fatalf("rm --force --yes: %d %q", code, stderr)
	}
	if out, _, _ := f.cli(t, "worktree", "ls", repo); strings.Contains(out, "exp/a") {
		t.Fatalf("removed worktree still listed: %q", out)
	}

	// repo rm unregisters and clears its home setting.
	if _, _, code := f.cli(t, "worktree", "pin", repo, "main"); code != 0 {
		t.Fatalf("pin main: %d", code)
	}
	if _, _, code := f.cli(t, "repo", "rm", repo); code != contract.ExitUsage {
		t.Fatalf("repo rm without --yes: %d", code)
	}
	if _, _, code := f.cli(t, "repo", "rm", repo, "--yes"); code != 0 {
		t.Fatalf("repo rm: %d", code)
	}
	var settings contract.SettingsSnapshot
	if out, _, _ := f.cli(t, "api", "/api/settings"); json.Unmarshal([]byte(out), &settings) != nil || len(settings.Values.HomeWorktrees) != 0 {
		t.Fatalf("home setting after repo rm: %q", out)
	}
}

func TestLaunchConfigAndForwardCommands(t *testing.T) {
	t.Parallel()
	f := newFixture(t, "")
	f.start(t, nil, true, "--config", f.config)

	stdout, stderr, code := f.cli(t, "launch-config", "add", "Dev", "--icon", "terminal", "--env", "TOKEN=s3cret", "--env", "MODE=dev", "--", "bash", "-lc", "make dev")
	if code != 0 || strings.TrimSpace(stdout) == "" || strings.Contains(stdout+stderr, "s3cret") {
		t.Fatalf("add: %d %q %q", code, stdout, stderr)
	}
	if _, _, code := f.cli(t, "launch-config", "add", "Dev", "--", "true"); code != contract.ExitError {
		t.Fatalf("duplicate name: %d", code)
	}
	if _, stderr, code := f.cli(t, "launch-config", "add", "Shell", "--chat", "--", "bash"); code != contract.ExitError || !strings.Contains(stderr, "chat view") {
		t.Fatalf("unsupported chat view: %d %q", code, stderr)
	}
	if _, _, code := f.cli(t, "launch-config", "add", "Shell", "--hybrid", "--", "bash"); code != contract.ExitUsage {
		t.Fatalf("--hybrid is an unknown option: %d", code)
	}
	show := func(args ...string) contract.LaunchConfig {
		t.Helper()
		out, stderr, code := f.cli(t, append([]string{"launch-config", "show", "Dev", "--json"}, args...)...)
		var config contract.LaunchConfig
		if err := json.Unmarshal([]byte(out), &config); err != nil || code != 0 {
			t.Fatalf("show: %d %q %q", code, out, stderr)
		}
		return config
	}
	if c := show(); c.Env["TOKEN"] != "<hidden>" || c.Env["MODE"] != "<hidden>" || strings.Join(c.Cmd, " ") != "bash -lc make dev" {
		t.Fatalf("show hides values: %+v", c)
	}
	if out, _, _ := f.cli(t, "launch-config", "show", "Dev"); strings.Contains(out, "s3cret") || !strings.Contains(out, "TOKEN=<hidden>") {
		t.Fatalf("show text: %q", out)
	}
	// edit changes only what it is told.
	if _, _, code := f.cli(t, "launch-config", "edit", "Dev", "--unset-env", "MODE", "--preview-lines", "5"); code != 0 {
		t.Fatalf("edit: %d", code)
	}
	c := show("--reveal-env")
	if c.Env["TOKEN"] != "s3cret" || len(c.Env) != 1 || c.Icon != "terminal" || c.Preview == nil || c.Preview.Lines != 5 || c.Preview.Skip != 1 || len(c.Cmd) != 3 {
		t.Fatalf("after edit: %+v", c)
	}
	if _, _, code := f.cli(t, "launch-config", "edit", "Dev", "--rename", "Dev 2", "--", "make", "serve"); code != 0 {
		t.Fatalf("rename: %d", code)
	}
	if _, stderr, code := f.cli(t, "launch-config", "show", "Dev"); code != contract.ExitUsage || !strings.Contains(stderr, `"Dev 2"`) {
		t.Fatalf("show an old name: %d %q", code, stderr)
	}
	if out, _, _ := f.cli(t, "launch-config", "ls"); !strings.Contains(out, "Dev 2") || !strings.Contains(out, "make serve") || strings.Contains(out, "s3cret") {
		t.Fatalf("ls: %q", out)
	}
	if _, _, code := f.cli(t, "launch-config", "rm", "Dev 2"); code != contract.ExitUsage {
		t.Fatalf("rm without --yes: %d", code)
	}
	if _, _, code := f.cli(t, "launch-config", "rm", "Dev 2", "--yes"); code != 0 {
		t.Fatalf("rm: %d", code)
	}
	stdout, _, code = f.cli(t, "launch-config", "presets", "--json")
	var presets []contract.LaunchPreset
	if err := json.Unmarshal([]byte(stdout), &presets); err != nil || code != 0 {
		t.Fatalf("presets: %d %q", code, stdout)
	}
	if len(presets) > 0 {
		if _, _, code := f.cli(t, "launch-config", "rm", presets[0].Name, "--yes"); code != 0 {
			t.Fatalf("rm a preset: %d", code)
		}
		if _, stderr, code := f.cli(t, "launch-config", "restore", presets[0].Name); code != 0 {
			t.Fatalf("restore: %d %q", code, stderr)
		}
		if out, _, _ := f.cli(t, "launch-config", "ls"); !strings.Contains(out, presets[0].Name) {
			t.Fatalf("restored preset missing: %q", out)
		}
	}

	// Forwards.
	stdout, stderr, code = f.cli(t, "forward", "add", "45973", "--label", "web")
	if code == contract.ExitError && strings.Contains(stderr, "turned off") {
		t.Log("forwarding is off in this configuration; skipped")
		return
	}
	if code != 0 || strings.TrimSpace(stdout) != "45973" {
		t.Fatalf("forward add: %d %q %q", code, stdout, stderr)
	}
	if _, _, code := f.cli(t, "forward", "add", "45973"); code != contract.ExitError {
		t.Fatalf("forward add twice: %d", code)
	}
	if _, _, code := f.cli(t, "forward", "add", "0"); code != contract.ExitUsage {
		t.Fatalf("bad port: %d", code)
	}
	if out, _, _ := f.cli(t, "forward", "ls"); !strings.Contains(out, "45973") || !strings.Contains(out, "web") {
		t.Fatalf("forward ls: %q", out)
	}
	if _, _, code := f.cli(t, "forward", "rm", "45973"); code != 0 {
		t.Fatalf("forward rm: %d", code)
	}
	if _, _, code := f.cli(t, "forward", "rm", "45973"); code != contract.ExitError {
		t.Fatalf("forward rm twice: %d", code)
	}
}
