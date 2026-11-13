package mcphttp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
)

func cancelCall(h *Handler, session, id string) *httptest.ResponseRecorder {
	return post(h, session, `{"jsonrpc":"2.0","method":"notifications/cancelled","params":{"requestId":`+id+`}}`)
}

func TestToolCapacityTypedDuplicatesAndControlBypass(t *testing.T) {
	entered := make(chan context.Context, 16)
	release := make(chan struct{})
	h, _ := testHandler(t, func(ctx context.Context, _ json.RawMessage) (any, error) {
		entered <- ctx
		select {
		case <-release:
			return struct{}{}, nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	})
	t.Cleanup(func() { close(release) })
	a := initialize(t, h, "2025-11-25", true)
	b := initialize(t, h, "2025-11-25", true)
	c := initialize(t, h, "2025-11-25", true)
	var pending []<-chan *httptest.ResponseRecorder
	for _, id := range []string{`1`, `"1"`, `2`, `3`} {
		pending = append(pending, startCall(h, a, id, context.Background()))
		receive(t, entered)
	}
	requireStatus(t, receive(t, startCall(h, a, `1.0`, context.Background())), http.StatusConflict)
	w := receive(t, startCall(h, a, `4`, context.Background()))
	requireStatus(t, w, http.StatusTooManyRequests)
	if w.Header().Get("Retry-After") != "1" {
		t.Fatal("saturation must request bounded retry")
	}
	for n := 1; n <= 4; n++ {
		pending = append(pending, startCall(h, b, strconv.Itoa(n), context.Background()))
		receive(t, entered)
	}
	requireStatus(t, receive(t, startCall(h, c, `1`, context.Background())), http.StatusTooManyRequests)
	requireStatus(t, post(h, a, `{"jsonrpc":"2.0","id":"ping","method":"ping"}`), http.StatusOK)
	requireStatus(t, post(h, a, `{"jsonrpc":"2.0","id":"list","method":"tools/list"}`), http.StatusOK)
	requireStatus(t, post(h, a, `{"jsonrpc":"2.0","method":"notifications/unknown","params":{}}`), http.StatusAccepted)
	requireStatus(t, cancelCall(h, a, `1e0`), http.StatusAccepted)
	response := requireError(t, receive(t, pending[0]), http.StatusOK, -32000)
	if string(response.Error.Data) != `{"kind":"request_cancelled"}` {
		t.Fatal("cancellation does not distinguish observation from task failure")
	}
	// Cancellation releases one actual execution slot, not just its ID index.
	next := startCall(h, c, `1`, context.Background())
	receive(t, entered)
	requireStatus(t, cancelCall(h, c, `1`), http.StatusAccepted)
	requireError(t, receive(t, next), http.StatusOK, -32000)
	// DELETE bypasses the saturated tool quota and joins only its own work.
	requireStatus(t, serve(h, request(context.Background(), http.MethodDelete, b, "")), http.StatusNoContent)
	for _, done := range pending[4:] {
		requireError(t, receive(t, done), http.StatusOK, -32000)
	}
	for _, id := range []string{`"1"`, `2`, `3`} {
		requireStatus(t, cancelCall(h, a, id), http.StatusAccepted)
	}
	for _, done := range pending[1:4] {
		requireError(t, receive(t, done), http.StatusOK, -32000)
	}
}

func TestCancellationIsScopedAndDisconnectDoesNotCancel(t *testing.T) {
	entered := make(chan context.Context, 4)
	release := make(chan struct{})
	h, _ := testHandler(t, func(ctx context.Context, _ json.RawMessage) (any, error) {
		entered <- ctx
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-release:
			return struct{}{}, nil
		}
	})
	t.Cleanup(func() { close(release) })
	a := initialize(t, h, "2025-11-25", true)
	b := initialize(t, h, "2025-11-25", true)
	incoming, disconnect := context.WithCancel(context.Background())
	defer disconnect()
	first := startCall(h, a, `7`, incoming)
	firstCtx := receive(t, entered)
	second := startCall(h, b, `7`, context.Background())
	secondCtx := receive(t, entered)
	disconnect()
	if firstCtx.Err() != nil || secondCtx.Err() != nil {
		t.Fatal("HTTP disconnect leaked into accepted operation context")
	}
	requireStatus(t, cancelCall(h, a, `7`), http.StatusAccepted)
	requireError(t, receive(t, first), http.StatusOK, -32000)
	if firstCtx.Err() == nil || secondCtx.Err() != nil {
		t.Fatal("cancel did not remain scoped to one transport session")
	}
	requireStatus(t, cancelCall(h, a, `7`), http.StatusAccepted)
	requireStatus(t, cancelCall(h, a, `999`), http.StatusAccepted)
	requireStatus(t, cancelCall(h, b, `7`), http.StatusAccepted)
	requireError(t, receive(t, second), http.StatusOK, -32000)
}

func TestDisconnectedOperationCanFinishSuccessfully(t *testing.T) {
	entered := make(chan context.Context, 1)
	release := make(chan struct{})
	h, _ := testHandler(t, func(ctx context.Context, _ json.RawMessage) (any, error) {
		entered <- ctx
		select {
		case <-release:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
		return struct {
			Committed bool `json:"committed"`
		}{true}, nil
	})
	session := initialize(t, h, "2025-11-25", true)
	ctx, disconnect := context.WithCancel(context.Background())
	done := startCall(h, session, `1`, ctx)
	operation := receive(t, entered)
	disconnect()
	if operation.Err() != nil {
		t.Fatal("disconnect cancelled a committed-capable operation")
	}
	close(release)
	w := receive(t, done)
	requireStatus(t, w, http.StatusOK)
	var response contractResponseResult
	if json.Unmarshal(w.Body.Bytes(), &response) != nil || response.Error != nil || response.Result.IsError {
		t.Fatal("disconnected operation did not finish normally")
	}
	// Once completion won, a late cancellation is an idempotent no-op. The
	// same RPC ID can execute again; this transport is not a response cache.
	requireStatus(t, cancelCall(h, session, `1`), http.StatusAccepted)
	requireStatus(t, receive(t, startCall(h, session, `1`, context.Background())), http.StatusOK)
}

type contractResponseResult struct {
	Error  json.RawMessage `json:"error"`
	Result struct {
		IsError bool `json:"isError"`
	} `json:"result"`
}
