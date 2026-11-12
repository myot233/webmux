package hybrid

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/rainchan/webmux/internal/acp"
	"github.com/rainchan/webmux/internal/conversation"
)

func historyHarness(t *testing.T) (*harness, *conversation.Service) {
	t.Helper()
	h := newHarness(t, 1024)
	db := conversation.Open(conversation.Options{StateDir: h.state})
	if err := db.Unavailable(); err != nil {
		t.Fatal(err)
	}
	h.svc.conversations = db
	t.Cleanup(func() {
		h.svc.Close()
		if err := db.Close(); err != nil {
			t.Error(err)
		}
	})
	return h, db
}

func TestActivityRecorderRunsBeforeUITruncation(t *testing.T) {
	ctx := context.Background()
	h, db := historyHarness(t)
	id := h.create(h.input("omp", SideACP)).Info().ID
	a, err := h.svc.actor(id)
	if err != nil {
		t.Fatal(err)
	}
	text := strings.Repeat("你好🙂", 40000)
	if !a.sync(func() {
		if err := a.beginActivityRun("question", nil); err != nil {
			t.Error(err)
			return
		}
		a.onUpdate(a.epoch, acp.SessionNotification{Update: &acp.AgentMessageChunk{Content: acp.TextBlock(text), MessageID: "provider-message"}})
		a.onUpdate(a.epoch, acp.SessionNotification{Update: &acp.AgentThoughtChunk{Content: acp.TextBlock("private reasoning")}})
		a.onUpdate(a.epoch, acp.SessionNotification{Update: &acp.ToolCall{ToolCallID: "tool", Title: "Read output", Status: "completed", Content: []acp.ToolCallContent{{Type: acp.ToolContentContent, Content: &acp.ContentBlock{Type: acp.ContentText, Text: text}}}, RawInput: []byte(`{"env":{"SECRET":"never copy"}}`)}})
		a.finishActivityRun(acp.StopEndTurn)
	}) {
		t.Fatal("actor stopped")
	}
	b, err := db.Binding(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	page, err := db.Activities(ctx, b.ConversationID, 0, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	found := 0
	for _, item := range page.Items {
		if item.Activity.ProviderItemID != "provider-message" && item.Activity.ProviderItemID != "tool" {
			continue
		}
		var body strings.Builder
		offset := int64(0)
		for {
			part, err := db.Activity(ctx, b.ConversationID, item.Activity.ID, offset, 4000)
			if err != nil {
				t.Fatal(err)
			}
			body.WriteString(part.Activity.Text)
			offset = part.NextTextOffset
			if !part.Truncated {
				break
			}
		}
		want := text
		if item.Activity.ProviderItemID == "tool" {
			want = "Read output\n" + text
		}
		if body.String() != want {
			t.Fatalf("full content lost after UI cap: %d/%d", body.Len(), len(text))
		}
		found++
	}
	if found != 2 {
		t.Fatalf("captured long items=%d", found)
	}
	for _, item := range page.Items {
		if strings.Contains(item.Activity.Text, "private reasoning") || strings.Contains(item.Activity.Text, "never copy") {
			t.Fatal("persisted private state")
		}
	}
	if strings.Contains(h.logs.String(), "你好") || strings.Contains(h.logs.String(), "private reasoning") {
		t.Fatal("body logged")
	}
}

func TestActivityRecorderKeepsBackgroundToolOnOriginalRun(t *testing.T) {
	ctx := context.Background()
	h, db := historyHarness(t)
	id := h.create(h.input("omp", SideACP)).Info().ID
	a, err := h.svc.actor(id)
	if err != nil {
		t.Fatal(err)
	}
	if !a.sync(func() {
		if err := a.beginActivityRun("question", nil); err != nil {
			t.Error(err)
			return
		}
		a.recordUpdate(&acp.ToolCall{ToolCallID: "background", Title: "Working", Status: "in_progress"})
		a.finishActivityRun(acp.StopEndTurn)
		if err := a.beginActivityRun("next", nil); err != nil {
			t.Error(err)
			return
		}
		status := "completed"
		a.recordUpdate(&acp.ToolCallUpdate{ToolCallID: "background", Status: &status, Content: []acp.ToolCallContent{{Type: acp.ToolContentContent, Content: &acp.ContentBlock{Type: acp.ContentText, Text: "done"}}}})
		a.finishActivityRun(acp.StopEndTurn)
	}) {
		t.Fatal("actor stopped")
	}
	b, err := db.Binding(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	page, err := db.Activities(ctx, b.ConversationID, 0, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, item := range page.Items {
		if item.Activity.ProviderItemID == "background" {
			found = true
			if item.Activity.RunOrdinal != 1 || item.Activity.Status != "completed" || item.Activity.Text != "Working\ndone" {
				t.Fatalf("background attribution: %+v", item)
			}
		}
	}
	if !found {
		t.Fatal("background result lost")
	}
}

func TestActivityRecorderNormalizesCommandExitAndFileChange(t *testing.T) {
	ctx := context.Background()
	h, db := historyHarness(t)
	id := h.create(h.input("omp", SideACP)).Info().ID
	a, err := h.svc.actor(id)
	if err != nil {
		t.Fatal(err)
	}
	if !a.sync(func() {
		if err := a.beginActivityRun("question", nil); err != nil {
			t.Error(err)
			return
		}
		a.recordUpdate(&acp.ToolCall{ToolCallID: "command", Kind: "execute", Status: "completed", Title: "Run", RawInput: []byte(`{"command":["printf","hello world"],"env":{"SECRET":"private-env"}}`), RawOutput: []byte(`{"exitCode":7,"privateState":"never-store"}`)})
		a.recordUpdate(&acp.ToolCall{ToolCallID: "file", Kind: "edit", Status: "completed", Content: []acp.ToolCallContent{{Type: acp.ToolContentDiff, Path: "file.go", NewText: "after"}}})
		a.finishActivityRun(acp.StopEndTurn)
	}) {
		t.Fatal("actor stopped")
	}
	binding, err := db.Binding(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	page, err := db.Activities(ctx, binding.ConversationID, 0, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	command, file := false, false
	for _, item := range page.Items {
		if strings.Contains(item.Activity.Text, "private-env") || strings.Contains(item.Activity.Text, "never-store") {
			t.Fatal("private state persisted")
		}
		switch item.Activity.Kind {
		case "command":
			command = strings.Contains(item.Activity.Text, `"printf" "hello world"`) && strings.Contains(item.Activity.Text, "Exit code: 7")
		case "file_change":
			file = strings.Contains(item.Activity.Text, "File: file.go") && strings.Contains(item.Activity.Text, "after")
		}
	}
	if !command || !file {
		t.Fatalf("missing normalized command/file: %v %v", command, file)
	}
}

func TestActivityCaptureAndColdReplayUseSameIdentities(t *testing.T) {
	ctx := context.Background()
	h, db := historyHarness(t)
	id := h.create(h.input("omp", SideACP)).Info().ID
	chatTurn(t, h, id, "a real prompt")
	binding, err := db.Binding(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	before, err := db.Activities(ctx, binding.ConversationID, 0, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	messages := map[string]string{}
	for _, part := range before.Items {
		if part.Activity.Kind == "message" {
			messages[part.Activity.ID] = part.Activity.Text
		}
	}
	if len(messages) != 2 {
		t.Fatalf("captured messages: %v", messages)
	}
	if err := h.manager.Close(id); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "closed", func() bool { r, err := h.svc.Record(id); return err == nil && r.State == StateClosed })
	if _, err := h.svc.Resume(ctx, id, ResumeInput{ClientRequestID: rid(), Side: SideACP, Env: h.input("omp", SideACP).Env}); err != nil {
		t.Fatal(err)
	}
	after, err := db.Activities(ctx, binding.ConversationID, 0, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, part := range after.Items {
		if part.Activity.Kind == "message" {
			count++
			if text, ok := messages[part.Activity.ID]; !ok || text != part.Activity.Text {
				t.Fatalf("cold replay duplicated message: %+v", part.Activity)
			}
		}
	}
	if count != 2 {
		t.Fatalf("cold replay changed logical count: %d", count)
	}
}

func TestActivityToolMetadataPatchesPreserveOutput(t *testing.T) {
	ctx := context.Background()
	h, db := historyHarness(t)
	id := h.create(h.input("omp", SideACP)).Info().ID
	a, err := h.svc.actor(id)
	if err != nil {
		t.Fatal(err)
	}
	if !a.sync(func() {
		if err := a.beginActivityRun("question", nil); err != nil {
			t.Error(err)
			return
		}
		a.recordUpdate(&acp.ToolCall{ToolCallID: "patch", Title: "Initial", Kind: "execute", Status: "in_progress", Content: []acp.ToolCallContent{{Type: acp.ToolContentContent, Content: &acp.ContentBlock{Type: acp.ContentText, Text: "existing output"}}}})
		title := "Updated title"
		a.recordUpdate(&acp.ToolCallUpdate{ToolCallID: "patch", Title: &title, RawInput: []byte(`{"command":"printf hello","env":{"SECRET":"private"}}`)})
		status := "completed"
		a.recordUpdate(&acp.ToolCallUpdate{ToolCallID: "patch", Status: &status, RawOutput: []byte(`{"exitCode":3}`)})
		a.finishActivityRun(acp.StopEndTurn)
	}) {
		t.Fatal("actor stopped")
	}
	binding, err := db.Binding(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	page, err := db.Activities(ctx, binding.ConversationID, 0, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, part := range page.Items {
		if part.Activity.ProviderItemID == "patch" {
			found = true
			want := "Updated title\nCommand input: printf hello\nExit code: 3\nexisting output"
			if part.Activity.Text != want {
				t.Fatalf("metadata patch lost content: %q", part.Activity.Text)
			}
		}
	}
	if !found {
		t.Fatal("missing patched tool")
	}
}

func TestActivityOmittedToolStatusStaysPendingAcrossTurns(t *testing.T) {
	ctx := context.Background()
	h, db := historyHarness(t)
	id := h.create(h.input("omp", SideACP)).Info().ID
	a, err := h.svc.actor(id)
	if err != nil {
		t.Fatal(err)
	}
	if !a.sync(func() {
		if err := a.beginActivityRun("question", nil); err != nil {
			t.Error(err)
			return
		}
		a.recordUpdate(&acp.ToolCall{ToolCallID: "omitted", Title: "Pending"})
		a.finishActivityRun(acp.StopEndTurn)
	}) {
		t.Fatal("actor stopped")
	}
	binding, err := db.Binding(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	page, err := db.Activities(ctx, binding.ConversationID, 0, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, part := range page.Items {
		if part.Activity.ProviderItemID == "omitted" {
			t.Fatal("unfinished tool published")
		}
	}
	if !a.sync(func() {
		if err := a.beginActivityRun("next", nil); err != nil {
			t.Error(err)
			return
		}
		status := "completed"
		a.recordUpdate(&acp.ToolCallUpdate{ToolCallID: "omitted", Status: &status, Content: []acp.ToolCallContent{{Type: acp.ToolContentContent, Content: &acp.ContentBlock{Type: acp.ContentText, Text: "late result"}}}})
		a.finishActivityRun(acp.StopEndTurn)
	}) {
		t.Fatal("actor stopped")
	}
	page, err = db.Activities(ctx, binding.ConversationID, 0, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, part := range page.Items {
		if part.Activity.ProviderItemID == "omitted" {
			count++
			if part.Activity.RunOrdinal != 1 || part.Activity.Text != "Pending\nlate result" {
				t.Fatalf("late result lost original run: %+v", part.Activity)
			}
		}
	}
	if count != 1 {
		t.Fatalf("tool duplicated or lost: %d", count)
	}
}

func TestActivitySteeringCreatesMessageBoundaryAndDiscardsExplicitFailure(t *testing.T) {
	for _, outcome := range []string{"injected", "failed"} {
		t.Run(outcome, func(t *testing.T) {
			ctx := context.Background()
			h, db := historyHarness(t)
			in := h.codexInput(SideACP, "")
			in.Env["FAKEAGENT_TURN_DELAY"] = "2s"
			in.Env["FAKEAGENT_STEER_OUTCOME"] = outcome
			id := h.create(in).Info().ID
			a, err := h.svc.actor(id)
			if err != nil {
				t.Fatal(err)
			}
			prompt(t, h, id, "task")
			if !a.sync(func() { a.recordUpdate(&acp.AgentMessageChunk{Content: acp.TextBlock("before")}) }) {
				t.Fatal("actor stopped")
			}
			_, err = h.svc.Prompt(ctx, id, PromptInput{ClientRequestID: rid(), Text: "nudge", Mode: ModeSteer})
			if outcome == "failed" && !errors.Is(err, ErrAgentCall) {
				t.Fatalf("steering failed: %v", err)
			}
			if outcome == "injected" && err != nil {
				t.Fatal(err)
			}
			if !a.sync(func() { a.recordUpdate(&acp.AgentMessageChunk{Content: acp.TextBlock("after")}) }) {
				t.Fatal("actor stopped")
			}
			if err := h.manager.Close(id); err != nil {
				t.Fatal(err)
			}
			waitFor(t, "closed", func() bool { r, err := h.svc.Record(id); return err == nil && r.State == StateClosed })
			binding, err := db.Binding(ctx, id)
			if err != nil {
				t.Fatal(err)
			}
			h.svc.Close()
			if err := db.Close(); err != nil {
				t.Fatal(err)
			}
			reopened := conversation.Open(conversation.Options{StateDir: h.state})
			defer func() { _ = reopened.Close() }()
			if err := reopened.Unavailable(); err != nil {
				t.Fatal(err)
			}
			page, err := reopened.Activities(ctx, binding.ConversationID, 0, 0, 0)
			if err != nil {
				t.Fatal(err)
			}
			var texts []string
			for _, part := range page.Items {
				if part.Activity.Kind == "message" {
					texts = append(texts, part.Activity.Text)
				}
			}
			want := "task,before,nudge,after"
			if outcome == "failed" {
				want = "task,beforeafter"
			}
			if strings.Join(texts, ",") != want {
				t.Fatalf("steering order/draft: %v want %s", texts, want)
			}
			if page.ReconcileRequired || len(page.Gaps) != 0 {
				t.Fatalf("explicit outcome created crash gap: %+v", page.Gaps)
			}
		})
	}
}

func TestCaptureDisabledKeepsLiveChatWithoutWritingNormalizedBodies(t *testing.T) {
	h := newHarness(t, 1024)
	db := conversation.Open(conversation.Options{StateDir: h.state, DisableCapture: true})
	h.svc.conversations = db
	t.Cleanup(func() { h.svc.Close(); _ = db.Close() })
	id := h.create(h.input("omp", SideACP)).Info().ID
	text := "中文🙂"
	chatTurn(t, h, id, text)
	b, err := db.Binding(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	page, err := db.Activities(context.Background(), b.ConversationID, 0, 20, 4000)
	if err != nil || len(page.Items) != 0 || len(page.Gaps) == 0 {
		t.Fatal("disabled capture retained bodies or lost gap", page, err)
	}
	sub, err := h.svc.Subscribe(id, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer sub.Close()
	var reply strings.Builder
	for _, e := range sub.Events {
		if e.Kind == EventMessage && e.Message.Role == RoleAgent {
			reply.WriteString(e.Message.Text)
		}
	}
	if reply.String() != "echo: "+text {
		t.Fatal("live UTF-8 chat damaged", reply.String())
	}
	if _, err := db.WriteActivity(context.Background(), conversation.ActivityInput{ProviderThreadID: b.ProviderThreadID, LocalKey: "bypass", Kind: "message", Role: "user", Text: "secret"}); !errors.Is(err, conversation.ErrUnavailable) {
		t.Fatal("direct capture bypassed global switch", err)
	}
}

func TestDisabledIntervalIsNotImportedWhenCaptureResumes(t *testing.T) {
	ctx := context.Background()
	h, db := historyHarness(t)
	id := h.create(h.input("omp", SideACP)).Info().ID
	b, err := db.Binding(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AddGap(ctx, b.ProviderThreadID, "capture_disabled"); err != nil {
		t.Fatal(err)
	}
	a, err := h.svc.actor(id)
	if err != nil {
		t.Fatal(err)
	}
	if !a.sync(func() {
		a.beginActivityReplay()
		a.onUpdate(a.epoch, acp.SessionNotification{Update: &acp.UserMessageChunk{Content: acp.TextBlock("private while disabled")}})
		a.onUpdate(a.epoch, acp.SessionNotification{Update: &acp.AgentMessageChunk{Content: acp.TextBlock("private old reply")}})
		a.finishActivityReplay()
		if err := a.beginActivityRun("new public task", nil); err != nil {
			t.Error(err)
		}
		a.onUpdate(a.epoch, acp.SessionNotification{Update: &acp.AgentMessageChunk{Content: acp.TextBlock("new public reply")}})
		a.finishActivityRun(acp.StopEndTurn)
	}) {
		t.Fatal("actor stopped")
	}
	page, err := db.Activities(ctx, b.ConversationID, 0, 20, 4000)
	if err != nil {
		t.Fatal(err)
	}
	var text strings.Builder
	for _, item := range page.Items {
		text.WriteString(item.Activity.Text)
	}
	if strings.Contains(text.String(), "private") || !strings.Contains(text.String(), "new public task") || !strings.Contains(text.String(), "new public reply") {
		t.Fatal("disabled interval was imported or new capture stopped", text.String())
	}
}

func TestNoIDReplayKeepsLaterLegitimateCopyOfHistoricalPacket(t *testing.T) {
	ctx := context.Background()
	h, db := historyHarness(t)
	id := h.create(h.input("omp", SideACP)).Info().ID
	b, err := db.Binding(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	thread, err := db.ProviderThread(ctx, b.ProviderThreadID)
	if err != nil {
		t.Fatal(err)
	}
	p, err := db.ReservePreview(ctx, conversation.PreviewReservation{SourceConversationID: b.ConversationID, ClientRequestID: "packet-preview", RequestHash: "preview", Request: conversation.PreviewRequest{ConfigID: thread.ConfigID, AgentName: thread.AgentName, TargetProviderThreadID: thread.ID, UserText: "original"}})
	if err != nil {
		t.Fatal(err)
	}
	p, err = db.CompletePreview(ctx, p.ID, conversation.PreviewResult{Budget: conversation.PreviewBudget{History: 16000}})
	if err != nil {
		t.Fatal(err)
	}
	e, err := db.BeginExecution(ctx, conversation.ExecutionInput{SourceConversationID: b.ConversationID, HandoffID: p.ID, ClientRequestID: "packet-command", RequestHash: "command", Revision: p.Revision})
	if err != nil {
		t.Fatal(err)
	}
	packet := "random-scoped historical envelope\nUser message:\noriginal"
	e.Phase = "sending"
	e.NativeThreadID = thread.AgentSessionID
	e.PromptHash = conversation.TextHash(packet)
	if err := db.CheckpointExecution(ctx, e); err != nil {
		t.Fatal(err)
	}
	if err := db.ClaimExecutionText(ctx, e.ID, e.PromptHash, e.PromptHash); err != nil {
		t.Fatal(err)
	}
	a, err := h.svc.actor(id)
	if err != nil {
		t.Fatal(err)
	}
	if !a.sync(func() {
		a.beginActivityReplay()
		for _, update := range []acp.Update{&acp.UserMessageChunk{Content: acp.TextBlock(packet)}, &acp.AgentMessageChunk{Content: acp.TextBlock("original reply")}, &acp.UserMessageChunk{Content: acp.TextBlock(packet)}, &acp.AgentMessageChunk{Content: acp.TextBlock("quoted packet reply")}} {
			a.onUpdate(a.epoch, acp.SessionNotification{Update: update})
		}
		a.finishActivityReplay()
	}) {
		t.Fatal("actor stopped")
	}
	users := h.replayedUsers(id)
	if len(users) != 2 || users[0] != "original" || users[1] != packet {
		t.Fatal("legitimate no-ID duplicate was stripped", users)
	}
}

func TestBackgroundCompactionWaitsForOwnCompletionAndKeepsOriginalRun(t *testing.T) {
	ctx := context.Background()
	h, db := historyHarness(t)
	id := h.create(h.input("omp", SideACP)).Info().ID
	b, err := db.Binding(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	a, err := h.svc.actor(id)
	if err != nil {
		t.Fatal(err)
	}
	if !a.sync(func() {
		if err := a.beginActivityRun("first task", nil); err != nil {
			t.Error(err)
		}
		a.recordUpdate(&acp.CompactionUpdate{CompactionID: "background", Status: "in_progress", Summary: []acp.ContentBlock{acp.TextBlock("complete ")}})
		a.finishActivityRun(acp.StopEndTurn)
	}) {
		t.Fatal("actor stopped")
	}
	first, err := db.Activities(ctx, b.ConversationID, 0, 20, 4000)
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range first.Items {
		if item.Activity.Kind == "compaction" {
			t.Fatal("turn end published unfinished compaction")
		}
	}
	if !a.sync(func() {
		if err := a.beginActivityRun("second task", nil); err != nil {
			t.Error(err)
		}
		a.recordUpdate(&acp.CompactionSummaryChunk{CompactionID: "background", Content: acp.TextBlock("summary")})
		a.recordUpdate(&acp.CompactionUpdate{CompactionID: "background", Status: "completed"})
		a.finishActivityRun(acp.StopEndTurn)
	}) {
		t.Fatal("actor stopped")
	}
	page, err := db.Activities(ctx, b.ConversationID, 0, 20, 4000)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, item := range page.Items {
		if item.Activity.Kind == "compaction" {
			found = true
			if item.Activity.RunOrdinal != 1 || item.Activity.Status != "completed" || item.Activity.Text != "complete summary" || item.Activity.Position <= first.NextPosition {
				t.Fatal("late compaction lost provenance", item.Activity)
			}
		}
	}
	if !found {
		t.Fatal("completed compaction missing")
	}
}
