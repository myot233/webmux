package hybrid

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/rainchan/webmux/internal/acp"
	"github.com/rainchan/webmux/internal/agentprofile"
	"github.com/rainchan/webmux/internal/agentprofile/codex"
	"github.com/rainchan/webmux/internal/session"
)

// codexInput is a Codex session whose PATH holds the fake codex and,
// unless pathDir says otherwise, its codex-acp connector. Writer locks
// live in a CODEX_HOME of the test's own.
func (h *harness) codexInput(side Side, pathDir string, args ...string) CreateInput {
	h.t.Helper()
	// Clear the host connector path before resolving this fixture's fake agent.
	// An explicit launch value would also override the connector's defaults.
	h.t.Setenv("CODEX_PATH", "")
	exe := agentPath(h.t, codex.Name)
	if pathDir == "" {
		pathDir = filepath.Dir(exe)
	}
	in := h.input(codex.Name, side, "FAKEAGENT_PROFILE=", "CODEX_HOME="+h.t.TempDir(), "PATH="+pathDir+":/usr/bin:/bin")
	in.Cmd = append([]string{exe}, args...)
	return in
}

func configValue(options []ConfigOption, id string) string {
	for _, o := range options {
		if o.ID == id {
			return o.Value
		}
	}
	return ""
}

// TestCodexRoundTrip: the terminal UI runs without the shared daemon, its
// session comes from its writer lock and is switchable once saved; the
// chat side runs codex-acp with the TUI's codex, gets the approval mode
// and model from the launch arguments back after every load, and hides
// /logout.
func TestCodexRoundTrip(t *testing.T) {
	t.Setenv("CODEX_PATH", "/nonexistent/host-codex")
	h := newHarness(t, 0)
	h.fast()
	s := h.create(h.codexInput(SideTUI, "", "--dangerously-bypass-approvals-and-sandbox", "-m", "gpt-b", "--search"))
	id := s.Info().ID
	if cmd := s.Info().Cmd; !slices.Contains(cmd, "--no-daemon") {
		t.Fatalf("terminal command %q", cmd)
	}
	waitFor(t, "session from the writer lock", func() bool { return s.Info().Hybrid.AgentSessionID != nil })
	h.waitReason(id, ReasonNoSessionID)
	tuiTurn(t, s, "one")
	h.waitEligible(id)
	sid := *s.Info().Hybrid.AgentSessionID

	r := h.mustSwitch(id, SideACP)
	if !slices.Equal(r.Unsupported, []string{"--search"}) {
		t.Fatalf("unsupported %q", r.Unsupported)
	}
	st := h.chatState(id)
	if configValue(st.Config, "mode") != "agent-full-access" || configValue(st.Config, "model") != "gpt-b" {
		t.Fatalf("chat config %+v", st.Config)
	}
	if rec := h.record(id); rec.Config["mode"] != "agent-full-access" || rec.AgentSessionID != sid {
		t.Fatalf("record %+v", rec)
	}
	if slices.ContainsFunc(st.Commands, func(c Command) bool { return c.Name == "logout" }) || len(st.Commands) == 0 {
		t.Fatalf("commands %+v", st.Commands)
	}
	log, err := readEventsLog(h.store)
	if err != nil || !strings.Contains(log, "CODEX_PATH="+agentPath(t, codex.Name)+" NO_BROWSER=1") {
		t.Fatalf("connector environment not logged: %v", err)
	}
	chatTurn(t, h, id, "two")

	h.mustSwitch(id, SideTUI)
	waitFor(t, "resumed transcript", func() bool { return strings.Contains(string(s.Buffer()), "> two") })
	tuiTurn(t, s, "three")
	h.waitEligible(id)
	if got := *s.Info().Hybrid.AgentSessionID; got != sid {
		t.Fatalf("resumed terminal names session %s, want %s", got, sid)
	}
	h.mustSwitch(id, SideACP)
	if got := h.replayedUsers(id); !slices.Equal(got, []string{"one", "two", "three"}) {
		t.Fatalf("replay %q", got)
	}
	if mode := configValue(h.chatState(id).Config, "mode"); mode != "agent-full-access" {
		t.Fatalf("mode after the second load %q", mode)
	}
}

// TestCodexConnectorMissing: without codex-acp a chat-first session is
// refused before anything is recorded, a terminal session runs and says
// why it cannot switch, and installing the connector is noticed without
// a restart.
func TestCodexConnectorMissing(t *testing.T) {
	h := newHarness(t, 0)
	h.fast()
	dir := t.TempDir()
	if err := os.Symlink(agentPath(t, codex.Name), filepath.Join(dir, "codex")); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()
	_, err := h.svc.Create(ctx, h.codexInput(SideACP, dir))
	var missing *ConnectorMissingError
	if !errors.As(err, &missing) || missing.Connector.Executable != "codex-acp" {
		t.Fatalf("chat-first create: %v", err)
	}
	if recs, _ := h.svc.Records(false); len(recs) != 0 || len(h.manager.List()) != 0 {
		t.Fatalf("refused create left records %v sessions %v", recs, h.manager.List())
	}
	if log, _ := readEventsLog(h.store); strings.Contains(log, " acp ") {
		t.Fatal("an ACP process started")
	}

	s := h.create(h.codexInput(SideTUI, dir))
	id := s.Info().ID
	h.waitReason(id, ReasonConnectorMissing)
	if e := h.eligibility(id); e.Connector == nil || e.Connector.Executable != "codex-acp" {
		t.Fatalf("eligibility %+v", e)
	}
	if _, err := h.switchTo(id, SideACP); !errors.As(err, &missing) {
		t.Fatalf("switch without the connector: %v", err)
	}
	if err := os.Symlink(agentPath(t, "codex-acp"), filepath.Join(dir, "codex-acp")); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "connector found", func() bool { return h.eligibility(id).Reason != ReasonConnectorMissing })
	tuiTurn(t, s, "one")
	h.waitEligible(id)
	h.mustSwitch(id, SideACP)
}

