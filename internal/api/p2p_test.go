package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/rainchan/webmux/internal/config"
	"github.com/rainchan/webmux/internal/contract"
	"github.com/rainchan/webmux/internal/p2p"
	"github.com/rainchan/webmux/internal/p2psettings"
	"github.com/rainchan/webmux/internal/session"
)

type p2pControlFixture struct {
	mu      sync.Mutex
	status  contract.P2PStatus
	err     error
	sigErr  error
	calls   int
	lastSet contract.P2PSettingsUpdate
	lastSig contract.P2PSignalRequest
}

func newP2PControlFixture() *p2pControlFixture {
	return &p2pControlFixture{
		status: contract.P2PStatus{
			Supported:        true,
			UDPBindings:      []contract.P2PUDPBinding{{Network: "udp4", Port: 45678}},
			Capabilities:     contract.P2PCapabilities{Networks: []string{"udp4", "udp6"}, ICEMode: "full"},
			Enabled:          true,
			PortMapping:      true,
			STUNServers:      config.DefaultSTUNServers(),
			Source:           "config",
			StorageAvailable: true,
			PortMap:          contract.P2PPortMapStatus{State: "probing"},
			ActivePeers:      0,
			Peers:            []contract.P2PPeerInfo{},
		},
	}
}

func (f *p2pControlFixture) Enabled() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.status.Enabled
}

func (f *p2pControlFixture) Status() contract.P2PStatus {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.status
}

func (f *p2pControlFixture) UpdateSettings(in contract.P2PSettingsUpdate) (contract.P2PStatus, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	if f.err != nil {
		return contract.P2PStatus{}, f.err
	}
	f.lastSet = in
	f.status.Enabled = in.Enabled
	f.status.PortMapping = in.PortMapping
	f.status.STUNServers = append([]string{}, in.STUNServers...)
	f.status.Source = "web"
	if !in.Enabled || !in.PortMapping {
		f.status.PortMap = contract.P2PPortMapStatus{State: "off"}
	}
	return f.status, nil
}

func (f *p2pControlFixture) ResetSettings() (contract.P2PStatus, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	if f.err != nil {
		return contract.P2PStatus{}, f.err
	}
	f.status = newP2PControlFixture().status
	return f.status, nil
}

func (f *p2pControlFixture) Signal(_ context.Context, req contract.P2PSignalRequest) (contract.P2PSignalResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	if f.sigErr != nil {
		return contract.P2PSignalResponse{}, f.sigErr
	}
	f.lastSig = req
	return contract.P2PSignalResponse{
		PeerID:      "peer-01234567",
		SDP:         "v=0\r\n",
		Candidates:  []contract.P2PICECandidate{},
		STUNServers: append([]string{}, f.status.STUNServers...),
	}, nil
}

func p2pAPIHandler(t *testing.T, control P2PControl) Bridge {
	t.Helper()
	m := session.NewManager(session.Options{DefaultCwd: t.TempDir()})
	t.Cleanup(func() { m.Shutdown(context.Background()) })
	return NewBridge(testDeps(t, Deps{
		Config:  config.Config{Token: "login-secret"},
		Manager: m,
		P2P:     control,
	}))
}

