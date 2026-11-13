package mcphttp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestMalformedInputCannotExecute(t *testing.T) {
	var calls atomic.Int32
	h, _ := testHandler(t, func(context.Context, json.RawMessage) (any, error) {
		calls.Add(1)
		return struct{}{}, nil
	})
	session := initialize(t, h, "2025-11-25", true)
	for _, tc := range []struct {
		name, body string
		code       int
	}{
		{"syntax", `{"jsonrpc":`, -32700},
		{"invalid-utf8", "{\"method\":\"\xff\"}", -32700},
		{"batch", `[{"jsonrpc":"2.0","id":1,"method":"tools/list"}]`, -32600},
		{"null-envelope", `null`, -32600},
		{"duplicate", `{"jsonrpc":"2.0","id":1,"method":"tools/list","method":"tools/call"}`, -32600},
		{"nested-duplicate", `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"observe","arguments":{"x":1,"x":2}}}`, -32600},
		{"wrong-version", `{"jsonrpc":"1.0","id":1,"method":"tools/list"}`, -32600},
		{"numeric-version", `{"jsonrpc":2,"id":1,"method":"tools/list"}`, -32600},
		{"object-method", `{"jsonrpc":"2.0","id":1,"method":{}}`, -32600},
		{"scalar-params", `{"jsonrpc":"2.0","id":1,"method":"unknown","params":1}`, -32602},
		{"response", `{"jsonrpc":"2.0","id":1,"result":{}}`, -32600},
		{"null-id", `{"jsonrpc":"2.0","id":null,"method":"tools/list"}`, -32600},
		{"fractional-id", `{"jsonrpc":"2.0","id":0.1,"method":"tools/list"}`, -32600},
		{"unsafe-id", `{"jsonrpc":"2.0","id":9007199254740992,"method":"tools/list"}`, -32600},
		{"empty-string-id", `{"jsonrpc":"2.0","id":"","method":"tools/list"}`, -32600},
		{"long-string-id", `{"jsonrpc":"2.0","id":"` + strings.Repeat("x", 129) + `","method":"tools/list"}`, -32600},
		{"invalid-list-cursor", `{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{"cursor":"next"}}`, -32602},
		{"null-initialized", `{"jsonrpc":"2.0","method":"notifications/initialized","params":null}`, -32602},
		{"cancel-unknown-field", `{"jsonrpc":"2.0","method":"notifications/cancelled","params":{"requestId":1,"extra":true}}`, -32602},
		{"cancel-null-reason", `{"jsonrpc":"2.0","method":"notifications/cancelled","params":{"requestId":1,"reason":null}}`, -32602},
		{"cancel-long-reason", `{"jsonrpc":"2.0","method":"notifications/cancelled","params":{"requestId":1,"reason":"` + strings.Repeat("x", 257) + `"}}`, -32602},
	} {
		t.Run(tc.name, func(t *testing.T) {
			requireError(t, post(h, session, tc.body), http.StatusBadRequest, tc.code)
		})
	}
	if calls.Load() != 0 {
		t.Fatal("malformed input executed a tool")
	}
	requireStatus(t, post(h, session, strings.Repeat(" ", maxInput+1)), http.StatusRequestEntityTooLarge)
}

func TestFramingHeadersAndFailClosedOwner(t *testing.T) {
	h, _ := testHandler(t, nil)
	session := initialize(t, h, "2025-06-18", true)
	body := `{"jsonrpc":"2.0","id":"p","method":"ping"}`
	for _, tc := range []struct {
		name   string
		mutate func(*http.Request)
		status int
	}{
		{"non-json", func(r *http.Request) { r.Header.Set("Content-Type", "text/plain") }, 415},
		{"duplicate-content-type", func(r *http.Request) { r.Header.Add("Content-Type", "application/json") }, 415},
		{"missing-accept", func(r *http.Request) { r.Header.Del("Accept") }, 406},
		{"json-only-accept", func(r *http.Request) { r.Header.Set("Accept", "application/json") }, 406},
		{"zero-stream-quality", func(r *http.Request) { r.Header.Set("Accept", "application/json, text/event-stream;q=0") }, 406},
		{"duplicate-session", func(r *http.Request) { r.Header.Add("Mcp-Session-Id", session) }, 400},
		{"comma-session", func(r *http.Request) { r.Header.Set("Mcp-Session-Id", session+","+session) }, 400},
		{"malformed-session", func(r *http.Request) { r.Header.Set("Mcp-Session-Id", strings.Repeat("!", 43)) }, 400},
		{"missing-session", func(r *http.Request) { r.Header.Del("Mcp-Session-Id") }, 400},
		{"wrong-version", func(r *http.Request) { r.Header.Set("Mcp-Protocol-Version", "2025-11-25") }, 400},
		{"duplicate-version", func(r *http.Request) { r.Header["Mcp-Protocol-Version"] = []string{"2025-06-18", "2025-06-18"} }, 400},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := request(context.Background(), http.MethodPost, session, body)
			tc.mutate(r)
			requireStatus(t, serve(h, r), tc.status)
		})
	}
	// Explicit rejection never poisons the negotiated session.
	requireStatus(t, post(h, session, body), http.StatusOK)
	r := httptest.NewRequest(http.MethodPost, "http://webmux/mcp", strings.NewReader(body))
	r.Header.Set("Mcp-Session-Id", session)
	r.Header.Set("X-WebMux-Owner", "true")
	r.Header.Set("Authorization", "Bearer forged")
	w := serve(h, r)
	requireStatus(t, w, http.StatusUnauthorized)
	if w.Header().Get("WWW-Authenticate") != `Bearer realm="webmux"` {
		t.Fatal("missing plain Bearer challenge")
	}
	for _, method := range []string{http.MethodGet, http.MethodPut, http.MethodPatch} {
		w := serve(h, request(context.Background(), method, session, ""))
		requireStatus(t, w, http.StatusMethodNotAllowed)
		if w.Header().Get("Allow") != "POST, DELETE" || w.Body.Len() != 0 {
			t.Fatal("method rejection is not the JSON-only profile")
		}
	}
}

func TestSafeTypedIDs(t *testing.T) {
	for _, tc := range []struct {
		raw, key string
		valid    bool
	}{
		{`1`, "n:1", true}, {`1.0`, "n:1", true}, {`10e-1`, "n:1", true},
		{`-0e99999999999999999999`, "n:0", true}, {`"1"`, "s:1", true},
		{`9007199254740991`, "n:9007199254740991", true},
		{`-9007199254740991`, "n:-9007199254740991", true},
		{`9007199254740991.1`, "", false}, {`1e999999999999999999`, "", false},
		{`1e-999999999999999999`, "", false}, {`null`, "", false}, {`{}`, "", false},
		{`"\ud800"`, "", false}, {`"\udfff"`, "", false},
		{`"\ud83d\ude00"`, "s:\U0001f600", true},
		{`"\\ud800"`, `s:\ud800`, true},
	} {
		key, valid := rpcID(json.RawMessage(tc.raw))
		if key != tc.key || valid != tc.valid {
			t.Errorf("ID %s => (%q,%v), want (%q,%v)", tc.raw, key, valid, tc.key, tc.valid)
		}
	}
}
