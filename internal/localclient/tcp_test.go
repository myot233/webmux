package localclient

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coder/websocket"
)

func TestTCPClientAuthenticatesRESTAndWebSocket(t *testing.T) {
	const token = "owner-credential"
	var host string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Host != host || r.Header.Get("Authorization") != "Bearer "+token || r.URL.Query().Has("token") || r.Header.Get("Cookie") != "" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if r.URL.Path == "/ws/sessions" {
			conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{Subprotocols: []string{WSProtocol}})
			if err != nil {
				return
			}
			defer conn.CloseNow()
			_ = conn.Write(r.Context(), websocket.MessageText, []byte(`{"type":"snapshot","sessions":[]}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()
	u, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	host = u.Host
	client, err := NewTCP(server.URL, token)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var result struct {
		OK bool `json:"ok"`
	}
	if err := client.Do(ctx, http.MethodPost, "/api/check", map[string]bool{"write": true}, &result); err != nil || !result.OK {
		t.Fatalf("authenticated POST: %+v %v", result, err)
	}
	result.OK = false
	if err := client.GetBounded(ctx, "/api/check", 128, &result); err != nil || !result.OK {
		t.Fatalf("authenticated bounded GET: %+v %v", result, err)
	}
	if _, err := client.Get(ctx, "/api/check"); err != nil {
		t.Fatal(err)
	}
	if status, _, err := client.Raw(ctx, http.MethodGet, "/api/check", nil); err != nil || status != http.StatusOK {
		t.Fatalf("authenticated raw GET: %d %v", status, err)
	}
	if err := client.Upload(ctx, "/api/check", "report.txt", strings.NewReader("report"), &result); err != nil {
		t.Fatal(err)
	}
	conn, err := client.DialWebSocket(ctx, "/ws/sessions")
	if err != nil {
		t.Fatal(err)
	}
	defer conn.CloseNow()
	_, data, err := conn.Read(ctx)
	if err != nil || string(data) != `{"type":"snapshot","sessions":[]}` {
		t.Fatalf("authenticated WS: %s %v", data, err)
	}
	unauthorized, err := NewTCP(server.URL, "wrong")
	if err != nil {
		t.Fatal(err)
	}
	defer unauthorized.Close()
	var apiErr *APIError
	if err := unauthorized.Do(ctx, http.MethodGet, "/api/check", nil, nil); !errors.As(err, &apiErr) || apiErr.Status != http.StatusUnauthorized {
		t.Fatalf("bad credential must be rejected: %v", err)
	}
}

func TestTCPClientNeverFollowsRedirectOrEnvironmentProxy(t *testing.T) {
	var leaked atomic.Int32
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		leaked.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer other.Close()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, other.URL+"/stolen", http.StatusTemporaryRedirect)
	}))
	defer server.Close()
	t.Setenv("HTTP_PROXY", other.URL)
	t.Setenv("HTTPS_PROXY", other.URL)
	t.Setenv("ALL_PROXY", other.URL)
	address := strings.TrimPrefix(server.URL, "http://")
	_, port, err := net.SplitHostPort(address)
	if err != nil {
		t.Fatal(err)
	}
	// A non-loopback authority ensures ProxyFromEnvironment would not get
	// its usual localhost exemption if it were accidentally enabled.
	origin := "http://webmux-api.test:" + port
	client, err := NewTCP(origin, "owner-credential")
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	client.HTTP.Transport.(*http.Transport).DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
		if strings.HasPrefix(addr, "webmux-api.test:") {
			addr = address
		}
		return (&net.Dialer{}).DialContext(ctx, network, addr)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var apiErr *APIError
	if err := client.Do(ctx, http.MethodGet, "/api/check", nil, nil); !errors.As(err, &apiErr) || apiErr.Status != http.StatusTemporaryRedirect {
		t.Fatalf("redirect must remain a response: %v", err)
	}
	if conn, err := client.DialWebSocket(ctx, "/ws/sessions"); err == nil {
		_ = conn.CloseNow()
		t.Fatal("WebSocket redirect unexpectedly accepted")
	}
	if leaked.Load() != 0 {
		t.Fatal("credential request followed a redirect or proxy")
	}
}

func TestTCPClientRequiresAnOriginAndCredential(t *testing.T) {
	for _, origin := range []string{"", "file:///tmp/socket", "http://user:pass@localhost", "http://localhost/path", "http://localhost?token=secret", "http://localhost#token"} {
		if client, err := NewTCP(origin, "owner"); err == nil {
			client.Close()
			t.Fatalf("accepted invalid origin %q", origin)
		}
	}
	if _, err := NewTCP("http://localhost", ""); err == nil {
		t.Fatal("accepted missing owner credential")
	}
}
