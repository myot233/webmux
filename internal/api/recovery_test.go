package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/rainchan/webmux/internal/agentprofile"
	"github.com/rainchan/webmux/internal/contract"
	"github.com/rainchan/webmux/internal/launch"
)

// request sends a request with optional token and Origin.
func (h *hybridAPI) request(method, path, body, token, origin string) *httptest.ResponseRecorder {
	h.t.Helper()
	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	r := httptest.NewRequest(method, "http://webmux.example"+path, reader)
	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	if origin != "" {
		r.Header.Set("Origin", origin)
	}
	w := httptest.NewRecorder()
	h.handler.ServeHTTP(w, r)
	return w
}

func (h *hybridAPI) records(query string) []contract.HybridRecord {
	h.t.Helper()
	w := h.request("GET", "/api/hybrid-sessions"+query, "", "secret", "")
	if w.Code != http.StatusOK {
		h.t.Fatalf("GET /api/hybrid-sessions%s: %d %s", query, w.Code, w.Body.String())
	}
	if strings.Contains(w.Body.String(), "secret-value") || strings.Contains(w.Body.String(), "API_KEY") {
		h.t.Fatalf("record list exposes the environment: %s", w.Body.String())
	}
	var list []contract.HybridRecord
	if err := json.Unmarshal(w.Body.Bytes(), &list); err != nil {
		h.t.Fatal(err)
	}
	return list
}

func (h *hybridAPI) createSide(configID string, side contract.HybridSide) string {
	h.t.Helper()
	var info contract.SessionInfo
	if err := json.Unmarshal(h.expect(fmt.Sprintf(`{"configId":%q,"startSide":%q}`, configID, side), 201), &info); err != nil {
		h.t.Fatal(err)
	}
	return info.ID
}

// closeSession closes id through the API and waits for its shell to go.
func (h *hybridAPI) closeSession(id string) {
	h.t.Helper()
	h.expectCommand("DELETE", "/api/sessions/"+id, nil, http.StatusNoContent)
	waitAPI(h.t, "closed "+id, func() bool { _, err := h.manager.Get(id); return err != nil })
}

func (h *hybridAPI) resume(id string, req contract.HybridResumeRequest) contract.SessionInfo {
	h.t.Helper()
	req.ClientRequestID = chatRID()
	var info contract.SessionInfo
	if err := json.Unmarshal(h.expectCommand("POST", "/api/sessions/"+id+"/resume", req, http.StatusOK), &info); err != nil {
		h.t.Fatal(err)
	}
	if info.ID != id || info.Hybrid == nil || info.Hybrid.Side != req.Side || info.Hybrid.State != contract.HybridStateActive {
		h.t.Fatalf("resumed %+v hybrid %+v", info, info.Hybrid)
	}
	return info
}

