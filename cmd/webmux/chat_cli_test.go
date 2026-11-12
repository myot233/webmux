//go:build linux || darwin

package main

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/rainchan/webmux/internal/contract"
)

func TestSessionVerbsDriveAChatSession(t *testing.T) {
	t.Parallel()
	agent := fakeAgent(t)
	f := newFixture(t, "")
	f.start(t, nil, true, "--config", f.config)
	repo := gitRepo(t, f.dir)
	client := f.local(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	if err := client.Do(ctx, http.MethodPost, "/api/repositories", contract.RegisterRepositoryRequest{Path: repo}, nil); err != nil {
		t.Fatal(err)
	}
	config := contract.LaunchConfigInput{Name: "Fake", Cmd: []string{agent}, Chat: true, Env: map[string]string{
		"FAKEAGENT_STORE": t.TempDir(), "FAKEAGENT_TURN_DELAY": "5ms", "FAKEAGENT_PERMISSION": "1",
	}}
	if err := client.Do(ctx, http.MethodPost, "/api/launch-configs", config, nil); err != nil {
		t.Fatal(err)
	}
	stdout, stderr, code := f.cli(t, "run", "--new-worktree", "--repo", repo, "--launch-config", "Fake", "--view", "chat",
		"--prompt", "start", "--reply", "--timeout", "40s")
	if code != 0 {
		t.Fatalf("run --reply: %d %q %q", code, stdout, stderr)
	}
	sessions, _, _ := f.cli(t, "ls", "--json")
	var list []contract.SessionInfo
	if err := json.Unmarshal([]byte(sessions), &list); err != nil || len(list) != 1 {
		t.Fatalf("sessions %q: %v", sessions, err)
	}
	id := list[0].ID

	// A session without a chat view: clear refusals, nothing sent.
	plain := detachedID(t, f, "sleep", "30")
	for _, args := range [][]string{{"chat", "status", plain}, {"view", plain, "chat"}} {
		if _, stderr, code := f.cli(t, args...); code != contract.ExitError || !strings.Contains(stderr, "no chat view") {
			t.Fatalf("%v: %d %q", args, code, stderr)
		}
	}
	if _, stderr, code := f.cli(t, "send", plain, "hi", "--reply"); code != contract.ExitError || !strings.Contains(stderr, "--wait") {
		t.Fatalf("send --reply to a terminal session: %d %q", code, stderr)
	}

	// --reply returns the reply of the input's own turn, and watch shows
	// it live without the earlier history.
	watch := f.command(nil, "chat", "watch", "--config", f.config, id, "--json")
	pipe, _ := watch.StdoutPipe()
	if err := watch.Start(); err != nil {
		t.Fatal(err)
	}
	lines := make(chan string, 100)
	go func() {
		scanner := bufio.NewScanner(pipe)
		scanner.Buffer(nil, 1<<20)
		for scanner.Scan() {
			lines <- scanner.Text()
		}
		close(lines)
	}()
	next := func() contract.ChatFrame {
		t.Helper()
		select {
		case line := <-lines:
			var frame contract.ChatFrame
			if err := json.Unmarshal([]byte(line), &frame); err != nil {
				t.Fatalf("watch line %q: %v", line, err)
			}
			return frame
		case <-time.After(10 * time.Second):
			t.Fatal("watch printed nothing")
		}
		return contract.ChatFrame{}
	}
	if frame := next(); frame.Type != contract.ChatFrameSnapshot {
		t.Fatalf("first watch frame %+v", frame)
	}
	stdout, stderr, code = f.cli(t, "send", id, "hello there", "--reply", "--timeout", "30s")
	if code != 0 || strings.TrimSpace(stdout) == "" {
		t.Fatalf("send --reply: %d %q %q", code, stdout, stderr)
	}
	var users []string
	for ended := false; !ended; {
		for _, ev := range next().Items {
			if ev.Message != nil && ev.Message.Role == "user" {
				users = append(users, ev.Message.Text)
			}
			ended = ended || ev.TurnEnd != nil && ev.TurnEnd.StopReason == "end_turn"
		}
	}
	_ = watch.Process.Kill()
	_ = watch.Wait()
	if strings.Join(users, "|") != "hello there" {
		t.Fatalf("watch saw user messages %q; want only the new one", users)
	}

	// chat log reads what the session said after the fact; the folded tool
	// call shows once in its final state.
	reply := strings.TrimSpace(stdout)
	if out, stderr, code := f.cli(t, "chat", "log", id); code != 0 || strings.TrimSpace(out) != "agent: "+reply || strings.Contains(out, "tool ") {
		t.Fatalf("chat log: %d %q %q; want the reply %q alone", code, out, stderr, reply)
	}
	out, _, code := f.cli(t, "chat", "log", id, "--last", "3", "--tools")
	if code != 0 || strings.Count(out, "tool ") != 1 || !strings.Contains(out, "(completed)") || strings.Contains(out, "in_progress") {
		t.Fatalf("chat log --last 3 --tools: %d %q", code, out)
	}
	out, _, code = f.cli(t, "chat", "log", id, "--last", "3", "--tools", "--json")
	var logged contract.ChatFrame
	if err := json.Unmarshal([]byte(out), &logged); err != nil || code != 0 || logged.Type != contract.ChatFrameEvents || len(logged.Items) == 0 {
		t.Fatalf("chat log --json: %d %q: %v", code, out, err)
	}
	if !slices.ContainsFunc(logged.Items, func(ev contract.ChatEvent) bool {
		return ev.Message != nil && ev.Message.Role == "agent" && ev.Message.Text == reply
	}) {
		t.Fatalf("chat log --json lacks the reply %q: %q", reply, out)
	}
	for _, last := range []string{"0", "-1", "1001", "x"} {
		if _, _, code := f.cli(t, "chat", "log", id, "--last", last); code != contract.ExitUsage {
			t.Fatalf("--last %s: exit %d", last, code)
		}
	}
	if _, stderr, code := f.cli(t, "chat", "log", plain); code != contract.ExitError || !strings.Contains(stderr, "no chat view") {
		t.Fatalf("chat log on a terminal session: %d %q", code, stderr)
	}
	if _, _, code := f.cli(t, "chat", "log", "zzzzzzzz"); code != contract.ExitError {
		t.Fatalf("chat log on a missing session: %d", code)
	}
	if _, _, code := f.cli(t, "interrupt", id); code != 0 {
		t.Fatalf("interrupt while idle: %d", code)
	}
	// Keys belong to the terminal view: refused, nothing sent.
	if _, stderr, code := f.cli(t, "send", id, "--key", "esc"); code != contract.ExitError || !strings.Contains(stderr, "wrong_view") {
		t.Fatalf("--key on the chat view: %d %q", code, stderr)
	}

	// Settings: a select takes one of its values; refusals name the cause.
	settings := chatState(t, f, id).Config
	if len(settings) == 0 || len(settings[0].Options) < 2 {
		t.Fatalf("the fake agent offers no select setting: %+v", settings)
	}
	setting, value := settings[0].ID, settings[0].Options[len(settings[0].Options)-1].Value
	if _, stderr, code := f.cli(t, "chat", "set", id, setting+"="+value); code != 0 {
		t.Fatalf("chat set: %d %q", code, stderr)
	}
	eventuallyCLI(t, "the new setting", func() bool { return chatState(t, f, id).Config[0].Value == value })
	if _, stderr, code := f.cli(t, "chat", "set", id, "nope=1"); code != contract.ExitError || !strings.Contains(stderr, setting) {
		t.Fatalf("unknown setting: %d %q", code, stderr)
	}
	if _, _, code := f.cli(t, "chat", "set", id, setting+"=nope"); code != contract.ExitError {
		t.Fatalf("unknown value: %d", code)
	}
	if _, _, code := f.cli(t, "chat", "set", id, "novalue"); code != contract.ExitUsage {
		t.Fatalf("malformed setting: %d", code)
	}

	// Ask: --reply stops at the request; status lists it; answer resolves it.
	if _, _, code := f.cli(t, "chat", "permission", id, "ask"); code != 0 {
		t.Fatalf("permission ask: %d", code)
	}
	_, stderr, code = f.cli(t, "send", id, "needs a tool", "--reply", "--timeout", "30s")
	if code != contract.ExitError || !strings.Contains(stderr, "chat answer") {
		t.Fatalf("send --reply at a request: %d %q", code, stderr)
	}
	state := chatState(t, f, id)
	if len(state.Requests) != 1 || len(state.Requests[0].Options) == 0 || !state.Turn.Running {
		t.Fatalf("status: %+v", state)
	}
	request := state.Requests[0]

	// Input sent during the turn is queued; the queue commands edit it.
	stdout, _, code = f.cli(t, "send", id, "later", "--json")
	var queued contract.SessionInputResult
	if err := json.Unmarshal([]byte(stdout), &queued); err != nil || code != 0 || queued.View != contract.ViewChat ||
		queued.Queued == nil || !*queued.Queued || queued.ItemID == "" {
		t.Fatalf("queued send: %d %q", code, stdout)
	}
	if _, _, code := f.cli(t, "chat", "queue", "edit", id, queued.ItemID, "later, edited"); code != 0 {
		t.Fatalf("queue edit: %d", code)
	}
	if out, _, _ := f.cli(t, "chat", "queue", "ls", id); !strings.Contains(out, queued.ItemID+"\tlater, edited") {
		t.Fatalf("queue ls: %q", out)
	}
	if _, _, code := f.cli(t, "chat", "queue", "rm", id, queued.ItemID); code != 0 {
		t.Fatalf("queue rm: %d", code)
	}
	if _, _, code := f.cli(t, "chat", "queue", "rm", id, queued.ItemID); code != contract.ExitError {
		t.Fatalf("queue rm twice: %d", code)
	}

	if _, _, code := f.cli(t, "chat", "answer", id, request.RequestID, "--content", "{}"); code != contract.ExitUsage {
		t.Fatalf("--content on a permission: %d", code)
	}
	if _, _, code := f.cli(t, "chat", "answer", id, request.RequestID, "--option", request.Options[0].OptionID); code != 0 {
		t.Fatalf("answer: %d", code)
	}
	if _, _, code := f.cli(t, "chat", "answer", id, request.RequestID, "--cancel"); code != contract.ExitError {
		t.Fatalf("answering twice: %d", code)
	}
	eventuallyCLI(t, "the answered turn to end", func() bool { return !chatState(t, f, id).Turn.Running })

	// A queued prompt removed before it is sent ends --reply with 1.
	if _, _, code := f.cli(t, "send", id, "blocks on a tool"); code != 0 {
		t.Fatalf("send: %d", code)
	}
	eventuallyCLI(t, "the next request", func() bool { return len(chatState(t, f, id).Requests) == 1 })
	waiting := f.command(nil, "send", "--config", f.config, id, "never sent", "--reply", "--timeout", "30s")
	var waitErr strings.Builder
	waiting.Stderr = &waitErr
	if err := waiting.Start(); err != nil {
		t.Fatal(err)
	}
	eventuallyCLI(t, "the queued prompt", func() bool { return len(chatState(t, f, id).Queue.Items) == 1 })
	if _, _, code := f.cli(t, "chat", "queue", "clear", id); code != 0 {
		t.Fatalf("queue clear: %d", code)
	}
	_ = waiting.Wait()
	if waiting.ProcessState.ExitCode() != contract.ExitError || !strings.Contains(waitErr.String(), "removed") {
		t.Fatalf("waiting for a removed prompt: %d %q", waiting.ProcessState.ExitCode(), waitErr.String())
	}
	// interrupt cancels the turn waiting on its request.
	if _, _, code := f.cli(t, "interrupt", id); code != 0 {
		t.Fatalf("interrupt: %d", code)
	}
	eventuallyCLI(t, "the cancelled turn to end", func() bool { return !chatState(t, f, id).Turn.Running })
	if out, _, _ := f.cli(t, "ls", "--archived", "--json"); strings.TrimSpace(out) != "[]" {
		t.Fatalf("ls --archived with nothing archived: %q", out)
	}

	// Change to the terminal view and back.
	if _, stderr, code := f.cli(t, "view", id, "terminal", "--force"); code != 0 {
		t.Fatalf("view terminal: %d %q", code, stderr)
	}
	if _, stderr, code := f.cli(t, "send", id, "hi", "--reply"); code != contract.ExitError || !strings.Contains(stderr, "--wait") {
		t.Fatalf("send --reply on the terminal view: %d %q", code, stderr)
	}
	if _, stderr, code := f.cli(t, "chat", "permission", id, "auto"); code != contract.ExitError || !strings.Contains(stderr, "webmux view "+id+" chat") {
		t.Fatalf("chat permission on the terminal view: %d %q", code, stderr)
	}
	if out, _, _ := f.cli(t, "ls"); !strings.Contains(out, "终端") {
		t.Fatalf("ls on the terminal view: %q", out)
	}
	if _, stderr, code := f.cli(t, "view", id, "chat"); code != 0 {
		t.Fatalf("view chat: %d %q", code, stderr)
	}
	if out, _, _ := f.cli(t, "chat", "status", id); !strings.Contains(out, "view") || !strings.Contains(out, "chat") {
		t.Fatalf("status after changing back: %q", out)
	}
	if _, _, code := f.cli(t, "resume", id, "--detach"); code != contract.ExitError {
		t.Fatalf("resume an active session: %d", code)
	}

	// Removing archives it; rm alone then points to --forget; resume brings
	// it back under the same ID on its chat view.
	if _, stderr, code := f.cli(t, "rm", id); code != 0 {
		t.Fatalf("rm: %d %q", code, stderr)
	}
	eventuallyCLI(t, "the archived session", func() bool {
		out, _, _ := f.cli(t, "ls", "--archived", "--repo", repo, "--json")
		return strings.Contains(out, id)
	})
	if _, stderr, code := f.cli(t, "rm", id); code != contract.ExitError || !strings.Contains(stderr, "--forget") {
		t.Fatalf("rm an archived session: %d %q", code, stderr)
	}
	stdout, stderr, code = f.cli(t, "resume", id, "--detach")
	if code != 0 || strings.TrimSpace(stdout) != id {
		t.Fatalf("resume: %d %q %q", code, stdout, stderr)
	}
	if out, _, _ := f.cli(t, "get", id, "--field", "session.view"); strings.TrimSpace(out) != "chat" {
		t.Fatalf("resumed view %q", out)
	}
	if _, stderr, code := f.cli(t, "rm", id); code != 0 {
		t.Fatalf("rm after resume: %d %q", code, stderr)
	}
	eventuallyCLI(t, "the closed session", func() bool {
		out, _, _ := f.cli(t, "ls", "--state", "closed", "--json")
		return strings.Contains(out, id)
	})
	if _, stderr, code := f.cli(t, "rm", id, "--forget"); code != contract.ExitUsage || !strings.Contains(stderr, "--yes") {
		t.Fatalf("forget without a terminal: %d %q", code, stderr)
	}
	if _, stderr, code := f.cli(t, "rm", id, "--forget", "--yes"); code != 0 {
		t.Fatalf("forget: %d %q", code, stderr)
	}
	if out, _, _ := f.cli(t, "ls", "--state", "all", "--json"); strings.Contains(out, id) {
		t.Fatalf("forgotten session still listed: %q", out)
	}

	// The removed commands and options are unknown: usage errors.
	for _, args := range [][]string{
		{"chat", "send", plain, "hi"}, {"chat", "cancel", plain}, {"chat", "switch", plain, "--to", "chat"},
		{"chat", "ls"}, {"chat", "resume", plain}, {"chat", "forget", plain, "--yes"},
		{"run", "--start-side", "chat", "--launch-config", "Fake"}, {"launch-config", "add", "X", "--hybrid", "--", "true"},
	} {
		if _, _, code := f.cli(t, args...); code != contract.ExitUsage {
			t.Errorf("%v: exit %d, want %d", args, code, contract.ExitUsage)
		}
	}
}

func chatState(t *testing.T, f *fixture, id string) contract.ChatState {
	t.Helper()
	stdout, stderr, code := f.cli(t, "chat", "status", id, "--json")
	var state contract.ChatState
	if err := json.Unmarshal([]byte(stdout), &state); err != nil || code != 0 {
		t.Fatalf("chat status: %d %q %q", code, stdout, stderr)
	}
	return state
}
