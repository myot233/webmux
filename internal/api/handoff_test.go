package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/rainchan/webmux/internal/config"
	"github.com/rainchan/webmux/internal/contract"
	"github.com/rainchan/webmux/internal/conversation"
	"github.com/rainchan/webmux/internal/handoff"
	"github.com/rainchan/webmux/internal/hybrid"
	"github.com/rainchan/webmux/internal/launch"
	"github.com/rainchan/webmux/internal/session"
	"github.com/rainchan/webmux/internal/workspace"
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

func TestHandoffDeliveryAPIAuthDurableFailuresAndExplicitRetry(t *testing.T) {
	_, store, source := historyAPI(t)
	manager := session.NewManager(session.Options{})
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		manager.Shutdown(ctx)
	})
	launches := launch.Open(t.TempDir())
	defer launches.Close()
	cfg, err := launches.Create(launch.Input{Name: "missing connector", Cmd: []string{fakeAgentPath(t, "codex")}, Env: map[string]string{"PATH": "/nonexistent"}, Hybrid: true})
	if err != nil {
		t.Fatal(err)
	}
	hybrids := hybrid.New(hybrid.Options{StateDir: t.TempDir(), Manager: manager})
	defer hybrids.Close()
	workspaces := workspace.New(t.TempDir(), manager, launches, t.TempDir())
	defer workspaces.Close()
	workspaces.UseHybrid(hybrids)
	service := handoff.New(handoff.Options{Conversations: store, Launch: launches, Hybrid: hybrids, Workspace: workspaces, TokenCap: 16000, MaxPromptBytes: hybrid.MaxPromptBytes, MaxPromptWireBytes: hybrid.MaxPromptWireBytes})
	defer service.Close()
	handler := New(testDeps(t, Deps{Manager: manager, Workspace: workspaces, Conversations: store, Handoffs: service, Config: config.Config{Token: "secret"}}))
	if err := store.SyncRuntime(context.Background(), conversation.RuntimeInput{RuntimeID: "archived", Cwd: t.TempDir(), AgentName: "test", Status: "closed"}); err != nil {
		t.Fatal(err)
	}
	p, err := service.Preview(context.Background(), source.ConversationID, handoff.Request{ClientRequestID: "preview-api", ConfigID: cfg.ID, Fork: true, UserText: "next request"})
	if err != nil {
		t.Fatal(err)
	}
	input := contract.ConversationMessageRequest{ClientRequestID: "message-api", HandoffID: p.ID, Revision: 1, Text: p.Request.UserText, ConfigID: cfg.ID, AgentConfig: p.Request.AgentConfig, Location: contract.HandoffLocation(p.Request.Location), Attachments: []contract.ChatAttachment{}}
	request := func(method, path string, body any, authorization, origin string) *httptest.ResponseRecorder {
		t.Helper()
		data, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		r := httptest.NewRequest(method, "http://webmux.example"+path, bytes.NewReader(data))
		r.Header.Set("Authorization", authorization)
		r.Header.Set("Origin", origin)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, r)
		return response
	}
	path := "/api/conversations/" + source.ConversationID + "/messages"
	for _, test := range []struct {
		method, path, auth, origin string
		status                     int
	}{{"POST", path, "", "", 401}, {"POST", path, "Bearer secret", "https://evil.example", 403}, {"GET", "/api/handoffs/" + p.ID + "/delivery", "", "", 401}, {"GET", "/api/handoffs/" + p.ID + "/delivery", "Bearer secret", "https://evil.example", 403}} {
		if response := request(test.method, test.path, input, test.auth, test.origin); response.Code != test.status {
			t.Fatalf("auth %s %d", test.path, response.Code)
		}
	}
	response := request("POST", path, input, "Bearer secret", "")
	if response.Code != 202 || response.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("delivery %d: %s", response.Code, response.Body.String())
	}
	var first contract.HandoffDelivery
	if err := json.Unmarshal(response.Body.Bytes(), &first); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	var outcome contract.HandoffDelivery
	for time.Now().Before(deadline) {
		response = request("GET", "/api/handoffs/"+p.ID+"/delivery", nil, "Bearer secret", "")
		if response.Code != 200 {
			t.Fatalf("read outcome: %s", response.Body.String())
		}
		if err := json.Unmarshal(response.Body.Bytes(), &outcome); err != nil {
			t.Fatal(err)
		}
		if outcome.Status == "failed" {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if outcome.Status != "failed" || outcome.Step != "precheck" || outcome.ErrorClass != "connector_missing" || outcome.SessionID != first.SessionID || len(manager.List()) != 0 {
		t.Fatalf("failure resources: %+v", outcome)
	}
	if response := request("PATCH", "/api/handoffs/"+p.ID, contract.HandoffEditRequest{Revision: 0, Text: "x"}, "Bearer secret", ""); response.Code != 400 {
		t.Fatal(response.Code)
	}
	input.ClientRequestID = "explicit-retry-api"
	input.Retry = true
	response = request("POST", path, input, "Bearer secret", "")
	if response.Code != 202 {
		t.Fatal(response.Body.String())
	}
	var retry contract.HandoffDelivery
	if err := json.Unmarshal(response.Body.Bytes(), &retry); err != nil {
		t.Fatal(err)
	}
	if retry.ExecutionID == first.ExecutionID || retry.SessionID != first.SessionID {
		t.Fatal("retry allocated new runtime")
	}
	input.ClientRequestID = "message-api"
	input.Retry = false
	response = request("POST", path, input, "Bearer secret", "")
	if err := json.Unmarshal(response.Body.Bytes(), &outcome); err != nil {
		t.Fatal(err)
	}
	if response.Code != 202 || outcome.ExecutionID != first.ExecutionID || outcome.Status != "failed" {
		t.Fatal("first receipt changed")
	}
	input.Text = "different"
	if response := request("POST", path, input, "Bearer secret", ""); response.Code != 409 {
		t.Fatal("payload conflict", response.Code)
	}
}
