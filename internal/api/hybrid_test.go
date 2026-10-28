package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/rainchan/webmux/internal/config"
	"github.com/rainchan/webmux/internal/contract"
	"github.com/rainchan/webmux/internal/hybrid"
	"github.com/rainchan/webmux/internal/launch"
	"github.com/rainchan/webmux/internal/session"
	"github.com/rainchan/webmux/internal/workspace"
)

type hybridAPI struct {
	t       *testing.T
	handler http.Handler
	manager *session.Manager
	store   *launch.Service
	hybrids *hybrid.Service
	state   string
	cwd     string
}

// newHybridAPI serves sessions with a hybrid service whose state directory
// prepare may fill first.
func newHybridAPI(t *testing.T, prepare func(state string)) *hybridAPI {
	t.Helper()
	h := &hybridAPI{t: t, state: t.TempDir(), cwd: t.TempDir()}
	if prepare != nil {
		prepare(h.state)
	}
	h.manager = session.NewManager(session.Options{DefaultCwd: h.cwd})
	h.store = openLaunchStore(t)
	hybrids := hybrid.New(hybrid.Options{StateDir: h.state, Manager: h.manager})
	h.hybrids = hybrids
	ws := workspace.New(t.TempDir(), h.manager, h.store, h.cwd)
	ws.UseHybrid(hybrids)
	t.Cleanup(func() {
		hybrids.Close()
		ws.Close()
		ctx, cancel := context.WithTimeout(context.Background(), 7*time.Second)
		defer cancel()
		h.manager.Shutdown(ctx)
	})
	h.handler = New(testDeps(t, Deps{Config: config.Config{Token: "secret", DefaultCwd: h.cwd}, Manager: h.manager, Launch: h.store, Workspace: ws, Hybrid: hybrids}))
	return h
}

// config saves a launch config; agent names an executable "omp" script
// that stands in for the agent's terminal UI.
func (h *hybridAPI) config(name string, hybridMode bool) string {
	h.t.Helper()
	dir := h.t.TempDir()
	agent := filepath.Join(dir, "omp")
	if err := os.WriteFile(agent, []byte("#!/bin/sh\nexec sleep 60\n"), 0o755); err != nil {
		h.t.Fatal(err)
	}
	c, err := h.store.Create(launch.Input{Name: name, Cmd: []string{agent}, Env: map[string]string{"API_KEY": "secret-value"}, Hybrid: hybridMode})
	if err != nil {
		h.t.Fatal(err)
	}
	return c.ID
}

func (h *hybridAPI) post(body string) *httptest.ResponseRecorder {
	h.t.Helper()
	r := httptest.NewRequest("POST", "http://webmux.example/api/sessions", strings.NewReader(body))
	r.Header.Set("Authorization", "Bearer secret")
	w := httptest.NewRecorder()
	h.handler.ServeHTTP(w, r)
	return w
}

func (h *hybridAPI) expect(body string, status int) []byte {
	h.t.Helper()
	w := h.post(body)
	if w.Code != status {
		h.t.Fatalf("POST /api/sessions %s: status %d want %d: %s", body, w.Code, status, w.Body.String())
	}
	return w.Body.Bytes()
}

func TestCreateHybridSession(t *testing.T) {
	h := newHybridAPI(t, nil)
	id := h.config("omp", true)
	var info contract.SessionInfo
	if err := json.Unmarshal(h.expect(fmt.Sprintf(`{"configId":%q,"startSide":"tui"}`, id), 201), &info); err != nil {
		t.Fatal(err)
	}
	if info.Hybrid == nil || info.Hybrid.Side != contract.HybridSideTUI || info.Hybrid.State != contract.HybridStateActive || info.Hybrid.Agent != "omp" || info.ConfigID != id {
		t.Fatalf("created %+v hybrid %+v", info, info.Hybrid)
	}
	file, err := os.ReadFile(filepath.Join(h.state, "hybrid-sessions.json"))
	if err != nil || !strings.Contains(string(file), info.ID) || strings.Contains(string(file), "secret-value") {
		t.Fatalf("record file %s: %v", file, err)
	}
	// Ordinary configs are unchanged: no hybrid object.
	plain := h.config("plain", false)
	var ordinary map[string]any
	if err := json.Unmarshal(h.expect(fmt.Sprintf(`{"configId":%q}`, plain), 201), &ordinary); err != nil {
		t.Fatal(err)
	}
	if _, ok := ordinary["hybrid"]; ok {
		t.Fatalf("ordinary session has hybrid: %v", ordinary)
	}
}

