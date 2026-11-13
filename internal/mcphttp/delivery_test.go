package mcphttp

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"
)

const deliveryInitialize = `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"delivery","version":"1"}}}`

type blockedDeliveryWriter struct {
	*httptest.ResponseRecorder
	entered chan<- struct{}
	release <-chan struct{}
}

func (w *blockedDeliveryWriter) Write(data []byte) (int, error) {
	w.entered <- struct{}{}
	<-w.release
	return w.ResponseRecorder.Write(data)
}

func TestDeliveryRetainsToolSlotsAndRequestIDs(t *testing.T) {
	h, _ := testHandler(t, nil)
	sessions := []string{
		initialize(t, h, "2025-11-25", true),
		initialize(t, h, "2025-11-25", true),
		initialize(t, h, "2025-11-25", true),
	}
	entered := make(chan struct{}, maxGlobalTools)
	release := make(chan struct{})
	done := make(chan *httptest.ResponseRecorder, maxGlobalTools)
	pending, released := 0, false
	t.Cleanup(func() {
		if !released {
			close(release)
		}
		for range pending {
			receive(t, done)
		}
	})
	start := func(session, id string) {
		w := &blockedDeliveryWriter{ResponseRecorder: httptest.NewRecorder(), entered: entered, release: release}
		pending++
		go func() {
			h.ServeHTTP(w, request(context.Background(), http.MethodPost, session,
				`{"jsonrpc":"2.0","id":`+id+`,"method":"tools/call","params":{"name":"observe","arguments":{}}}`))
			done <- w.ResponseRecorder
		}()
		receive(t, entered)
	}
	for i := range maxSessionTools {
		start(sessions[0], strconv.Itoa(i))
	}
	requireStatus(t, receive(t, startCall(h, sessions[0], `0`, context.Background())), http.StatusConflict)
	requireStatus(t, receive(t, startCall(h, sessions[0], `99`, context.Background())), http.StatusTooManyRequests)
	for i := range maxSessionTools {
		start(sessions[1], strconv.Itoa(i))
	}
	requireStatus(t, receive(t, startCall(h, sessions[2], `99`, context.Background())), http.StatusTooManyRequests)
	// Control messages bypass occupied tool slots; a committed result wins late cancellation.
	requireStatus(t, post(h, sessions[0], ping), http.StatusOK)
	requireStatus(t, post(h, sessions[0], `{"jsonrpc":"2.0","method":"notifications/cancelled","params":{"requestId":0}}`), http.StatusAccepted)
	close(release)
	released = true
	for range pending {
		w := receive(t, done)
		requireStatus(t, w, http.StatusOK)
		var response contractResponseResult
		if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil || response.Error != nil || response.Result.IsError {
			t.Fatalf("late cancellation replaced committed result: %s, %v", w.Body.String(), err)
		}
	}
	pending = 0
	requireStatus(t, receive(t, startCall(h, sessions[0], `0`, context.Background())), http.StatusOK)
}

type failedInitializationWriter struct {
	*httptest.ResponseRecorder
	stage string
}

func (w *failedInitializationWriter) Write(data []byte) (int, error) {
	if w.stage == "write" {
		return 0, io.ErrClosedPipe
	}
	if w.stage == "short-write" {
		return len(data) - 1, nil
	}
	return w.ResponseRecorder.Write(data)
}

func (w *failedInitializationWriter) FlushError() error {
	if w.stage == "flush" {
		return io.ErrClosedPipe
	}
	w.Flush()
	return nil
}

func TestFailedInitializationDeliveryRemovesAllocatedSession(t *testing.T) {
	for _, stage := range []string{"write", "short-write", "flush"} {
		t.Run(stage, func(t *testing.T) {
			h, _ := testHandler(t, nil)
			w := &failedInitializationWriter{ResponseRecorder: httptest.NewRecorder(), stage: stage}
			h.ServeHTTP(w, request(context.Background(), http.MethodPost, "", deliveryInitialize))
			lost := w.Header().Get("Mcp-Session-Id")
			if lost == "" {
				t.Fatal("fixture did not reach initialized response delivery")
			}
			requireStatus(t, post(h, lost, ping), http.StatusNotFound)
			retried := initialize(t, h, "2025-11-25", true)
			if retried == lost {
				t.Fatal("retry reused a failed handshake")
			}
		})
	}
}

func TestHardExpiryWinsWithoutScannerScheduling(t *testing.T) {
	var clock *testClock
	remaining := 20 * time.Millisecond
	h, timer := testHandler(t, func(ctx context.Context, _ json.RawMessage) (any, error) {
		<-ctx.Done()
		clock.advance(remaining)
		return nil, ctx.Err()
	})
	clock = timer
	session := initialize(t, h, "2025-11-25", true)
	start := clock.now()
	for clock.now().Sub(start) < hardTimeout-remaining {
		step := min(idleTimeout/2, hardTimeout-remaining-clock.now().Sub(start))
		clock.advance(step)
		requireStatus(t, post(h, session, ping), http.StatusOK)
	}
	// The test handler has no scanner ticks; only the operation's hard deadline fires.
	w := receive(t, startCall(h, session, `17`, context.Background()))
	response := requireError(t, w, http.StatusOK, -32000)
	var cause struct {
		Kind string `json:"kind"`
	}
	if err := json.Unmarshal(response.Error.Data, &cause); err != nil || cause.Kind != "request_cancelled" {
		t.Fatalf("hard expiry lost cancellation classification: %s, %v", response.Error.Data, err)
	}
	requireStatus(t, post(h, session, ping), http.StatusNotFound)
}
