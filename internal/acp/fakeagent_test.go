package acp

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"
)

var fakeAgent struct {
	once sync.Once
	dir  string
	path string
	err  error
}

// macOS places TMPDIR behind the /var -> /private/var symlink, while the
// fake agent reports and stores canonical working directories.
func TestMain(m *testing.M) {
	if dir, err := filepath.EvalSymlinks(os.TempDir()); err == nil {
		_ = os.Setenv("TMPDIR", dir)
	}
	code := m.Run()
	if fakeAgent.dir != "" {
		_ = os.RemoveAll(fakeAgent.dir)
	}
	os.Exit(code)
}

// fakeAgentPath builds testdata/fakeagent once per test binary.
func fakeAgentPath(t *testing.T) string {
	t.Helper()
	if testing.Short() {
		t.Skip("builds and runs the fake agent")
	}
	fakeAgent.once.Do(func() {
		fakeAgent.dir, fakeAgent.err = os.MkdirTemp("", "acp-fakeagent-")
		if fakeAgent.err != nil {
			return
		}
		fakeAgent.path = filepath.Join(fakeAgent.dir, "fakeagent")
		out, err := exec.Command("go", "build", "-o", fakeAgent.path, "./testdata/fakeagent").CombinedOutput()
		if err != nil {
			fakeAgent.err = fmt.Errorf("go build: %w\n%s", err, out)
		}
	})
	if fakeAgent.err != nil {
		t.Fatal(fakeAgent.err)
	}
	return fakeAgent.path
}

type fakeEnv struct {
	store string
	cwd   string
	env   []string
}

func newFakeEnv(t *testing.T, env ...string) fakeEnv {
	return fakeEnv{store: t.TempDir(), cwd: t.TempDir(), env: env}
}

func (e fakeEnv) command(t *testing.T, args ...string) *exec.Cmd {
	cmd := exec.Command(fakeAgentPath(t), args...)
	cmd.Dir = e.cwd
	cmd.Env = append(append(os.Environ(), "FAKEAGENT_STORE="+e.store, "FAKEAGENT_TURN_DELAY=5ms"), e.env...)
	return cmd
}

func (e fakeEnv) events(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(e.store, "events.log"))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// acpAgent is a fake agent process in ACP mode behind a Conn.
type acpAgent struct {
	cmd  *exec.Cmd
	conn *Conn
	rec  *recorder
	done chan error
}

func (e fakeEnv) startACP(t *testing.T, rec *recorder, args ...string) *acpAgent {
	t.Helper()
	cmd := e.command(t, append(args, "acp")...)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	a := &acpAgent{cmd: cmd, conn: NewConn(stdout, stdin, Options{Handler: rec}), rec: rec, done: make(chan error, 1)}
	go func() {
		<-a.conn.Done() // stdout fully read before Wait closes it
		a.done <- cmd.Wait()
	}()
	t.Cleanup(func() {
		a.conn.Close()
		select {
		case <-a.done:
		case <-time.After(testTimeout):
			_ = cmd.Process.Kill()
			t.Error("fake agent did not exit")
		}
	})
	return a
}

// exit closes stdin and returns the process exit code.
func (a *acpAgent) exit(t *testing.T) int {
	t.Helper()
	a.conn.Close()
	return a.wait(t)
}

func (a *acpAgent) wait(t *testing.T) int {
	t.Helper()
	select {
	case err := <-a.done:
		a.done <- err // let Cleanup see it too
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			return exit.ExitCode()
		}
		if err != nil {
			t.Fatal(err)
		}
		return 0
	case <-time.After(testTimeout):
		t.Fatal("fake agent did not exit")
		return -1
	}
}

