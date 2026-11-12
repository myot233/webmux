package hybrid

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/rainchan/webmux/internal/acp"
	"github.com/rainchan/webmux/internal/agentprofile/agy"
	"github.com/rainchan/webmux/internal/agentprofile/kimi"
	"github.com/rainchan/webmux/internal/agentprofile/omp"
	"github.com/rainchan/webmux/internal/attentionkind"
)

func strPtr(s string) *string { return &s }

func TestWorkLinePrefersLatestActiveTool(t *testing.T) {
	var w workTracker
	pending := "pending"
	inProgress := "in_progress"
	done := "completed"
	read := "read"
	execute := "execute"
	w.tool(Tool{ToolCallID: "a", Kind: &read, Status: &pending})
	w.tool(Tool{ToolCallID: "b", Kind: &execute, Status: &inProgress})
	if got := w.line(); got != "执行" {
		t.Fatalf("kind fallback without title/locations: %q", got)
	}
	// A title arrives for the earlier call; the newest active call wins.
	w.tool(Tool{ToolCallID: "a", Title: strPtr("Read README.md")})
	if got := w.line(); got != "执行" {
		t.Fatalf("latest active tool should win: %q", got)
	}
	// The execute call finishes; the read tool is still pending.
	w.tool(Tool{ToolCallID: "b", Status: &done})
	if got := w.line(); got != "Read README.md" {
		t.Fatalf("falling back to the remaining active call: %q", got)
	}
	// Once every call in the turn has completed, the latest completed call
	// remains visible while the LLM thinks between tool calls.
	w.tool(Tool{ToolCallID: "a", Status: &done})
	if got := w.line(); got != "执行" {
		t.Fatalf("falling back to the latest completed call in the turn: %q", got)
	}
}

func TestWorkLineDefaultPendingAndSparsePatch(t *testing.T) {
	var w workTracker
	// A new tool_call without an explicit status defaults to pending, and
	// a later status patch with empty title/locations keeps the earlier ones.
	w.tool(Tool{ToolCallID: "a", Kind: strPtr("edit"), Title: strPtr("Edit service.go"), Locations: []Location{{Path: "/repo/service.go"}}})
	if got := w.line(); got != "Edit service.go" {
		t.Fatalf("default pending tool line: %q", got)
	}
	inProgress := "in_progress"
	empty := ""
	w.tool(Tool{ToolCallID: "a", Kind: &empty, Title: &empty, Status: &inProgress, Locations: []Location{}})
	if got := w.line(); got != "Edit service.go" {
		t.Fatalf("sparse patch preserved title: %q", got)
	}
}

func TestWorkLineToolTitleWins(t *testing.T) {
	var w workTracker
	inProgress := "in_progress"
	read := "read"
	title := "read the config\ncarefully"
	loc := []Location{{Path: "/repo/config.toml"}}
	w.tool(Tool{ToolCallID: "a", Kind: &read, Title: &title, Status: &inProgress, Locations: loc})
	if got := w.line(); got != "read the config carefully" {
		t.Fatalf("title kept on one line: %q", got)
	}
}

func TestWorkLineKindAndLocationFallback(t *testing.T) {
	var w workTracker
	pending := "pending"
	edit := "edit"
	w.tool(Tool{ToolCallID: "a", Kind: &edit, Status: &pending, Locations: []Location{{Path: "/repo/main.go"}}})
	if got := w.line(); got != "编辑 /repo/main.go" {
		t.Fatalf("kind label and location: %q", got)
	}
}

func TestWorkLinePlanFallback(t *testing.T) {
	var w workTracker
	w.setPlan(&Plan{Entries: []PlanEntry{
		{Content: "先写测试", Status: "completed"},
		{Content: "实现登录\n流程", Status: "in_progress"},
	}})
	if got := w.line(); got != "计划：实现登录 流程" {
		t.Fatalf("plan fallback: %q", got)
	}
	// A pending tool call takes precedence over the plan.
	pending := "pending"
	w.tool(Tool{ToolCallID: "a", Status: &pending, Title: strPtr("Run tests")})
	if got := w.line(); got != "Run tests" {
		t.Fatalf("tool wins over plan: %q", got)
	}
}

