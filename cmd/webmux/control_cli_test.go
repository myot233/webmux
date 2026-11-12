//go:build linux || darwin

package main

import (
	"bufio"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/rainchan/webmux/internal/contract"
)

// detachedID starts a detached session and returns its ID.
func detachedID(t *testing.T, f *fixture, argv ...string) string {
	t.Helper()
	stdout, stderr, code := f.cli(t, append([]string{"run", "--detach", "--"}, argv...)...)
	if code != 0 {
		t.Fatalf("run %v: %d %q", argv, code, stderr)
	}
	return strings.TrimSpace(stdout)
}

func eventuallyCLI(t *testing.T, what string, check func() bool) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for !check() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func TestSessionControlCommands(t *testing.T) {
	t.Parallel()
	f := newFixture(t, "")
	f.start(t, nil, true, "--config", f.config)

	// api: raw requests, refused WebSocket paths, failures.
	stdout, _, code := f.cli(t, "api", "/api/sessions")
	if code != 0 || !json.Valid([]byte(stdout)) {
		t.Fatalf("api: %d %q", code, stdout)
	}
	if _, stderr, code := f.cli(t, "api", "/ws/sessions"); code != contract.ExitUsage || !strings.Contains(stderr, "webmux events") {
		t.Fatalf("api /ws: %d %q", code, stderr)
	}
	if _, _, code := f.cli(t, "api", "-d", "{not json", "/api/sessions"); code != contract.ExitUsage {
		t.Fatalf("api invalid JSON: %d", code)
	}
	if stdout, stderr, code := f.cli(t, "api", "-X", "DELETE", "/api/sessions/aaaaaaaa"); code != contract.ExitError || !strings.Contains(stderr, "HTTP 404") || stdout == "" {
		t.Fatalf("api 404: %d %q %q", code, stdout, stderr)
	}

	// send writes confirmed input; logs reads it back.
	echo := detachedID(t, f, "sh", "-c", `read line; echo "got:$line"; sleep 30`)
	if _, stderr, code := f.cli(t, "send", echo, "hello world", "--enter"); code != 0 {
		t.Fatalf("send: %d %q", code, stderr)
	}
	eventuallyCLI(t, "the echoed line", func() bool {
		out, _, _ := f.cli(t, "logs", echo, "--strip")
		return strings.Contains(out, "got:hello world")
	})
	if out, _, _ := f.cli(t, "logs", echo, "--strip", "--tail", "1"); strings.TrimSpace(out) != "got:hello world" {
		t.Fatalf("logs --tail 1: %q", out)
	}
	// A named key is an acknowledged submission too: text first, then Enter.
	keyed := detachedID(t, f, "sh", "-c", `read line; echo "key:$line"; sleep 30`)
	if _, stderr, code := f.cli(t, "send", keyed, "typed"); code != 0 {
		t.Fatalf("send text: %d %q", code, stderr)
	}
	if _, stderr, code := f.cli(t, "send", keyed, "--key", "enter"); code != 0 {
		t.Fatalf("send --key enter: %d %q", code, stderr)
	}
	eventuallyCLI(t, "the line ended by --key enter", func() bool {
		out, _, _ := f.cli(t, "logs", keyed, "--strip")
		return strings.Contains(out, "key:typed")
	})
	if _, _, code := f.cli(t, "send", echo, "--key", "nope"); code != contract.ExitUsage {
		t.Fatalf("unknown key: %d", code)
	}
	if _, _, code := f.cli(t, "send", echo); code != contract.ExitUsage {
		t.Fatalf("nothing to send: %d", code)
	}
	// Chat view options on the terminal view are refused before writing.
	if _, stderr, code := f.cli(t, "send", echo, "x", "--mode", "steer"); code != contract.ExitError || !strings.Contains(stderr, "wrong_view") {
		t.Fatalf("--mode on the terminal view: %d %q", code, stderr)
	}
	if _, _, code := f.cli(t, "send", echo, "x", "--reply", "--wait", "idle"); code != contract.ExitUsage {
		t.Fatalf("--reply with --wait: %d", code)
	}
	// send --json reports the view the input went to.
	if out, stderr, code := f.cli(t, "send", keyed, "more", "--json"); code != 0 || !strings.Contains(out, `"view": "terminal"`) {
		t.Fatalf("send --json: %d %q %q", code, out, stderr)
	}

	// notify raises attention with a message; send and --clear clear it.
	if _, _, code := f.cli(t, "notify", "--kind", "done", "built"); code != contract.ExitUsage {
		t.Fatalf("notify outside a session without --session: %d", code)
	}
	if _, _, code := f.cli(t, "notify", "--session", echo, strings.Repeat("x", 201)); code != contract.ExitUsage {
		t.Fatalf("notify with a long message: %d", code)
	}
	if _, _, code := f.cli(t, "notify", "--session", echo, "--clear", "x"); code != contract.ExitUsage {
		t.Fatalf("notify --clear with a message: %d", code)
	}
	stdout, stderr, code := f.cli(t, "notify", "--session", echo, "--kind", "done", "--json", "release", "built")
	var raised contract.SessionInfo
	if err := json.Unmarshal([]byte(stdout), &raised); err != nil || code != 0 || raised.Attention == nil ||
		raised.Attention.Source != contract.AttentionSourceCLI || raised.Attention.Message != "release built" {
		t.Fatalf("notify: %d %q %q", code, stdout, stderr)
	}
	if out, _, code := f.cli(t, "wait", echo, "--until", "attention", "--timeout", "5s"); code != 0 {
		t.Fatalf("wait --until attention: %d %q", code, out)
	}
	if _, _, code := f.cli(t, "send", echo, "--key", "right"); code != 0 {
		t.Fatalf("send: %d", code)
	}
	if out, _, _ := f.cli(t, "get", echo, "--json"); strings.Contains(out, `"source": "cli"`) {
		t.Fatalf("send left the attention latched: %q", out)
	}
	if _, _, code := f.cli(t, "notify", "--session", echo, "--kind", "question"); code != 0 {
		t.Fatalf("notify question: %d", code)
	}
	if _, _, code := f.cli(t, "notify", "--session", echo, "--clear"); code != 0 {
		t.Fatalf("notify --clear: %d", code)
	}
	if out, _, _ := f.cli(t, "get", echo, "--json"); strings.Contains(out, `"source": "cli"`) {
		t.Fatalf("--clear left the attention latched: %q", out)
	}

	// send --wait sees the change its own input causes.
	exiting := detachedID(t, f, "sh", "-c", "read x; exit 4")
	stdout, stderr, code = f.cli(t, "send", exiting, "go", "--enter", "--wait", "exited", "--timeout", "10s", "--json")
	var after contract.SessionInfo
	if err := json.Unmarshal([]byte(stdout), &after); err != nil || code != 0 || after.ExitCode == nil || *after.ExitCode != 4 {
		t.Fatalf("send --wait: %d %q %q", code, stdout, stderr)
	}

	// wait: the current state counts; a timeout is 124; exited without
	// waiting for it is 1.
	if _, _, code := f.cli(t, "wait", exiting, "--until", "exited"); code != 0 {
		t.Fatalf("wait exited: %d", code)
	}
	if _, _, code := f.cli(t, "wait", echo, "--until", "exited", "--timeout", "300ms"); code != contract.ExitTimeout {
		t.Fatalf("wait timeout: %d", code)
	}
	if _, _, code := f.cli(t, "wait", exiting, "--until", "running,exited", "--changed", "--timeout", "300ms"); code != contract.ExitTimeout {
		t.Fatalf("wait --changed on a finished session: %d", code)
	}

	// screen renders what a full-screen program shows.
	painter := detachedID(t, f, "sh", "-c", `printf '\033[2J\033[3;5Hhello screen\033[1;1Htop'; sleep 30`)
	eventuallyCLI(t, "the painted screen", func() bool {
		out, _, _ := f.cli(t, "screen", painter)
		return strings.Contains(out, "    hello screen")
	})
	stdout, _, _ = f.cli(t, "screen", painter, "--json")
	var view struct {
		Cols  int      `json:"cols"`
		Rows  int      `json:"rows"`
		Lines []string `json:"lines"`
	}
	if err := json.Unmarshal([]byte(stdout), &view); err != nil || view.Cols != 80 || len(view.Lines) != 3 || view.Lines[0] != "top" {
		t.Fatalf("screen --json %q: %v", stdout, err)
	}

	// events: the first line is the snapshot.
	cmd := f.command(nil, "events", "--session", painter, "--config", f.config)
	pipe, _ := cmd.StdoutPipe()
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	line, err := bufio.NewReader(pipe).ReadString('\n')
	_ = cmd.Process.Kill()
	_ = cmd.Wait()
	var event contract.SessionEvent
	if err != nil || json.Unmarshal([]byte(line), &event) != nil || event.Type != "snapshot" || len(event.Sessions) != 1 || event.Sessions[0].ID != painter {
		t.Fatalf("events: %q %v", line, err)
	}

	// rename, kill and rm.
	if _, _, code := f.cli(t, "rename", painter, "painter"); code != 0 {
		t.Fatalf("rename: %d", code)
	}
	if out, _, _ := f.cli(t, "get", painter, "--field", "session.title"); strings.TrimSpace(out) != "painter" {
		t.Fatalf("title after rename: %q", out)
	}
	if _, _, code := f.cli(t, "kill", painter); code != 0 {
		t.Fatalf("kill: %d", code)
	}
	if _, _, code := f.cli(t, "wait", painter, "--until", "exited", "--timeout", "10s"); code != 0 {
		t.Fatalf("killed session did not exit: %d", code)
	}
	if _, _, code := f.cli(t, "rm", painter); code != 0 {
		t.Fatalf("rm: %d", code)
	}
	if _, _, code := f.cli(t, "get", painter); code != contract.ExitError {
		t.Fatalf("removed session still found: %d", code)
	}

	// rm ends a running session first.
	running := detachedID(t, f, "sleep", "30")
	if _, stderr, code := f.cli(t, "rm", running); code != 0 {
		t.Fatalf("rm a running session: %d %q", code, stderr)
	}
	if _, _, code := f.cli(t, "get", running); code != contract.ExitError {
		t.Fatalf("removed running session still found: %d", code)
	}
	// rm --forget needs --yes without a terminal; it ends a running
	// session first, like rm.
	forgotten := detachedID(t, f, "sleep", "30")
	if _, stderr, code := f.cli(t, "rm", forgotten, "--forget"); code != contract.ExitUsage || !strings.Contains(stderr, "--yes") {
		t.Fatalf("rm --forget without --yes: %d %q", code, stderr)
	}
	if _, stderr, code := f.cli(t, "rm", forgotten, "--forget", "--yes"); code != 0 {
		t.Fatalf("rm --forget --yes: %d %q", code, stderr)
	}
	if _, _, code := f.cli(t, "get", forgotten); code != contract.ExitError {
		t.Fatalf("forgotten session still found: %d", code)
	}

	// interrupt presses Ctrl-C in a terminal session; view needs a chat
	// view; resume needs an archived session.
	sleeper := detachedID(t, f, "sh", "-c", "sleep 100; echo after")
	if _, stderr, code := f.cli(t, "interrupt", sleeper); code != 0 {
		t.Fatalf("interrupt: %d %q", code, stderr)
	}
	if _, _, code := f.cli(t, "wait", sleeper, "--until", "exited", "--timeout", "10s"); code != 0 {
		t.Fatalf("interrupted sleep did not end: %d", code)
	}
	if _, stderr, code := f.cli(t, "view", echo, "chat"); code != contract.ExitError || !strings.Contains(stderr, "no chat view") {
		t.Fatalf("view on a terminal session: %d %q", code, stderr)
	}
	if _, _, code := f.cli(t, "view", echo, "sideways"); code != contract.ExitUsage {
		t.Fatalf("unknown view: %d", code)
	}
	if _, stderr, code := f.cli(t, "resume", echo, "--detach"); code != contract.ExitError || !strings.Contains(stderr, "not_archived") {
		t.Fatalf("resume an active session: %d %q", code, stderr)
	}

	// Completion offers commands, options, values and live session IDs.
	if out, err := f.command(nil, "completion", "bash").Output(); err != nil || !strings.Contains(string(out), "complete -o default -F _webmux webmux") {
		t.Fatalf("completion bash: %v %q", err, out)
	}
	for _, tc := range []struct {
		words []string
		want  string
	}{
		{[]string{"se"}, "send\t"},
		{[]string{"chat", ""}, "answer\t"},
		{[]string{"in"}, "interrupt\t"},
		{[]string{"run", "--vi"}, "--view\t"},
		{[]string{"run", "--view", ""}, "chat\t"},
		{[]string{"view", echo, ""}, "terminal\t"},
		{[]string{"guide", "art"}, "artifacts\t"},
		{[]string{"send", ""}, echo + "\t"},
		{[]string{"wait", echo, "--until", "id"}, "idle\t"},
		{[]string{"resume", "--view", ""}, "terminal\t"},
		{[]string{"chat", "queue", ""}, "send-now\t"},
		{[]string{"worktree", ""}, "pin\t"},
		{[]string{"repo", "s"}, "settings\t"},
		{[]string{"ls", "--state", "ar"}, "archived\t"},
	} {
		cmd := f.command([]string{"WEBMUX_SOCKET=" + f.record(t).Socket}, append([]string{"__complete"}, tc.words...)...)
		out, _ := cmd.Output()
		if !strings.Contains(string(out), tc.want) {
			t.Errorf("__complete %q: %q lacks %q", tc.words, out, tc.want)
		}
	}
}