func TestP2PEndpointsSecurityAndValidation(t *testing.T) {
	control := newP2PControlFixture()
	bridge := p2pAPIHandler(t, control)
	h := bridge.HTTPHandler

	for _, endpoint := range []struct{ method, path, body string }{
		{"GET", "/api/p2p", ""},
		{"PUT", "/api/p2p/settings", `{"enabled":true,"portMapping":false,"stunServers":[]}`},
		{"DELETE", "/api/p2p/settings", ""},
		{"POST", "/api/p2p/signal", `{"action":"offer","sdp":"v=0"}`},
		{"GET", "/api/p2p/ping?seq=7", ""},
	} {
		for _, auth := range []struct {
			token, origin string
			status        int
		}{
			{"", "", 401},
			{"wrong", "", 401},
			{"login-secret", "https://evil.example", 403},
		} {
			w := tunnelAPIRequest(h, endpoint.method, endpoint.path, endpoint.body, auth.token, auth.origin)
			if w.Code != auth.status {
				t.Fatalf("%s %s (%s): got %d, want %d", endpoint.method, endpoint.path, auth.origin, w.Code, auth.status)
			}
		}
	}
	if control.calls != 0 {
		t.Fatal("unauthorized request reached P2P control")
	}

	// Ping returns 200 with seq and serverTs.
	wPing := tunnelAPIRequest(h, "GET", "/api/p2p/ping?seq=42", "", "login-secret", "")
	if wPing.Code != 200 {
		t.Fatalf("ping status = %d", wPing.Code)
	}
	var ping contract.P2PPingResponse
	if err := json.Unmarshal(wPing.Body.Bytes(), &ping); err != nil {
		t.Fatal(err)
	}
	if ping.Seq != 42 || ping.ServerTs <= 0 {
		t.Fatalf("unexpected ping response: %+v", ping)
	}

	// Strict JSON rejection on settings update.
	for _, body := range []string{
		`{"enabled":true,"portMapping":true,"stunServers":[],"extra":1}`,
		`{"enabled":true,"enabled":false,"portMapping":true,"stunServers":[]}`,
		`{"enabled":"yes","portMapping":true,"stunServers":[]}`,
	} {
		if w := tunnelAPIRequest(h, "PUT", "/api/p2p/settings", body, "login-secret", ""); w.Code != 400 {
			t.Fatalf("body %s: got %d, want 400", body, w.Code)
		}
	}

	// Signal error status mappings.
	for _, row := range []struct {
		err  error
		want int
	}{
		{fmt.Errorf("%w: bad sdp", p2p.ErrInvalid), 400},
		{p2p.ErrPeerMissing, 404},
		{p2p.ErrDisabled, 409},
		{p2p.ErrUnsupported, 409},
		{p2p.ErrRateLimited, 429},
		{p2p.ErrTooMany, 429},
	} {
		control.sigErr = row.err
		w := tunnelAPIRequest(h, "POST", "/api/p2p/signal", `{"action":"offer","sdp":"v=0"}`, "login-secret", "")
		if w.Code != row.want {
			t.Fatalf("signal err %v: got %d, want %d", row.err, w.Code, row.want)
		}
	}
	control.sigErr = nil

	// Nil P2P control returns 503.
	nilBridge := p2pAPIHandler(t, nil)
	if w := tunnelAPIRequest(nilBridge.HTTPHandler, "GET", "/api/p2p", "", "login-secret", ""); w.Code != 503 {
		t.Fatalf("nil control GET /api/p2p = %d, want 503", w.Code)
	}
}

func TestLocalSessionProcessCannotControlP2P(t *testing.T) {
	m := session.NewManager(session.Options{})
	defer m.Shutdown(context.Background())
	control := newP2PControlFixture()
	inside := map[int]bool{20: true}
	handler := NewLocal(testDeps(t, Deps{
		Config:    config.Config{},
		Manager:   m,
		Workspace: testWorkspace(t, m, t.TempDir()),
		P2P:       control,
	}), Local{
		Status:    func() contract.StatusV1 { return contract.StatusV1{V: 1} },
		InSession: func(pid int) bool { return inside[pid] },
	})

	for _, path := range []string{"/api/p2p", "/api/p2p/settings", "/api/p2p/signal"} {
		rInside := httptest.NewRequest(http.MethodGet, "http://webmux"+path, nil)
		rInside = rInside.WithContext(context.WithValue(rInside.Context(), peerKey{}, 20))
		wInside := httptest.NewRecorder()
		handler.ServeHTTP(wInside, rInside)
		if wInside.Code != http.StatusForbidden {
			t.Fatalf("session process on %s: got %d, want 403", path, wInside.Code)
		}
	}

	// The new local capability allows only reads, including unknown peer PIDs.
	for _, pid := range []int{0, 20, 10} {
		r := httptest.NewRequest(http.MethodGet, "http://webmux"+contract.PathP2PDiagnostics, nil)
		r = r.WithContext(context.WithValue(r.Context(), peerKey{}, pid))
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Code != http.StatusOK {
			t.Fatalf("diagnostics pid=%d: %d", pid, w.Code)
		}
	}

	rHead := httptest.NewRequest(http.MethodHead, "http://webmux"+contract.PathP2PDiagnostics, nil)
	wHead := httptest.NewRecorder()
	handler.ServeHTTP(wHead, rHead)
	if wHead.Code != http.StatusOK {
		t.Fatalf("HEAD diagnostics: %d", wHead.Code)
	}
	for _, method := range []string{"POST", "PUT", "DELETE", "PATCH"} {
		r := httptest.NewRequest(method, "http://webmux"+contract.PathP2PDiagnostics, nil)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Code != http.StatusMethodNotAllowed {
			t.Fatalf("diagnostics %s: %d", method, w.Code)
		}
	}
	for _, tc := range []struct{ method, path string }{
		{"PUT", "/api/p2p/settings"}, {"DELETE", "/api/p2p/settings"}, {"POST", "/api/p2p/signal"},
	} {
		r := httptest.NewRequest(tc.method, "http://webmux"+tc.path, strings.NewReader(`{}`))
		r = r.WithContext(context.WithValue(r.Context(), peerKey{}, 20))
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Code != http.StatusForbidden {
			t.Fatalf("session %s %s: %d", tc.method, tc.path, w.Code)
		}
	}
	if control.calls != 0 {
		t.Fatal("read diagnostics mutated P2P")
	}
	bridge := p2pAPIHandler(t, control)
	if w := tunnelAPIRequest(bridge.HTTPHandler, "GET", contract.PathP2PDiagnostics, "", "login-secret", ""); w.Code != http.StatusNotFound {
		t.Fatalf("TCP diagnostics exposed: %d", w.Code)
	}

	rHost := httptest.NewRequest(http.MethodGet, "http://webmux/api/p2p", nil)
	rHost = rHost.WithContext(context.WithValue(rHost.Context(), peerKey{}, 10))
	wHost := httptest.NewRecorder()
	handler.ServeHTTP(wHost, rHost)
	if wHost.Code != http.StatusOK {
		t.Fatalf("host process on /api/p2p: got %d, want 200", wHost.Code)
	}
}

