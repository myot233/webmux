package mcphttp

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/rainchan/webmux/internal/contract"
)

func TestOversizedToolOutputIsOneCompleteControlledError(t *testing.T) {
	var calls atomic.Int32
	h, _ := testHandler(t, func(context.Context, json.RawMessage) (any, error) {
		if calls.Add(1) == 1 {
			return struct {
				Body string `json:"body"`
			}{strings.Repeat("x", maxOutput/2)}, nil
		}
		return struct {
			Completed bool `json:"completed"`
		}{true}, nil
	})
	session := initialize(t, h, "2025-11-25", true)
	w := receive(t, startCall(h, session, `1`, context.Background()))
	requireError(t, w, http.StatusOK, -32603)
	if w.Body.Len() > 1024 || !json.Valid(w.Body.Bytes()) || w.Header().Get("Content-Length") != strconv.Itoa(w.Body.Len()) {
		t.Fatal("oversized result was partially written or silently truncated")
	}
	// Failure is isolated and the ID is not cached or implicitly retried.
	requireStatus(t, receive(t, startCall(h, session, `1`, context.Background())), http.StatusOK)
	if calls.Load() != 2 {
		t.Fatal("unexpected transport retry/cache behavior")
	}
}

func TestBoundedEnvelopeExactLimitAndInvalidEncoding(t *testing.T) {
	response := &contract.MCPResponse{JSONRPC: "2.0", ID: json.RawMessage("1"), Result: json.RawMessage(`""`)}
	base, ok := encodeResponse(response)
	if !ok {
		t.Fatal("empty JSON string should encode")
	}
	response.Result = json.RawMessage(`"` + strings.Repeat("x", maxOutput-len(base)) + `"`)
	encoded, ok := encodeResponse(response)
	if !ok || len(encoded) != maxOutput || !json.Valid(encoded) {
		t.Fatal("exact output bound rejected or corrupted")
	}
	response.Result = append(response.Result[:len(response.Result)-1], 'x', '"')
	if encoded, ok := encodeResponse(response); ok || encoded != nil {
		t.Fatal("output above bound must fail without partial data")
	}
	h, _ := testHandler(t, func(context.Context, json.RawMessage) (any, error) {
		return func() {}, nil
	})
	session := initialize(t, h, "2025-11-25", true)
	requireError(t, receive(t, startCall(h, session, `1`, context.Background())), http.StatusOK, -32603)
}

func TestHandlerBusinessFailureStaysToolResult(t *testing.T) {
	h, _ := testHandler(t, func(context.Context, json.RawMessage) (any, error) {
		return nil, errors.New("operation rejected")
	})
	session := initialize(t, h, "2025-11-25", true)
	w := receive(t, startCall(h, session, `1`, context.Background()))
	requireStatus(t, w, http.StatusOK)
	var response contractResponseResult
	if json.Unmarshal(w.Body.Bytes(), &response) != nil || response.Error != nil || !response.Result.IsError {
		t.Fatal("business failure was promoted to a protocol/framework error")
	}
}
