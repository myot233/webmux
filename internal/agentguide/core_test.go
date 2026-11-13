package agentguide

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestMCPGuideContextAndTopics(t *testing.T) {
	id := "trusted-session"
	for _, transport := range []string{"mcp-stdio", "mcp-http"} {
		for _, locality := range []string{"local", "remote"} {
			c := Context{Transport: transport, Locality: locality, SelfSessionID: &id, Tools: []ToolSummary{{Name: "custom-read", Description: "Read"}}}
			r, err := MCPGuide(c, "")
			if err != nil {
				t.Fatal(err)
			}
			if r.Transport != transport || r.Locality != locality || !reflect.DeepEqual(r.Tools, []string{"custom-read"}) {
				t.Fatalf("wrong context: %+v", r)
			}
			if (r.SelfSessionID != nil) != (locality == "local") {
				t.Fatalf("identity boundary: %+v", r)
			}
			if r.Kind != "index" || r.Topic != nil || len(r.Topics) != 7 {
				t.Fatalf("index: %+v", r)
			}
			want := []string{"connect", "operations", "completion", "requests", "launch", "artifacts", "rules"}
			for i, topic := range r.Topics {
				if topic.ID != want[i] {
					t.Fatalf("topic %d: %+v", i, topic)
				}
				page, err := MCPGuide(c, topic.ID)
				if err != nil || page.Kind != "topic" || page.Topic == nil || *page.Topic != topic.ID || page.Topics == nil || len(page.Topics) != 0 || page.Markdown == "" {
					t.Fatalf("page: %+v, %v", page, err)
				}
			}
		}
	}
}

func TestMCPGuideUnknownIdentityAndWireFields(t *testing.T) {
	r, err := MCPGuide(Context{Transport: "mcp-stdio", Locality: "local"}, "")
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		t.Fatal(err)
	}
	if len(fields) != 8 || string(fields["selfSessionId"]) != "null" || string(fields["tools"]) != "[]" || string(fields["topic"]) != "null" {
		t.Fatalf("wire fields: %s", data)
	}
	for _, topic := range []string{"commands", "recipes", "https://example.com", "unknown"} {
		if _, err := MCPGuide(Context{}, topic); err == nil {
			t.Errorf("accepted unavailable topic %q", topic)
		}
	}
}
