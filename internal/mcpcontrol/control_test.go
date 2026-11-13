package mcpcontrol

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/rainchan/webmux/internal/contract"
	"github.com/rainchan/webmux/internal/localclient"
)

type roundTrip func(*http.Request) (*http.Response, error)

func (f roundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func testClient(t *testing.T, handler func(*http.Request) string) Client {
	t.Helper()
	return Client{API: &localclient.Client{HTTP: &http.Client{Transport: roundTrip(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(handler(r))), Header: make(http.Header)}, nil
	})}}}
}
func TestDiscoveryAndFiltering(t *testing.T) {
	c := testClient(t, func(r *http.Request) string {
		switch r.URL.Path {
		case "/api/launch-configs":
			return `[{"id":"config","name":"Codex","chat":true,"cmd":["secret"],"envNames":["secret"]}]`
		case "/api/sessions":
			if r.URL.Query().Get("state") != "all" {
				t.Fatal("state not forwarded")
			}
			return `[{"id":"one","parentId":"parent"},{"id":"two","parentId":"other"},{"id":"three","parentId":"parent"}]`
		case "/api/sessions/one/details":
			return `{"session":{"id":"one"},"children":[],"labels":{}}`
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
			return ""
		}
	})
	result, err := c.capabilities(context.Background(), json.RawMessage(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	capabilities := result.(contract.MCPControlCapabilitiesResult)
	if strings.Contains(string(data), "secret") || len(capabilities.LaunchConfigs) != 1 || capabilities.LaunchConfigs[0].Name != "Codex" {
		t.Fatalf("capabilities: %s", data)
	}
	result, err = c.list(context.Background(), json.RawMessage(`{"state":"all","parentId":"parent","limit":1}`))
	if err != nil {
		t.Fatal(err)
	}
	page := result.(contract.MCPSessionListResult)
	if page.Total != 2 || page.Done || len(page.Sessions) != 1 || page.Sessions[0].ID != "one" {
		t.Fatalf("page: %+v", page)
	}
	if _, err := c.get(context.Background(), json.RawMessage(`{"sessionId":"one"}`)); err != nil {
		t.Fatal(err)
	}
}
func TestInvalidArgumentsNeverReachAPI(t *testing.T) {
	c := testClient(t, func(*http.Request) string { t.Fatal("invalid arguments reached API"); return "" })
	for _, raw := range []string{`null`, `{"limit":0}`, `{"limit":101}`, `{"state":"unknown"}`, `{"limit":1,"limit":2}`, `{"Limit":1}`, `{"extra":true}`} {
		if _, err := c.list(context.Background(), json.RawMessage(raw)); err == nil {
			t.Fatalf("accepted %s", raw)
		}
	}
	for _, raw := range []string{`{}`, `{"sessionId":"../token"}`, `{"sessionId":"a?b"}`} {
		if _, err := c.get(context.Background(), json.RawMessage(raw)); err == nil {
			t.Fatalf("accepted %s", raw)
		}
	}
}