func TestWorkLineTruncation(t *testing.T) {
	var w workTracker
	inProgress := "in_progress"
	long := strings.Repeat("很长的标题", 40)
	w.tool(Tool{ToolCallID: "a", Status: &inProgress, Title: &long})
	got := w.line()
	if n := len([]rune(got)); n != workMaxRunes {
		t.Fatalf("truncated to %d runes, got %d: %q", workMaxRunes, n, got)
	}
	if !strings.HasSuffix(got, "…") {
		t.Fatalf("truncated line ends with ellipsis: %q", got)
	}
}

func TestWorkLineClearedWhenNothingActive(t *testing.T) {
	var w workTracker
	failed := "failed"
	w.tool(Tool{ToolCallID: "a", Status: &failed, Title: strPtr("Run tests")})
	if got := w.line(); got != "" {
		t.Fatalf("finished tool is not work: %q", got)
	}
}

func TestWorkEndTurnDropsInterruptedCalls(t *testing.T) {
	var w workTracker
	inProgress := "in_progress"
	pending := "pending"
	w.tool(Tool{ToolCallID: "a", Status: &inProgress, Title: strPtr("Run tests")})
	w.tool(Tool{ToolCallID: "b", Status: &pending, Title: strPtr("Read README.md")})
	w.endTurn()
	if got := w.line(); got != "" {
		t.Fatalf("interrupted calls are not work: %q", got)
	}
}

func TestWorkRunningOnlyForInProgress(t *testing.T) {
	var w workTracker
	inProgress, pending, done := "in_progress", "pending", "completed"
	w.tool(Tool{ToolCallID: "a", Status: &pending})
	w.tool(Tool{ToolCallID: "b", Status: &done})
	if w.running() {
		t.Fatal("pending and completed calls are not running")
	}
	w.tool(Tool{ToolCallID: "c", Status: &inProgress})
	if !w.running() {
		t.Fatal("in-progress call must count as running")
	}
	w.tool(Tool{ToolCallID: "c", Status: &done})
	if w.running() {
		t.Fatal("finished call must not count as running")
	}
}

func TestWorkBackgroundInput(t *testing.T) {
	tests := []struct {
		name   string
		update acp.Update
		want   bool
	}{
		{name: "Kimi", update: &acp.ToolCallUpdate{RawInput: json.RawMessage(`{"run_in_background":true}`)}, want: true},
		{name: "agy", update: &acp.ToolCall{RawInput: json.RawMessage(`{"IsDaemon":true}`)}, want: true},
		{name: "foreground", update: &acp.ToolCallUpdate{RawInput: json.RawMessage(`{"run_in_background":false,"IsDaemon":false}`)}},
		{name: "unrelated", update: &acp.ToolCallUpdate{RawInput: json.RawMessage(`{"command":"sleep 2"}`)}},
		{name: "malformed", update: &acp.ToolCallUpdate{RawInput: json.RawMessage(`{`)}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var w workTracker
			w.noteBackground(tt.update)
			if w.unfinished() != tt.want {
				t.Fatalf("unfinished=%v, want %v", w.unfinished(), tt.want)
			}
			w.endTurn()
			if w.unfinished() {
				t.Fatal("background state leaked into the next turn")
			}
		})
	}
}

func TestTurnWithBackgroundCommandIsDoneOnlyAfterItEnds(t *testing.T) {
	h := newHarness(t, 0)
	h.fast()
	s := h.create(h.input(agy.Name, SideACP))
	id := s.Info().ID
	st := h.follow(id)
	prompt(t, h, id, "hi /background")
	events := st.until("turn end", hasKind(EventTurnEnd))
	foundUnfinished := false
	for _, event := range events {
		if event.TurnEnd != nil && event.TurnEnd.Unfinished {
			foundUnfinished = true
		}
	}
	if !foundUnfinished {
		t.Fatal("turn-end event must carry the authoritative background-work state")
	}
	if a := s.Info().Attention; a != nil {
		t.Fatalf("attention %+v while the command still runs", a)
	}
	if !h.chatState(id).Turn.Unfinished {
		t.Fatal("turn should report Unfinished while tool was left in_progress")
	}
	waitFor(t, "done once the command ended", func() bool {
		a := s.Info().Attention
		return a != nil && a.Kind == attentionkind.Done
	})
	if got := h.replayedUsers(id); !slices.Equal(got, []string{"hi /background", agy.BackgroundStatusPrompt}) {
		t.Fatalf("background refresh prompts %q", got)
	}
	if h.chatState(id).Turn.Unfinished {
		t.Fatal("refresh must clear the unfinished turn state")
	}
	if actorState(h, id, func(a *actor) bool { return a.bgCancel != nil }) {
		t.Fatal("completed background input must not start another refresh")
	}
}

