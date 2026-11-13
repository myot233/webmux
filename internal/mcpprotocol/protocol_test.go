package mcpprotocol

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/rainchan/webmux/internal/contract"
)

func request(t *testing.T, session *Session, raw string) *contract.MCPResponse {
	t.Helper()
	req, response := Decode([]byte(raw))
	if response != nil {
		return response
	}
	response, err := session.Dispatch(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	return response
}

func readySession(t *testing.T, registry *Registry) *Session {
	t.Helper()
	s := registry.NewSession()
	response := request(t, s, `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"unknown"}}`)
	if response.Error != nil {
		t.Fatal(response.Error)
	}
	if response := request(t, s, `{"jsonrpc":"2.0","method":"notifications/initialized"}`); response != nil {
		t.Fatal(response)
	}
	return s
}

func TestIndependentSessionsAndImmutableDefinitions(t *testing.T) {
	definition := map[string]any{"name": "read", "inputSchema": map[string]any{"type": "object"}}
	r, err := New(Config{Name: "fixture", CallTimeout: time.Second, Tools: []Tool{{Definition: definition, Call: func(context.Context, json.RawMessage) (any, error) { return nil, nil }}}})
	if err != nil {
		t.Fatal(err)
	}
	definition["name"] = "changed"
	definition["inputSchema"].(map[string]any)["type"] = "array"
	s := readySession(t, r)
	if !s.Ready() || s.ProtocolVersion() != defaultVersion {
		t.Fatal("handshake not retained")
	}
	other := r.NewSession()
	if other.Ready() || other.ProtocolVersion() != "" {
		t.Fatal("session state shared")
	}
	response := request(t, other, `{"jsonrpc":"2.0","id":2,"method":"tools/list"}`)
	if response.Error == nil || response.Error.Code != -32600 {
		t.Fatal(response)
	}
	response = request(t, s, `{"jsonrpc":"2.0","id":3,"method":"tools/list"}`)
	if string(response.Result) != `{"tools":[{"inputSchema":{"type":"object"},"name":"read"}]}` {
		t.Fatalf("mutated definitions: %s", response.Result)
	}
}

func TestHandlerDoesNotHoldSessionLock(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	r, err := New(Config{Name: "fixture", CallTimeout: time.Second, Tools: []Tool{{Definition: map[string]any{"name": "wait"}, Call: func(ctx context.Context, _ json.RawMessage) (any, error) {
		close(entered)
		select {
		case <-release:
			return true, nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}}}})
	if err != nil {
		t.Fatal(err)
	}
	s := readySession(t, r)
	finished := make(chan error, 1)
	go func() {
		req, _ := Decode([]byte(`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"wait"}}`))
		_, err := s.Dispatch(context.Background(), req)
		finished <- err
	}()
	<-entered
	defer close(release)
	ping := make(chan *contract.MCPResponse, 1)
	go func() {
		req, _ := Decode([]byte(`{"jsonrpc":"2.0","id":3,"method":"ping"}`))
		response, _ := s.Dispatch(context.Background(), req)
		ping <- response
	}()
	select {
	case response := <-ping:
		if response == nil || string(response.Result) != `{}` {
			t.Fatal(response)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("handler holds session lock")
	}
	// The call is bounded by its configured deadline even if not released yet.
	if err := <-finished; err != nil {
		t.Fatal(err)
	}
}

func TestProtocolErrorsAndNegotiation(t *testing.T) {
	r, err := New(Config{Name: "fixture", CallTimeout: time.Second, Versions: []string{"2025-03-26", defaultVersion}})
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		give string
		want int
	}{
		{`{`, -32700},
		{`[]`, -32700},
		{`{"jsonrpc":"1.0","id":9,"method":"ping"}`, -32600},
		{`{"jsonrpc":"2.0","id":null,"method":"ping"}`, -32600},
		{`{"jsonrpc":"2.0","id":{},"method":"ping"}`, -32600},
		{`{"jsonrpc":"2.0","id":1,"method":"server/discover"}`, -32601},
		{`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`, -32602},
	}
	for _, tt := range tests {
		t.Run(tt.give, func(t *testing.T) {
			response := request(t, r.NewSession(), tt.give)
			if response == nil || response.Error == nil || response.Error.Code != tt.want {
				t.Fatalf("response: %+v", response)
			}
		})
	}
	for _, version := range []string{"2025-03-26", "2024-11-05", "unknown"} {
		s := r.NewSession()
		response := request(t, s, `{"jsonrpc":"2.0","id":"init","method":"initialize","params":{"protocolVersion":"`+version+`"}}`)
		if response.Error != nil {
			t.Fatal(response.Error)
		}
		want := defaultVersion
		if version == "2025-03-26" {
			want = version
		}
		if s.ProtocolVersion() != want {
			t.Fatal(s.ProtocolVersion())
		}
		response = request(t, s, `{"jsonrpc":"2.0","id":2,"method":"initialize","params":{"protocolVersion":"2025-11-25"}}`)
		if response.Error == nil || response.Error.Message != "Already initialized" {
			t.Fatal(response)
		}
	}
}

func TestToolErrorAndEncodingFailure(t *testing.T) {
	r, err := New(Config{Name: "fixture", CallTimeout: time.Second, Tools: []Tool{
		{Definition: map[string]any{"name": "error"}, Call: func(context.Context, json.RawMessage) (any, error) { return nil, errors.New("denied") }},
		{Definition: map[string]any{"name": "unencodable"}, Call: func(context.Context, json.RawMessage) (any, error) { return make(chan int), nil }},
	}})
	if err != nil {
		t.Fatal(err)
	}
	s := readySession(t, r)
	response := request(t, s, `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"error"}}`)
	if response.Error != nil || string(response.Result) != `{"isError":true,"content":[{"type":"text","text":"denied"}]}` {
		t.Fatal(response)
	}
	req, _ := Decode([]byte(`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"unencodable"}}`))
	if _, err := s.Dispatch(context.Background(), req); err == nil {
		t.Fatal("encoding failure lost")
	}
}
