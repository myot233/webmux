package api

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/rainchan/webmux/internal/agentprofile"
	"github.com/rainchan/webmux/internal/codeview"
	"github.com/rainchan/webmux/internal/config"
	"github.com/rainchan/webmux/internal/contract"
	"github.com/rainchan/webmux/internal/hybrid"
	"github.com/rainchan/webmux/internal/launch"
	"github.com/rainchan/webmux/internal/push"
	"github.com/rainchan/webmux/internal/session"
	"github.com/rainchan/webmux/internal/settings"
)

// The wire snapshots pin the JSON the browser's Zod contract
// (web/src/schemas.ts) reads. Re-record an intended protocol change with
//
//	go test ./internal/api -run TestWireSnapshots -update
//
// and review the testdata/wire diff together with the schema change.
var updateWire = flag.Bool("update", false, "re-record internal/api/testdata/wire snapshots")

// Rules are scoped to a recorded endpoint/frame and its schema path. An ID or
// timestamp in user content is data, even when it equals a dynamic field.
type wireRules map[string]string

func wireSchema(name string) wireRules {
	rules := wireRules{}
	add := func(prefix, kind string, fields ...string) {
		for _, field := range fields {
			rules[prefix+"/"+field] = kind
		}
	}
	sessionFields := func(prefix string) {
		add(prefix, "id", "id", "generation", "configId", "workspace/repositoryId", "workspace/worktreeId")
		add(prefix, "time", "createdAt", "lastActiveAt", "attention/since", "reportedCwd/since")
		add(prefix, "path", "cwd", "reportedCwd/path", "workspace/worktreePath")
	}
	worktreeFields := func(prefix string) {
		add(prefix, "id", "id", "repositoryId")
		add(prefix, "path", "path", "error")
	}
	repositoryFields := func(prefix string) {
		add(prefix, "id", "id")
		add(prefix, "time", "checkedAt")
		add(prefix, "path", "anchorPath", "commonDir", "error")
		worktreeFields(prefix + "/worktrees/*")
	}
	documentFields := func(prefix string) {
		add(prefix, "path", "absolutePath")
	}
	switch name {
	case "settings-patch", "settings-reset", "ws-settings-update":
		add("", "time", "updatedAt")
	case "session-create", "session-create-exiting", "session-create-from-config", "session-create-workspace", "session-get",
		"ws-terminal-state", "ws-terminal-state-exited", "session-get-hybrid", "ws-terminal-state-hybrid", "hybrid-resume":
		sessionFields("")
		add("", "path", "cmd/*")
	case "hybrid-records":
		add("/*", "id", "id", "configId", "agentSessionId", "previousIds/*")
		add("/*", "time", "createdAt", "updatedAt", "interruptedAt")
		add("/*", "path", "cwd", "cmd/*")
	case "sessions-list", "sessions-list-empty":
		sessionFields("/*")
	case "ws-sessions-created", "ws-sessions-updated":
		sessionFields("/session")
	case "ws-sessions-snapshot", "ws-sessions-snapshot-empty":
		sessionFields("/sessions/*")
	case "ws-sessions-removed":
		add("", "id", "id")
	case "ws-terminal-position-live", "ws-terminal-position-reset":
		add("", "id", "generation")
	case "ws-terminal-control-initial", "ws-terminal-control-observer", "ws-terminal-control-claimed", "ws-terminal-control-uncontrolled":
		add("/controller", "time", "since")
		// Explicit client query parameters stay literal; the anonymous exited
		// session's controller, when present, has a server-generated ID.
		if name == "ws-terminal-control-uncontrolled" {
			add("/controller", "id", "clientId")
		}
	case "config":
		add("", "path", "defaultCwd")
		add("", "hostname", "hostname")
	case "fs-dirs-default", "fs-dirs-prefix", "fs-dirs-empty":
		rules["/*"] = "path"
	case "error-400-fs-dirs":
		add("", "path", "error")
	case "upload":
		add("", "upload", "path")
	case "launch-create", "launch-create-no-env", "launch-get", "launch-update":
		add("", "id", "id")
		add("", "time", "createdAt", "updatedAt")
	case "launch-list", "launch-list-empty":
		add("/*", "id", "id")
		add("/*", "time", "createdAt", "updatedAt")
	case "repository-register", "repository-register-again", "repository-worktrees":
		repositoryFields("")
	case "repositories", "repositories-empty":
		repositoryFields("/*")
	case "repository-branches":
		add("", "path", "defaultParent", "branches/*/worktreePaths/*")
	case "worktree-create":
		worktreeFields("")
	case "worktree-removal-preview":
		add("", "id", "confirmationId")
		add("", "time", "expiresAt")
		worktreeFields("/worktree")
		sessionFields("/sessions/*")
	case "code-context":
		add("", "id", "contextId")
		add("", "path", "rootPath")
	case "code-tree", "code-files":
		add("", "time", "checkedAt")
	case "code-file-pdf":
		// Header-only documents version the file identity (inode, mtime).
		add("", "id", "snapshotId")
		add("", "time", "checkedAt")
		add("/document", "id", "contentVersion")
		documentFields("/document")
	case "code-file", "code-snapshot-file":
		add("", "id", "snapshotId")
		add("", "time", "checkedAt")
		documentFields("/document")
	case "code-changes":
		add("", "id", "nextCursor")
		add("", "time", "checkedAt")
	case "code-diff":
		add("", "id", "snapshotId")
		add("", "time", "checkedAt")
		documentFields("/old")
		documentFields("/new")
	case "code-check":
		add("", "time", "checkedAt")
		rules["/snapshots"] = "snapshotKeys"
	case "ws-chat-snapshot":
		add("", "id", "generation")
		add("/state", "id", "agentSessionId")
	case "ws-chat-events":
		add("", "id", "generation")
		add("/items/*", "time", "at")
	case "ws-chat-state":
		add("/state", "id", "agentSessionId")
		add("/state/queue/items/*", "time", "createdAt")
	case "push-status", "push-status-quick-tunnel", "push-status-tailscale", "push-status-disabled":
		add("", "id", "publicKey")
	case "push-subscribe", "push-subscribe-gone":
		add("", "id", "id")
	case "push-test":
		add("", "id", "removed/*")
	}
	return rules
}

