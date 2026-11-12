package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/rainchan/webmux/internal/config"
	"github.com/rainchan/webmux/internal/contract"
	"github.com/rainchan/webmux/internal/hybrid"
	"github.com/rainchan/webmux/internal/session"
	"github.com/rainchan/webmux/internal/settings"
)

func settingsHandler(t *testing.T) http.Handler {
	t.Helper()
	prefs := settings.Open(t.TempDir())
	t.Cleanup(prefs.Close)
	m := session.NewManager(session.Options{DefaultCwd: t.TempDir()})
	t.Cleanup(func() { m.Shutdown(context.Background()) })
	return New(testDeps(t, Deps{Config: config.Config{Token: "login-secret"}, Manager: m, Settings: prefs}))
}
func TestSettingsHTTPContract(t *testing.T) {
	h := settingsHandler(t)
	for _, row := range []struct {
		method, body, token, origin string
		status                      int
	}{
		{"GET", "", "", "", 401}, {"PATCH", `{"baseRevision":0,"changes":{"theme":"dark"}}`, "login-secret", "https://foreign.example", 403},
		{"GET", "", "login-secret", "", 200},
		{"PATCH", `{"baseRevision":0,"changes":{"theme":"dark"}}`, "login-secret", "", 200},
		{"PATCH", `{"baseRevision":0,"changes":{"theme":"light"}}`, "login-secret", "", 409},
		{"PATCH", `{"baseRevision":1,"changes":{"theme":null,"quickCommands":[]}}`, "login-secret", "", 200},
		{"PATCH", `{"baseRevision":2,"changes":{"unknown":true}}`, "login-secret", "", 400},
		{"PATCH", `{"baseRevision":2,"changes":{"theme":"dark","theme":"light"}}`, "login-secret", "", 400},
		{"PATCH", `{"baseRevision":2,"changes":{"Theme":"dark"}}`, "login-secret", "", 400},
		{"PATCH", `{"changes":{"theme":"dark"}}`, "login-secret", "", 400},
		{"PATCH", `{"baseRevision":2,"changes":{"fontSize":"14"}}`, "login-secret", "", 400},
		{"PATCH", `{"baseRevision":2,"changes":{"quickCommands":[{"id":"a","name":"a","command":"pwd"}]}}`, "login-secret", "", 400},
		{"PATCH", `{"baseRevision":2,"changes":{"quickCommands":[{"id":"a","name":"a","command":"pwd","enter":null}]}}`, "login-secret", "", 400},
		{"PATCH", `{"baseRevision":2,"changes":{"preview":{"lines":10}}}`, "login-secret", "", 400},
		{"PATCH", `{"baseRevision":2,"changes":{"sessionPins":["invalid"]}}`, "login-secret", "", 400},
		{"PATCH", `{"baseRevision":2,"changes":{"sessionPins":["aaaaaaaa","aaaaaaaa"]}}`, "login-secret", "", 400},
		{"PATCH", strings.Repeat(" ", 2<<20) + `{}`, "login-secret", "", 400},
	} {
		w := tunnelAPIRequest(h, row.method, "/api/settings", row.body, row.token, row.origin)
		if w.Code != row.status {
			t.Fatalf("%s: status %d want %d: %s", row.method, w.Code, row.status, w.Body.String())
		}
	}
	w := tunnelAPIRequest(h, "GET", "/api/settings", "", "login-secret", "")
	var v contract.SettingsSnapshot
	if err := json.Unmarshal(w.Body.Bytes(), &v); err != nil {
		t.Fatal(err)
	}
	if v.Revision != 2 || v.Values.Theme != nil || v.Values.QuickCommands == nil || len(*v.Values.QuickCommands) != 0 {
		t.Fatalf("snapshot: %+v", v)
	}
}
func TestSettingsWebSocketSnapshotThenUpdates(t *testing.T) {
	h := settingsHandler(t)
	server := httptest.NewServer(h)
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	c, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(server.URL, "http")+"/ws/settings", &websocket.DialOptions{Subprotocols: []string{"webmux", "webmux.token.bG9naW4tc2VjcmV0"}})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = c.CloseNow() }()
	_, data, err := c.Read(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var first contract.SettingsEvent
	if err = json.Unmarshal(data, &first); err != nil {
		t.Fatal(err)
	}
	if first.Type != "settings" || first.Settings == nil || first.Settings.Revision != 0 {
		t.Fatalf("first: %s", data)
	}
	w := tunnelAPIRequest(h, "PATCH", "/api/settings", `{"baseRevision":0,"changes":{"theme":"dark"}}`, "login-secret", "")
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	_, data, err = c.Read(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var update contract.SettingsEvent
	if err = json.Unmarshal(data, &update); err != nil {
		t.Fatal(err)
	}
	if update.Type != "settings.update" || update.Revision != 1 || update.Changes["theme"] != "dark" || len(update.Changes) != 1 {
		t.Fatalf("update: %s", data)
	}
}

func TestSessionPinsRetainRecoverableInterruptedSessions(t *testing.T) {
	dir := t.TempDir()
	now := time.Now().UTC()
	interrupted := hybrid.Record{ID: "aaaaaaaa", State: hybrid.StateInterrupted, Side: hybrid.SideACP, Agent: "omp", Cwd: dir, Cmd: []string{"omp"}, CreatedAt: now, UpdatedAt: now}
	closed := interrupted
	closed.ID, closed.State = "bbbbbbbb", hybrid.StateClosed
	data, err := json.Marshal(map[string]any{"v": 1, "records": []hybrid.Record{interrupted, closed}})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "hybrid-sessions.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}
	m := session.NewManager(session.Options{DefaultCwd: dir})
	t.Cleanup(func() { m.Shutdown(context.Background()) })
	h := hybrid.New(hybrid.Options{StateDir: dir, Manager: m})
	t.Cleanup(h.Close)
	prefs := settings.Open(dir)
	t.Cleanup(prefs.Close)
	handler := New(testDeps(t, Deps{Config: config.Config{Token: "login-secret"}, Manager: m, Settings: prefs, Hybrid: h}))
	check := func(w *httptest.ResponseRecorder, revision int64, want []string) {
		t.Helper()
		var got contract.SettingsSnapshot
		if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &got) != nil ||
			got.Revision != revision || !slices.Equal(got.Values.SessionPins, want) {
			t.Fatalf("settings response %d %s, want revision %d and pins %v", w.Code, w.Body.String(), revision, want)
		}
	}
	w := tunnelAPIRequest(handler, "PATCH", "/api/settings",
		`{"baseRevision":0,"changes":{"sessionPins":["bbbbbbbb","aaaaaaaa","cccccccc"]}}`, "login-secret", "")
	check(w, 1, []string{interrupted.ID})

	if err := h.RemoveRecord(context.Background(), interrupted.ID); err != nil {
		t.Fatal(err)
	}
	w = tunnelAPIRequest(handler, "GET", "/api/settings", "", "login-secret", "")
	check(w, 2, nil)
	w = tunnelAPIRequest(handler, "GET", "/api/settings", "", "login-secret", "")
	check(w, 2, nil)
}