func TestP2PRPCAuthAndRouting(t *testing.T) {
	control := newP2PControlFixture()
	bridge := p2pAPIHandler(t, control)

	// Missing token -> 401
	unauth := bridge.ServeRPC(context.Background(), p2p.RPCRequest{
		Method: "GET",
		Path:   "/api/health",
	})
	if unauth.Status != 401 {
		t.Fatalf("unauthenticated RPC status = %d, want 401", unauth.Status)
	}

	// Non-/api path (e.g. /local/v1/token) -> 404
	localAttempt := bridge.ServeRPC(context.Background(), p2p.RPCRequest{
		Method:  "GET",
		Path:    "/local/v1/token",
		Headers: map[string]string{"Authorization": "Bearer login-secret"},
	})
	if localAttempt.Status != 404 {
		t.Fatalf("/local/ RPC status = %d, want 404", localAttempt.Status)
	}

	// Valid /api/config request -> 200 with p2p: true
	okResp := bridge.ServeRPC(context.Background(), p2p.RPCRequest{
		Method:  "GET",
		Path:    "/api/config",
		Headers: map[string]string{"Authorization": "Bearer login-secret"},
	})
	if okResp.Status != 200 || !strings.Contains(string(okResp.Body), `"p2p":true`) {
		t.Fatalf("RPC /api/config = %d %s", okResp.Status, string(okResp.Body))
	}
}

func TestWireSnapshotsP2P(t *testing.T) {
	rec := &wireRecorder{t: t, norm: newWireNormalizer(), seen: map[string]bool{}}
	control := newP2PControlFixture()
	bridge := p2pAPIHandler(t, control)
	h := bridge.HTTPHandler

	for _, row := range []struct {
		name, method, path, body, token string
		err, sigErr                     error
	}{
		{"p2p-get", "GET", "/api/p2p", "", "login-secret", nil, nil},
		{"p2p-settings-put", "PUT", "/api/p2p/settings", `{"enabled":true,"portMapping":false,"stunServers":["stun:stun.cloudflare.com:3478"]}`, "login-secret", nil, nil},
		{"p2p-settings-reset", "DELETE", "/api/p2p/settings", "", "login-secret", nil, nil},
		{"p2p-signal-offer", "POST", "/api/p2p/signal", `{"action":"offer","sdp":"v=0\r\n"}`, "login-secret", nil, nil},
		{"error-400-p2p-settings", "PUT", "/api/p2p/settings", `{"enabled":true,"portMapping":true,"stunServers":["http://bad"]}`, "login-secret", fmt.Errorf("%w: stun URL must start with stun: or stuns:", p2psettings.ErrInvalid), nil},
		{"error-409-p2p-disabled", "POST", "/api/p2p/signal", `{"action":"offer","sdp":"v=0"}`, "login-secret", nil, p2p.ErrDisabled},
		{"error-503-p2p-settings", "PUT", "/api/p2p/settings", `{"enabled":true,"portMapping":true,"stunServers":[]}`, "login-secret", p2psettings.ErrUnavailable, nil},
	} {
		control.err = row.err
		control.sigErr = row.sigErr
		w := tunnelAPIRequest(h, row.method, row.path, row.body, row.token, "")
		rec.record(row.name, w.Code, w.Body.Bytes())
	}
}