// TestCompletionScriptsRun sources the generated scripts in real shells
// against a stand-in webmux on PATH.
func TestCompletionScriptsRun(t *testing.T) {
	bin := t.TempDir()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	wrapper := "#!/bin/sh\nWEBMUX_TEST_EXEC=argv exec " + exe + " \"$@\"\n"
	if err := os.WriteFile(filepath.Join(bin, "webmux"), []byte(wrapper), 0o755); err != nil {
		t.Fatal(err)
	}
	path := bin + string(os.PathListSeparator) + os.Getenv("PATH")
	for _, tc := range []struct{ shell, script string }{
		{"bash", `source <(webmux completion bash); COMP_WORDS=(webmux chat a); COMP_CWORD=2; _webmux; printf '%s\n' "${COMPREPLY[@]}"`},
		{"fish", `webmux completion fish | source; complete -C 'webmux chat a'`},
	} {
		if _, err := exec.LookPath(tc.shell); err != nil {
			t.Logf("%s is not installed; skipped", tc.shell)
			continue
		}
		cmd := exec.Command(tc.shell, "-c", tc.script)
		cmd.Env = append(os.Environ(), "PATH="+path, "WEBMUX_SOCKET=/nonexistent/webmux.sock")
		out, err := cmd.CombinedOutput()
		if err != nil || !strings.Contains(string(out), "answer") {
			t.Errorf("%s completion: %v %q", tc.shell, err, out)
		}
	}
}
