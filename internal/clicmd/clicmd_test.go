//go:build linux || darwin

package clicmd

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/rainchan/webmux/internal/attach"
	"github.com/rainchan/webmux/internal/contract"
	"github.com/rainchan/webmux/internal/localclient"

	"github.com/coder/websocket"
)

const sessionsJSON = `[{"id":"abcdefgh","title":"review","status":"running","cmd":["codex"],"cwd":"/w","exitCode":null,"state":"active","views":["terminal"],"view":"terminal","transition":null,"agent":null},` +
	`{"id":"hybrid22","title":"chat","status":"idle","cmd":["omp"],"cwd":"/w","exitCode":null,"state":"active","views":["terminal","chat"],"view":"chat","transition":null,"agent":{"name":"omp","title":"omp","icon":"pi","sessionId":"01a"}},` +
	`{"id":"hybrid33","title":"moving","status":"running","cmd":["omp"],"cwd":"/w","exitCode":null,"state":"active","views":["terminal","chat"],"view":"chat","transition":{"to":"terminal","phase":"stopping"},"agent":{"name":"omp","title":"omp","icon":"pi","sessionId":null}}]`

const archivedJSON = `[{"id":"old00001","title":"yesterday","status":"exited","cmd":["omp"],"cwd":"/w","exitCode":null,"state":"interrupted","views":["terminal","chat"],"view":"chat","transition":null,"agent":{"name":"omp","title":"omp","icon":"pi","sessionId":"01b"}}]`

type fakeServer struct {
	mu          sync.Mutex
	bodies      []map[string]any
	inputBodies []map[string]any
	queries     []string
	rejectEnv   bool
	inputStatus int
	inputBody   string
}

func (f *fakeServer) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/sessions", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		// The request ID is fresh per run; tests compare the rest.
		if id, _ := body["clientRequestId"].(string); len(id) < 8 {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = io.WriteString(w, `{"error":"no clientRequestId","step":"precheck"}`)
			return
		}
		delete(body, "clientRequestId")
		f.mu.Lock()
		f.bodies = append(f.bodies, body)
		reject := f.rejectEnv
		f.mu.Unlock()
		if _, ok := body["env"]; ok && reject {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = io.WriteString(w, `{"error":"json: unknown field \"env\""}`)
			return
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = io.WriteString(w, `{"session":{"id":"newsess1","status":"running"},"configWarnings":["model: unknown value"]}`)
	})
	mux.HandleFunc("GET /api/sessions", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		f.queries = append(f.queries, r.URL.RawQuery)
		f.mu.Unlock()
		if states := r.URL.Query()["state"]; len(states) > 0 && !slices.Contains(states, "active") {
			_, _ = io.WriteString(w, archivedJSON)
			return
		}
		_, _ = io.WriteString(w, sessionsJSON)
	})
	mux.HandleFunc("GET /api/launch-configs", func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `[{"id":"cfgrevw1","name":"review","cmd":["claude"]},{"id":"cfgomp01","name":"omp","cmd":["omp"]},{"id":"cfgagnt1","name":"agent","cmd":["codex"],"chat":true}]`)
	})
	mux.HandleFunc("GET /api/repositories", func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `[{"id":"repo0001","name":"app","anchorPath":"/w/app","worktrees":[{"id":"main","path":"/w/app","kind":"main"}]}]`)
	})
	mux.HandleFunc("GET /api/sessions/{id}", func(w http.ResponseWriter, r *http.Request) {
		if r.PathValue("id") != "abcdefgh" {
			w.WriteHeader(http.StatusNotFound)
			_, _ = io.WriteString(w, `{"error":"session not found"}`)
			return
		}
		_, _ = io.WriteString(w, `{"id":"abcdefgh"}`)
	})
	mux.HandleFunc("POST /api/sessions/{id}/input", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		f.mu.Lock()
		f.inputBodies = append(f.inputBodies, body)
		status, response := f.inputStatus, f.inputBody
		f.mu.Unlock()
		if status == 0 {
			status = http.StatusOK
		}
		if response == "" {
			response = `{"view":"chat"}`
		}
		w.WriteHeader(status)
		_, _ = io.WriteString(w, response)
	})
	// Hybrid sessions: one on its chat side refuses the terminal at once,
	// the other moves away after the replay. Other IDs have no endpoint.
	mux.HandleFunc("GET /ws/s/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if id != "chatside" && id != "movedawy" {
			http.NotFound(w, r)
			return
		}
		c, err := websocket.Accept(w, r, &websocket.AcceptOptions{Subprotocols: []string{"webmux"}})
		if err != nil {
			return
		}
		if id == "movedawy" {
			_ = c.Write(r.Context(), websocket.MessageBinary, []byte{0})
		}
		_ = c.Close(contract.CloseMoved, "session moved: chat")
	})
	mux.HandleFunc("GET "+contract.PathStatus, func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(contract.StatusV1{V: 1, URLs: contract.URLs{Local: "http://127.0.0.1:7681"}})
	})
	return mux
}

