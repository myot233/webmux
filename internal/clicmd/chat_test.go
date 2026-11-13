package clicmd

import (
	"bufio"
	"strings"
	"testing"

	"github.com/rainchan/webmux/internal/contract"
)

func TestChatTextRendersEvents(t *testing.T) {
	var out strings.Builder
	w := bufio.NewWriter(&out)
	text := chatText{w: w}
	title, status := "Read go.mod", "completed"
	for _, ev := range []contract.ChatEvent{
		{TurnStart: &contract.ChatTurnStart{TurnID: "t1", PromptID: "p1"}},
		{Message: &contract.ChatMessageChunk{MessageID: "u1", Role: "user", Text: "hi"}},
		{Message: &contract.ChatMessageChunk{MessageID: "a1", Role: "thought", Text: "hidden"}},
		{Message: &contract.ChatMessageChunk{MessageID: "a2", Role: "agent", Text: "hel"}},
		{Message: &contract.ChatMessageChunk{MessageID: "a2", Role: "agent", Text: "lo"}},
		{Tool: &contract.ChatTool{ToolCallID: "c1", Title: &title, Status: &status}},
		{Request: &contract.ChatRequest{RequestID: "r1", Kind: "permission", Title: "Run make"}},
		{Resolved: &contract.ChatRequestOutcome{RequestID: "r1", Outcome: "selected"}},
		{TurnEnd: &contract.ChatTurnEnd{TurnID: "t1", StopReason: "error", Error: "boom"}},
	} {
		text.event(ev)
	}
	_ = w.Flush()
	want := "--- turn t1 started\nuser: hi\nagent: hello\ntool c1: Read go.mod (completed)\n" +
		"request r1 (permission): Run make\nrequest r1 closed: selected\n--- turn t1 ended: error: boom\n"
	if out.String() != want {
		t.Fatalf("got\n%s\nwant\n%s", out.String(), want)
	}
}

func tool(id, title, status string) contract.ChatEvent {
	t := &contract.ChatTool{ToolCallID: id}
	if title != "" {
		t.Title = &title
	}
	if status != "" {
		t.Status = &status
	}
	return contract.ChatEvent{Kind: contract.ChatEventTool, Tool: t}
}

func message(id, role, text string) contract.ChatEvent {
	return contract.ChatEvent{Kind: contract.ChatEventMessage, Message: &contract.ChatMessageChunk{MessageID: id, Role: role, Text: text}}
}

func TestFoldToolsKeepsTheFinalStatePerCall(t *testing.T) {
	in := []contract.ChatEvent{
		tool("c1", "Read go.mod", "pending"), message("a1", "agent", "x"),
		tool("c2", "Run make", "in_progress"), tool("c1", "", "in_progress"), tool("c1", "", "in_progress"),
		tool("c1", "", "completed"), tool("c2", "", "failed"),
	}
	for i := range in {
		in[i].Seq = uint64(i + 1)
	}
	got := foldTools(in)
	if len(got) != 3 || got[0].Tool.ToolCallID != "c1" || got[1].Message == nil || got[2].Tool.ToolCallID != "c2" {
		t.Fatalf("folded %+v", got)
	}
	if *got[0].Tool.Title != "Read go.mod" || *got[0].Tool.Status != "completed" || got[0].Seq != 6 {
		t.Fatalf("c1 folded to %+v seq %d", *got[0].Tool, got[0].Seq)
	}
	if *got[2].Tool.Title != "Run make" || *got[2].Tool.Status != "failed" {
		t.Fatalf("c2 folded to %+v", *got[2].Tool)
	}
	if *in[0].Tool.Status != "pending" {
		t.Fatal("foldTools modified its input")
	}
}

func TestFoldToolsIsScopedToATurn(t *testing.T) {
	start := contract.ChatEvent{Kind: contract.ChatEventTurnStart, TurnStart: &contract.ChatTurnStart{TurnID: "t2"}}
	got := foldTools([]contract.ChatEvent{tool("call_1", "Read", "completed"), start, tool("call_1", "Read", "in_progress"), tool("call_1", "", "failed")})
	if len(got) != 3 || *got[0].Tool.Status != "completed" || *got[2].Tool.Status != "failed" {
		t.Fatalf("a reused tool call ID folded across turns: %+v", got)
	}
}

func TestSelectChatLog(t *testing.T) {
	items := []contract.ChatEvent{
		message("u1", "user", "one"), message("t1", "thought", "hmm"), tool("c1", "Read", "in_progress"),
		message("a1", "agent", "two "), tool("c1", "", "completed"), message("a1", "agent", "parts"),
		message("u2", "user", "three"), tool("c2", "Run", "completed"), message("a2", "agent", "four"),
		{Kind: contract.ChatEventTurnEnd, TurnEnd: &contract.ChatTurnEnd{TurnID: "t"}},
	}
	texts := func(events []contract.ChatEvent) (out []string) {
		for _, ev := range events {
			switch {
			case ev.Message != nil:
				out = append(out, ev.Message.Text)
			case ev.Tool != nil:
				out = append(out, "tool:"+*ev.Tool.Status)
			default:
				out = append(out, "other")
			}
		}
		return out
	}
	for _, row := range []struct {
		name  string
		n     int
		tools bool
		want  string
	}{
		{"default is the last message", 1, false, "four"},
		{"default leaves tools out", 2, false, "three four"},
		{"a tool between the selected messages", 2, true, "three tool:completed four"},
		{"a message split by a tool joins at its first chunk", 3, false, "two parts three four"},
		{"more than the history holds", 50, false, "one two parts three four"},
		{"a tool folds to its final state at its first patch", 50, true, "one tool:completed two parts three tool:completed four"},
	} {
		if got := strings.Join(texts(selectChatLog(items, row.n, row.tools)), " "); got != row.want {
			t.Errorf("%s: got %q, want %q", row.name, got, row.want)
		}
	}
	if got := selectChatLog(nil, 1, true); len(got) != 0 {
		t.Fatalf("empty history: %+v", got)
	}
}

func TestViewNames(t *testing.T) {
	for _, name := range []string{"chat", "terminal"} {
		if got, ok := viewNamed(name); !ok || string(got) != name {
			t.Errorf("%s: %q %v", name, got, ok)
		}
	}
	for _, old := range []string{"tui", "acp", "switching", ""} {
		if _, ok := viewNamed(old); ok {
			t.Errorf("%q is not a view name", old)
		}
	}
}

func TestViewLabel(t *testing.T) {
	both := []contract.SessionView{contract.ViewTerminal, contract.ViewChat}
	for want, s := range map[string]contract.SessionInfo{
		"终端":  {Views: both, View: contract.ViewTerminal},
		"聊天":  {Views: both, View: contract.ViewChat},
		"切换中": {Views: both, View: contract.ViewTerminal, Transition: &contract.SessionTransition{To: contract.ViewChat, Phase: contract.PhaseStarting}},
	} {
		if got := viewLabel(s); got != want || !hasChatView(s) {
			t.Errorf("viewLabel %+v = %q, want %q", s, got, want)
		}
	}
	if hasChatView(contract.SessionInfo{Views: []contract.SessionView{contract.ViewTerminal}}) {
		t.Error("a terminal-only session has no chat view")
	}
}
