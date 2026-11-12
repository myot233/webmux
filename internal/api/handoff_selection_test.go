package api

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rainchan/webmux/internal/config"
	"github.com/rainchan/webmux/internal/contract"
	"github.com/rainchan/webmux/internal/conversation"
	"github.com/rainchan/webmux/internal/handoff"
	"github.com/rainchan/webmux/internal/launch"
	"github.com/rainchan/webmux/internal/localclient"
	"github.com/rainchan/webmux/internal/session"
	"github.com/rainchan/webmux/internal/threadread"
)

func TestHandoffSelectionLongHistoryRemainsRecoverable(t *testing.T) {
	ctx := context.Background()
	state := t.TempDir()
	store := conversation.Open(conversation.Options{StateDir: state})
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Error(err)
		}
	})
	source, err := store.EnsureRuntime(ctx, conversation.RuntimeInput{RuntimeID: "archived", Cwd: "/workspace", AgentName: "test", Status: "closed", HistoryOrigin: "unknown"})
	if err != nil {
		t.Fatal(err)
	}
	launches := launch.Open(t.TempDir())
	defer launches.Close()
	cfg, err := launches.Create(launch.Input{Name: "target", Cmd: []string{"omp"}, Hybrid: true})
	if err != nil {
		t.Fatal(err)
	}
	const count = 40000
	// Exercise normal normalization once, then clone its persisted committed
	// representation in one fixture transaction. This boundary test measures
	// preview/history reads, not 40,000 independently fsynced activity writes.
	seed := seedHistory(t, store, source, strings.Repeat("history ", 20))
	db, err := sql.Open("sqlite", filepath.Join(state, "conversations.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `WITH RECURSIVE positions(n) AS (VALUES(2) UNION ALL SELECT n+1 FROM positions WHERE n<?)
INSERT INTO activity(id,conversation_id,provider_thread_id,run_id,position,kind,role,status,source,provider_item_id,local_key,byte_length,heading_length,created_at,json_bytes,heading_json_bytes)
SELECT printf('%032x',n),a.conversation_id,a.provider_thread_id,a.run_id,n,a.kind,a.role,a.status,a.source,'',printf('large-%d',n),a.byte_length,a.heading_length,a.created_at,a.json_bytes,a.heading_json_bytes
FROM positions CROSS JOIN activity a WHERE a.id=?`, count, seed); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO activity_chunk SELECT a.id,c.byte_offset,c.byte_length,c.text FROM activity a CROSS JOIN activity_chunk c WHERE a.conversation_id=? AND a.id!=? AND c.activity_id=?`, source.ConversationID, seed, seed); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.ExecContext(ctx, "UPDATE conversation SET next_activity_position=? WHERE id=?", count+1, source.ConversationID); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	ids := make(map[string]bool, count)
	ids[seed] = true
	for i := 2; i <= count; i++ {
		ids[fmt.Sprintf("%032x", i)] = true
	}
	previews := handoff.New(handoff.Options{Conversations: store, Launch: launches, TokenCap: 1024, MaxPromptBytes: 64 << 10, MaxPromptWireBytes: 14 << 20})
	defer previews.Close()
	manager := session.NewManager(session.Options{})
	handler := New(testDeps(t, Deps{Manager: manager, Workspace: testWorkspace(t, manager, t.TempDir()), Conversations: store, Handoffs: previews, Config: config.Config{Token: "secret"}}))
	request := func(method, path string, body any) contract.HandoffPreview {
		t.Helper()
		data, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		r := httptest.NewRequest(method, "http://webmux.example"+path, bytes.NewReader(data))
		r.Header.Set("Authorization", "Bearer secret")
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, r)
		if response.Code < 200 || response.Code >= 300 || response.Body.Len() >= 4<<20 {
			t.Fatalf("%s preview status=%d bytes=%d", method, response.Code, response.Body.Len())
		}
		var p contract.HandoffPreview
		if err := json.Unmarshal(response.Body.Bytes(), &p); err != nil {
			t.Fatal(err)
		}
		if !p.SelectionTruncated || p.SelectedCount+p.OmittedCount != count || len(p.SelectedItemIDs) > 100 || len(p.OmittedItemIDs) > 100 || len(p.OmittedItems) > 100 {
			t.Fatalf("unbounded/lost preview metadata: selected=%d omitted=%d", p.SelectedCount, p.OmittedCount)
		}
		return p
	}
	// Build the stress fixture through the service: race/atomic instrumentation
	// must not turn this recovery contract into a REST wall-clock benchmark.
	// GET, PATCH, selection pagination and MCP below still use real HTTP.
	prepared, err := previews.Preview(ctx, source.ConversationID, handoff.Request{
		ClientRequestID: "long-preview",
		ConfigID:        cfg.ID,
		Fork:            true,
		UserText:        "next",
	})
	if err != nil {
		t.Fatal(err)
	}
	p := request("GET", "/api/handoffs/"+prepared.ID, nil)
	// Manual editing makes every candidate omitted, reproducing the >4 MiB
	// duplicated-ID preview that previously blocked both forms of thread_read.
	p = request("PATCH", "/api/handoffs/"+p.ID, contract.HandoffEditRequest{Revision: p.Revision, Text: new("manual context")})
	p = request("GET", "/api/handoffs/"+p.ID, nil)
	if p.OmittedCount != count || p.SelectedCount != 0 {
		t.Fatal("manual preview lost omissions")
	}
	saved, err := store.Preview(ctx, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if saved.OmittedCount != count || saved.SelectedCount != 0 || len(saved.OmittedItemIDs) != 100 || len(saved.SelectedItemIDs) != 0 {
		t.Fatal("stored preview must load bounded omission samples and exact counts")
	}
	if !strings.Contains(handoff.InlineContext(saved), "omitted 40000.") {
		t.Fatal("manual context attribution lost the full omission count")
	}
	base := "/api/handoffs/" + p.ID + "/selection"
	for _, tc := range []struct {
		query, token, origin string
		status               int
	}{
		{"", "", "", 401}, {"", "secret", "https://evil.example", 403},
		{"?afterPosition=01", "secret", "", 400}, {"?limit=101", "secret", "", 400},
		{"?limit=0", "secret", "", 400}, {"?afterPosition=0&afterPosition=1", "secret", "", 400},
		{"?itemId=../escape", "secret", "", 400}, {"?unknown=1", "secret", "", 400},
		{"?afterPosition=40001", "secret", "", 400},
	} {
		if response := historyRequest(handler, base+tc.query, tc.token, tc.origin); response.Code != tc.status {
			t.Fatalf("selection %s: %d", tc.query, response.Code)
		}
	}
	seen := make(map[string]bool, count)
	after := "0"
	for {
		response := historyRequest(handler, base+"?afterPosition="+after+"&limit=100", "secret", "")
		if response.Code != 200 || response.Body.Len() >= 64<<10 || response.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("selection status=%d bytes=%d", response.Code, response.Body.Len())
		}
		var page contract.HandoffSelection
		if err := json.Unmarshal(response.Body.Bytes(), &page); err != nil {
			t.Fatal(err)
		}
		if len(page.Items) > 100 || page.OmittedCount != count || page.SourcePosition != p.SourcePosition || page.Revision != p.Revision {
			t.Fatal("incorrect selection page scope/count")
		}
		for _, item := range page.Items {
			if !ids[item.ActivityID] || seen[item.ActivityID] || item.Selected || item.Reason != "manual_context" {
				t.Fatal("lost, duplicate or misclassified selection item")
			}
			seen[item.ActivityID] = true
		}
		if page.Done {
			break
		}
		if page.NextPosition == after {
			t.Fatal("selection cursor stalled")
		}
		after = page.NextPosition
	}
	if len(seen) != count {
		t.Fatalf("recovered %d/%d omissions", len(seen), count)
	}
	// Exercise the real consumer over HTTP rather than a mocked preview object.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Header.Set("Authorization", "Bearer secret")
		handler.ServeHTTP(w, r)
	}))
	defer server.Close()
	transport := &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		var d net.Dialer
		return d.DialContext(ctx, "tcp", server.Listener.Addr().String())
	}}
	client := &localclient.Client{HTTP: &http.Client{Transport: transport}}
	defer client.Close()
	reader := threadread.Reader{Client: client, ConversationID: source.ConversationID, HandoffID: p.ID}
	seen = make(map[string]bool, 300)
	for _, after := range []string{"0", "19900", "39900"} {
		data, _ := json.Marshal(threadread.Arguments{View: "activity", AfterPosition: after, Limit: 100, MaxBytesPerItem: 400})
		page, err := reader.Read(ctx, data)
		if err != nil || len(page.Items) != 100 || page.OmittedCount != count {
			t.Fatalf("thread_read at %s: %d items, %v", after, len(page.Items), err)
		}
		body, err := json.Marshal(page)
		if err != nil || len(body) >= 4<<20 {
			t.Fatal("unbounded history page")
		}
		for _, item := range page.Items {
			if !item.Omitted || item.Selected || !ids[item.ActivityID] || seen[item.ActivityID] {
				t.Fatal("activity classification lost or duplicate activity")
			}
			seen[item.ActivityID] = true
		}
		if page.NextPosition == after || (after == "39900") != page.Done {
			t.Fatal("incorrect history boundary cursor")
		}
	}
	if len(seen) != 300 {
		t.Fatal("history boundary pages lost items")
	}
	id := p.OmittedItemIDs[0]
	data, _ := json.Marshal(map[string]any{"view": "activity", "itemId": id})
	if item, err := reader.Read(ctx, data); err != nil || len(item.Items) != 1 || !item.Items[0].Omitted {
		t.Fatalf("individual omitted read: %v", err)
	}
	reader.ConversationID = strings.Repeat("0", 32)
	if _, err := reader.Read(ctx, data); err == nil {
		t.Fatal("cross-conversation scope accepted")
	}
}