// TestCodexStartupConnectorCheck: through the configured startup file the
// check sees the PATH it sets, and the file's output does not matter.
func TestCodexStartupConnectorCheck(t *testing.T) {
	tests := []struct {
		shell, rc string
	}{
		{shell: "bash", rc: ".bashrc"},
		{shell: "zsh", rc: ".zshrc"},
	}
	for _, tt := range tests {
		t.Run(tt.shell, func(t *testing.T) {
			if _, err := exec.LookPath(tt.shell); err != nil {
				t.Skip(tt.shell + " not installed")
			}
			h := newHarness(t, 0)
			h.fast()
			home := t.TempDir()
			agents := filepath.Dir(agentPath(t, codex.Name))
			rc := "echo welcome\nexport PATH=" + agents + ":$PATH\n"
			if err := os.WriteFile(filepath.Join(home, tt.rc), []byte(rc), 0o600); err != nil {
				t.Fatal(err)
			}
			in := h.codexInput(SideACP, "/nonexistent")
			in.Env["HOME"] = home
			in.Cmd = []string{tt.shell, "-ic", `exec -- "$@"`, "webmux", "codex"}
			id := h.create(in).Info().ID
			chatTurn(t, h, id, "hello")
			log, err := readEventsLog(h.store)
			if err != nil || !strings.Contains(log, "CODEX_PATH="+filepath.Join(agents, "codex")) {
				t.Fatalf("connector environment not logged: %v", err)
			}
		})
	}
}

// TestExitedNotFound: a startup-file wrapper that exits 127 before
// answering means the connector disappeared, for either supported shell;
// a direct command has no wrapper exit status to interpret.
func TestExitedNotFound(t *testing.T) {
	m := session.NewManager(session.Options{DefaultCwd: t.TempDir()})
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
		defer cancel()
		m.Shutdown(ctx)
	})
	sess, err := m.CreateShell(session.ShellInput{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	sidecar, err := sess.StartSidecar([]string{"/bin/sh", "-c", "exit 127"}, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	<-sidecar.Done()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
		defer cancel()
		_ = sidecar.Stop(ctx)
	})
	registry := agentprofile.NewRegistry(codex.Agent)
	for _, tt := range []struct {
		name string
		argv []string
		want bool
	}{
		{name: "bash", argv: []string{"bash", "-ic", `exec -- "$@"`, "webmux", "codex"}, want: true},
		{name: "zsh", argv: []string{"zsh", "-ic", `exec -- "$@"`, "webmux", "codex"}, want: true},
		{name: "direct", argv: []string{"codex"}, want: false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			cmd, err := registry.Parse(tt.argv)
			if err != nil {
				t.Fatal(err)
			}
			got := exitedNotFound(cmd, sidecar)
			if (got != nil) != tt.want {
				t.Fatalf("exitedNotFound = %+v, want failure=%v", got, tt.want)
			}
		})
	}
}

// TestCodexExternalWriter: a Codex client outside WebMux holding the
// session refuses resuming it to the chat with occupied_external.
func TestCodexExternalWriter(t *testing.T) {
	h := newHarness(t, 0)
	h.fast()
	in := h.codexInput(SideTUI, "")
	s := h.create(in)
	id := s.Info().ID
	tuiTurn(t, s, "one")
	h.waitEligible(id)
	waitFor(t, "session from the writer lock", func() bool { return s.Info().Hybrid.AgentSessionID != nil })
	sid := *s.Info().Hybrid.AgentSessionID
	h.closeSession(id)
	other, err := h.manager.Create(session.CreateInput{Cwd: in.Cwd, Cmd: []string{in.Cmd[0], "resume", sid}, Env: in.Env})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = h.manager.Close(other.Info().ID) })
	waitFor(t, "outside client holds the session", func() bool { return strings.Contains(string(other.Buffer()), "> one") })
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()
	if _, err := h.svc.Resume(ctx, id, ResumeInput{ClientRequestID: rid(), Side: SideACP, Env: in.Env}); !errors.Is(err, ErrExternalWriter) {
		t.Fatalf("resume while held: %v", err)
	}
	if rec := h.record(id); rec.State != StateClosed {
		t.Fatalf("record after the refused resume %+v", rec)
	}
}

