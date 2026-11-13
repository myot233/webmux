package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/rainchan/webmux/internal/config"
	"github.com/rainchan/webmux/internal/mcphttp"
	"github.com/rainchan/webmux/internal/mcpprotocol"
	"github.com/rainchan/webmux/internal/p2p"
	"github.com/rainchan/webmux/internal/session"
)

func TestMCPAuthenticationAndExposure(t *testing.T) {
	registry, err := mcpprotocol.New(mcpprotocol.Config{Name: "test", CallTimeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	endpoint := mcphttp.New(context.Background(), registry)
	defer endpoint.Close()
	manager := session.NewManager(session.Options{})
	defer manager.Shutdown(context.Background())
	deps := testDeps(t, Deps{
		Config:  config.Config{Listen: "127.0.0.1:7681", Token: "secret", MCP: config.MCPConfig{Enabled: true}, AllowedOrigins: []string{"https://trusted.example"}},
		Manager: manager, Workspace: testWorkspace(t, manager, t.TempDir()), MCP: endpoint,
	})
	bridge := NewBridge(deps)
	const initialize = `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"security-test","version":"1"}}}`
	for _, tt := range []struct {
		name, host, origin, query, cookie string
		auth                              []string
		want                              int
	}{
		{name: "missing credential", host: "127.0.0.1:7681", want: 401},
		{name: "wrong credential", host: "127.0.0.1:7681", auth: []string{"Bearer wrong"}, want: 401},
		{name: "cookie is not credential", host: "127.0.0.1:7681", cookie: "webmux_token=secret", want: 401},
		{name: "query is not credential", host: "127.0.0.1:7681", query: "?token=secret", want: 401},
		{name: "duplicate header", host: "127.0.0.1:7681", auth: []string{"Bearer secret", "Bearer secret"}, want: 401},
		{name: "untrusted origin", host: "127.0.0.1:7681", origin: "https://evil.example", auth: []string{"Bearer secret"}, want: 403},
		{name: "forged forwarded host", host: "evil.example", auth: []string{"Bearer secret"}, want: 403},
		{name: "nonbrowser bearer", host: "127.0.0.1:7681", auth: []string{"Bearer secret"}, want: 200},
		{name: "trusted cross origin", host: "127.0.0.1:7681", origin: "https://trusted.example", auth: []string{"Bearer secret"}, want: 200},
	} {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodPost, "http://"+tt.host+"/mcp"+tt.query, strings.NewReader(initialize))
			r.Header.Set("Content-Type", "application/json")
			r.Header.Set("Accept", "application/json, text/event-stream")
			r.Header.Set("Origin", tt.origin)
			r.Header.Set("Cookie", tt.cookie)
			r.Header.Set("X-Forwarded-Host", "127.0.0.1:7681")
			for _, a := range tt.auth {
				r.Header.Add("Authorization", a)
			}
			w := httptest.NewRecorder()
			bridge.HTTPHandler.ServeHTTP(w, r)
			if w.Code != tt.want {
				t.Fatalf("status %d want %d: %s", w.Code, tt.want, w.Body.String())
			}
			if tt.want == http.StatusUnauthorized && w.Header().Get("WWW-Authenticate") != `Bearer realm="webmux"` {
				t.Fatalf("missing Bearer challenge: %v", w.Header())
			}
			if tt.want != http.StatusOK && w.Header().Get("MCP-Session-Id") != "" {
				t.Fatal("unauthorized request created a session")
			}
			if tt.want == http.StatusOK && w.Header().Get("MCP-Session-Id") == "" {
				t.Fatalf("authenticated initialization did not create a session: %s", w.Body.String())
			}
		})
	}
	for _, method := range []string{http.MethodGet, http.MethodDelete, http.MethodOptions} {
		r := httptest.NewRequest(method, "http://127.0.0.1:7681/mcp", nil)
		w := httptest.NewRecorder()
		bridge.HTTPHandler.ServeHTTP(w, r)
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("%s bypassed auth: %d", method, w.Code)
		}
	}
	for _, path := range []string{"/mcp", "/mcp/"} {
		w := httptest.NewRecorder()
		NewLocal(deps, Local{}).ServeHTTP(w, httptest.NewRequest(http.MethodPost, "http://webmux"+path, nil))
		if w.Code != http.StatusNotFound {
			t.Fatalf("local %s exposed MCP: %d", path, w.Code)
		}
		res := bridge.ServeRPC(context.Background(), p2p.RPCRequest{Method: http.MethodPost, Path: path})
		if res.Status != http.StatusNotFound {
			t.Fatalf("P2P %s exposed MCP: %+v", path, res)
		}
	}
	deps.Config.MCP.Enabled = false
	for _, path := range []string{"/mcp", "/mcp/"} {
		w := httptest.NewRecorder()
		New(deps).ServeHTTP(w, httptest.NewRequest(http.MethodGet, "http://127.0.0.1:7681"+path, nil))
		if w.Code != http.StatusNotFound || strings.Contains(w.Body.String(), "<!doctype") {
			t.Fatalf("disabled %s returned %d %s", path, w.Code, w.Body.String())
		}
	}
	deps.Config.MCP.Enabled, deps.Config.Token = true, ""
	w := httptest.NewRecorder()
	New(deps).ServeHTTP(w, httptest.NewRequest(http.MethodPost, "http://127.0.0.1:7681/mcp", strings.NewReader(initialize)))
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("empty-token embedder opened MCP: %d", w.Code)
	}
}
