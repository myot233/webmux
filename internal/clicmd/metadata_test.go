//go:build linux || darwin

package clicmd

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/rainchan/webmux/internal/contract"
	"github.com/rainchan/webmux/internal/localclient"
)

const detailsJSON = `{"session":{"id":"abcdefgh","title":"review","status":"running","cmd":["codex"],"cwd":"/w","exitCode":null,` +
	`"listeningPorts":[{"port":5173}],"labels":{"run":"42"},"parentId":"parent12"},` +
	`"link":"http://127.0.0.1:7681/s/abcdefgh","repository":{"id":"repo1","name":"app","path":"/w"},` +
	`"worktree":{"id":"wt1","branch":"fix/lint","path":"/w","main":false},"launchConfig":{"id":"cfg1","name":"Codex","hybrid":true},` +
	`"hybrid":null,"artifactsDir":null,"forwards":[],"parentId":"parent12","labels":{"run":"42"},"children":["child123"]}`

// metaServer answers the endpoints of the metadata commands; old makes the
// details and labels routes unknown, like a server that predates them.
type metaServer struct {
	mu     sync.Mutex
	old    bool
	labels []contract.LabelsUpdate
}

func (m *metaServer) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/sessions/{id}/details", func(w http.ResponseWriter, r *http.Request) {
		if m.old {
			http.NotFound(w, r)
			return
		}
		if r.PathValue("id") != "abcdefgh" {
			w.WriteHeader(http.StatusNotFound)
			_, _ = io.WriteString(w, `{"error":"session not found"}`)
			return
		}
		_, _ = io.WriteString(w, detailsJSON)
	})
	mux.HandleFunc("PATCH /api/sessions/{id}/labels", func(w http.ResponseWriter, r *http.Request) {
		var update contract.LabelsUpdate
		_ = json.NewDecoder(r.Body).Decode(&update)
		m.mu.Lock()
		m.labels = append(m.labels, update)
		m.mu.Unlock()
		_, _ = io.WriteString(w, `{"id":"abcdefgh","labels":{"run":"42","task":"lint"}}`)
	})
	mux.HandleFunc("GET /api/sessions", func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `[`+
			`{"id":"aaaaaaaa","title":"a","status":"running","cmd":["sh"],"cwd":"/w","exitCode":null,"parentId":"parent12","labels":{"run":"42"}},`+
			`{"id":"bbbbbbbb","title":"b","status":"exited","cmd":["sh"],"cwd":"/w","exitCode":0,"labels":{"run":"42","x":"1"}},`+
			`{"id":"cccccccc","title":"c","status":"idle","cmd":["sh"],"cwd":"/w","exitCode":null}]`)
	})
	mux.HandleFunc("GET "+contract.PathStatus, func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(contract.StatusV1{V: 1, Version: "0.9.0", URLs: contract.URLs{Local: "http://127.0.0.1:7681"}})
	})
	return mux
}

func metaEnv(t *testing.T, m *metaServer, session string) (Env, *bytes.Buffer, *bytes.Buffer) {
	t.Helper()
	dir, err := os.MkdirTemp("/tmp", "scm")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	socket := filepath.Join(dir, "s.sock")
	l, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatal(err)
	}
	srv := &http.Server{Handler: m.handler()}
	go func() { _ = srv.Serve(l) }()
	t.Cleanup(func() { _ = srv.Close() })
	var stdout, stderr bytes.Buffer
	return Env{
		Connect: func() (*localclient.Client, error) { return localclient.New(localclient.UnixDialer{Path: socket}), nil },
		Stdout:  &stdout, Stderr: &stderr, Version: "1.0.0",
		Getenv: func(name string) string {
			if name == contract.SessionIDEnv {
				return session
			}
			return ""
		},
		NotRunningHint: "start it",
	}, &stdout, &stderr
}

func run(t *testing.T, e Env, args ...string) int {
	t.Helper()
	code, ok := e.Dispatch(context.Background(), Commands(), args)
	if !ok {
		t.Fatalf("%v: no such command", args)
	}
	return code
}

func TestGetDefaultsToSelfAndPrintsFields(t *testing.T) {
	m := &metaServer{}
	e, stdout, stderr := metaEnv(t, m, "abcdefgh")
	if code := run(t, e, "get"); code != 0 {
		t.Fatalf("get: %d %s", code, stderr)
	}
	for _, want := range []string{"ID:", "abcdefgh", "fix/lint · /w", "Codex", "parent12", "child123", "run=42", "5173"} {
		if !strings.Contains(stdout.String(), want) {
			t.Errorf("get output lacks %q:\n%s", want, stdout)
		}
	}
	if strings.Contains(stdout.String(), "Link:") || strings.Contains(stdout.String(), "http://") {
		t.Errorf("get output mentions URL:\n%s", stdout)
	}
	stdout.Reset()
	if code := run(t, e, "get", "--field", "worktree.branch"); code != 0 || stdout.String() != "fix/lint\n" {
		t.Fatalf("--field: %d %q", code, stdout)
	}
	stdout.Reset()
	if code := run(t, e, "get", "--field", "children"); code != 0 || stdout.String() != "[\"child123\"]\n" {
		t.Fatalf("--field array: %d %q", code, stdout)
	}
	stdout.Reset()
	if code := run(t, e, "get", "--field", "hybrid"); code != contract.ExitError || stdout.String() != "\n" {
		t.Fatalf("--field null: %d %q", code, stdout)
	}
	stdout.Reset()
	// Flags may follow the session argument.
	if code := run(t, e, "get", "self", "--json"); code != 0 || !json.Valid(stdout.Bytes()) {
		t.Fatalf("get self --json: %d %q", code, stdout)
	}
	if code := run(t, e, "get", "--json", "--field", "x"); code != contract.ExitUsage {
		t.Fatalf("--json with --field: %d", code)
	}
	if code := run(t, e, "get", "missing1"); code != contract.ExitError || !strings.Contains(stderr.String(), "session not found") {
		t.Fatalf("missing: %d %q", code, stderr)
	}
}

