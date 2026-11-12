package api

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/rainchan/webmux/internal/conversation"
	"net/http/httptest"
	"testing"

	"github.com/rainchan/webmux/internal/config"
	"github.com/rainchan/webmux/internal/contract"
	"github.com/rainchan/webmux/internal/handoff"
	"github.com/rainchan/webmux/internal/launch"
	"github.com/rainchan/webmux/internal/session"
)

func TestHandoffDraftDiscoveryAndSeparateTaskRevision(t *testing.T) {
	_, store, source := historyAPI(t)
	launches := launch.Open(t.TempDir())
	defer launches.Close()
	cfg, err := launches.Create(launch.Input{Name: "target", Cmd: []string{"omp"}, Hybrid: true})
	if err != nil {
		t.Fatal(err)
	}
	service := handoff.New(handoff.Options{Conversations: store, Launch: launches})
	defer service.Close()
	manager := session.NewManager(session.Options{})
	handler := New(testDeps(t, Deps{Manager: manager, Workspace: testWorkspace(t, manager, t.TempDir()), Conversations: store, Handoffs: service, Config: config.Config{Token: "secret"}}))
	send := func(method, path string, body any, auth bool) *httptest.ResponseRecorder {
		t.Helper()
		data, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		req := httptest.NewRequest(method, "http://webmux.example"+path, bytes.NewReader(data))
		if auth {
			req.Header.Set("Authorization", "Bearer secret")
		}
		out := httptest.NewRecorder()
		handler.ServeHTTP(out, req)
		return out
	}
	path := "/api/conversations/" + source.ConversationID + "/handoff-previews"
	if r := send("GET", path+"?fork=1", nil, false); r.Code != 401 {
		t.Fatal("history draft unauthenticated", r.Code)
	}
	for _, q := range []string{"", "?fork=true", "?fork=1&fork=0", "?fork=1&scope=other"} {
		if r := send("GET", path+q, nil, true); r.Code != 400 {
			t.Fatal("invalid query", q, r.Code)
		}
	}
	r := send("GET", path+"?fork=1", nil, true)
	var draft contract.HandoffDraft
	if err := json.Unmarshal(r.Body.Bytes(), &draft); r.Code != 200 || err != nil || draft.Source.ID != source.ConversationID || draft.Preview != nil || draft.Delivery != nil || r.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("empty draft", r.Code, r.Body.String(), err)
	}
	seedHistory(t, store, source, "saved source")
	r = send("POST", path, contract.HandoffPreviewRequest{ClientRequestID: "draft-create", ConfigID: cfg.ID, UserText: "first task", Fork: true}, true)
	var p contract.HandoffPreview
	if err := json.Unmarshal(r.Body.Bytes(), &p); r.Code != 201 || err != nil {
		t.Fatal(r.Body.String(), err)
	}
	r = send("PATCH", "/api/handoffs/"+p.ID, contract.HandoffEditRequest{Revision: p.Revision, UserText: new("separate updated task")}, true)
	var updated contract.HandoffPreview
	if err := json.Unmarshal(r.Body.Bytes(), &updated); r.Code != 200 || err != nil || updated.Manual || updated.RenderedText != p.RenderedText || updated.UserText != "separate updated task" || updated.Revision != p.Revision+1 {
		t.Fatal("task edit", r.Body.String(), err)
	}
	r = send("GET", path+"?fork=1", nil, true)
	if err := json.Unmarshal(r.Body.Bytes(), &draft); r.Code != 200 || err != nil || draft.Preview == nil || draft.Preview.ID != p.ID || draft.Preview.UserText != updated.UserText {
		t.Fatal("restore", r.Body.String(), err)
	}
	r = send("GET", path+"?fork=0", nil, true)
	if err := json.Unmarshal(r.Body.Bytes(), &draft); r.Code != 200 || err != nil || draft.Preview != nil {
		t.Fatal("fork leaked into switch", r.Body.String(), err)
	}
	if r := send("PATCH", "/api/handoffs/"+p.ID, contract.HandoffEditRequest{Revision: updated.Revision}, true); r.Code != 400 {
		t.Fatal("empty edit accepted", r.Code)
	}
}

func TestDisabledHistoryRefusesHistoryDraftAndDeliveryRoutes(t *testing.T) {
	store := conversation.Open(conversation.Options{StateDir: t.TempDir(), DisableCapture: true})
	defer store.Close()
	svc := handoff.New(handoff.Options{Conversations: store})
	defer svc.Close()
	manager := session.NewManager(session.Options{})
	defer manager.Shutdown(context.Background())
	handler := New(testDeps(t, Deps{Manager: manager, Conversations: store, Handoffs: svc, Workspace: testWorkspace(t, manager, t.TempDir())}))
	id := "0123456789abcdef0123456789abcdef"
	for _, path := range []string{"/api/conversations/" + id + "/activities", "/api/conversations/" + id + "/activities/" + id, "/api/conversations/" + id + "/handoff-previews?fork=1", "/api/handoffs/" + id, "/api/handoffs/" + id + "/selection", "/api/handoffs/" + id + "/delivery"} {
		r := httptest.NewRecorder()
		handler.ServeHTTP(r, httptest.NewRequest("GET", path, nil))
		if r.Code != 503 {
			t.Fatal(path, r.Code, r.Body.String())
		}
	}
}
