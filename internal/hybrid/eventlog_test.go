package hybrid

import (
	"errors"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/rainchan/webmux/internal/acp"
)

func textEvent(text string) Event {
	return Event{Kind: EventMessage, Message: &MessageChunk{MessageID: "m", Role: RoleAgent, Text: text}}
}

func TestChatLogResume(t *testing.T) {
	l := newChatLog(1<<20, ChatState{Agent: "omp"})
	now := time.Now()
	for range 5 {
		l.append(now, textEvent("x"))
	}
	fresh, err := l.subscribe(nil)
	if err != nil {
		t.Fatal(err)
	}
	if s := fresh.Snapshot; s.From != 1 || s.Next != 6 || s.Reset || s.Truncated || len(fresh.Events) != 5 || s.State.Agent != "omp" {
		t.Fatalf("fresh %+v, %d events", s, len(fresh.Events))
	}
	tail, _ := l.subscribe(&ResumePoint{Generation: fresh.Snapshot.Generation, Seq: 4})
	if tail.Snapshot.Reset || len(tail.Events) != 2 || tail.Events[0].Seq != 4 {
		t.Fatalf("resume within the log: %+v %d", tail.Snapshot, len(tail.Events))
	}
	current, _ := l.subscribe(&ResumePoint{Generation: fresh.Snapshot.Generation, Seq: 6})
	if current.Snapshot.Reset || len(current.Events) != 0 {
		t.Fatalf("resume at the end: %+v", current.Snapshot)
	}
	other, _ := l.subscribe(&ResumePoint{Generation: "other", Seq: 3})
	if !other.Snapshot.Reset || len(other.Events) != 5 {
		t.Fatalf("resume of another generation: %+v", other.Snapshot)
	}
	// Live frames follow the snapshot without a gap.
	l.append(now, textEvent("y"))
	l.setState(ChatState{Agent: "opencode"})
	f := <-tail.Out
	if len(f.Events) != 1 || f.Events[0].Seq != 6 || f.Generation != fresh.Snapshot.Generation {
		t.Fatalf("live events %+v", f)
	}
	if f = <-tail.Out; f.State == nil || f.State.Agent != "opencode" {
		t.Fatalf("live state %+v", f)
	}
	l.restart()
	if <-tail.Done; !errors.Is(tail.Err(), ErrReset) {
		t.Fatalf("restart ended subscriber with %v", tail.Err())
	}
	after, _ := l.subscribe(&ResumePoint{Generation: fresh.Snapshot.Generation, Seq: 6})
	if !after.Snapshot.Reset || after.Snapshot.Generation == fresh.Snapshot.Generation || len(after.Events) != 0 || after.Snapshot.From != 7 {
		t.Fatalf("after restart %+v", after.Snapshot)
	}
	l.end()
	if <-after.Done; !errors.Is(after.Err(), ErrEnded) {
		t.Fatalf("end: %v", after.Err())
	}
	if _, err := l.subscribe(nil); !errors.Is(err, ErrNotFound) {
		t.Fatalf("subscribe to an ended log: %v", err)
	}
}

func TestChatLogDropsOldestByBytes(t *testing.T) {
	one := eventSize(textEvent(strings.Repeat("a", 100)))
	l := newChatLog(3*one, ChatState{})
	for range 5 {
		l.append(time.Now(), textEvent(strings.Repeat("a", 100)))
	}
	sub, _ := l.subscribe(nil)
	if s := sub.Snapshot; !s.Truncated || s.From != 3 || s.Next != 6 || len(sub.Events) != 3 {
		t.Fatalf("snapshot %+v, %d events", s, len(sub.Events))
	}
	// The newest event stays even when it alone exceeds the budget.
	l.append(time.Now(), textEvent(strings.Repeat("b", 10*one)))
	sub, _ = l.subscribe(nil)
	if len(sub.Events) != 1 || sub.Snapshot.From != 6 {
		t.Fatalf("oversized newest event: %+v", sub.Snapshot)
	}
}

func TestChatLogDropsSlowSubscriber(t *testing.T) {
	l := newChatLog(1<<20, ChatState{})
	sub, _ := l.subscribe(nil)
	for range subscriberQueue + 1 {
		l.append(time.Now(), textEvent("x"))
	}
	select {
	case <-sub.Done:
	default:
		t.Fatal("slow subscriber kept")
	}
	if !errors.Is(sub.Err(), ErrSlow) {
		t.Fatalf("slow subscriber ended with %v", sub.Err())
	}
	sub.Close() // after the drop: no effect
}

func TestCapTextKeepsUTF8(t *testing.T) {
	s := strings.Repeat("a", maxFieldBytes-1) + "é" + "tail"
	got, cut := capText(s)
	if !cut || len(got) != maxFieldBytes-1 || !utf8.ValidString(got) {
		t.Fatalf("cut %v at %d bytes", cut, len(got))
	}
	if got, cut := capText("short"); cut || got != "short" {
		t.Fatal("short text changed")
	}
}

