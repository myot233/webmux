package handoff

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/rainchan/webmux/internal/conversation"
	"github.com/rainchan/webmux/internal/hybrid"
	"github.com/rainchan/webmux/internal/launch"
	"github.com/rainchan/webmux/internal/workspace"
	"maps"
)

func waitIdle(t *testing.T, s *hybrid.Service, id string) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		sub, err := s.Subscribe(id, nil)
		if err == nil {
			running := sub.Snapshot.State.Turn.Running
			sub.Close()
			if !running {
				return
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("agent did not become idle")
}

func TestArchivedProviderCannotResumeBesideActiveConversationWriter(t *testing.T) {
	h, original, b := switchToB(t, nil)
	ctx := context.Background()
	before := h.promptCalls()
	for _, side := range []string{"acp", "tui"} {
		_, err := h.workspace.ResumeHybrid(ctx, original.ID, workspace.HybridResumeInput{ClientRequestID: "ordinary-old-provider-" + side, Side: side})
		if !errors.Is(err, hybrid.ErrSwitching) {
			t.Fatalf("old provider admitted on %s: %v", side, err)
		}
	}
	if _, err := h.manager.Get(original.ID); err == nil {
		t.Fatal("old provider got a second shell")
	}
	rec, err := h.hybrids.Record(b.RuntimeID)
	if err != nil || rec.State != hybrid.StateActive || h.promptCalls() != before {
		t.Fatalf("active writer changed: %+v %v", rec, err)
	}
}

func TestUnknownDeliveryRecoveryCannotStartTerminalOrReplaceNativeIdentity(t *testing.T) {
	h := deliveryTest(t, map[string]string{"FAKEAGENT_CRASH": "prompt-saved", "FAKEAGENT_FAIL": "session/load=-32603"})
	ctx := context.Background()
	p, in := h.preview(conversation.PreviewLocation{})
	if _, err := h.service.Deliver(ctx, h.source.ConversationID, in); err != nil {
		t.Fatal(err)
	}
	e := h.waitStatus(p.ID, "unknown")
	h.close()
	h.open()
	for _, in := range []workspace.HybridResumeInput{
		{ClientRequestID: "unknown-terminal", Side: "tui"},
		{ClientRequestID: "unknown-new-native", Side: "acp", AgentSessionID: "another-native-thread"},
	} {
		if _, err := h.workspace.ResumeHybrid(ctx, e.RuntimeID, in); !errors.Is(err, hybrid.ErrSwitching) {
			t.Fatalf("unknown gate bypassed: %+v %v", in, err)
		}
	}
	if _, err := h.manager.Get(e.RuntimeID); err == nil {
		t.Fatal("unknown recovery started a shell")
	}
	if h.promptCalls() != 1 {
		t.Fatal("unknown delivery resent")
	}
}

func TestProviderSwitchAtoBtoAResumesOnlyUnseenDelta(t *testing.T) {
	h := deliveryTest(t, nil)
	ctx := context.Background()
	sourceIDs, err := h.store.RuntimeIDs(ctx, h.source.ConversationID)
	if err != nil || len(sourceIDs) != 1 {
		t.Fatal(sourceIDs, err)
	}
	originalRuntime := sourceIDs[0]
	original, err := h.hybrids.Record(originalRuntime)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.hybrids.Prompt(ctx, originalRuntime, hybrid.PromptInput{ClientRequestID: "initial-A-command", Text: "original A instruction"}); err != nil {
		t.Fatal(err)
	}
	waitIdle(t, h.hybrids, originalRuntime)
	aThread, err := h.store.ProviderThread(ctx, h.source.ProviderThreadID)
	if err != nil {
		t.Fatal(err)
	}
	if aThread.LastDeliveredRunOrdinal == 0 || aThread.LastDeliveredPosition == 0 {
		t.Fatalf("own work was not acknowledged: %+v", aThread)
	}
	p, err := h.service.Preview(ctx, h.source.ConversationID, Request{ClientRequestID: "switch-preview-B", ConfigID: h.config.ID, UserText: "task for B"})
	if err != nil {
		t.Fatal(err)
	}
	if p.TargetConversationID != h.source.ConversationID || p.Strategy != "full_thread_summary" {
		t.Fatalf("B preview: %+v", p)
	}
	message := func(p conversation.Preview, key string) MessageRequest {
		return MessageRequest{ClientRequestID: key, HandoffID: p.ID, Revision: p.Revision, Text: p.Request.UserText, ConfigID: p.Request.ConfigID, AgentConfig: p.Request.AgentConfig, Location: p.Request.Location, Attachments: p.Request.Attachments}
	}
	if _, err := h.service.Deliver(ctx, h.source.ConversationID, message(p, "switch-message-B")); err != nil {
		t.Fatal(err)
	}
	b := h.waitStatus(p.ID, "inline")
	live, err := h.manager.Get(b.RuntimeID)
	if err != nil || live.Info().Hybrid == nil || live.Info().Hybrid.ConversationID != h.source.ConversationID || live.Info().Hybrid.ProviderThreadID != b.TargetProviderThreadID {
		t.Fatal("runtime has no Conversation links", err)
	}
	waitIdle(t, h.hybrids, b.RuntimeID)
	rec, err := h.hybrids.Record(originalRuntime)
	if err != nil || rec.State != hybrid.StateClosed || rec.PID != 0 {
		t.Fatalf("old writer stayed active: %+v %v", rec, err)
	}
	back, err := h.service.Preview(ctx, h.source.ConversationID, Request{ClientRequestID: "switch-preview-A", ConfigID: original.ConfigID, UserText: "back to A"})
	if err != nil {
		t.Fatal(err)
	}
	if back.TargetProviderThreadID != h.source.ProviderThreadID || back.Strategy != "delta_since_target_last_seen" || !strings.Contains(back.RenderedText, "task for B") {
		t.Fatalf("wrong A delta: %+v", back)
	}
	for _, id := range back.SelectedItemIDs {
		item, err := h.store.Activity(ctx, h.source.ConversationID, id, 0, conversation.MaxActivityBytes)
		if err != nil || item.Activity.ProviderThreadID != b.TargetProviderThreadID {
			t.Fatalf("old native item was re-imported: %+v %v", item, err)
		}
	}
	if _, err := h.service.Deliver(ctx, h.source.ConversationID, message(back, "switch-message-A")); err != nil {
		t.Fatal(err)
	}
	a := h.waitStatus(back.ID, "inline")
	waitIdle(t, h.hybrids, a.RuntimeID)
	if a.RuntimeID != originalRuntime || !a.NativeContinue || a.TargetConversationID != h.source.ConversationID {
		t.Fatalf("A did not resume: %+v", a)
	}
	restored, err := h.hybrids.Record(a.RuntimeID)
	if err != nil || restored.AgentSessionID != original.AgentSessionID || restored.Cwd != original.Cwd || restored.Workspace != nil {
		t.Fatalf("native identity/location changed: %+v %v", restored, err)
	}
	records, err := h.hybrids.Records(false)
	if err != nil {
		t.Fatal(err)
	}
	writers := 0
	for _, rec := range records {
		if rec.State == hybrid.StateActive {
			writers++
		}
	}
	if writers != 1 {
		t.Fatalf("active writers=%d", writers)
	}
}

func switchMessage(p conversation.Preview, key string) MessageRequest {
	return MessageRequest{ClientRequestID: key, HandoffID: p.ID, Revision: p.Revision, Text: p.Request.UserText, ConfigID: p.Request.ConfigID, AgentConfig: p.Request.AgentConfig, Location: p.Request.Location, Attachments: p.Request.Attachments}
}

func switchToB(t *testing.T, env map[string]string) (*deliveryHarness, hybrid.Record, conversation.Execution) {
	t.Helper()
	h := deliveryTest(t, env)
	ctx := context.Background()
	ids, err := h.store.RuntimeIDs(ctx, h.source.ConversationID)
	if err != nil {
		t.Fatal(err)
	}
	original, err := h.hybrids.Record(ids[0])
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.hybrids.Prompt(ctx, ids[0], hybrid.PromptInput{ClientRequestID: "initial-A", Text: "initial A work"}); err != nil {
		t.Fatal(err)
	}
	waitIdle(t, h.hybrids, ids[0])
	p, err := h.service.Preview(ctx, h.source.ConversationID, Request{ClientRequestID: "preview-B", ConfigID: h.config.ID, UserText: "B work"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.service.Deliver(ctx, h.source.ConversationID, switchMessage(p, "message-B")); err != nil {
		t.Fatal(err)
	}
	e := h.waitStatus(p.ID, "inline")
	waitIdle(t, h.hybrids, e.RuntimeID)
	return h, original, e
}

func TestMissingNativeResumePreparesFreshWithoutSending(t *testing.T) {
	h, original, _ := switchToB(t, nil)
	ctx := context.Background()
	cfg, err := h.launches.Get(original.ConfigID)
	if err != nil {
		t.Fatal(err)
	}
	env := maps.Clone(cfg.Env)
	env["FAKEAGENT_NO_RESUME"] = "1"
	if _, err := h.launches.Update(cfg.ID, launch.Input{Name: cfg.Name, Cmd: cfg.Cmd, Env: env, Hybrid: true}); err != nil {
		t.Fatal(err)
	}
	p, err := h.service.Preview(ctx, h.source.ConversationID, Request{ClientRequestID: "preview-back-A", ConfigID: cfg.ID, UserText: "back A"})
	if err != nil {
		t.Fatal(err)
	}
	before := h.promptCalls()
	if _, err := h.service.Deliver(ctx, h.source.ConversationID, switchMessage(p, "message-back-A")); err != nil {
		t.Fatal(err)
	}
	e := h.waitStatus(p.ID, "failed")
	if e.ErrorClass != "fresh_preview_required" || e.ReplacementHandoffID == "" || h.promptCalls() != before {
		t.Fatalf("unsafe resume fallback: %+v", e)
	}
	old, err := h.service.Get(ctx, p.ID)
	if err != nil || old.State != "superseded" {
		t.Fatalf("old preview executable: %+v %v", old, err)
	}
	replacement, err := h.service.Get(ctx, e.ReplacementHandoffID)
	if err != nil || !replacement.Request.ForceFresh || replacement.TargetProviderThreadID == p.TargetProviderThreadID || replacement.Strategy != "full_thread_summary" || replacement.Budget.NativeUsed != 0 {
		t.Fatalf("wrong fresh draft: %+v %v", replacement, err)
	}
	if _, err := h.service.Deliver(ctx, h.source.ConversationID, switchMessage(replacement, "message-fresh-A")); err != nil {
		t.Fatal(err)
	}
	fresh := h.waitStatus(replacement.ID, "inline")
	if fresh.RuntimeID == original.ID || fresh.NativeContinue || h.promptCalls() != before+1 {
		t.Fatalf("fresh inherited identity: %+v", fresh)
	}
}

func TestActiveTurnPreventsSwitchAndFreshFallback(t *testing.T) {
	h, original, b := switchToB(t, map[string]string{"FAKEAGENT_TURN_DELAY": "250ms"})
	ctx := context.Background()
	if _, err := h.hybrids.Prompt(ctx, b.RuntimeID, hybrid.PromptInput{ClientRequestID: "busy-B-turn", Text: "still working"}); err != nil {
		t.Fatal(err)
	}
	p, err := h.service.Preview(ctx, h.source.ConversationID, Request{ClientRequestID: "busy-preview-A", ConfigID: original.ConfigID, UserText: "switch A"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.service.Deliver(ctx, h.source.ConversationID, switchMessage(p, "busy-message-A")); err != nil {
		t.Fatal(err)
	}
	e := h.waitStatus(p.ID, "failed")
	if e.ErrorClass != "unfinished_work" || e.ReplacementHandoffID != "" {
		t.Fatalf("forced a busy writer: %+v", e)
	}
	record, err := h.hybrids.Record(b.RuntimeID)
	if err != nil || record.State != hybrid.StateActive {
		t.Fatalf("busy writer was stopped: %+v %v", record, err)
	}
}

func TestPendingSwitchSurvivesRestartAndRejectsAnotherDevice(t *testing.T) {
	h := deliveryTest(t, nil)
	ctx := context.Background()
	first, err := h.service.Preview(ctx, h.source.ConversationID, Request{ClientRequestID: "pending-preview-B", ConfigID: h.config.ID, UserText: "B work"})
	if err != nil {
		t.Fatal(err)
	}
	h.service.pauseAfterPhase = func(phase string) bool { return phase == "session" }
	e, err := h.service.Deliver(ctx, h.source.ConversationID, switchMessage(first, "pending-message-B"))
	if err != nil {
		t.Fatal(err)
	}
	h.service.workers.Wait()
	cfg, err := h.launches.Create(launch.Input{Name: "C", Cmd: h.config.Cmd, Env: h.config.Env, Hybrid: true})
	if err != nil {
		t.Fatal(err)
	}
	other, err := h.service.Preview(ctx, h.source.ConversationID, Request{ClientRequestID: "other-device-preview", ConfigID: cfg.ID, UserText: "C work"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.service.Deliver(ctx, h.source.ConversationID, switchMessage(other, "other-device-message")); !errors.Is(err, conversation.ErrConflict) {
		t.Fatal("second device acquired the writer", err)
	}
	h.close()
	h.open()
	again, err := h.service.Deliver(ctx, h.source.ConversationID, switchMessage(first, "pending-message-B"))
	if err != nil || again.ID != e.ID || again.RuntimeID != e.RuntimeID {
		t.Fatalf("restart changed dispatch: %+v %v", again, err)
	}
	h.waitStatus(first.ID, "inline")
	if h.promptCalls() != 1 {
		t.Fatal("dispatch was repeated")
	}
}

func TestReportedBackgroundWorkPreventsSwitchAfterTurnEnds(t *testing.T) {
	h, original, b := switchToB(t, nil)
	ctx := context.Background()
	if _, err := h.hybrids.Prompt(ctx, b.RuntimeID, hybrid.PromptInput{ClientRequestID: "background-B", Text: "/background-noprocess"}); err != nil {
		t.Fatal(err)
	}
	waitIdle(t, h.hybrids, b.RuntimeID)
	p, err := h.service.Preview(ctx, h.source.ConversationID, Request{ClientRequestID: "background-preview-A", ConfigID: original.ConfigID, UserText: "switch A"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.service.Deliver(ctx, h.source.ConversationID, switchMessage(p, "background-message-A")); err != nil {
		t.Fatal(err)
	}
	e := h.waitStatus(p.ID, "failed")
	if e.ErrorClass != "unfinished_work" || e.ReplacementHandoffID != "" {
		t.Fatalf("fresh forced over background work: %+v", e)
	}
	record, err := h.hybrids.Record(b.RuntimeID)
	if err != nil || record.State != hybrid.StateActive {
		t.Fatal("background writer was stopped", err)
	}
}

func TestProviderSwitchAtoBtoCFixesConversationAndLocation(t *testing.T) {
	h, original, _ := switchToB(t, nil)
	ctx := context.Background()
	c, err := h.launches.Create(launch.Input{Name: "C", Cmd: h.config.Cmd, Env: h.config.Env, Hybrid: true})
	if err != nil {
		t.Fatal(err)
	}
	p, err := h.service.Preview(ctx, h.source.ConversationID, Request{ClientRequestID: "preview-C", ConfigID: c.ID, UserText: "C work"})
	if err != nil {
		t.Fatal(err)
	}
	if p.Strategy != "full_thread_summary" || p.TargetConversationID != h.source.ConversationID || p.Request.Location.Cwd != original.Cwd {
		t.Fatalf("wrong C target: %+v", p)
	}
	if _, err := h.service.Deliver(ctx, h.source.ConversationID, switchMessage(p, "message-C")); err != nil {
		t.Fatal(err)
	}
	e := h.waitStatus(p.ID, "inline")
	waitIdle(t, h.hybrids, e.RuntimeID)
	records, err := h.hybrids.Records(false)
	if err != nil {
		t.Fatal(err)
	}
	active := 0
	for _, record := range records {
		if record.State == hybrid.StateActive {
			active++
			if record.ID != e.RuntimeID || record.Cwd != original.Cwd {
				t.Fatal("wrong active writer")
			}
		}
	}
	if active != 1 {
		t.Fatalf("active writers=%d", active)
	}
}