// TestCodexSteeringAfterTurnEnd: steering that reaches Codex after its
// turn ended comes back as promptRequired and goes out as the next turn
// instead of a turn Codex would start on its own.
func TestCodexSteeringAfterTurnEnd(t *testing.T) {
	h := newHarness(t, 0)
	in := h.codexInput(SideACP, "")
	in.Env["FAKEAGENT_DELAY"] = "_session/steering=300ms"
	id := h.create(in).Info().ID
	st := h.follow(id)
	if !st.state.Steering {
		t.Fatal("codex chat does not offer steering")
	}
	prompt(t, h, id, "busy")
	if _, err := h.svc.Prompt(ctxT(t), id, PromptInput{ClientRequestID: rid(), Text: "nudge", Mode: ModeSteer}); err != nil {
		t.Fatalf("steer: %v", err)
	}
	st.until("nudge as the next turn", func(events []Event) bool {
		return strings.Join(userTexts(events), ",") == "busy,nudge" && count(EventTurnEnd, 2)(events)
	})
	if n := countCalls(t, h.store, acp.MethodSessionPrompt); n != 2 {
		t.Fatalf("session/prompt calls %d", n)
	}
}

// TestLockTracker follows Codex's measured lock behaviour: one lock at
// start, the old lock kept after /new, and a blank session's lock beside
// the resumed one.
func TestLockTracker(t *testing.T) {
	const a, b, blank = "a", "b", "blank"
	steps := []struct {
		name     string
		held     []string
		starting bool
		resumed  string
		current  string
		unknown  bool
	}{
		{name: "loading", held: nil, starting: true},
		{name: "started", held: []string{a}, starting: true, current: a},
		{name: "new session keeps the old lock", held: []string{a, b}, current: b},
		{name: "unchanged", held: []string{b, a}, current: b},
		{name: "new session closed", held: []string{a}, current: a},
	}
	lt := lockTracker{seen: map[string]bool{}}
	for _, s := range steps {
		lt.track(s.held, false, s.resumed, s.starting)
		if lt.current != s.current || lt.unknown != s.unknown {
			t.Fatalf("%s: current %q unknown %v", s.name, lt.current, lt.unknown)
		}
	}

	resumed := lockTracker{seen: map[string]bool{}}
	resumed.track([]string{blank}, false, a, true)
	resumed.track([]string{blank, a}, false, a, true)
	if resumed.current != a {
		t.Fatalf("resumed run follows %q", resumed.current)
	}
	resumed.track([]string{blank, a, b}, false, a, false)
	if resumed.current != b {
		t.Fatalf("new session after resuming: %q", resumed.current)
	}

	both := lockTracker{seen: map[string]bool{}}
	both.track([]string{a, b}, false, "", true)
	if both.current != "" || !both.unknown {
		t.Fatalf("two locks at start: %+v", both)
	}
	both.track(nil, true, "", false)
	if both.current != "" || !both.unknown {
		t.Fatalf("unreadable files: %+v", both)
	}
}

func TestLookupAnswers(t *testing.T) {
	out := []byte("welcome\n" + agentprofile.LookupMarker + "/x/codex-acp\n" + agentprofile.LookupMarker + "\n")
	got, ok := lookupAnswers(out, 2)
	if !ok || !slices.Equal(got, []string{"/x/codex-acp", ""}) {
		t.Fatalf("answers %q %v", got, ok)
	}
	if _, ok := lookupAnswers([]byte("bashrc exited early\n"), 1); ok {
		t.Fatal("answers without markers")
	}
}

// TestCodexStartupIsNotATurn: Codex reports working while it loads; a
// switch right after the terminal UI (re)started neither interrupts it
// nor waits for it.
func TestCodexStartupIsNotATurn(t *testing.T) {
	h := newHarness(t, 0)
	h.fast()
	in := h.codexInput(SideACP, "")
	in.Env["FAKEAGENT_STARTUP_DELAY"] = "5000ms"
	s := h.create(in)
	id := s.Info().ID
	chatTurn(t, h, id, "one")
	h.mustSwitch(id, SideTUI)
	waitFor(t, "loading terminal reports working", func() bool { return tuiBusy(s.Info()) })
	h.waitEligible(id)
	if !tuiBusy(s.Info()) {
		t.Fatal("the terminal finished loading before the quiet-period check")
	}
	h.mustSwitch(id, SideACP)
	lines := strings.Split(h.logs.String(), "\n")
	last := ""
	for _, line := range lines {
		if strings.Contains(line, `msg="hybrid switch started"`) && strings.Contains(line, "to=acp") {
			last = line
		}
	}
	if !strings.Contains(last, "interrupt=false") {
		t.Fatalf("switch during startup: %s", last)
	}
}

func TestStartFailurePreservesCause(t *testing.T) {
	h := newHarness(t, 0)
	cmd, _, _, err := h.svc.admit(h.codexInput(SideACP, ""))
	if err != nil {
		t.Fatal(err)
	}
	failure := startFailure(cmd, context.Canceled)
	if !errors.Is(failure.err, context.Canceled) || !errors.Is(failure.err, ErrAgent) {
		t.Fatalf("start failure lost its causes: %v", failure.err)
	}
}
