package mcpcontrol

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestTerminalReadUsesBoundedRawEndpoint(t *testing.T) {
	c := testClient(t, func(r *http.Request) string {
		if r.Method != "GET" || r.URL.Path != "/api/sessions/s/output" || r.URL.Query().Get("maxBytes") != "2" || r.URL.Query().Get("offset") != "8" {
			t.Fatalf("unexpected raw read: %s %s", r.Method, r.URL)
		}
		return `{"generation":"0123456789abcdef","start":"10","end":"16","nextOffset":"12","encoding":"base64","data":"Yf8=","reset":true,"historyTruncated":true,"limited":true}`
	})
	result, err := c.read(context.Background(), json.RawMessage(`{"sessionId":"s","view":"terminal","generation":"0123456789abcdef","offset":"8","maxBytes":2}`))
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"data":"Yf8="`, `"reset":true`, `"nextOffset":"12"`, `"limited":true`, `"historyTruncated":true`} {
		if !strings.Contains(string(data), want) {
			t.Fatalf("missing %s: %s", want, data)
		}
	}
}
func TestActivityReadFiltersPrivateItemsAndPreservesCursor(t *testing.T) {
	cid := strings.Repeat("a", 32)
	c := testClient(t, func(r *http.Request) string {
		if r.URL.Path == "/api/sessions/s" {
			return `{"conversationId":"` + cid + `"}`
		}
		if r.URL.Path != "/api/conversations/"+cid+"/activities" {
			t.Fatalf("wrong path %s", r.URL.Path)
		}
		if r.URL.Query().Get("afterPosition") != "12" {
			t.Error("lost durable cursor")
		}
		return `{"items":[{"exportable":false,"text":"private"},{"exportable":true,"text":"visible","position":"14"}],"nextPosition":"14","gaps":[],"historyOrigin":"bounded_import","reconcileRequired":true}`
	})
	result, err := c.read(context.Background(), json.RawMessage(`{"sessionId":"s","afterPosition":"12","limit":2}`))
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "private") || !strings.Contains(string(data), `"nextPosition":"14"`) || !strings.Contains(string(data), `"done":false`) {
		t.Fatalf("page: %s", data)
	}
}
func TestReadRejectsInvalidCursorsBeforeAPI(t *testing.T) {
	c := testClient(t, func(*http.Request) string { t.Fatal("invalid cursor reached API"); return "" })
	for _, raw := range []string{`{"sessionId":"s","afterPosition":"01"}`, `{"sessionId":"s","afterPosition":"9223372036854775808"}`, `{"sessionId":"s","limit":100,"maxBytesPerItem":65536}`, `{"sessionId":"s","view":"terminal","generation":"bad","offset":"0"}`, `{"sessionId":"s","view":"terminal","offset":"0"}`} {
		if _, err := c.read(context.Background(), json.RawMessage(raw)); err == nil {
			t.Fatalf("accepted %s", raw)
		}
	}
}