var wireUpload = regexp.MustCompile(`^webmux-upload-\d+$`)

type wireNormalizer struct {
	hostname string
	literal  []string
	paths    map[string]string
	names    map[string]string
	counts   map[string]int
}

func newWireNormalizer() *wireNormalizer {
	return &wireNormalizer{paths: map[string]string{}, names: map[string]string{}, counts: map[string]int{}}
}

func (n *wireNormalizer) path(value, name string) {
	if value == "" {
		return
	}
	if _, ok := n.paths[value]; ok {
		return
	}
	n.paths[value] = name
	n.literal = append(n.literal, value)
	sort.Slice(n.literal, func(i, j int) bool { return len(n.literal[i]) > len(n.literal[j]) })
}

func (n *wireNormalizer) id(key, value string) string {
	if value == "" {
		return value
	}
	if name, ok := n.names[value]; ok {
		return name
	}
	n.counts[key]++
	name := fmt.Sprintf("<%s#%d>", key, n.counts[key])
	n.names[value] = name
	return name
}

// Only complete registered path prefixes are replaced, and only in schema
// fields opted in above. In particular, error text never substitutes IDs.
func (n *wireNormalizer) pathText(s string) string {
	for _, literal := range n.literal {
		var out strings.Builder
		for {
			at := strings.Index(s, literal)
			if at < 0 {
				out.WriteString(s)
				break
			}
			end := at + len(literal)
			before := at == 0 || strings.ContainsRune(" \t\n\"'(:", rune(s[at-1]))
			after := end == len(s) || strings.ContainsRune("/ \t\n\"'):;", rune(s[end]))
			out.WriteString(s[:at])
			if before && after {
				out.WriteString(n.paths[literal])
			} else {
				out.WriteString(literal)
			}
			s = s[end:]
		}
		s = out.String()
	}
	return s
}

func (n *wireNormalizer) normalize(name string, v any) any {
	return n.value(wireSchema(name), "", v)
}

func (n *wireNormalizer) value(rules wireRules, path string, v any) any {
	switch v := v.(type) {
	case map[string]any:
		keys := make([]string, 0, len(v))
		for key := range v {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		out := make(map[string]any, len(v))
		for _, key := range keys {
			if rules[path] == "snapshotKeys" {
				out[n.id("snapshotId", key)] = v[key]
			} else {
				out[key] = n.value(rules, path+"/"+key, v[key])
			}
		}
		return out
	case []any:
		out := make([]any, len(v))
		for i, item := range v {
			out[i] = n.value(rules, path+"/*", item)
		}
		return out
	case string:
		switch rules[path] {
		case "id":
			key := path[strings.LastIndex(path, "/")+1:]
			if path == "/removed/*" {
				key = "id"
			}
			return n.id(key, v)
		case "time":
			if parsed, err := time.Parse(time.RFC3339Nano, v); err == nil {
				if parsed.IsZero() {
					return "<zero-time>"
				}
				return "<time>"
			}
		case "hostname":
			if n.hostname != "" && v == n.hostname {
				return "<hostname>"
			}
		case "path":
			return n.pathText(v)
		case "upload":
			parts := strings.Split(n.pathText(v), "/")
			if len(parts) == 3 && parts[0] == "<tmp>" && wireUpload.MatchString(parts[1]) {
				parts[1] = "webmux-upload-<n>"
			}
			return strings.Join(parts, "/")
		}
	}
	return v
}

// wireRecord is one snapshot: an HTTP status (0 for a WebSocket payload)
// and the normalized JSON body (absent for an empty body).
type wireRecord struct {
	Status int `json:"status,omitempty"`
	Body   any `json:"body"`
}

type wireRecorder struct {
	t    *testing.T
	norm *wireNormalizer
	seen map[string]bool
}

// record compares one normalized JSON payload with testdata/wire/name.json.
func (w *wireRecorder) record(name string, status int, body []byte) any {
	w.t.Helper()
	if w.seen[name] {
		w.t.Fatalf("duplicate wire snapshot %s", name)
	}
	w.seen[name] = true
	var decoded any
	if len(bytes.TrimSpace(body)) > 0 {
		if err := json.Unmarshal(body, &decoded); err != nil {
			w.t.Fatalf("%s: response is not JSON: %v: %q", name, err, body)
		}
	}
	var buffer bytes.Buffer
	encoder := json.NewEncoder(&buffer)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(wireRecord{Status: status, Body: w.norm.normalize(name, decoded)}); err != nil {
		w.t.Fatal(err)
	}
	got := buffer.Bytes()
	path := filepath.Join("testdata", "wire", name+".json")
	if *updateWire {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			w.t.Fatal(err)
		}
		if err := os.WriteFile(path, got, 0o644); err != nil {
			w.t.Fatal(err)
		}
		return decoded
	}
	want, err := os.ReadFile(path)
	if err != nil {
		w.t.Errorf("%s: missing snapshot %s (record with -update): %v", name, path, err)
		return decoded
	}
	if !bytes.Equal(got, want) {
		w.t.Errorf("wire snapshot %s changed (re-record with -update if intended):\n%s", name, lineDiff(string(want), string(got)))
	}
	return decoded
}

// lineDiff lists the lines that differ between the recorded and current
// snapshot; snapshots are short, so a positional comparison is enough.
func lineDiff(want, got string) string {
	a, b := strings.Split(want, "\n"), strings.Split(got, "\n")
	var out strings.Builder
	for i := 0; i < len(a) || i < len(b); i++ {
		var left, right string
		if i < len(a) {
			left = a[i]
		}
		if i < len(b) {
			right = b[i]
		}
		if left != right {
			fmt.Fprintf(&out, "line %d\n- %s\n+ %s\n", i+1, left, right)
		}
	}
	return out.String()
}

type wireFixture struct {
	*wireRecorder
	ctx      context.Context
	server   *httptest.Server
	manager  *session.Manager
	hybrids  *hybrid.Service
	launches *launch.Service
	root     string
}