func (a *acpAgent) initialize(t *testing.T) InitializeResponse {
	t.Helper()
	resp, err := a.conn.Initialize(context.Background(), Implementation{Name: "webmux-test", Version: "0"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

// drain returns the updates handled so far.
func (r *recorder) drain() []Update {
	var out []Update
	for {
		select {
		case n := <-r.updates:
			out = append(out, n.Update)
		default:
			return out
		}
	}
}

// transcript joins message chunks as "user: …" / "agent: …" lines.
func transcript(updates []Update) string {
	var b strings.Builder
	last := ""
	for _, u := range updates {
		role, text := "", ""
		switch u := u.(type) {
		case *UserMessageChunk:
			role, text = "user", u.Content.Text
		case *AgentMessageChunk:
			role, text = "agent", u.Content.Text
		default:
			continue
		}
		if role != last {
			if last != "" {
				b.WriteString("\n")
			}
			b.WriteString(role + ": ")
			last = role
		}
		b.WriteString(text)
	}
	return b.String()
}

// tuiAgent is a fake agent in TUI mode on pipes.
type tuiAgent struct {
	t     *testing.T
	stdin io.WriteCloser
	out   syncBuffer
	done  chan error
}

func (e fakeEnv) startTUI(t *testing.T, args ...string) *tuiAgent {
	t.Helper()
	cmd := e.command(t, args...)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	a := &tuiAgent{t: t, stdin: stdin, done: make(chan error, 1)}
	cmd.Stdout = &a.out
	cmd.Stderr = &a.out
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	go func() { a.done <- cmd.Wait() }()
	t.Cleanup(func() {
		stdin.Close()
		select {
		case <-a.done:
		case <-time.After(testTimeout):
			_ = cmd.Process.Kill()
		}
	})
	return a
}

func (a *tuiAgent) send(s string) {
	a.t.Helper()
	if _, err := io.WriteString(a.stdin, s); err != nil {
		a.t.Fatal(err)
	}
}

// waitFor waits until the output contains want n times.
func (a *tuiAgent) waitFor(want string, n int) string {
	a.t.Helper()
	deadline := time.Now().Add(testTimeout)
	for time.Now().Before(deadline) {
		if out := a.out.String(); strings.Count(out, want) >= n {
			return out
		}
		time.Sleep(5 * time.Millisecond)
	}
	a.t.Fatalf("output never had %d× %q:\n%q", n, want, a.out.String())
	return ""
}

func (a *tuiAgent) exit() int {
	a.t.Helper()
	a.send("/exit\n")
	select {
	case err := <-a.done:
		a.done <- err // let Cleanup see it too
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			return exit.ExitCode()
		}
		if err != nil {
			a.t.Fatal(err)
		}
		return 0
	case <-time.After(testTimeout):
		a.t.Fatal("TUI did not exit")
		return -1
	}
}

var warpSessionID = regexp.MustCompile(`"event":"session_start"[^\x07]*"session_id":"([^"]+)"`)

func sessionIDs(out string) []string {
	var ids []string
	for _, m := range warpSessionID.FindAllStringSubmatch(out, -1) {
		ids = append(ids, m[1])
	}
	return ids
}

const stopEvent = `"event":"stop"`

func TestFakeAgentSharesStoreAcrossModes(t *testing.T) {
	e := newFakeEnv(t)
	ctx := context.Background()

	// TUI creates the session and runs a turn.
	tui := e.startTUI(t)
	out := tui.waitFor(`"event":"session_start"`, 1)
	ids := sessionIDs(out)
	if len(ids) != 1 {
		t.Fatalf("session_start ids %v in %q", ids, out)
	}
	id := ids[0]
	for _, want := range []string{
		"\x1b]7501;state=idle:app=omp\x1b\\",
		"\x1b]7501;state=idle:id=" + id + "\x1b\\",
		`"agent":"omp"`, `"plugin_version":"1.0.0"`, `"cwd":"` + e.cwd + `"`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("TUI output lacks %q", want)
		}
	}
	tui.send("hello\n")
	out = tui.waitFor(stopEvent, 1)
	for _, want := range []string{"\x1b]7501;state=working\x1b\\", "\x1b]7501;state=working:id=" + id + "\x1b\\", "echo: hello"} {
		if !strings.Contains(out, want) {
			t.Errorf("TUI turn output lacks %q", want)
		}
	}
	if code := tui.exit(); code != 0 {
		t.Fatalf("TUI exit code %d", code)
	}

	// ACP lists and loads the TUI's session, then continues it.
	a := e.startACP(t, newRecorder())
	init := a.initialize(t)
	if init.AgentInfo == nil || init.AgentInfo.Name != "omp" || !init.AgentCapabilities.LoadSession ||
		init.AgentCapabilities.SessionCapabilities.List == nil || init.AgentCapabilities.SessionCapabilities.Resume == nil ||
		!strings.Contains(string(init.Meta), "steering") {
		t.Errorf("initialize = %+v", init)
	}
	list, err := a.conn.ListSessions(ctx, ListSessionsRequest{Cwd: e.cwd})
	if err != nil {
		t.Fatal(err)
	}
	if len(list.Sessions) != 1 || list.Sessions[0].SessionID != id || list.Sessions[0].Cwd != e.cwd || list.Sessions[0].UpdatedAt == "" {
		t.Fatalf("session/list = %+v, want %s", list, id)
	}
	if other, err := a.conn.ListSessions(ctx, ListSessionsRequest{Cwd: "/elsewhere"}); err != nil || len(other.Sessions) != 0 {
		t.Errorf("list for another cwd = %+v, %v", other, err)
	}
	load, err := a.conn.LoadSession(ctx, LoadSessionRequest{SessionID: id, Cwd: e.cwd})
	if err != nil {
		t.Fatal(err)
	}
	replay := a.rec.drain()
	if got, want := transcript(replay), "user: hello\nagent: echo: hello"; got != want {
		t.Errorf("replay = %q, want %q", got, want)
	}
	if len(load.ConfigOptions) != 2 || load.ConfigOptions[1].Current.ID != "fast" {
		t.Errorf("config options = %+v", load.ConfigOptions)
	}
	set, err := a.conn.SetConfigOption(ctx, SetConfigOptionRequest{SessionID: id, ConfigID: "model", Value: ConfigValue{ID: "smart"}})
	if err != nil || set.ConfigOptions[1].Current.ID != "smart" || set.ConfigOptions[1].Options[1].Group != "builtin" {
		t.Errorf("set_config_option = %+v, %v", set, err)
	}
	resp, err := a.conn.Prompt(ctx, PromptRequest{SessionID: id, Prompt: []ContentBlock{TextBlock("second")}})
	if err != nil || resp.StopReason != StopEndTurn {
		t.Fatalf("prompt = %+v, %v", resp, err)
	}
	updates := a.rec.drain()
	if got := transcript(updates); !strings.HasSuffix(got, "agent: echo: second") {
		t.Errorf("turn transcript = %q", got)
	}
	// Commands are advertised after the load answer, so they may land in
	// either batch.
	kinds := map[string]bool{}
	for _, u := range append(replay, updates...) {
		kinds[u.UpdateKind()] = true
	}
	for _, want := range []string{KindAvailableCommandsUpdate, KindAgentThoughtChunk, KindToolCall, KindToolCallUpdate, KindAgentMessageChunk, KindUsageUpdate} {
		if !kinds[want] {
			t.Errorf("turn lacks %s update (got %v)", want, kinds)
		}
	}
	if err := a.conn.CallExtension(ctx, "_session/steering", PromptRequest{SessionID: id, Prompt: []ContentBlock{TextBlock("steer")}}, nil); err != nil {
		t.Errorf("steering: %v", err)
	}
	if err := a.conn.CloseSession(ctx, id); err != nil {
		t.Fatal(err)
	}
	if code := a.exit(t); code != 0 {
		t.Fatalf("ACP exit code %d", code)
	}

	// The TUI resumes what ACP wrote, by --resume and by -s.
	for _, args := range [][]string{{"--resume", id}, {"-s", id}} {
		tui := e.startTUI(t, args...)
		out := tui.waitFor(`"event":"session_start"`, 1)
		if ids := sessionIDs(out); len(ids) != 1 || ids[0] != id {
			t.Errorf("%v: session_start ids %v, want %s", args, ids, id)
		}
		for _, want := range []string{"> hello", "< echo: hello", "> second", "< echo: second", "> steer"} {
			if !strings.Contains(out, want) {
				t.Errorf("%v: transcript lacks %q:\n%q", args, want, out)
			}
		}
		if code := tui.exit(); code != 0 {
			t.Fatalf("TUI exit code %d", code)
		}
	}

	// -s creates a missing session that ACP then sees.
	tui = e.startTUI(t, "-s", "chosen-id")
	tui.waitFor(`"session_id":"chosen-id"`, 1)
	tui.exit()
	a = e.startACP(t, newRecorder())
	a.initialize(t)
	if _, err := a.conn.ResumeSession(ctx, ResumeSessionRequest{SessionID: "chosen-id", Cwd: e.cwd}); err != nil {
		t.Errorf("resume: %v", err)
	}
	if got := transcript(a.rec.drain()); got != "" {
		t.Errorf("resume replayed %q", got)
	}
	if _, err := a.conn.LoadSession(ctx, LoadSessionRequest{SessionID: "missing", Cwd: e.cwd}); !isRPCCode(err, CodeResourceNotFound) {
		t.Errorf("load missing = %v", err)
	}

	events := e.events(t)
	if strings.Contains(events, "violation") {
		t.Errorf("sequential writers reported a violation:\n%s", events)
	}
	for _, want := range []string{"tui open " + id, "tui close " + id, "acp open " + id, "acp close " + id, "acp call session/load"} {
		if !strings.Contains(events, want) {
			t.Errorf("events.log lacks %q:\n%s", want, events)
		}
	}
	if _, err := exec.Command(fakeAgentPath(t), "--resume", "nope", "--store", e.store).CombinedOutput(); err == nil {
		t.Error("--resume of a missing session succeeded")
	}
}

func isRPCCode(err error, code int) bool {
	var rpcErr *RPCError
	return errors.As(err, &rpcErr) && rpcErr.Code == code
}

func TestFakeAgentProfiles(t *testing.T) {
	tests := []struct {
		profile   string
		interrupt []string // key presses, written separately
		prefix    string
		eager     bool // session exists before the first message
	}{
		{"omp", []string{"\x1b"}, "", true},
		{"opencode", []string{"\x1b", "\x1b"}, "ses_", false},
	}
	for _, tt := range tests {
		t.Run(tt.profile, func(t *testing.T) {
			e := newFakeEnv(t, "FAKEAGENT_PROFILE="+tt.profile)
			tui := e.startTUI(t, "--turn-delay", "200ms")
			out := tui.waitFor("prompt> ", 1)
			if !strings.Contains(out, "app="+tt.profile) {
				t.Errorf("root record lacks app=%s", tt.profile)
			}
			if got := len(sessionIDs(out)) > 0; got != tt.eager {
				t.Errorf("session before first message = %v, want %v", got, tt.eager)
			}
			tui.send("one two three four five six\n")
			tui.waitFor("state=working", 1)
			if tt.profile == "opencode" {
				// Neither a single Esc nor Esc Esc in one write (read as
				// Alt+Esc) interrupts OpenCode.
				for _, keys := range []string{"\x1b", "\x1b\x1b"} {
					tui.send(keys)
					time.Sleep(20 * time.Millisecond)
					if strings.Contains(tui.out.String(), "[interrupted]") {
						t.Fatalf("%q interrupted the opencode profile", keys)
					}
				}
			}
			for i, key := range tt.interrupt {
				if i > 0 {
					time.Sleep(20 * time.Millisecond)
				}
				tui.send(key)
			}
			out = tui.waitFor("[interrupted]", 1)
			if strings.Contains(out, "six") {
				t.Error("turn ran to completion despite the interrupt")
			}
			ids := sessionIDs(out)
			if len(ids) != 1 || !strings.HasPrefix(ids[0], tt.prefix) {
				t.Errorf("session ids %v, want one with prefix %q", ids, tt.prefix)
			}
			if !strings.Contains(out, "state=idle:id="+ids[0]) {
				t.Error("no child record with the session id")
			}
			tui.exit()
		})
	}
}

func TestFakeAgentFaults(t *testing.T) {
	ctx := context.Background()
	t.Run("crash before initialize answer", func(t *testing.T) {
		a := newFakeEnv(t, "FAKEAGENT_CRASH=initialize").startACP(t, newRecorder())
		_, err := a.conn.Initialize(ctx, Implementation{Name: "t", Version: "0"}, nil)
		if !errors.Is(err, ErrDisconnected) {
			t.Fatalf("Initialize error = %v, want ErrDisconnected", err)
		}
		if code := a.wait(t); code != 3 {
			t.Errorf("exit code %d, want 3", code)
		}
	})
	t.Run("crash during load fails the pending call", func(t *testing.T) {
		e := newFakeEnv(t, "FAKEAGENT_CRASH=session/load")
		a := e.startACP(t, newRecorder())
		a.initialize(t)
		s, err := a.conn.NewSession(ctx, NewSessionRequest{Cwd: e.cwd})
		if err != nil {
			t.Fatal(err)
		}
		_, err = a.conn.LoadSession(ctx, LoadSessionRequest{SessionID: s.SessionID, Cwd: e.cwd})
		if !errors.Is(err, ErrDisconnected) {
			t.Fatalf("LoadSession error = %v, want ErrDisconnected", err)
		}
	})
	t.Run("crash mid turn", func(t *testing.T) {
		e := newFakeEnv(t, "FAKEAGENT_CRASH=prompt-mid")
		a := e.startACP(t, newRecorder())
		a.initialize(t)
		s, err := a.conn.NewSession(ctx, NewSessionRequest{Cwd: e.cwd})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := a.conn.Prompt(ctx, PromptRequest{SessionID: s.SessionID, Prompt: []ContentBlock{TextBlock("x")}}); !errors.Is(err, ErrDisconnected) {
			t.Fatalf("Prompt error = %v, want ErrDisconnected", err)
		}
	})
	t.Run("injected error", func(t *testing.T) {
		a := newFakeEnv(t, "FAKEAGENT_FAIL=session/new=-32000").startACP(t, newRecorder())
		a.initialize(t)
		if _, err := a.conn.NewSession(ctx, NewSessionRequest{Cwd: "/"}); !isRPCCode(err, CodeAuthRequired) {
			t.Fatalf("NewSession error = %v, want auth required", err)
		}
	})
	t.Run("delay and cancel request", func(t *testing.T) {
		e := newFakeEnv(t)
		a := e.startACP(t, newRecorder(), "--delay", "session/list=30s")
		a.initialize(t)
		cctx, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
		defer cancel()
		if _, err := a.conn.ListSessions(cctx, ListSessionsRequest{}); !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("ListSessions error = %v, want deadline exceeded", err)
		}
		// The agent stopped the delayed call; the connection still works.
		if _, err := a.conn.NewSession(ctx, NewSessionRequest{Cwd: e.cwd}); err != nil {
			t.Fatal(err)
		}
		if code := a.exit(t); code != 0 {
			t.Errorf("exit code %d", code)
		}
	})
	t.Run("unsupported protocol version", func(t *testing.T) {
		a := newFakeEnv(t, "FAKEAGENT_PROTOCOL_VERSION=2").startACP(t, newRecorder())
		if _, err := a.conn.Initialize(ctx, Implementation{Name: "t", Version: "0"}, nil); !errors.Is(err, ErrUnsupportedVersion) {
			t.Fatalf("Initialize error = %v, want ErrUnsupportedVersion", err)
		}
	})
	t.Run("stdout noise", func(t *testing.T) {
		a := newFakeEnv(t, "FAKEAGENT_STDOUT_NOISE=welcome to bash").startACP(t, newRecorder())
		if _, err := a.conn.Initialize(ctx, Implementation{Name: "t", Version: "0"}, nil); !errors.Is(err, ErrProtocol) {
			t.Fatalf("Initialize error = %v, want ErrProtocol", err)
		}
	})
}

