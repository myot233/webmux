package mcphttp

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"
)

const ping = `{"jsonrpc":"2.0","id":"ping","method":"ping"}`

func TestLifecycleSchemasOrderingAndVersions(t *testing.T) {
	h, _ := testHandler(t, nil)
	for _, params := range []string{
		`{}`, `{"protocolVersion":"2025-11-25"}`,
		`{"protocolVersion":"2025-11-25","capabilities":[],"clientInfo":{"name":"n","version":"v"}}`,
		`{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"","version":"v"}}`,
		`{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":null}`,
	} {
		requireError(t, post(h, "", `{"jsonrpc":"2.0","id":1,"method":"initialize","params":`+params+`}`), 400, -32602)
	}
	for _, version := range []string{"2025-03-26", "2025-06-18", "2025-11-25", "2024-11-05", "future"} {
		t.Run(version, func(t *testing.T) {
			session := initialize(t, h, version, false)
			wantVersion := version
			if version == "future" || version == "2024-11-05" {
				wantVersion = "2025-11-25"
			}
			r := request(context.Background(), http.MethodPost, session, ping)
			r.Header.Set("Mcp-Protocol-Version", wantVersion)
			requireStatus(t, serve(h, r), http.StatusOK)
			requireError(t, post(h, session, `{"jsonrpc":"2.0","id":2,"method":"tools/list"}`), 400, -32600)
			requireStatus(t, post(h, session, `{"jsonrpc":"2.0","method":"notifications/initialized","params":{"_meta":{}}}`), 202)
			requireStatus(t, post(h, session, `{"jsonrpc":"2.0","id":2,"method":"tools/list"}`), 200)
			requireError(t, post(h, session, `{"jsonrpc":"2.0","id":3,"method":"server/discover"}`), 200, -32601)
			requireError(t, post(h, session, `{"jsonrpc":"2.0","id":4,"method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"n","version":"v"}}}`), 400, -32600)
		})
	}
}

func TestSessionCapacityDeleteAndRestartNamespace(t *testing.T) {
	h, _ := testHandler(t, nil)
	var first string
	for range maxSessions {
		id := initialize(t, h, "2025-11-25", true)
		if first == "" {
			first = id
		}
	}
	w := post(h, "", `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"n","version":"v"}}}`)
	requireStatus(t, w, http.StatusTooManyRequests)
	if w.Header().Get("Retry-After") != "1" {
		t.Fatal("missing session saturation retry bound")
	}
	w = serve(h, request(context.Background(), http.MethodDelete, first, ""))
	requireStatus(t, w, http.StatusNoContent)
	if w.Body.Len() != 0 {
		t.Fatal("DELETE must not return JSON")
	}
	requireStatus(t, post(h, first, ping), http.StatusNotFound)
	initialize(t, h, "2025-11-25", true)
	other, _ := testHandler(t, nil)
	requireStatus(t, post(other, first, ping), http.StatusNotFound)
	unknown := strings.Repeat("A", 43)
	requireStatus(t, post(h, unknown, ping), http.StatusNotFound)
}

func TestAwaitingIdleAndHardExpiry(t *testing.T) {
	t.Run("awaiting-initialized", func(t *testing.T) {
		h, clock := testHandler(t, nil)
		session := initialize(t, h, "2025-11-25", false)
		clock.advance(readyTimeout - time.Nanosecond)
		requireStatus(t, post(h, session, ping), 200)
		clock.advance(time.Nanosecond)
		requireStatus(t, post(h, session, ping), 404)
	})
	t.Run("idle-since-last-completion", func(t *testing.T) {
		entered := make(chan context.Context, 1)
		release := make(chan struct{})
		h, clock := testHandler(t, func(ctx context.Context, _ json.RawMessage) (any, error) {
			entered <- ctx
			select {
			case <-release:
				return struct{}{}, nil
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		})
		session := initialize(t, h, "2025-11-25", true)
		done := startCall(h, session, `1`, context.Background())
		receive(t, entered)
		clock.advance(idleTimeout + time.Second)
		requireStatus(t, post(h, session, ping), 200)
		close(release)
		requireStatus(t, receive(t, done), 200)
		clock.advance(idleTimeout - time.Nanosecond)
		// GET is not an accepted operation and does not extend idle lifetime.
		requireStatus(t, serve(h, request(context.Background(), http.MethodGet, session, "")), 405)
		clock.advance(time.Nanosecond)
		requireStatus(t, post(h, session, ping), 404)
	})
	t.Run("hard-expiry-cancels-active-work", func(t *testing.T) {
		entered := make(chan context.Context, 1)
		h, clock := testHandler(t, func(ctx context.Context, _ json.RawMessage) (any, error) {
			entered <- ctx
			<-ctx.Done()
			return nil, ctx.Err()
		})
		session := initialize(t, h, "2025-11-25", true)
		done := startCall(h, session, `1`, context.Background())
		op := receive(t, entered)
		clock.advance(hardTimeout)
		requireStatus(t, post(h, session, ping), 404)
		requireError(t, receive(t, done), 200, -32000)
		if op.Err() == nil {
			t.Fatal("hard expiry did not cancel operation")
		}
	})
}

func TestCloseCancelsAndJoinsCleanup(t *testing.T) {
	entered := make(chan context.Context, 1)
	cleaned := make(chan struct{})
	h, _ := testHandler(t, func(ctx context.Context, _ json.RawMessage) (any, error) {
		entered <- ctx
		<-ctx.Done()
		close(cleaned)
		return nil, ctx.Err()
	})
	session := initialize(t, h, "2025-11-25", true)
	done := startCall(h, session, `1`, context.Background())
	receive(t, entered)
	h.Close()
	select {
	case <-cleaned:
	default:
		t.Fatal("Close returned before operation cleanup")
	}
	requireError(t, receive(t, done), 200, -32000)
	h.Close()
	requireStatus(t, post(h, session, ping), http.StatusServiceUnavailable)
}
