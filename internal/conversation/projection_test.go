package conversation

import "testing"

func TestPortableProjectionKeepsTypedContentAndExcludesPrivateKinds(t *testing.T) {
	for _, kind := range []string{"message", "command", "tool", "error", "interrupt", "file_change", "plan", "compaction"} {
		a := Activity{ID: "stable", RunID: "run", Kind: kind, Role: "agent", Text: "中文🙂\ncommand input / exit / output", Status: "failed", Source: "live"}
		p := ProjectActivity(a)
		if !p.Exportable || p.Text != a.Text || p.Kind != kind || p.ActivityID != a.ID || p.Status != a.Status {
			t.Fatalf("lost typed projection: %+v", p)
		}
	}
	for _, kind := range []string{"reasoning", "thought", "native_tool_state", "raw_acp", "permission", "turn_start", "marker", "unknown"} {
		p := ProjectActivity(Activity{Kind: kind, Role: "user", Text: "private"})
		if p.Exportable || p.Text != "" || p.ExclusionReason == "" {
			t.Fatalf("private kind promoted to message: %+v", p)
		}
	}
	p := ProjectActivity(Activity{Kind: "message", Role: "system", Text: "not a user message"})
	if p.Exportable {
		t.Fatal("non-user/agent message was exported")
	}
}