func TestTranslateContentAndMessages(t *testing.T) {
	var tr translator
	big := strings.Repeat("z", maxFieldBytes+1)
	old := "before"
	status := acp.ToolCompleted
	events := tr.update(&acp.ToolCallUpdate{ToolCallID: "c", Status: &status, Content: []acp.ToolCallContent{
		{Type: acp.ToolContentDiff, Path: "a.go", OldText: &old, NewText: big},
		{Type: acp.ToolContentTerminal, TerminalID: "t"},
		{Type: acp.ToolContentContent, Content: &acp.ContentBlock{Type: acp.ContentImage}},
		{Type: acp.ToolContentContent, Content: &acp.ContentBlock{Type: acp.ContentResourceLink, URI: "file:///a", Name: "a"}},
	}})
	c := events[0].Tool.Content
	if !c[0].Truncated || len(c[0].NewText) != maxFieldBytes || *c[0].OldText != old || c[0].Type != "diff" {
		t.Fatalf("diff %+v", c[0].Type)
	}
	if c[1].Type != "unsupported" || c[1].Unsupported != "terminal" || c[2].Unsupported != "image" || c[3].Type != "resource_link" || c[3].URI != "file:///a" {
		t.Fatalf("content %+v", c[1:])
	}
	if events[0].Tool.Title != nil || events[0].Tool.Locations != nil {
		t.Fatal("patch carries unchanged fields")
	}
	// Chunks without a messageId share an ID until another event or role.
	chunk := func(u acp.Update) string { return tr.update(u)[0].Message.MessageID }
	a := chunk(&acp.AgentMessageChunk{Content: acp.TextBlock("a")})
	b := chunk(&acp.AgentMessageChunk{Content: acp.TextBlock("b")})
	th := chunk(&acp.AgentThoughtChunk{Content: acp.TextBlock("t")})
	c2 := chunk(&acp.AgentMessageChunk{Content: acp.TextBlock("c")})
	tr.update(&acp.UsageUpdate{Used: 1, Size: 2})
	d := chunk(&acp.AgentMessageChunk{Content: acp.TextBlock("d")})
	given := chunk(&acp.AgentMessageChunk{Content: acp.TextBlock("e"), MessageID: "msg_9"})
	// A thought sharing the reply's ACP messageId is its own entry.
	thought := chunk(&acp.AgentThoughtChunk{Content: acp.TextBlock("f"), MessageID: "msg_9"})
	again := chunk(&acp.AgentMessageChunk{Content: acp.TextBlock("g"), MessageID: "msg_9"})
	if a != b || a == th || th == c2 || c2 == a || d == c2 || given != "msg_9" || thought == given || again != given {
		t.Fatalf("message IDs %q %q %q %q %q %q %q %q", a, b, th, c2, d, given, thought, again)
	}
	if text := tr.update(&acp.UserMessageChunk{Content: acp.ContentBlock{Type: acp.ContentAudio}})[0].Message.Text; !strings.Contains(text, "audio") {
		t.Fatalf("unsupported message content %q", text)
	}
	if tr.update(&acp.UnknownUpdate{Kind: "future"}) != nil {
		t.Fatal("unknown update produced events")
	}
	comp := tr.update(&acp.CompactionUpdate{CompactionID: "k", Status: acp.CompactionInProgress})[0].Compaction
	sum := tr.update(&acp.CompactionSummaryChunk{CompactionID: "k", Content: acp.TextBlock("sum")})[0].Compaction
	doneC := tr.update(&acp.CompactionUpdate{CompactionID: "k", Status: acp.CompactionCancelled})[0].Compaction
	if comp.Status != "running" || sum.Summary != "sum" || doneC.Status != "failed" || doneC.Summary != "sum" {
		t.Fatalf("compaction %+v %+v %+v", comp, sum, doneC)
	}
}

func TestFormRequest(t *testing.T) {
	r := formRequest("r1", acp.CreateElicitationRequest{Mode: acp.ElicitationForm, Message: "m", RequestedSchema: &acp.ElicitationSchema{
		Properties: map[string]acp.ElicitationProperty{
			"name":  {Type: "string", Default: []byte(`"x"`)},
			"color": {Type: "string", OneOf: []acp.EnumOption{{Const: "r", Title: "Red"}}},
			"ok":    {Type: "boolean"},
		},
		Required: []string{"name"},
	}})
	f := r.Form
	if r.Kind != RequestForm || !f.Supported || len(f.Fields) != 3 || f.Fields[0].Name != "color" || f.Fields[0].Type != "enum" || f.Fields[0].EnumNames[0] != "Red" {
		t.Fatalf("form %+v", f)
	}
	if f.Fields[1].Name != "name" || !f.Fields[1].Required || string(f.Fields[1].Default) != `"x"` || f.Fields[2].Type != "boolean" {
		t.Fatalf("fields %+v", f.Fields)
	}
	r = formRequest("r2", acp.CreateElicitationRequest{Mode: acp.ElicitationForm, RequestedSchema: &acp.ElicitationSchema{
		Properties: map[string]acp.ElicitationProperty{"n": {Type: "number"}},
	}})
	if r.Form.Supported || r.Form.Fields[0].Type != "unsupported" {
		t.Fatalf("number field %+v", r.Form)
	}
}

func TestStopReason(t *testing.T) {
	for _, row := range []struct {
		resp acp.PromptResponse
		err  error
		stop string
	}{
		{acp.PromptResponse{StopReason: acp.StopRefusal}, nil, acp.StopRefusal},
		{acp.PromptResponse{StopReason: "weird"}, nil, StopError},
		{acp.PromptResponse{}, &acp.RPCError{Code: acp.CodeAuthRequired, Message: "x"}, StopAuthRequired},
		{acp.PromptResponse{}, &acp.RPCError{Code: -32603, Message: "Authentication required: provider"}, StopAuthRequired},
		{acp.PromptResponse{}, &acp.RPCError{Code: -32603, Message: "Upstream request failed"}, StopError},
		{acp.PromptResponse{}, acp.ErrDisconnected, acp.StopCancelled},
	} {
		if stop, _ := stopReason(row.resp, row.err); stop != row.stop {
			t.Fatalf("%+v %v: %s", row.resp, row.err, stop)
		}
	}
}
