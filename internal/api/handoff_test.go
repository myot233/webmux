package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/rainchan/webmux/internal/config"
	"github.com/rainchan/webmux/internal/contract"
	"github.com/rainchan/webmux/internal/handoff"
	"github.com/rainchan/webmux/internal/launch"
	"github.com/rainchan/webmux/internal/session"
)

func TestHandoffPreviewAPIRevisionStalenessAndPrivacy(t *testing.T) {
	_, store, source := historyAPI(t)
	launches := launch.Open(t.TempDir())
	defer launches.Close()
	cfg, err := launches.Create(launch.Input{Name: "target", Cmd: []string{"omp"}, Env: map[string]string{"PRIVATE_ENV": "never-return-this"}, Hybrid: true})
	if err != nil {
		t.Fatal(err)
	}
	previews := handoff.New(handoff.Options{Conversations: store, Launch: launches, TokenCap: 16000, MaxPromptBytes: 64 << 10, MaxPromptWireBytes: 14 << 20})
	// historyAPI provides an isolated service and manager; only this fixture's
	// conversation dependency is needed for previews, with no runtime creation.
	manager := session.NewManager(session.Options{})
	handler := New(testDeps(t, Deps{Manager: manager, Workspace: testWorkspace(t, manager, t.TempDir()), Conversations: store, Handoffs: previews, Config: config.Config{Token: "secret"}}))
	seedHistory(t, store, source, "source text 中文🙂")
	request := func(method, path string, input any, authorization, origin string) *httptest.ResponseRecorder {
		t.Helper()
		body, err := json.Marshal(input)
		if err != nil {
			t.Fatal(err)
		}
		req := httptest.NewRequest(method, "http://webmux.example"+path, bytes.NewReader(body))
		req.Header.Set("Authorization", authorization)
		req.Header.Set("Origin", origin)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, req)
		return response
	}
	body := contract.HandoffPreviewRequest{ClientRequestID: "preview-request", ConfigID: cfg.ID, UserText: "new request", Fork: true}
	path := "/api/conversations/" + source.ConversationID + "/handoff-previews"
	if r := request("POST", path, body, "", ""); r.Code != 401 {
		t.Fatalf("unauthenticated preview=%d", r.Code)
	}
	if r := request("POST", path, body, "Bearer secret", "https://evil.example"); r.Code != 403 {
		t.Fatalf("Origin preview=%d", r.Code)
	}
	r := request("POST", path, body, "Bearer secret", "")
	if r.Code != 201 {
		t.Fatal(r.Body.String())
	}
	var p contract.HandoffPreview
	if err := json.Unmarshal(r.Body.Bytes(), &p); err != nil {
		t.Fatal(err)
	}
	if p.State != "ready" || p.NativeAvailable || p.HistoryToolAvailable || len(p.SelectedItemIDs) != 1 || p.UserText != "new request" || p.TargetConversationID == source.ConversationID {
		t.Fatalf("preview: %+v", p)
	}
	if strings.Contains(r.Body.String(), "never-return-this") || strings.Contains(r.Body.String(), "PRIVATE_ENV") || strings.Contains(r.Body.String(), "configFingerprint") {
		t.Fatal("private launch details exposed")
	}
	if r.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("preview cacheable")
	}
	repeated := request("POST", path, body, "Bearer secret", "")
	var retry contract.HandoffPreview
	if err := json.Unmarshal(repeated.Body.Bytes(), &retry); err != nil || retry.ID != p.ID {
		t.Fatalf("duplicate preview: %s %v", repeated.Body.String(), err)
	}
	edit := contract.HandoffEditRequest{Revision: 1, Text: "manual context"}
	r = request("PATCH", "/api/handoffs/"+p.ID, edit, "Bearer secret", "")
	if r.Code != 200 {
		t.Fatal(r.Body.String())
	}
	if err := json.Unmarshal(r.Body.Bytes(), &p); err != nil {
		t.Fatal(err)
	}
	if p.Revision != 2 || !p.Manual || p.Strategy != "manual_context" || p.RenderedText != "manual context" || len(p.SelectedItemIDs) != 0 {
		t.Fatalf("manual revision: %+v", p)
	}
	if r = request("PATCH", "/api/handoffs/"+p.ID, edit, "Bearer secret", ""); r.Code != 409 {
		t.Fatalf("stale editor=%d", r.Code)
	}
	seedHistory(t, store, source, "another device's new turn")
	r = request("GET", "/api/handoffs/"+p.ID, nil, "Bearer secret", "")
	if r.Code != 200 {
		t.Fatal(r.Body.String())
	}
	if err := json.Unmarshal(r.Body.Bytes(), &p); err != nil {
		t.Fatal(err)
	}
	if !p.Stale || p.State != "stale" || p.RenderedText != "manual context" {
		t.Fatalf("stale edit lost: %+v", p)
	}
	// Deleting a config does not delete a saved draft or its first receipt.
	if err := launches.Delete(cfg.ID); err != nil {
		t.Fatal(err)
	}
	if r := request("POST", path, body, "Bearer secret", ""); r.Code != 201 {
		t.Fatalf("retry after config deletion: %d %s", r.Code, r.Body.String())
	}
	if err := store.ForgetRuntime(context.Background(), "archived"); err != nil {
		t.Fatal(err)
	}
	if r := request("GET", "/api/handoffs/"+p.ID, nil, "Bearer secret", ""); r.Code != 404 {
		t.Fatalf("forgotten draft retained=%d", r.Code)
	}
}