func waitAPI(t *testing.T, what string, ok func() bool) {
	t.Helper()
	deadline := time.Now().Add(chatTimeout)
	for !ok() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// processEnv reads the environment of session id's recorded root process
// (Linux only; nil elsewhere).
func (h *hybridAPI) processEnv(id string) []string {
	h.t.Helper()
	if runtime.GOOS != "linux" {
		return nil
	}
	rec, err := h.hybrids.Record(id)
	if err != nil || rec.PID <= 0 {
		h.t.Fatalf("record %+v: %v", rec, err)
	}
	b, err := os.ReadFile(fmt.Sprintf("/proc/%d/environ", rec.PID))
	if err != nil {
		h.t.Fatal(err)
	}
	return strings.Split(string(b), "\x00")
}

func hasEnv(env []string, kv string) bool {
	for _, e := range env {
		if e == kv {
			return true
		}
	}
	return false
}

func TestHybridRecordEndpoints(t *testing.T) {
	h := newHybridAPI(t, nil)
	configID := h.config("omp", true)
	closed := h.createSide(configID, contract.HybridSideTUI)
	active := h.createSide(configID, contract.HybridSideTUI)
	var folder contract.ArtifactsFolder
	if err := json.Unmarshal(h.expectCommand("POST", "/api/sessions/"+closed+"/artifacts", nil, 200), &folder); err != nil {
		t.Fatal(err)
	}
	h.closeSession(closed)
	// Closing keeps the folder for a resume.
	time.Sleep(100 * time.Millisecond)
	if _, err := os.Stat(folder.Path); err != nil {
		t.Fatalf("closing removed the folder: %v", err)
	}

	list := h.records("?state=recoverable")
	if len(list) != 1 || list[0].ID != closed || list[0].State != contract.HybridStateClosed || list[0].ConfigMissing || list[0].ConfigID != configID ||
		list[0].Agent != agentprofile.OMP || list[0].Side != contract.HybridSideTUI || list[0].PreviousIDs == nil || list[0].InterruptedAt != nil {
		t.Fatalf("recoverable %+v", list)
	}
	if all := h.records(""); len(all) != 2 {
		t.Fatalf("all records %+v", all)
	}
	var one contract.HybridRecord
	if err := json.Unmarshal(h.expectCommand("GET", "/api/hybrid-sessions/"+closed, nil, 200), &one); err != nil || one.ID != closed {
		t.Fatalf("record %+v: %v", one, err)
	}
	h.expectCommand("GET", "/api/hybrid-sessions/zzzzzzzz", nil, 404)
	h.expectCommand("GET", "/api/hybrid-sessions?state=active", nil, 400)
	if w := h.request("GET", "/api/hybrid-sessions", "", "", ""); w.Code != http.StatusUnauthorized {
		t.Fatalf("without token: %d", w.Code)
	}
	if w := h.request("DELETE", "/api/hybrid-sessions/"+closed, "", "secret", "https://evil.example"); w.Code != http.StatusForbidden {
		t.Fatalf("foreign origin: %d", w.Code)
	}
	if w := h.request("POST", "/api/sessions/"+closed+"/resume", `{"clientRequestId":"api-req-cross","side":"tui"}`, "secret", "https://evil.example"); w.Code != http.StatusForbidden {
		t.Fatalf("foreign origin resume: %d", w.Code)
	}

	if err := h.store.Delete(configID); err != nil {
		t.Fatal(err)
	}
	if list := h.records("?state=recoverable"); len(list) != 1 || !list[0].ConfigMissing {
		t.Fatalf("after deleting the config %+v", list)
	}
	h.expectCommand("DELETE", "/api/hybrid-sessions/"+active, nil, 409)
	h.expectCommand("POST", "/api/sessions/"+active+"/resume", contract.HybridResumeRequest{ClientRequestID: chatRID(), Side: contract.HybridSideTUI}, 409)
	h.expectCommand("DELETE", "/api/hybrid-sessions/"+closed, nil, 204)
	h.expectCommand("GET", "/api/hybrid-sessions/"+closed, nil, 404)
	if _, err := os.Stat(folder.Path); !os.IsNotExist(err) {
		t.Fatalf("removing the record kept the folder: %v", err)
	}
	h.expectCommand("DELETE", "/api/hybrid-sessions/"+closed, nil, 404)
	if list := h.records("?state=recoverable"); len(list) != 0 {
		t.Fatalf("removed record listed %+v", list)
	}
}

// Resuming reads the config's environment again, asks before resuming
// without a deleted config's environment, and offers candidates when the
// agent session is gone.
func TestResumeHybridSession(t *testing.T) {
	h := newHybridAPI(t, nil)
	agent := fakeAgentPath(t, agentprofile.OMP)
	store := t.TempDir()
	// The store reaches the agent even without the config's environment.
	t.Setenv("FAKEAGENT_STORE", store)
	saved, err := h.store.Create(launch.Input{
		Name: "omp-chat", Cmd: []string{agent}, Hybrid: true,
		Env: map[string]string{"API_KEY": "secret-value", "FAKEAGENT_TURN_DELAY": "5ms"},
	})
	if err != nil {
		t.Fatal(err)
	}
	id := h.createSide(saved.ID, contract.HybridSideACP)
	h.closeSession(id)

	h.resume(id, contract.HybridResumeRequest{Side: contract.HybridSideTUI})
	if env := h.processEnv(id); env != nil && !hasEnv(env, "API_KEY=secret-value") {
		t.Fatal("resumed terminal lacks the config's environment")
	}
	h.expectCommand("POST", "/api/sessions/"+id+"/resume", contract.HybridResumeRequest{ClientRequestID: chatRID(), Side: contract.HybridSideACP}, 409)
	h.closeSession(id)

	if err := h.store.Delete(saved.ID); err != nil {
		t.Fatal(err)
	}
	h.expectConflict("POST", "/api/sessions/"+id+"/resume", contract.HybridResumeRequest{ClientRequestID: chatRID(), Side: contract.HybridSideACP}, "config_missing")
	if _, err := h.manager.Get(id); err == nil {
		t.Fatal("config_missing started a shell")
	}
	h.resume(id, contract.HybridResumeRequest{Side: contract.HybridSideACP, ConfirmWithoutEnv: true})
	if env := h.processEnv(id); env != nil && hasEnv(env, "API_KEY=secret-value") {
		t.Fatal("resumed chat kept the deleted config's environment")
	}
	h.closeSession(id)

	rec, err := h.hybrids.Record(id)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(store, "sessions", rec.AgentSessionID+".json")); err != nil {
		t.Fatal(err)
	}
	candidate := fmt.Sprintf(`{"id":"candidate1","cwd":%q,"title":"earlier","updatedAt":"2026-01-02T03:04:05Z","messages":[],"config":{"mode":"code","model":"fast"}}`, rec.Cwd)
	if err := os.WriteFile(filepath.Join(store, "sessions", "candidate1.json"), []byte(candidate), 0o600); err != nil {
		t.Fatal(err)
	}
	body := h.expectCommand("POST", "/api/sessions/"+id+"/resume", contract.HybridResumeRequest{ClientRequestID: chatRID(), Side: contract.HybridSideTUI, ConfirmWithoutEnv: true}, 409)
	var conflict contract.HybridConflict
	if err := json.Unmarshal(body, &conflict); err != nil {
		t.Fatal(err)
	}
	if conflict.Reason != "session_missing" || len(conflict.Candidates) != 1 || conflict.Candidates[0].AgentSessionID != "candidate1" ||
		conflict.Candidates[0].Title != "earlier" || !conflict.Candidates[0].UpdatedAt.Equal(time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)) {
		t.Fatalf("conflict %s", body)
	}
	info := h.resume(id, contract.HybridResumeRequest{Side: contract.HybridSideTUI, AgentSessionID: "candidate1", ConfirmWithoutEnv: true})
	if info.Hybrid.AgentSessionID == nil || *info.Hybrid.AgentSessionID != "candidate1" {
		t.Fatalf("resumed agent session %v", info.Hybrid.AgentSessionID)
	}
	file, err := os.ReadFile(filepath.Join(h.state, "hybrid-sessions.json"))
	if err != nil || strings.Contains(string(file), "secret-value") {
		t.Fatalf("record file %s: %v", file, err)
	}
}