func env(t *testing.T, f *fakeServer) (Env, *bytes.Buffer, *bytes.Buffer, *[]string) {
	t.Helper()
	dir, err := os.MkdirTemp("/tmp", "scm")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	socket := filepath.Join(dir, "s.sock")
	if f != nil {
		l, err := net.Listen("unix", socket)
		if err != nil {
			t.Fatal(err)
		}
		srv := &http.Server{Handler: f.handler()}
		go func() { _ = srv.Serve(l) }()
		t.Cleanup(func() { _ = srv.Close() })
	}
	var stdout, stderr bytes.Buffer
	opened := &[]string{}
	return Env{
		Connect:        func() (*localclient.Client, error) { return localclient.New(localclient.UnixDialer{Path: socket}), nil },
		Stdout:         &stdout,
		Stderr:         &stderr,
		OpenConsole:    func() (Console, error) { return &fakeConsole{}, nil },
		OpenBrowser:    func(url string) error { *opened = append(*opened, url); return nil },
		ResolveCwd:     func(_ context.Context, cwd string) (string, error) { return "/mnt/c/work/" + cwd, nil },
		NotRunningHint: "start it",
	}, &stdout, &stderr, opened
}

func TestRunSendsArgvCwdEnvAndDetaches(t *testing.T) {
	f := &fakeServer{}
	e, stdout, _, _ := env(t, f)
	code := Run(context.Background(), e, []string{"--detach", "--name", "review", "--env", "A=1", "--env", "B=x=y", "--cwd", "app", "--", "printf", "%s;$(id)", "a b"})
	if code != 0 || stdout.String() != "newsess1\n" {
		t.Fatalf("detach %d %q", code, stdout.String())
	}
	want := map[string]any{"cmd": []any{"printf", "%s;$(id)", "a b"}, "cwd": "/mnt/c/work/app", "name": "review", "env": map[string]any{"A": "1", "B": "x=y"}}
	if !reflect.DeepEqual(f.bodies[0], want) {
		t.Fatalf("request %#v", f.bodies[0])
	}
	if code := Run(context.Background(), e, []string{"--detach"}); code != contract.ExitUsage {
		t.Fatalf("missing command exit %d", code)
	}
	if code := Run(context.Background(), e, []string{"--env", "novalue", "--", "true"}); code != contract.ExitUsage {
		t.Fatalf("bad --env exit %d", code)
	}
}

// TestRunRejectsOldAndInvalidCombinations covers what run refuses before
// any request: a word before --, the removed --start-side, and the chat
// view's options without --view chat.
func TestRunRejectsOldAndInvalidCombinations(t *testing.T) {
	f := &fakeServer{}
	e, _, stderr, _ := env(t, f)
	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"--detach", "make", "test"}, "after --"},
		{[]string{"--detach", "--new-worktree", "fix/lint", "--", "true"}, "--branch"},
		{[]string{"--detach", "--start-side", "chat", "--launch-config", "omp"}, "start-side"},
		{[]string{"--detach", "--launch-config", "omp", "--permission", "auto"}, "--view chat"},
		{[]string{"--detach", "--launch-config", "omp", "--set", "model=x"}, "--view chat"},
		{[]string{"--launch-config", "omp", "--view", "chat", "--prompt", "hi", "--reply", "--wait", "idle"}, "cannot be combined"},
		{[]string{"--launch-config", "omp", "--view", "chat", "--reply"}, "--prompt"},
		{[]string{"--launch-config", "omp", "--view", "chat", "--attach-file", "x.png"}, "--prompt"},
		{[]string{"--attach", "--launch-config", "omp", "--view", "chat"}, "cannot be attached"},
		{[]string{"--view", "chat", "--", "omp"}, "--launch-config"},
		{[]string{"--detach", "--branch", "x", "--", "true"}, "--new-worktree"},
		{[]string{"--detach", "--new-worktree", "--existing-branch", "--", "true"}, "--branch"},
	} {
		stderr.Reset()
		if code := Run(context.Background(), e, tc.args); code != contract.ExitUsage || !strings.Contains(stderr.String(), tc.want) {
			t.Errorf("%v: exit %d, stderr %q, want %q", tc.args, code, stderr.String(), tc.want)
		}
	}
	if len(f.bodies) != 0 {
		t.Fatalf("refused runs sent %d requests", len(f.bodies))
	}
}

