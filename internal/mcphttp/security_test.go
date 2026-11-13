package mcphttp

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func TestNotificationsNeverExecuteTools(t *testing.T) {
	var calls atomic.Int32
	h, _ := testHandler(t, func(context.Context, json.RawMessage) (any, error) {
		calls.Add(1)
		return struct{}{}, nil
	})
	session := initialize(t, h, "2025-11-25", true)
	for _, body := range []string{
		`{"jsonrpc":"2.0","method":"tools/call","params":{"name":"observe","arguments":{}}}`,
		`{"jsonrpc":"2.0","method":"notifications/unknown","params":{"_meta":{"claim":"owner"}}}`,
		`{"jsonrpc":"2.0","method":"notifications/cancelled","params":{"requestId":"unknown","reason":"do not store"}}`,
	} {
		w := post(h, session, body)
		requireStatus(t, w, http.StatusAccepted)
		if w.Body.Len() != 0 {
			t.Fatal("accepted notification returned a response")
		}
	}
	if calls.Load() != 0 {
		t.Fatal("notification executed a tool")
	}
}

func TestRejectedEnvelopeEchoesOnlySafeID(t *testing.T) {
	h, _ := testHandler(t, nil)
	session := initialize(t, h, "2025-11-25", true)
	for _, tc := range []struct{ body, id string }{
		{`{"jsonrpc":"wrong","id":"safe","method":"ping"}`, `"safe"`},
		{`{"jsonrpc":"2.0","id":9007199254740992,"method":"ping"}`, `null`},
		{`{"jsonrpc":"2.0","id":{},"method":"ping"}`, `null`},
		{`{"jsonrpc":"2.0","method":"ping","id":1,"id":2}`, `null`},
	} {
		response := requireError(t, post(h, session, tc.body), http.StatusBadRequest, -32600)
		if string(response.ID) != tc.id {
			t.Fatalf("unsafe response ID = %s, want %s", response.ID, tc.id)
		}
	}
}

type lostResponseWriter struct {
	*httptest.ResponseRecorder
	writes int
}

func (w *lostResponseWriter) Write(data []byte) (int, error) {
	w.writes++
	return 0, io.ErrClosedPipe
}

func TestLostResponseDoesNotRetryOrRetainRPCID(t *testing.T) {
	var committed atomic.Int32
	h, _ := testHandler(t, func(context.Context, json.RawMessage) (any, error) {
		return struct {
			Committed int32 `json:"committed"`
		}{committed.Add(1)}, nil
	})
	session := initialize(t, h, "2025-11-25", true)
	body := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"observe","arguments":{}}}`
	w := &lostResponseWriter{ResponseRecorder: httptest.NewRecorder()}
	h.ServeHTTP(w, request(context.Background(), http.MethodPost, session, body))
	if committed.Load() != 1 || w.writes != 1 {
		t.Fatal("transport retried an operation after failed delivery")
	}
	requireStatus(t, post(h, session, body), http.StatusOK)
	if committed.Load() != 2 {
		t.Fatal("transport retained the completed RPC ID as a cache")
	}
}