func TestFakeAgentRequests(t *testing.T) {
	ctx := context.Background()
	e := newFakeEnv(t, "FAKEAGENT_PERMISSION=1", "FAKEAGENT_ELICIT=1")
	r := newRecorder()
	permissions := make(chan RequestPermissionRequest, 1)
	answer := make(chan PermissionOutcome, 1)
	r.permission = func(ctx context.Context, req RequestPermissionRequest) (RequestPermissionResponse, error) {
		permissions <- req
		return RequestPermissionResponse{Outcome: <-answer}, nil
	}
	r.elicit = func(ctx context.Context, req CreateElicitationRequest) (CreateElicitationResponse, error) {
		if req.RequestedSchema == nil || req.RequestedSchema.Properties["name"].Type != "string" {
			return CreateElicitationResponse{Action: ElicitDecline}, nil
		}
		return CreateElicitationResponse{Action: ElicitAccept, Content: map[string]any{"name": "Ann"}}, nil
	}
	a := e.startACP(t, r)
	a.initialize(t)
	s, err := a.conn.NewSession(ctx, NewSessionRequest{Cwd: e.cwd})
	if err != nil {
		t.Fatal(err)
	}
	answer <- PermissionOutcome{Outcome: OutcomeSelected, OptionID: "allow"}
	resp, err := a.conn.Prompt(ctx, PromptRequest{SessionID: s.SessionID, Prompt: []ContentBlock{TextBlock("hi /plan /notice /compact /title /unknown /fs")}})
	if err != nil || resp.StopReason != StopEndTurn {
		t.Fatalf("prompt = %+v, %v", resp, err)
	}
	if req := <-permissions; req.ToolCall.ToolCallID != "call_1" || len(req.Options) != 2 {
		t.Errorf("permission request = %+v", req)
	}
	updates := r.drain()
	if got := transcript(updates); !strings.Contains(got, "(fs error -32601)") || !strings.Contains(got, "(hello Ann)") {
		t.Errorf("transcript = %q", got)
	}
	kinds := map[string]int{}
	for _, u := range updates {
		kinds[u.UpdateKind()]++
	}
	for kind, n := range map[string]int{KindPlan: 1, KindNotice: 1, KindCompactionUpdate: 2, KindSessionInfoUpdate: 1, "future_kind": 1} {
		if kinds[kind] != n {
			t.Errorf("%d %s updates, want %d (%v)", kinds[kind], kind, n, kinds)
		}
	}

	// Stopping a turn while permission is pending: session/cancel, then the
	// client answers the request with cancelled and the turn ends cancelled.
	pctx, cancel := context.WithCancel(ctx)
	turn := async(func() (PromptResponse, error) {
		return a.conn.Prompt(pctx, PromptRequest{SessionID: s.SessionID, Prompt: []ContentBlock{TextBlock("again")}})
	})
	<-permissions
	cancel()
	answer <- PermissionOutcome{Outcome: OutcomeCancelled}
	if resp, err := await(t, turn); err != nil || resp.StopReason != StopCancelled {
		t.Fatalf("cancelled prompt = %+v, %v", resp, err)
	}
	if events := e.events(t); !strings.Contains(events, "permission allow") || !strings.Contains(events, "permission cancelled") {
		t.Errorf("events.log:\n%s", events)
	}
}

func TestFakeAgentReportsConcurrentWriters(t *testing.T) {
	e := newFakeEnv(t)
	tui := e.startTUI(t)
	id := sessionIDs(tui.waitFor(`"event":"session_start"`, 1))[0]
	// Like the real TUIs, the session is only on disk after its first message.
	tui.send("hello\n")
	tui.waitFor(stopEvent, 1)
	a := e.startACP(t, newRecorder())
	a.initialize(t)
	if _, err := a.conn.LoadSession(context.Background(), LoadSessionRequest{SessionID: id, Cwd: e.cwd}); err != nil {
		t.Fatal(err)
	}
	if events := e.events(t); !strings.Contains(events, "acp violation "+id) || !strings.Contains(events, "acp fork ") {
		t.Errorf("events.log:\n%s", events)
	}
	tui.exit()
}