// newWireFixture runs every service the router serves, against a Git
// repository with fixed commit dates so object IDs are reproducible.
func newWireFixture(t *testing.T, rec *wireRecorder) *wireFixture {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("requires real Git")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	for _, key := range []string{"GIT_AUTHOR", "GIT_COMMITTER"} {
		t.Setenv(key+"_NAME", "Wire test")
		t.Setenv(key+"_EMAIL", "wire@example.invalid")
		t.Setenv(key+"_DATE", "2026-01-02T03:04:05Z")
	}
	tmp := t.TempDir()
	t.Setenv("TMPDIR", tmp)
	root := filepath.Join(t.TempDir(), "repo")
	if err := os.MkdirAll(filepath.Join(root, "sub", "deeper"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, dir := range []string{".hidden", "beta"} {
		if err := os.MkdirAll(filepath.Join(root, dir), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("one\ntwo\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "sub", "b.txt"), []byte("bee\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "doc.pdf"), []byte("%PDF-1.4\n%%EOF\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"init", "-b", "main"}, {"add", "."}, {"commit", "-m", "initial"}} {
		if out, err := exec.Command("git", append([]string{"-C", root}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("Git fixture: %v %s", err, out)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("one\nthree\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "new.txt"), []byte("fresh\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	hostname, _ := os.Hostname()
	rec.norm.path(root, "<repo>")
	rec.norm.path(root+".worktrees", "<repo>.worktrees")
	rec.norm.path(home, "<home>")
	rec.norm.path(tmp, "<tmp>")
	rec.norm.hostname = hostname

	pushService := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/gone") {
			w.WriteHeader(http.StatusGone)
			return
		}
		w.WriteHeader(http.StatusCreated)
	}))
	t.Cleanup(pushService.Close)
	rec.norm.path(pushService.URL, "<push-service>")

	c := config.Config{
		Listen: "127.0.0.1:7681", Token: "secret", DefaultCwd: root,
		Push: config.PushConfig{Enabled: true, Subject: config.DefaultPushSubject, Kinds: config.DefaultPushKinds()},
	}
	m := session.NewManager(testSessionOptions(c))
	pushes := push.New(c.Push, t.TempDir(), m, slog.New(slog.NewTextHandler(io.Discard, nil)), pushService.Client().Transport)
	launches := launch.Open(t.TempDir())
	prefs := settings.Open(t.TempDir())
	t.Cleanup(prefs.Close)
	codes := codeview.New(m)
	public := &PublicOrigins{}
	if err := public.Set("cloudflare", "https://abc.trycloudflare.com"); err != nil {
		t.Fatal(err)
	}
	if err := public.Set("tailscale", "https://node.tailnet.ts.net:8443"); err != nil {
		t.Fatal(err)
	}
	hybrids := hybrid.New(hybrid.Options{StateDir: t.TempDir(), Manager: m})
	server := httptest.NewServer(New(testDeps(t, Deps{Config: c, Manager: m, Public: public, Push: pushes, Launch: launches, Code: codes, Settings: prefs, Hybrid: hybrids})))
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(func() {
		cancel()
		server.Close()
		codes.Close()
		launches.Close()
		pushes.Close()
		hybrids.Close()
		shutdown, done := context.WithTimeout(context.Background(), 8*time.Second)
		defer done()
		m.Shutdown(shutdown)
	})
	rec.norm.path(strings.TrimPrefix(server.URL, "http://"), "<server>")
	return &wireFixture{wireRecorder: rec, ctx: ctx, server: server, manager: m, hybrids: hybrids, launches: launches, root: root}
}

type wireRequest struct {
	host, origin, token, contentType string
	body                             io.Reader
}

// call sends one request and records its status and JSON body as name.
func (f *wireFixture) call(name, method, path string, body any, want int, opts ...func(*wireRequest)) any {
	f.t.Helper()
	request := wireRequest{token: "secret", contentType: "application/json"}
	switch body := body.(type) {
	case nil:
	case string:
		request.body = strings.NewReader(body)
	default:
		data, err := json.Marshal(body)
		if err != nil {
			f.t.Fatal(err)
		}
		request.body = bytes.NewReader(data)
	}
	for _, opt := range opts {
		opt(&request)
	}
	r, err := http.NewRequestWithContext(f.ctx, method, f.server.URL+path, request.body)
	if err != nil {
		f.t.Fatal(err)
	}
	if request.host != "" {
		r.Host = request.host
	}
	if request.origin != "" {
		r.Header.Set("Origin", request.origin)
	}
	if request.token != "" {
		r.Header.Set("Authorization", "Bearer "+request.token)
	}
	if request.body != nil {
		r.Header.Set("Content-Type", request.contentType)
	}
	res, err := http.DefaultClient.Do(r)
	if err != nil {
		f.t.Fatal(err)
	}
	defer res.Body.Close()
	data, err := io.ReadAll(res.Body)
	if err != nil {
		f.t.Fatal(err)
	}
	if res.StatusCode != want {
		f.t.Fatalf("%s: %s %s status %d want %d: %s", name, method, path, res.StatusCode, want, data)
	}
	return f.record(name, res.StatusCode, data)
}

func (f *wireFixture) dial(path string) *websocket.Conn {
	f.t.Helper()
	conn, _, err := websocket.Dial(f.ctx, "ws"+strings.TrimPrefix(f.server.URL, "http")+path, &websocket.DialOptions{Subprotocols: tokenProtocols("secret")})
	if err != nil {
		f.t.Fatal(err)
	}
	conn.SetReadLimit(4 << 20)
	f.t.Cleanup(func() { _ = conn.CloseNow() })
	return conn
}

// event reads /ws/sessions events until match accepts one, then records it.
func (f *wireFixture) event(conn *websocket.Conn, name string, match func(map[string]any) bool) map[string]any {
	f.t.Helper()
	for {
		_, data, err := conn.Read(f.ctx)
		if err != nil {
			f.t.Fatalf("%s: %v", name, err)
		}
		var event map[string]any
		if err := json.Unmarshal(data, &event); err != nil {
			f.t.Fatal(err)
		}
		if match(event) {
			f.record(name, 0, data)
			return event
		}
	}
}

// frame reads terminal frames until one of kind satisfies match, then
// records its JSON payload.
func (f *wireFixture) frame(conn *websocket.Conn, name string, kind byte, match func(map[string]any) bool) map[string]any {
	f.t.Helper()
	for {
		_, p, err := conn.Read(f.ctx)
		if err != nil {
			f.t.Fatalf("%s: waiting for frame %d: %v", name, kind, err)
		}
		if len(p) == 0 || p[0] != kind {
			continue
		}
		var payload map[string]any
		if err := json.Unmarshal(p[1:], &payload); err != nil {
			f.t.Fatalf("%s: %v: %q", name, err, p)
		}
		if match == nil || match(payload) {
			f.record(name, 0, p[1:])
			return payload
		}
	}
}

func (f *wireFixture) send(conn *websocket.Conn, kind byte, p []byte) {
	f.t.Helper()
	if err := conn.Write(f.ctx, websocket.MessageBinary, append([]byte{kind}, p...)); err != nil {
		f.t.Fatal(err)
	}
}

func withHost(host string) func(*wireRequest) { return func(r *wireRequest) { r.host = host } }
func withOrigin(origin string) func(*wireRequest) {
	return func(r *wireRequest) { r.origin = origin }
}
func withToken(token string) func(*wireRequest) { return func(r *wireRequest) { r.token = token } }

func wireUploadBody(t *testing.T, name string, data []byte) func(*wireRequest) {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	header := make(textproto.MIMEHeader)
	header.Set("Content-Disposition", mime.FormatMediaType("form-data", map[string]string{"name": "file", "filename": name}))
	header.Set("Content-Type", "application/octet-stream")
	part, err := writer.CreatePart(header)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = part.Write(data)
	_ = writer.Close()
	return func(r *wireRequest) {
		r.body = &body
		r.contentType = writer.FormDataContentType()
	}
}

func str(v any, key string) string {
	s, _ := v.(map[string]any)[key].(string)
	return s
}

func TestWireSnapshots(t *testing.T) {
	for _, group := range []struct {
		name string
		run  func(*wireFixture)
	}{
		{"sessions-terminal", wireSessions},
		{"basics-errors-fs-upload", wireBasics},
		{"launch", wireLaunch},
		{"workspace", wireWorkspace},
		{"codeview", wireCodeview},
		{"push", wirePush},
		{"settings", wireSettings},
		{"hybrid", wireHybrid},
		{"hybrid-records", wireHybridRecords},
	} {
		t.Run(group.name, func(t *testing.T) {
			rec := &wireRecorder{t: t, norm: newWireNormalizer(), seen: map[string]bool{}}
			group.run(newWireFixture(t, rec))
		})
	}
}

func wireSessions(f *wireFixture) {

	// Session list events: the empty snapshot, then one session's life.
	list := f.dial("/ws/sessions")
	f.event(list, "ws-sessions-snapshot-empty", func(e map[string]any) bool { return e["type"] == "snapshot" })
	// The command waits for a line so the create response is always its running
	// state; a shell that exits immediately can publish the exit before the
	// handler reads Info.
	exited := f.call("session-create-exiting", "POST", "/api/sessions", map[string]any{"cmd": []string{"/bin/sh", "-c", "read _; exit 3"}, "name": "exiting"}, 201)
	exitedID := str(exited, "id")
	f.event(list, "ws-sessions-created", func(e map[string]any) bool { return e["type"] == "created" })
	item, err := f.manager.Get(exitedID)
	if err != nil {
		f.t.Fatal(err)
	}
	if err := item.Write([]byte("\n")); err != nil {
		f.t.Fatal(err)
	}
	f.event(list, "ws-sessions-updated", func(e map[string]any) bool {
		return e["type"] == "updated" && str(e["session"], "status") == "exited"
	})

	// Terminal frames of an exited session end with the exit frame.
	ended := f.dial("/ws/s/" + exitedID)
	f.frame(ended, "ws-terminal-control-uncontrolled", 6, nil)
	f.frame(ended, "ws-terminal-position-reset", 4, nil)
	f.frame(ended, "ws-terminal-state-exited", 2, nil)
	f.frame(ended, "ws-terminal-exit", 3, nil)

	f.call("session-delete-remove", "DELETE", "/api/sessions/"+exitedID+"?remove=1", nil, 204)
	f.event(list, "ws-sessions-removed", func(e map[string]any) bool { return e["type"] == "removed" })
	_ = list.CloseNow()

	f.call("sessions-list-empty", "GET", "/api/sessions", nil, 200)
	running := f.call("session-create", "POST", "/api/sessions", map[string]any{"cmd": []string{"/bin/sh", "-c", "sleep 60"}, "name": "wire", "env": map[string]string{"WIRE": "1"}}, 201)
	runningID := str(running, "id")
	f.call("sessions-list", "GET", "/api/sessions", nil, 200)
	f.call("session-get", "GET", "/api/sessions/"+runningID, nil, 200)

	// Live terminal frames: control, position, state, submissions.
	live := f.dial("/ws/s/" + runningID + "?client=wire-client-1&label=Wire")
	f.frame(live, "ws-terminal-control-initial", 6, nil)
	f.frame(live, "ws-terminal-position-live", 4, nil)
	f.frame(live, "ws-terminal-state", 2, nil)
	observer := f.dial("/ws/s/" + runningID + "?client=wire-client-2&label=Observer")
	f.frame(observer, "ws-terminal-control-observer", 6, nil)
	f.send(observer, 5, nil)
	f.frame(live, "ws-terminal-control-claimed", 6, func(p map[string]any) bool { return str(p["controller"], "label") == "Observer" })
	f.send(live, 5, nil)
	_ = observer.CloseNow()
	f.send(live, 4, []byte(`{"ids":["missing-submission"]}`))
	f.frame(live, "ws-terminal-submission-missing", 5, nil)
	f.send(live, 3, submitFrame("stale-generation", "0000000000000000", "x", "\r"))
	f.frame(live, "ws-terminal-submission-failed", 5, nil)
	generation := str(running, "generation")
	f.send(live, 3, submitFrame("wire-submission", generation, "x", "\r"))
	f.frame(live, "ws-terminal-submission-received", 5, func(p map[string]any) bool { return p["state"] == "received" })
	f.frame(live, "ws-terminal-submission-written", 5, func(p map[string]any) bool { return p["state"] == "written" })
	_ = live.CloseNow()
	// Closing the socket does not join the server's detach goroutine.
	// A written submission acknowledges PTY input, not its echoed output.
	// Wait for both before recording a scheduler-independent list snapshot.
	current, err := f.manager.Get(runningID)
	if err != nil {
		f.t.Fatal(err)
	}
	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()
	for {
		info := current.Info()
		if info.Clients == 0 && info.Preview == "x" {
			break
		}
		select {
		case <-ticker.C:
		case <-f.ctx.Done():
			f.t.Fatal("terminal echo or client detach did not settle")
		}
	}

	// The non-empty list snapshot.
	again := f.dial("/ws/sessions")
	f.event(again, "ws-sessions-snapshot", func(e map[string]any) bool { return e["type"] == "snapshot" })
	_ = again.CloseNow()
	f.call("session-delete-close", "DELETE", "/api/sessions/"+runningID, nil, 204)
}

func wireBasics(f *wireFixture) {
	t := f.t
	// REST basics.
	f.call("health", "GET", "/api/health", nil, 200)
	f.call("config", "GET", "/api/config", nil, 200)

	// Session errors.
	runningID := f.startSession()
	f.call("error-400-session-json", "POST", "/api/sessions", `{"cwd":`, 400)
	f.call("error-400-session-conflict", "POST", "/api/sessions", map[string]any{"configId": "abcdefgh", "cmd": []string{"/bin/sh"}}, 400)
	f.call("error-400-session-config-empty", "POST", "/api/sessions", map[string]any{"configId": ""}, 400)
	f.call("error-400-session-env", "POST", "/api/sessions", map[string]any{"cmd": []string{"/bin/sh"}, "env": map[string]string{"WEBMUX_SESSION_ID": "forged"}}, 400)
	f.call("error-401", "GET", "/api/health", nil, 401, withToken(""))
	f.call("error-403-origin", "GET", "/api/health", nil, 403, withOrigin("http://evil.example"))
	f.call("error-404-session", "GET", "/api/sessions/missing", nil, 404)
	f.call("error-409-session-running", "DELETE", "/api/sessions/"+runningID+"?remove=1", nil, 409)

	// Directory picker and mkdir.
	f.call("fs-dirs-default", "GET", "/api/fs/dirs", nil, 200)
	f.call("fs-dirs-prefix", "GET", "/api/fs/dirs?prefix="+f.root+"/s", nil, 200)
	f.call("fs-dirs-empty", "GET", "/api/fs/dirs?prefix="+f.root+"/zzz", nil, 200)
	f.call("error-400-fs-dirs", "GET", "/api/fs/dirs?prefix="+f.root+"/missing/", nil, 400)
	f.call("fs-mkdir", "POST", "/api/fs/mkdir", map[string]string{"path": filepath.Join(f.root, "made")}, 204)
	f.call("error-400-fs-mkdir", "POST", "/api/fs/mkdir", map[string]string{"path": ""}, 400)

	// Upload.
	f.call("upload", "POST", "/api/sessions/"+runningID+"/upload", nil, 201, wireUploadBody(t, "note.txt", []byte("hello")))
	f.call("error-400-upload-name", "POST", "/api/sessions/"+runningID+"/upload", nil, 400, wireUploadBody(t, "..", []byte("hello")))
	// The declared length alone is refused; no client sends such a body.
	oversized := httptest.NewRequest("POST", "/api/sessions/"+runningID+"/upload", strings.NewReader("x"))
	oversized.Host = strings.TrimPrefix(f.server.URL, "http://")
	oversized.Header.Set("Authorization", "Bearer secret")
	oversized.Header.Set("Content-Type", "multipart/form-data; boundary=x")
	oversized.ContentLength = maxUploadBodyBytes + 1
	tooLarge := httptest.NewRecorder()
	f.server.Config.Handler.ServeHTTP(tooLarge, oversized)
	f.record("error-413-upload", tooLarge.Code, tooLarge.Body.Bytes())
}

func wireLaunch(f *wireFixture) {

	// Launch configs.
	f.call("launch-list-empty", "GET", "/api/launch-configs", nil, 200)
	created := f.call("launch-create", "POST", "/api/launch-configs", map[string]any{"name": "Claude", "cmd": []string{"claude", "--resume"}, "env": map[string]string{"API_KEY": "value"}}, 201)
	shell := f.call("launch-create-no-env", "POST", "/api/launch-configs", map[string]any{"name": "Shell", "cmd": []string{"/bin/sh", "-c", "sleep 60"}}, 201)
	launchID := str(created, "id")
	f.call("launch-list", "GET", "/api/launch-configs", nil, 200)
	f.call("launch-get", "GET", "/api/launch-configs/"+launchID, nil, 200)
	f.call("launch-update", "PUT", "/api/launch-configs/"+launchID, map[string]any{"name": "Claude 2", "cmd": []string{"claude"}}, 200)
	f.call("error-400-launch", "POST", "/api/launch-configs", map[string]any{"name": "", "cmd": []string{"x"}}, 400)
	f.call("error-404-launch", "GET", "/api/launch-configs/zzzzzzzz", nil, 404)
	f.call("error-409-launch", "POST", "/api/launch-configs", map[string]any{"name": "Shell", "cmd": []string{"x"}}, 409)
	// The started config runs everywhere; claude need not be installed.
	f.call("session-create-from-config", "POST", "/api/sessions", map[string]any{"configId": str(shell, "id"), "cwd": f.root}, 201)
	f.call("error-404-session-config", "POST", "/api/sessions", map[string]any{"configId": "zzzzzzzz"}, 404)
	f.call("launch-delete", "DELETE", "/api/launch-configs/"+launchID, nil, 204)
}

func wireWorkspace(f *wireFixture) {

	// Repositories and worktrees.
	f.call("repositories-empty", "GET", "/api/repositories", nil, 200)
	repository := f.call("repository-register", "POST", "/api/repositories", map[string]string{"path": f.root}, 201)
	f.call("repository-register-again", "POST", "/api/repositories", map[string]string{"path": f.root}, 200)
	repositoryID := str(repository, "id")
	base := "/api/repositories/" + repositoryID
	f.call("repositories", "GET", "/api/repositories", nil, 200)
	f.call("repository-worktrees", "GET", base+"/worktrees", nil, 200)
	f.call("repository-branches", "GET", base+"/branches", nil, 200)
	linked := filepath.Join(filepath.Dir(f.root), "linked")
	f.norm.path(linked, "<linked>")
	tree := f.call("worktree-create", "POST", base+"/worktrees", map[string]string{"path": linked, "mode": "new", "branch": "linked", "base": "HEAD"}, 201)
	treeID := str(tree, "id")
	f.call("error-400-worktree", "POST", base+"/worktrees", map[string]string{"path": filepath.Join(filepath.Dir(f.root), "bad"), "mode": "new", "branch": "--force", "base": "HEAD"}, 400)
	f.call("error-404-repository", "GET", "/api/repositories/missing/worktrees", nil, 404)
	f.call("session-create-workspace", "POST", "/api/sessions", map[string]any{"cmd": []string{"/bin/sh", "-c", "sleep 60"}, "workspace": map[string]string{"repositoryId": repositoryID, "worktreeId": treeID}}, 201)
	treeURL := base + "/worktrees/" + treeID
	preview := f.call("worktree-removal-preview", "POST", treeURL+"/removal-preview", map[string]any{}, 200)
	f.call("error-409-worktree-remove", "POST", treeURL+"/remove", map[string]any{"confirmationId": str(preview, "confirmationId"), "mode": "safe"}, 409)
	f.call("repository-unregister", "DELETE", base, nil, 204)
}

func wireCodeview(f *wireFixture) {
	runningID := f.startSession()

	// Code view.
	code := "/api/sessions/" + runningID + "/code"
	view := f.call("code-context", "GET", code+"/context", nil, 200)
	contextID := str(view, "contextId")
	root := "contextId=" + contextID
	f.call("code-tree", "GET", code+"/tree?"+root, nil, 200)
	f.call("code-files", "GET", code+"/files?"+root+"&q=b", nil, 200)
	located := f.call("code-locate", "GET", code+"/locate?"+root+"&path=sub/b.txt", nil, 200)
	file := f.call("code-file", "GET", code+"/file?"+root+"&nodeId="+str(located, "nodeId"), nil, 200)
	pdf := f.call("code-locate-pdf", "GET", code+"/locate?"+root+"&path=doc.pdf", nil, 200)
	f.call("code-file-pdf", "GET", code+"/file?"+root+"&nodeId="+str(pdf, "nodeId"), nil, 200)
	snapshotID := str(file, "snapshotId")
	changes := f.call("code-changes", "GET", code+"/changes?"+root, nil, 200)
	var changeID string
	for _, entry := range changes.(map[string]any)["entries"].([]any) {
		if str(entry, "newPath") == "a.txt" {
			changeID = str(entry, "changeId")
		}
	}
	diff := f.call("code-diff", "GET", code+"/diff?"+root+"&changeId="+changeID, nil, 200)
	f.call("code-snapshot-file", "GET", code+"/snapshots/"+str(diff, "snapshotId")+"/file?side=new", nil, 200)
	f.call("code-check", "POST", code+"/check", map[string]any{"contextId": contextID, "snapshotIds": []string{snapshotID}}, 200)
	f.call("error-400-code-path", "GET", code+"/file?"+root+"&nodeId="+base64.RawURLEncoding.EncodeToString([]byte("../other")), nil, 400)
	f.call("error-404-code-session", "GET", "/api/sessions/missing/code/context", nil, 404)
	f.call("error-409-code-context", "GET", code+"/tree?contextId=wrong", nil, 409)
	f.call("error-410-code-snapshot", "GET", code+"/snapshots/unknown/file?side=new", nil, 410)
}

func wirePush(f *wireFixture) {
	t := f.t

	// Push.
	f.call("push-status", "GET", "/api/push", nil, 200)
	f.call("push-status-quick-tunnel", "GET", "/api/push", nil, 200, withHost("abc.trycloudflare.com"), withOrigin("https://abc.trycloudflare.com"))
	f.call("push-status-tailscale", "GET", "/api/push", nil, 200, withHost("node.tailnet.ts.net:8443"), withOrigin("https://node.tailnet.ts.net:8443"))
	f.call("error-409-push-quick-tunnel", "POST", "/api/push/subscriptions", browserSubscription(t, f.pushURL(t, "/device")), 409, withHost("abc.trycloudflare.com"), withOrigin("https://abc.trycloudflare.com"))
	f.call("error-400-push-endpoint", "POST", "/api/push/subscriptions", browserSubscription(t, "http://push.example/x"), 400)
	subscribed := f.call("push-subscribe", "POST", "/api/push/subscriptions", browserSubscription(t, f.pushURL(t, "/device")), 200)
	f.call("push-subscribe-gone", "POST", "/api/push/subscriptions", browserSubscription(t, f.pushURL(t, "/gone")), 200)
	f.call("push-test", "POST", "/api/push/test", nil, 200)
	f.call("push-unsubscribe", "DELETE", "/api/push/subscriptions/"+str(subscribed, "id"), nil, 204)

}

// These groups need a live session, not the sessions group's lifecycle or IDs.
func (f *wireFixture) startSession() string {
	f.t.Helper()
	item, err := f.manager.Create(session.CreateInput{Cwd: f.root, Cmd: []string{"/bin/sh", "-c", "sleep 60"}, Name: "wire", Env: nil, ConfigID: ""})
	if err != nil {
		f.t.Fatal(err)
	}
	return item.Info().ID
}

func (f *wireFixture) pushURL(t *testing.T, path string) string {
	t.Helper()
	for literal, name := range f.norm.paths {
		if name == "<push-service>" {
			return literal + path
		}
	}
	t.Fatal("push service not registered")
	return ""
}

// TestWireSnapshotsUnavailableServices pins the 503 and disabled-service
// bodies of a server without launch and push stores.
func TestWireSnapshotsUnavailableServices(t *testing.T) {
	rec := &wireRecorder{t: t, norm: newWireNormalizer(), seen: map[string]bool{}}
	m := session.NewManager(session.Options{})
	defer m.Shutdown(context.Background())
	handler := New(testDeps(t, Deps{Config: config.Config{Token: "secret"}, Manager: m, Workspace: testWorkspace(t, m, t.TempDir())}))
	for _, row := range []struct {
		name, method, path, body string
		want                     int
	}{
		{"error-503-launch", "GET", "/api/launch-configs", "", 503},
		{"error-503-session-config", "POST", "/api/sessions", `{"configId":"abcdefgh"}`, 503},
		{"push-status-disabled", "GET", "/api/push", "", 200},
		{"error-409-push-disabled", "POST", "/api/push/test", "", 409},
	} {
		r := httptest.NewRequest(row.method, "http://127.0.0.1:7681"+row.path, strings.NewReader(row.body))
		r.Header.Set("Authorization", "Bearer secret")
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Code != row.want {
			t.Fatalf("%s: status %d want %d: %s", row.name, w.Code, row.want, w.Body.String())
		}
		rec.record(row.name, w.Code, w.Body.Bytes())
	}
}

func wireSettings(f *wireFixture) {
	c := f.dial("/ws/settings")
	f.event(c, "ws-settings-snapshot", func(e map[string]any) bool { return e["type"] == "settings" })
	f.call("settings-get", "GET", "/api/settings", nil, 200)
	f.call("settings-patch", "PATCH", "/api/settings", map[string]any{"baseRevision": 0, "changes": map[string]any{"theme": "dark", "quickCommands": []any{}}}, 200)
	f.event(c, "ws-settings-update", func(e map[string]any) bool { return e["type"] == "settings.update" })
	f.call("settings-conflict", "PATCH", "/api/settings", map[string]any{"baseRevision": 0, "changes": map[string]any{"theme": nil}}, 409)
	f.call("settings-reset", "PATCH", "/api/settings", map[string]any{"baseRevision": 1, "changes": map[string]any{"theme": nil}}, 200)
}

// wireDriver is a hybrid shell driver that does nothing.
type wireDriver struct{}

func (wireDriver) Close() error  { return nil }
func (wireDriver) RunExited(int) {}

// wireHybrid pins a hybrid shell's session info and the 4409 close of its
// terminal socket after a switch.
func wireHybrid(f *wireFixture) {
	item, err := f.manager.CreateShell(session.ShellInput{
		CreateInput: session.CreateInput{Cwd: f.root, Cmd: []string{"omp"}, Name: "hybrid"},
		Argv:        []string{"/bin/sh", "-c", "sleep 60"},
	}, wireDriver{})
	if err != nil {
		f.t.Fatal(err)
	}
	id := item.Info().ID
	item.SetOverlay(&session.Overlay{Hybrid: &session.HybridInfo{Side: "tui", State: "active", Agent: "omp"}})
	f.call("session-get-hybrid", "GET", "/api/sessions/"+id, nil, 200)

	terminal := f.dial("/ws/s/" + id + "?client=wire-client-1&label=Wire")
	f.frame(terminal, "ws-terminal-state-hybrid", 2, nil)
	agentSessionID := "ses_wire"
	item.SetOverlay(&session.Overlay{Hybrid: &session.HybridInfo{Side: "acp", Phase: "loading", State: "active", Agent: "omp", AgentSessionID: &agentSessionID}})
	if err := item.EndRun(f.ctx); err != nil {
		f.t.Fatal(err)
	}
	f.closed(terminal, "ws-terminal-moved")
	// A terminal socket of a shell on its other side is refused the same way.
	refused := f.dial("/ws/s/" + id)
	if _, _, err := refused.Read(f.ctx); websocket.CloseStatus(err) != contract.CloseMoved {
		f.t.Fatalf("attach without a run: %v", err)
	}
	wireChat(f)
}

// wireChat pins the chat channel: its three frame types, prompt results
// and conflict bodies. The agent's own start updates arrive in no fixed
// order, so the backlog frame is read but not recorded.
func wireChat(f *wireFixture) {
	id := fakeChat(f.t, f.hybrids, f.root, agentprofile.OMP, "FAKEAGENT_PERMISSION=1")
	watch := observe(f.t, f.hybrids, id)
	watch.until("start updates", func(o *observer) bool { return len(o.state.Commands) > 0 && len(o.state.Config) == 2 })
	conn := f.dial("/ws/s/" + id + "/chat")
	f.event(conn, "ws-chat-snapshot", func(e map[string]any) bool { return e["type"] == contract.ChatFrameSnapshot })
	if _, _, err := conn.Read(f.ctx); err != nil {
		f.t.Fatal(err)
	}
	base := "/api/sessions/" + id + "/chat/"
	f.call("chat-prompt-sent", "POST", base+"prompts", contract.ChatPromptRequest{ClientRequestID: "wire-prompt-1", Text: "hello"}, 200)
	f.event(conn, "ws-chat-events", func(e map[string]any) bool {
		items, _ := e["items"].([]any)
		return e["type"] == contract.ChatFrameEvents && len(items) > 0 && str(items[0], "kind") == contract.ChatEventTurnStart
	})
	f.call("chat-prompt-queued", "POST", base+"prompts", contract.ChatPromptRequest{ClientRequestID: "wire-prompt-2", Text: "later"}, 200)
	f.event(conn, "ws-chat-state", func(e map[string]any) bool {
		state, _ := e["state"].(map[string]any)
		requests, _ := state["requests"].([]any)
		queue, _ := state["queue"].(map[string]any)
		items, _ := queue["items"].([]any)
		return e["type"] == contract.ChatFrameState && len(requests) == 1 && len(items) == 1
	})
	f.call("chat-answer", "POST", base+"requests/r1", contract.ChatAnswerRequest{ClientRequestID: "wire-answer-1", Outcome: "selected", OptionID: "allow"}, 204)
	f.call("chat-conflict-answered", "POST", base+"requests/r1", contract.ChatAnswerRequest{ClientRequestID: "wire-answer-2", Outcome: "cancelled"}, 409)
	f.call("chat-permission-mode", "PUT", base+"permission-mode", contract.ChatPermissionModeRequest{ClientRequestID: "wire-permission-1", Mode: contract.ChatPermissionAuto}, 204)
	f.call("error-400-chat-permission-mode", "PUT", base+"permission-mode", contract.ChatPermissionModeRequest{ClientRequestID: "wire-permission-2", Mode: "never"}, 400)

	ctx, cancel := context.WithTimeout(f.ctx, chatTimeout)
	defer cancel()
	terminal, err := f.hybrids.Create(ctx, hybrid.CreateInput{
		Cwd: f.root, Name: "tui", Cmd: []string{fakeAgentPath(f.t, agentprofile.OMP)}, StartSide: hybrid.SideTUI,
		Env: map[string]string{"FAKEAGENT_STORE": f.t.TempDir()},
	})
	if err != nil {
		f.t.Fatal(err)
	}
	f.call("chat-conflict-wrong-side", "POST", "/api/sessions/"+terminal.Info().ID+"/chat/prompts", contract.ChatPromptRequest{ClientRequestID: "wire-prompt-3", Text: "hi"}, 409)
	// omp reports its session at start but saves it with the first
	// message: the switch is refused as not listed.
	deadline := time.Now().Add(chatTimeout)
	for terminal.Info().Hybrid.AgentSessionID == nil {
		if time.Now().After(deadline) {
			f.t.Fatal("terminal reported no agent session")
		}
		time.Sleep(10 * time.Millisecond)
	}
	f.call("hybrid-conflict-not-eligible", "POST", "/api/sessions/"+terminal.Info().ID+"/switch", contract.HybridSwitchRequest{ClientRequestID: "wire-switch-1", To: contract.HybridSideACP}, 409)
	chat := fakeChat(f.t, f.hybrids, f.root, agentprofile.OMP)
	f.call("hybrid-switch", "POST", "/api/sessions/"+chat+"/switch", contract.HybridSwitchRequest{ClientRequestID: "wire-switch-2", To: contract.HybridSideTUI}, 200)
}

// wireHybridRecords pins the record list, the resume conflicts (a deleted
// launch config, a vanished agent session with its candidates), a resumed
// session and the removal of a record.
func wireHybridRecords(f *wireFixture) {
	agent := fakeAgentPath(f.t, agentprofile.OMP)
	f.norm.path(filepath.Dir(agent), "<fakeagent>")
	cwd, err := filepath.EvalSymlinks(f.root)
	if err != nil {
		f.t.Fatal(err)
	}
	f.norm.path(cwd, "<repo>")
	store := f.t.TempDir()
	saved, err := f.launches.Create(launch.Input{Name: "agent", Cmd: []string{agent}, Env: map[string]string{"FAKEAGENT_STORE": store}, Hybrid: true})
	if err != nil {
		f.t.Fatal(err)
	}
	// Like elsewhere, a launch config ID is a session-style ID.
	f.norm.id("id", saved.ID)
	ctx, cancel := context.WithTimeout(f.ctx, chatTimeout)
	defer cancel()
	item, err := f.hybrids.Create(ctx, hybrid.CreateInput{Cwd: f.root, Name: "agent", ConfigID: saved.ID, Cmd: saved.Cmd, Env: saved.Env, StartSide: hybrid.SideACP})
	if err != nil {
		f.t.Fatal(err)
	}
	id, sid := item.Info().ID, *item.Info().Hybrid.AgentSessionID
	closeShell := func() {
		if err := f.manager.Close(id); err != nil {
			f.t.Fatal(err)
		}
		select {
		case <-item.Done():
		case <-ctx.Done():
			f.t.Fatal("session did not close")
		}
	}
	closeShell()
	f.call("hybrid-records", "GET", "/api/hybrid-sessions?state=recoverable", nil, 200)

	// The agent lost the session; another one in the directory is offered.
	if err := os.Remove(filepath.Join(store, "sessions", sid+".json")); err != nil {
		f.t.Fatal(err)
	}
	candidate := fmt.Sprintf(`{"id":"wirecandidate","cwd":%q,"title":"Earlier work","updatedAt":"2026-01-02T03:04:05Z","messages":[],"config":{"mode":"code","model":"fast"}}`, cwd)
	if err := os.WriteFile(filepath.Join(store, "sessions", "wirecandidate.json"), []byte(candidate), 0o600); err != nil {
		f.t.Fatal(err)
	}
	path := "/api/sessions/" + id + "/resume"
	f.call("hybrid-resume-session-missing", "POST", path, contract.HybridResumeRequest{ClientRequestID: "wire-resume-1", Side: contract.HybridSideACP}, 409)
	f.call("hybrid-resume", "POST", path, contract.HybridResumeRequest{ClientRequestID: "wire-resume-2", Side: contract.HybridSideACP, AgentSessionID: "wirecandidate"}, 200)
	if item, err = f.manager.Get(id); err != nil {
		f.t.Fatal(err)
	}
	closeShell()
	if err := f.launches.Delete(saved.ID); err != nil {
		f.t.Fatal(err)
	}
	f.call("hybrid-resume-config-missing", "POST", path, contract.HybridResumeRequest{ClientRequestID: "wire-resume-3", Side: contract.HybridSideTUI}, 409)
	f.call("hybrid-record-delete", "DELETE", "/api/hybrid-sessions/"+id, nil, 204)
}

// closed reads until the server closes conn and records the close code and
// reason.
func (f *wireFixture) closed(conn *websocket.Conn, name string) {
	f.t.Helper()
	for {
		_, _, err := conn.Read(f.ctx)
		if err == nil {
			continue
		}
		var closeError websocket.CloseError
		if !errors.As(err, &closeError) {
			f.t.Fatalf("%s: %v", name, err)
		}
		data, err := json.Marshal(struct {
			Code   int    `json:"code"`
			Reason string `json:"reason"`
		}{int(closeError.Code), closeError.Reason})
		if err != nil {
			f.t.Fatal(err)
		}
		f.record(name, 0, data)
		return
	}
}
