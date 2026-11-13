package mcphttp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/rainchan/webmux/internal/contract"
	"github.com/rainchan/webmux/internal/mcpprotocol"
)

type testClock struct {
	mu   sync.Mutex
	time time.Time
}

func (c *testClock) now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.time
}

func (c *testClock) advance(d time.Duration) {
	c.mu.Lock()
	c.time = c.time.Add(d)
	c.mu.Unlock()
}

func testHandler(t *testing.T, call func(context.Context, json.RawMessage) (any, error)) (*Handler, *testClock) {
	t.Helper()
	if call == nil {
		call = func(context.Context, json.RawMessage) (any, error) { return struct{}{}, nil }
	}
	registry, err := mcpprotocol.New(mcpprotocol.Config{Name: "transport-test", Instructions: "Observe the test operation.",
		CallTimeout: callTimeout, Versions: []string{"2025-03-26", "2025-06-18", "2025-11-25"},
		Tools: []mcpprotocol.Tool{{Definition: map[string]any{"name": "observe", "description": "Observe one bounded operation.",
			"inputSchema": map[string]any{"type": "object", "properties": map[string]any{}}}, Call: call}}})
	if err != nil {
		t.Fatal(err)
	}
	clock := &testClock{time: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}
	h := newHandler(context.Background(), registry, clock.now, make(chan time.Time), func() {})
	t.Cleanup(h.Close)
	return h, clock
}

func request(ctx context.Context, method, session, body string) *http.Request {
	r := httptest.NewRequest(method, "http://webmux/mcp", strings.NewReader(body))
	r = r.WithContext(contract.WithMCPOwner(ctx))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Accept", "application/json, text/event-stream")
	if session != "" {
		r.Header.Set("Mcp-Session-Id", session)
	}
	return r
}

func serve(h *Handler, r *http.Request) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func post(h *Handler, session, body string) *httptest.ResponseRecorder {
	return serve(h, request(context.Background(), http.MethodPost, session, body))
}

func requireStatus(t *testing.T, w *httptest.ResponseRecorder, status int) {
	t.Helper()
	if w.Code != status {
		t.Fatalf("HTTP status = %d, want %d; body=%s", w.Code, status, w.Body.String())
	}
	if w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("missing no-store")
	}
}

func requireError(t *testing.T, w *httptest.ResponseRecorder, status, code int) contract.MCPResponse {
	t.Helper()
	requireStatus(t, w, status)
	var response contract.MCPResponse
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.Error == nil || response.Error.Code != code {
		t.Fatalf("response error = %#v, want %d", response.Error, code)
	}
	return response
}

func initialize(t *testing.T, h *Handler, version string, ready bool) string {
	t.Helper()
	w := post(h, "", fmt.Sprintf(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":%q,"capabilities":{},"clientInfo":{"name":"remote","version":"1"}}}`, version))
	requireStatus(t, w, http.StatusOK)
	id := w.Header().Get("Mcp-Session-Id")
	if len(id) != 43 {
		t.Fatalf("invalid session header length %d", len(id))
	}
	if ready {
		requireStatus(t, post(h, id, `{"jsonrpc":"2.0","method":"notifications/initialized"}`), http.StatusAccepted)
	}
	return id
}

func startCall(h *Handler, session, id string, ctx context.Context) <-chan *httptest.ResponseRecorder {
	done := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		done <- serve(h, request(ctx, http.MethodPost, session,
			`{"jsonrpc":"2.0","id":`+id+`,"method":"tools/call","params":{"name":"observe","arguments":{}}}`))
	}()
	return done
}

func receive[T any](t *testing.T, channel <-chan T) T {
	t.Helper()
	select {
	case result := <-channel:
		return result
	case <-time.After(3 * time.Second):
		t.Fatal("bounded operation did not complete")
		var zero T
		return zero
	}
}