func TestGetOutsideSessionNeedsID(t *testing.T) {
	e, stdout, stderr := metaEnv(t, &metaServer{}, "")
	if code := run(t, e, "get"); code != contract.ExitUsage || stdout.Len() != 0 || !strings.Contains(stderr.String(), "--help") {
		t.Fatalf("get outside: %d %q %q", code, stdout, stderr)
	}
	stderr.Reset()
	if code := run(t, e, "get", "self"); code != contract.ExitUsage || !strings.Contains(stderr.String(), "WEBMUX_SESSION_ID") {
		t.Fatalf("self outside: %d %q", code, stderr)
	}
}

func TestOldServerIsReported(t *testing.T) {
	e, _, stderr := metaEnv(t, &metaServer{old: true}, "abcdefgh")
	if code := run(t, e, "get"); code != contract.ExitError {
		t.Fatalf("exit %d", code)
	}
	for _, want := range []string{"too old", "server 0.9.0", "CLI 1.0.0", "WEBMUX_BIN"} {
		if !strings.Contains(stderr.String(), want) {
			t.Errorf("stderr lacks %q: %s", want, stderr)
		}
	}
}

func TestLabelSetsAndRemoves(t *testing.T) {
	m := &metaServer{}
	e, stdout, _ := metaEnv(t, m, "abcdefgh")
	if code := run(t, e, "label", "self", "task=lint", "old-", "url=a=b"); code != 0 {
		t.Fatalf("label: %d", code)
	}
	got := m.labels[0]
	if got.Set["task"] != "lint" || got.Set["url"] != "a=b" || len(got.Remove) != 1 || got.Remove[0] != "old" {
		t.Fatalf("request %+v", got)
	}
	if stdout.String() != "run=42\ntask=lint\n" {
		t.Fatalf("output %q", stdout)
	}
	if code := run(t, e, "label", "self", "bad"); code != contract.ExitUsage {
		t.Fatalf("bad argument: %d", code)
	}
}

func TestListFilters(t *testing.T) {
	e, stdout, _ := metaEnv(t, &metaServer{}, "parent12")
	ids := func() []string {
		var list []contract.SessionInfo
		if err := json.Unmarshal(stdout.Bytes(), &list); err != nil {
			t.Fatalf("json: %v %q", err, stdout)
		}
		out := make([]string, 0, len(list))
		for _, s := range list {
			out = append(out, s.ID)
		}
		stdout.Reset()
		return out
	}
	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"--label", "run=42"}, "aaaaaaaa bbbbbbbb"},
		{[]string{"--label", "run=42", "--label", "x"}, "bbbbbbbb"},
		{[]string{"--parent", "self"}, "aaaaaaaa"},
		{[]string{"--status", "exited", "--status", "idle"}, "bbbbbbbb cccccccc"},
	} {
		args := append([]string{"ls", "--json"}, tc.args...)
		if code := run(t, e, args...); code != 0 {
			t.Fatalf("%v: %d", tc.args, code)
		}
		if got := strings.Join(ids(), " "); got != tc.want {
			t.Errorf("%v: %s, want %s", tc.args, got, tc.want)
		}
	}
	if code := run(t, e, "ls", "--status", "bogus"); code != contract.ExitUsage {
		t.Fatalf("unknown status: %d", code)
	}
}

func TestParseIntersperseAndTerminator(t *testing.T) {
	e := Env{Stdout: io.Discard, Stderr: io.Discard}
	var json, observe bool
	c := &Command{Name: "x", Bind: func(fs *flag.FlagSet) Action {
		fs.BoolVar(&json, "json", false, "json")
		fs.BoolVar(&observe, "observe", false, "observe")
		return func(context.Context, Env, []string) int { return 0 }
	}}
	_, args, _, ok := e.Parse(c, []string{"a", "--json", "b", "--", "--observe", "c"})
	if !ok || !json || observe || strings.Join(args, " ") != "a b --observe c" {
		t.Fatalf("args %q json %v observe %v", args, json, observe)
	}
	strict := &Command{Name: "y", Strict: true, Bind: c.Bind}
	json = false
	_, args, _, ok = e.Parse(strict, []string{"a", "--json"})
	if !ok || json || strings.Join(args, " ") != "a --json" {
		t.Fatalf("strict args %q json %v", args, json)
	}
}

func TestHelpGoesToStdout(t *testing.T) {
	var stdout, stderr bytes.Buffer
	e := Env{Stdout: &stdout, Stderr: &stderr}
	for _, c := range Commands() {
		if c.Hidden {
			continue
		}
		stdout.Reset()
		stderr.Reset()
		if code := e.Run(context.Background(), c, []string{"--help"}); code != 0 || !strings.HasPrefix(stdout.String(), "Usage: webmux "+c.Name) || stderr.Len() != 0 {
			t.Errorf("%s --help: %d %q %q", c.Name, code, stdout.String(), stderr.String())
		}
	}
	stdout.Reset()
	if code := e.HelpCommand(Commands(), "1.0", []string{"--json", "label"}); code != 0 {
		t.Fatalf("help --json: %d", code)
	}
	var tree HelpTree
	if err := json.Unmarshal(stdout.Bytes(), &tree); err != nil || len(tree.Commands) != 1 || tree.Commands[0].Path[0] != "label" || tree.Version != "1.0" {
		t.Fatalf("help --json: %v %+v", err, tree)
	}
}