func TestTurnWithStartedBackgroundCommandIsDoneOnlyAfterItEnds(t *testing.T) {
	h := newHarness(t, 0)
	h.fast()
	s := h.create(h.input(kimi.Name, SideACP))
	id := s.Info().ID
	st := h.follow(id)
	prompt(t, h, id, "hi /background-started")
	st.until("turn end", hasKind(EventTurnEnd))
	if a := s.Info().Attention; a != nil {
		t.Fatalf("attention %+v while the command still runs", a)
	}
	if !h.chatState(id).Turn.Unfinished {
		t.Fatal("completed start call must not hide the running background command")
	}
	waitFor(t, "done once the command ended", func() bool {
		a := s.Info().Attention
		return a != nil && a.Kind == attentionkind.Done
	})
	if h.chatState(id).Turn.Unfinished {
		t.Fatal("completed background command must clear the unfinished turn state")
	}
	if got := h.replayedUsers(id); !slices.Equal(got, []string{"hi /background-started"}) {
		t.Fatalf("Kimi must not start an agy-specific refresh: %q", got)
	}
}

func TestTurnWithOpenToolWithoutNewProcessBecomesDoneAfterQuietPolls(t *testing.T) {
	h := newHarness(t, 0)
	h.fast()
	s := h.create(h.input(omp.Name, SideACP))
	id := s.Info().ID
	st := h.follow(id)
	prompt(t, h, id, "hi /background-noprocess")
	st.until("turn end", hasKind(EventTurnEnd))
	if !h.chatState(id).Turn.Unfinished {
		t.Fatal("turn should report Unfinished")
	}
	waitFor(t, "done after two quiet polls", func() bool {
		a := s.Info().Attention
		return a != nil && a.Kind == attentionkind.Done
	})
}

func TestBackgroundWatchStoppedByNewTurnOrSwitch(t *testing.T) {
	h := newHarness(t, 0)
	h.fast()
	s := h.create(h.input(omp.Name, SideACP))
	id := s.Info().ID
	st := h.follow(id)
	prompt(t, h, id, "first /background")
	st.until("first turn end", hasKind(EventTurnEnd))
	if !actorState(h, id, func(a *actor) bool { return a.bgCancel != nil }) {
		t.Fatal("background watch should be active")
	}
	// A new turn clears Unfinished and stops the previous watch.
	chatTurn(t, h, id, "second")
	if h.chatState(id).Turn.Unfinished {
		t.Fatal("normal turn must clear Unfinished")
	}
	// Start another backgrounded turn, then switch to TUI: the watch stops.
	prompt(t, h, id, "third /background")
	st.until("third turn end", func(ev []Event) bool {
		n := 0
		for _, e := range ev {
			if e.Kind == EventTurnEnd {
				n++
			}
		}
		return n >= 3
	})
	h.mustSwitch(id, SideTUI)
	if actorState(h, id, func(a *actor) bool { return a.bgCancel != nil }) {
		t.Fatal("switching to TUI must stop the background watch")
	}
}

func TestHybridWorkDetailDuringTurn(t *testing.T) {
	h := newHarness(t, 0)
	s := h.create(h.input(omp.Name, SideACP, "FAKEAGENT_TURN_DELAY=300ms"))
	id := s.Info().ID
	listSub := h.svc.manager.Subscribe()
	defer h.svc.manager.Unsubscribe(listSub)
	st := h.follow(id)
	prompt(t, h, id, "hello")
	waitFor(t, "work detail published to session list", func() bool {
		for {
			select {
			case ev := <-listSub.Out:
				if ev.Type == "updated" && ev.Session != nil && ev.Session.ID == id &&
					ev.Session.Hybrid != nil && ev.Session.Hybrid.Work == "Read file" {
					return true
				}
			default:
				return false
			}
		}
	})
	st.until("turn end", hasKind(EventTurnEnd))
	waitFor(t, "work cleared in session list", func() bool {
		for {
			select {
			case ev := <-listSub.Out:
				if ev.Type == "updated" && ev.Session != nil && ev.Session.ID == id &&
					ev.Session.Hybrid != nil && ev.Session.Hybrid.Work == "" {
					return true
				}
			default:
				return false
			}
		}
	})
}