func TestCreateSessionStartSideNeedsHybridConfig(t *testing.T) {
	h := newHybridAPI(t, nil)
	plain := h.config("plain", false)
	for _, body := range []string{
		fmt.Sprintf(`{"configId":%q,"startSide":"acp"}`, plain),
		`{"cmd":["sh"],"startSide":"tui"}`,
		fmt.Sprintf(`{"configId":%q,"startSide":"chat"}`, h.config("omp", true)),
	} {
		h.expect(body, 400)
	}
	if len(h.manager.List()) != 0 {
		t.Fatal("a rejected request created a session")
	}
}

func TestCreateHybridSessionStoreUnavailable(t *testing.T) {
	h := newHybridAPI(t, func(state string) {
		if err := os.WriteFile(filepath.Join(state, "hybrid-sessions.json"), []byte("{broken"), 0o600); err != nil {
			t.Fatal(err)
		}
	})
	body := h.expect(fmt.Sprintf(`{"configId":%q}`, h.config("omp", true)), 503)
	if !strings.Contains(string(body), "hybrid-sessions.json") {
		t.Fatalf("503 body lacks the store path: %s", body)
	}
	h.expect(fmt.Sprintf(`{"configId":%q}`, h.config("plain", false)), 201)
	// Recovery is disabled the same way; the file stays as it was.
	for _, req := range []struct {
		method, path string
		body         any
	}{
		{"GET", "/api/hybrid-sessions?state=recoverable", nil},
		{"GET", "/api/hybrid-sessions/abcdefgh", nil},
		{"DELETE", "/api/hybrid-sessions/abcdefgh", nil},
		{"POST", "/api/sessions/abcdefgh/resume", contract.HybridResumeRequest{ClientRequestID: chatRID(), Side: contract.HybridSideTUI}},
	} {
		if body := h.expectCommand(req.method, req.path, req.body, 503); !strings.Contains(string(body), "hybrid-sessions.json") {
			t.Fatalf("%s %s: 503 body lacks the store path: %s", req.method, req.path, body)
		}
	}
	if b, _ := os.ReadFile(filepath.Join(h.state, "hybrid-sessions.json")); string(b) != "{broken" {
		t.Fatalf("corrupt store rewritten: %q", b)
	}
}

func TestCreateHybridSessionRecordLimit(t *testing.T) {
	h := newHybridAPI(t, func(state string) {
		const alphabet = "abcdefghijklmnopqrstuvwxyz234567"
		records := make([]map[string]any, hybrid.MaxRecords)
		for i := range records {
			records[i] = map[string]any{
				"id": fmt.Sprintf("lim%c%caaa", alphabet[i/32], alphabet[i%32]), "name": "old", "configId": "c",
				"agent": "omp", "cwd": "/p", "cmd": []string{"omp"}, "side": "tui", "previousIds": []string{},
				"config": map[string]string{}, "state": "closed", "queuedCount": 0,
				"createdAt": "2026-01-01T00:00:00Z", "updatedAt": "2026-01-01T00:00:00Z",
			}
		}
		b, err := json.Marshal(map[string]any{"v": 1, "records": records})
		if err == nil {
			err = os.WriteFile(filepath.Join(state, "hybrid-sessions.json"), b, 0o600)
		}
		if err != nil {
			t.Fatal(err)
		}
	})
	var conflict contract.HybridConflict
	if err := json.Unmarshal(h.expect(fmt.Sprintf(`{"configId":%q}`, h.config("omp", true)), 409), &conflict); err != nil {
		t.Fatal(err)
	}
	if conflict.Reason != "limit" || conflict.Error == "" {
		t.Fatalf("conflict %+v", conflict)
	}
	// Removing an old record makes room again.
	h.expectCommand("DELETE", "/api/hybrid-sessions/limaaaaa", nil, 204)
	h.expect(fmt.Sprintf(`{"configId":%q}`, h.config("omp-again", true)), 201)
}
