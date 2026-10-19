package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/coder/websocket"
	"github.com/rainchan/webmux/internal/api"
	"github.com/rainchan/webmux/internal/config"
	"github.com/rainchan/webmux/internal/session"
	"github.com/rainchan/webmux/internal/workspace"
)

const token = "baseline-local-fixture"
const capacity = 2 * 1024 * 1024

type fixture struct {
	closeOnce   sync.Once
	stopSignals context.CancelFunc
	root        string
	manager     *session.Manager
	workspace   *workspace.Service
	server      *httptest.Server
	client      *http.Client
}

func newFixture() *fixture {
	root, err := os.MkdirTemp("", "webmux-baseline-")
	must(err)
	root, err = filepath.EvalSymlinks(root)
	must(err)
	f := &fixture{root: root, client: &http.Client{Timeout: 15 * time.Second}}
	c, err := config.Load(filepath.Join(root, "absent.toml"), false)
	must(err)
	c.DefaultCwd = root
	c.StateDir = filepath.Join(root, "state")
	c.Token = token
	c.ScrollbackBytes = capacity
	c.Shell = "/bin/bash"
	must(c.Validate())
	f.manager = session.NewManager(c.SessionOptions())
	f.workspace = workspace.New(c.StateDir, f.manager, root)
	f.server = httptest.NewServer(api.New(c, f.manager, f.workspace))
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	f.stopSignals = stop
	go func() { <-ctx.Done(); f.close() }()
	return f
}
func (f *fixture) close() {
	f.closeOnce.Do(func() {
		f.stopSignals()
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		f.manager.Shutdown(ctx)
		f.server.Close()
		f.client.CloseIdleConnections()
		f.workspace.Close()
		_ = os.RemoveAll(f.root)
	})
}
func (f *fixture) request(method, path string, body any, want int) []byte {
	b, err := json.Marshal(body)
	must(err)
	r, err := http.NewRequest(method, f.server.URL+path, bytes.NewReader(b))
	must(err)
	r.Header.Set("Authorization", "Bearer "+token)
	r.Header.Set("Content-Type", "application/json")
	resp, err := f.client.Do(r)
	must(err)
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	must(err)
	check(resp.StatusCode == want, "%s %s: got %d want %d: %s", method, path, resp.StatusCode, want, data)
	return data
}
func decode[T any](data []byte) T { var v T; must(json.Unmarshal(data, &v)); return v }
func (f *fixture) create(script string) string {
	v := decode[session.Info](f.request("POST", "/api/sessions", map[string]any{"preset": "custom", "cmd": []string{"/bin/bash", "--noprofile", "--norc", "-c", script}}, 201))
	return v.ID
}
func (f *fixture) info(id string) session.Info {
	return decode[session.Info](f.request("GET", "/api/sessions/"+id, nil, 200))
}
func (f *fixture) dispose(id string) {
	_ = f.manager.Close(id)
	s, err := f.manager.Get(id)
	if err != nil {
		return
	}
	select {
	case <-s.Done():
		_ = f.manager.Remove(id)
	case <-time.After(10 * time.Second):
	}
}
func (f *fixture) dial(id string) *websocket.Conn {
	ctx, cancel := timeout()
	defer cancel()
	c, resp, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(f.server.URL, "http")+"/ws/s/"+id, &websocket.DialOptions{HTTPHeader: http.Header{"Authorization": []string{"Bearer " + token}}})
	if err != nil && resp != nil {
		panic(fmt.Errorf("dial: HTTP %d: %w", resp.StatusCode, err))
	}
	must(err)
	c.SetReadLimit(capacity + 65536)
	return c
}
func read(c *websocket.Conn) []byte {
	ctx, cancel := timeout()
	defer cancel()
	return readContext(ctx, c)
}
func readContext(ctx context.Context, c *websocket.Conn) []byte {
	k, p, err := c.Read(ctx)
	must(err)
	check(k == websocket.MessageBinary && len(p) > 0, "invalid terminal frame")
	return p
}
func write(c *websocket.Conn, p []byte) {
	ctx, cancel := timeout()
	defer cancel()
	must(c.Write(ctx, websocket.MessageBinary, session.Frame(0, p)))
}
func until(c *websocket.Conn, marker []byte) []byte {
	ctx, cancel := timeout()
	defer cancel()
	var output []byte
	for !bytes.Contains(output, marker) {
		p := readContext(ctx, c)
		if p[0] == 0 || p[0] == 1 {
			check(len(output)+len(p)-1 <= capacity+65536, "unexpected excess terminal output")
			output = append(output, p[1:]...)
		}
		check(p[0] != 3, "process exited before expected output")
	}
	return output
}
func (f *fixture) await(id string, predicate func(session.Info) bool) session.Info {
	deadline := time.Now().Add(15 * time.Second)
	for {
		v := f.info(id)
		if predicate(v) {
			return v
		}
		check(time.Now().Before(deadline), "session condition timed out: %+v", v)
		time.Sleep(5 * time.Millisecond)
	}
}
func (f *fixture) git(args ...string) {
	cmd := exec.Command("git", args...)
	cmd.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL="+os.DevNull, "GIT_AUTHOR_NAME=Baseline", "GIT_AUTHOR_EMAIL=baseline@example.invalid", "GIT_COMMITTER_NAME=Baseline", "GIT_COMMITTER_EMAIL=baseline@example.invalid")
	b, err := cmd.CombinedOutput()
	check(err == nil, "git fixture: %v: %s", err, b)
}