// TestSendQueuedPromptNotices covers the script contract: a queued prompt
// explains itself on standard error, --json keeps it quiet, refuse turns a
// busy chat into exit 1, and now reaches the server as one request.
func TestSendQueuedPromptNotices(t *testing.T) {
	f := &fakeServer{inputBody: `{"view":"chat","queued":true,"itemId":"q1","position":2}`}
	e, stdout, stderr, _ := env(t, f)
	ctx := context.Background()
	send := func(args ...string) int {
		t.Helper()
		code, ok := e.Dispatch(ctx, Commands(), append([]string{"send"}, args...))
		if !ok {
			t.Fatalf("send is not a command")
		}
		return code
	}
	if code := send("hybrid22", "later"); code != contract.ExitOK {
		t.Fatalf("queued send exit %d: %s", code, stderr.String())
	}
	if stdout.Len() != 0 {
		t.Fatalf("queued send wrote standard output: %q", stdout.String())
	}
	want := "webmux: queued as q1 (position 2); the chat is busy. Use --mode steer to inject it now, or --mode now to cancel the turn\n"
	if stderr.String() != want {
		t.Fatalf("queued notice %q, want %q", stderr.String(), want)
	}

	stdout.Reset()
	stderr.Reset()
	if code := send("hybrid22", "later", "--json"); code != contract.ExitOK {
		t.Fatalf("json send exit %d: %s", code, stderr.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("--json wrote a notice: %q", stderr.String())
	}
	var result contract.SessionInputResult
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil || result.Position != 2 {
		t.Fatalf("json result %q: %v", stdout.String(), err)
	}

	f.mu.Lock()
	f.inputStatus, f.inputBody = http.StatusConflict, `{"error":"会话忙，消息未发送","reason":"busy"}`
	f.mu.Unlock()
	stdout.Reset()
	stderr.Reset()
	if code := send("hybrid22", "later", "--mode", "refuse"); code != contract.ExitError {
		t.Fatalf("refused send exit %d: %s", code, stderr.String())
	}
	if stdout.Len() != 0 || !strings.Contains(stderr.String(), "busy") {
		t.Fatalf("refused send stdout %q stderr %q", stdout.String(), stderr.String())
	}

	f.mu.Lock()
	f.inputStatus, f.inputBody = http.StatusOK, `{"view":"chat","queued":true,"itemId":"q2","position":1}`
	f.mu.Unlock()
	stdout.Reset()
	stderr.Reset()
	if code := send("hybrid22", "later", "--mode", "now"); code != contract.ExitOK {
		t.Fatalf("now send exit %d: %s", code, stderr.String())
	}
	f.mu.Lock()
	last := f.inputBodies[len(f.inputBodies)-1]
	f.mu.Unlock()
	if last["mode"] != "now" {
		t.Fatalf("now sent mode %v", last["mode"])
	}
	if !strings.Contains(stderr.String(), "cancelling the running turn") {
		t.Fatalf("now notice %q", stderr.String())
	}
	if code := send("hybrid22", "later", "--mode", "shout"); code != contract.ExitUsage {
		t.Fatalf("unknown mode exit %d", code)
	}
}

// TestRunNewWorktreeOnTheChatView puts the location, view, settings and
// first prompt into one create request.
func TestRunNewWorktreeOnTheChatView(t *testing.T) {
	f := &fakeServer{}
	e, stdout, stderr, _ := env(t, f)
	args := []string{"--new-worktree", "--repo", "repo0001", "--launch-config", "agent", "--view", "chat", "--prompt", "go", "--set", "model=fast", "--permission", "ask"}
	if code := Run(context.Background(), e, append([]string{"--dry-run", "--branch", "feat/x", "--base", "main"}, args...)); code != 0 {
		t.Fatalf("dry run: %d %q", code, stderr)
	}
	var dry struct {
		Request map[string]any `json:"request"`
		Pin     bool           `json:"pin"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &dry); err != nil || !dry.Pin {
		t.Fatalf("dry run output %q: %v", stdout, err)
	}
	want := map[string]any{
		"newWorktree": map[string]any{"repositoryId": "repo0001", "mode": "new", "branch": "feat/x", "base": "main"},
		"configId":    "cfgagnt1", "view": "chat", "prompt": "go", "agentConfig": map[string]any{"model": "fast"}, "permissionMode": "ask",
	}
	if !reflect.DeepEqual(dry.Request, want) {
		t.Fatalf("dry run request %#v, want %#v", dry.Request, want)
	}
	if len(f.bodies) != 0 {
		t.Fatal("--dry-run created a session")
	}
	// The chat view implies --detach; the server picks the branch name.
	stdout.Reset()
	if code := Run(context.Background(), e, append([]string{"--no-pin"}, args...)); code != 0 || stdout.String() != "newsess1\n" {
		t.Fatalf("run: %d %q %q", code, stdout, stderr)
	}
	if got := f.bodies[0]["newWorktree"]; !reflect.DeepEqual(got, map[string]any{"repositoryId": "repo0001", "mode": "new"}) {
		t.Fatalf("newWorktree %#v", got)
	}
	// A config without the chat view cannot start on it.
	stderr.Reset()
	if code := Run(context.Background(), e, []string{"--launch-config", "omp", "--view", "chat"}); code != contract.ExitUsage || !strings.Contains(stderr.String(), "chat view") {
		t.Fatalf("--view chat on omp: %d %q", code, stderr)
	}
}

func TestRunInheritsCallerEnvironment(t *testing.T) {
	f := &fakeServer{}
	e, _, stderr, _ := env(t, f)
	e.Environ = func() []string {
		return []string{
			"HOME=/home/u", "PATH=/opt/bin:/usr/bin", "API_URL=old", "EMPTY=",
			// Server-owned terminal identity and the reserved session marker.
			"TERM=tmux-256color", "TERM_PROGRAM=tmux", "TMUX=/tmp/tmux", "KITTY_PID=1", "WEBMUX_SESSION_ID=parent01",
			// Caller-only and names a create request rejects.
			"PWD=/elsewhere", "OLDPWD=/", "BASH_FUNC_f%%=() { :; }", "=C:=C:\\", "NOEQUALS",
		}
	}
	if code := Run(context.Background(), e, []string{"--detach", "--env", "API_URL=new", "--env", "TERM=dumb", "--", "true"}); code != 0 {
		t.Fatalf("run exit %d stderr=%q", code, stderr.String())
	}
	want := map[string]any{"HOME": "/home/u", "PATH": "/opt/bin:/usr/bin", "API_URL": "new", "EMPTY": "", "TERM": "dumb"}
	if got := f.bodies[0]["env"]; !reflect.DeepEqual(got, want) {
		t.Fatalf("env %#v, want %#v", got, want)
	}
	// A launch config keeps its saved environment: nothing is inherited.
	if code := Run(context.Background(), e, []string{"--detach", "--launch-config", "review"}); code != 0 {
		t.Fatalf("launch config exit %d stderr=%q", code, stderr.String())
	}
	if _, sent := f.bodies[1]["env"]; sent {
		t.Fatalf("launch config request sent env: %#v", f.bodies[1])
	}
}

func TestRunUsesSavedWebLaunchConfig(t *testing.T) {
	f := &fakeServer{}
	e, stdout, stderr, _ := env(t, f)
	code := Run(context.Background(), e, []string{"--detach", "--launch-config", "review", "--name", "review", "--cwd", "app"})
	if code != 0 || stdout.String() != "newsess1\n" || !strings.Contains(stderr.String(), "model: unknown value") {
		t.Fatalf("saved launch config %d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	want := map[string]any{"configId": "cfgrevw1", "cwd": "/mnt/c/work/app", "name": "review"}
	if !reflect.DeepEqual(f.bodies[0], want) {
		t.Fatalf("saved launch config request %#v, want %#v", f.bodies[0], want)
	}

	stdout.Reset()
	code = Run(context.Background(), e, []string{"--detach", "--config-id=omp"})
	if code != 0 || stdout.String() != "newsess1\n" {
		t.Fatalf("config-id alias %d stdout=%q", code, stdout.String())
	}
	if got := f.bodies[1]["configId"]; got != "cfgomp01" {
		t.Fatalf("config-id alias request %#v", f.bodies[1])
	}

	// An ID is not a name; unknown names list the saved ones.
	stderr.Reset()
	if code := Run(context.Background(), e, []string{"--detach", "--launch-config", "cfgomp01"}); code != contract.ExitUsage || !strings.Contains(stderr.String(), `"review", "omp"`) {
		t.Fatalf("unknown name %d stderr=%q", code, stderr.String())
	}

	for _, args := range [][]string{
		{"--detach", "--launch-config", "review", "--", "claude"},
		{"--detach", "--launch-config", "review", "--env", "X=1"},
		{"--detach", "--launch-config="},
		{"--detach", "--launch-config", "review", "--config-id", "omp"},
	} {
		if code := Run(context.Background(), e, args); code != contract.ExitUsage {
			t.Errorf("invalid saved launch config args %v: exit %d", args, code)
		}
	}
	if len(f.bodies) != 2 {
		t.Fatalf("invalid launch config arguments created sessions: %d", len(f.bodies))
	}
}

func TestRunReportsUnsupportedEnvAndStoppedServer(t *testing.T) {
	f := &fakeServer{rejectEnv: true}
	e, _, stderr, _ := env(t, f)
	if code := Run(context.Background(), e, []string{"--detach", "--env", "A=1", "--", "true"}); code != 1 || !strings.Contains(stderr.String(), `unknown field \"env\"`) && !strings.Contains(stderr.String(), `unknown field "env"`) {
		t.Fatalf("unsupported env %d %q", code, stderr.String())
	}
	stopped, _, stderr, _ := env(t, nil)
	if code := Run(context.Background(), stopped, []string{"--", "claude"}); code != contract.ExitNotRunning || !strings.Contains(stderr.String(), "not running; start it") {
		t.Fatalf("stopped server %d %q", code, stderr.String())
	}
	if code := List(context.Background(), stopped, nil); code != contract.ExitNotRunning {
		t.Fatalf("ls on stopped server %d", code)
	}
}

func TestListAndOpen(t *testing.T) {
	f := &fakeServer{}
	e, stdout, _, opened := env(t, f)
	if code := List(context.Background(), e, []string{"--json"}); code != 0 || strings.TrimSpace(stdout.String()) != sessionsJSON {
		t.Fatalf("ls --json %d %q", code, stdout.String())
	}
	stdout.Reset()
	if code := List(context.Background(), e, nil); code != 0 {
		t.Fatalf("ls %d %q", code, stdout.String())
	}
	// Sessions with a chat view name the view they are on; ordinary rows
	// stay unchanged.
	rows := map[string]string{}
	for _, line := range strings.Split(stdout.String(), "\n") {
		if id, rest, ok := strings.Cut(line, "  "); ok {
			rows[id] = strings.Join(strings.Fields(rest), " ")
		}
	}
	for id, want := range map[string]string{
		"abcdefgh": "review running codex /w",
		"hybrid22": "chat idle · 聊天 omp /w",
		"hybrid33": "moving running · 切换中 omp /w",
	} {
		if rows[id] != want {
			t.Fatalf("ls row %s = %q, want %q", id, rows[id], want)
		}
	}
	// --archived asks the server; archived rows show their state.
	stdout.Reset()
	if code := List(context.Background(), e, []string{"--archived"}); code != 0 || !strings.Contains(stdout.String(), "interrupted · 聊天") {
		t.Fatalf("ls --archived %d %q", code, stdout.String())
	}
	if q := f.queries[len(f.queries)-1]; q != "state=archived" {
		t.Fatalf("ls --archived query %q", q)
	}
	stdout.Reset()
	if code := List(context.Background(), e, []string{"--state", "closed", "--state", "interrupted", "--json"}); code != 0 || strings.TrimSpace(stdout.String()) != archivedJSON {
		t.Fatalf("ls --state --json %d %q", code, stdout.String())
	}
	if q := f.queries[len(f.queries)-1]; q != "state=closed&state=interrupted" {
		t.Fatalf("ls --state query %q", q)
	}
	for _, args := range [][]string{{"--state", "gone"}, {"--archived", "--state", "all"}} {
		if code := List(context.Background(), e, args); code != contract.ExitUsage {
			t.Errorf("ls %v: exit %d", args, code)
		}
	}
	stdout.Reset()
	if code := Open(context.Background(), e, []string{"--browser", "abcdefgh"}); code != 0 || stdout.String() != "http://127.0.0.1:7681/s/abcdefgh\n" || len(*opened) != 1 {
		t.Fatalf("open %d %q %v", code, stdout.String(), *opened)
	}
	if code := Open(context.Background(), e, []string{"missing"}); code != 1 {
		t.Fatalf("open missing %d", code)
	}
}

// fakeConsole is a terminal whose input is already closed.
type fakeConsole struct{ restored bool }

func (c *fakeConsole) ReadContext(context.Context, []byte) (int, error) { return 0, io.EOF }
func (c *fakeConsole) Write(p []byte) (int, error)                      { return len(p), nil }
func (c *fakeConsole) Size() (int, int, error)                          { return 80, 24, nil }
func (c *fakeConsole) Restore() error                                   { c.restored = true; return nil }

func TestAttachedRunAnnouncesSessionAndNeedsTerminal(t *testing.T) {
	f := &fakeServer{}
	e, stdout, stderr, _ := env(t, f)
	console := &fakeConsole{}
	e.OpenConsole = func() (Console, error) { return console, nil }
	// The fake server has no WebSocket endpoint, so attaching then fails.
	code := Run(context.Background(), e, []string{"--", "claude"})
	if code != 1 || stdout.Len() != 0 || !console.restored {
		t.Fatalf("attached run %d stdout %q restored %v", code, stdout.String(), console.restored)
	}
	if !strings.Contains(stderr.String(), "webmux run: model: unknown value\r\nwebmux: session newsess1 · Ctrl+] to detach\r\n") {
		t.Fatalf("announcement %q", stderr.String())
	}
	// Without a terminal run detaches instead; --attach insists and fails
	// before anything is created.
	e.OpenConsole = func() (Console, error) { return nil, attach.ErrNotTerminal }
	stdout.Reset()
	stderr.Reset()
	if code := Run(context.Background(), e, []string{"--", "claude"}); code != 0 || len(f.bodies) != 2 ||
		stdout.String() != "newsess1\n" || !strings.Contains(stderr.String(), "detaching") {
		t.Fatalf("no terminal: exit %d, %d sessions, stdout %q stderr %q", code, len(f.bodies), stdout, stderr)
	}
	if code := Run(context.Background(), e, []string{"--attach", "--", "claude"}); code != contract.ExitUsage || len(f.bodies) != 2 {
		t.Fatalf("--attach without a terminal: exit %d, %d sessions created", code, len(f.bodies))
	}
}

// heldConsole is a terminal whose input stays open until attach ends.
type heldConsole struct{ fakeConsole }

func (*heldConsole) ReadContext(ctx context.Context, _ []byte) (int, error) {
	<-ctx.Done()
	return 0, ctx.Err()
}

func TestAttachHybridSessionOnChatSide(t *testing.T) {
	e, _, stderr, _ := env(t, &fakeServer{})
	console := &heldConsole{}
	e.OpenConsole = func() (Console, error) { return console, nil }
	if code := Attach(context.Background(), e, []string{"chatside"}); code != contract.ExitError || !console.restored {
		t.Fatalf("refused attach exit %d restored %v", code, console.restored)
	}
	if !strings.Contains(stderr.String(), "会话在聊天视图") {
		t.Fatalf("refusal message %q", stderr.String())
	}
	stderr.Reset()
	if code := Attach(context.Background(), e, []string{"movedawy"}); code != contract.ExitOK {
		t.Fatalf("passive detach exit %d: %s", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "已切换到聊天视图") || !strings.Contains(stderr.String(), "webmux attach movedawy") {
		t.Fatalf("passive detach message %q", stderr.String())
	}
}

func TestConnectUsageErrorExitsTwo(t *testing.T) {
	var stderr bytes.Buffer
	e := Env{Stderr: &stderr, Stdout: io.Discard, Connect: func() (*localclient.Client, error) {
		return nil, UsageError("--socket and --config cannot be combined")
	}}
	if code := e.Run(context.Background(), listCommand, nil); code != contract.ExitUsage || !strings.Contains(stderr.String(), "cannot be combined") {
		t.Fatalf("exit %d, stderr %q", code, stderr.String())
	}
}
