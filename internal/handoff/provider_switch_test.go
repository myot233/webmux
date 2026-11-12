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
	"github.com/rainchan/webmux/internal/session"
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

func TestProviderSwitchAtoBtoAPreservesEarlierRejectedRequest(t *testing.T) {
	h := deliveryTestWithSourceEnv(t, nil, map[string]string{"FAKEAGENT_FAIL": "session/prompt=-32000", "FAKEAGENT_FAIL_ONCE": "1"})
	ctx := context.Background()
	ids, err := h.store.RuntimeIDs(ctx, h.source.ConversationID)
	if err != nil || len(ids) != 1 {
		t.Fatal(ids, err)
	}
	id := ids[0]
	original, err := h.hybrids.Record(id)
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := h.launches.Get(original.ConfigID)
	if err != nil {
		t.Fatal(err)
	}
	// A rejects one request, then accepts the next in the same native process.
	// The rejected request must remain unseen despite the later acknowledgement.
	for i, text := range []string{"rejected A request", "accepted A request"} {
		if _, err := h.hybrids.Prompt(ctx, id, hybrid.PromptInput{ClientRequestID: "prompt-A-" + text[:8], Text: text}); err != nil {
			t.Fatal(err)
		}
		waitIdle(t, h.hybrids, id)
		sub, err := h.hybrids.Subscribe(id, nil)
		if err != nil {
			t.Fatal(err)
		}
		authRequired := sub.Snapshot.State.AuthRequired
		sub.Close()
		if authRequired != (i == 0) {
			t.Fatalf("prompt %q authRequired=%v", text, authRequired)
		}
	}
	// A's next process should resume normally rather than reinjecting the fault.
	delete(cfg.Env, "FAKEAGENT_FAIL")
	delete(cfg.Env, "FAKEAGENT_FAIL_ONCE")
	if _, err := h.launches.Update(cfg.ID, launch.Input{Name: cfg.Name, Cmd: cfg.Cmd, Env: cfg.Env, Hybrid: true}); err != nil {
		t.Fatal(err)
	}
	history, err := h.store.Activities(ctx, h.source.ConversationID, 0, 100, 2000)
	if err != nil {
		t.Fatal(err)
	}
	var rejectedID, acceptedID string
	for _, item := range history.Items {
		if item.Activity.Role == "user" {
			switch item.Activity.Text {
			case "rejected A request":
				rejectedID = item.Activity.ID
			case "accepted A request":
				acceptedID = item.Activity.ID
			}
		}
	}
	if rejectedID == "" || acceptedID == "" {
		t.Fatalf("missing actual prompt activities: rejected=%s accepted=%s", rejectedID, acceptedID)
	}
	bp, err := h.service.Preview(ctx, h.source.ConversationID, Request{ClientRequestID: "rejected-delta-B-preview", ConfigID: h.config.ID, UserText: "B work"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.service.Deliver(ctx, h.source.ConversationID, switchMessage(bp, "rejected-delta-B-message")); err != nil {
		t.Fatal(err)
	}
	b := h.waitStatus(bp.ID, "inline")
	waitIdle(t, h.hybrids, b.RuntimeID)
	back, err := h.service.Preview(ctx, h.source.ConversationID, Request{ClientRequestID: "rejected-delta-A-preview", ConfigID: cfg.ID, UserText: "return to A"})
	if err != nil {
		t.Fatal(err)
	}
	if back.Strategy != "delta_since_target_last_seen" || back.TargetProviderThreadID != h.source.ProviderThreadID {
		t.Fatalf("wrong continuation: %+v", back)
	}
	rejectedSelected, acceptedSelected := false, false
	for _, selected := range back.SelectedItemIDs {
		rejectedSelected = rejectedSelected || selected == rejectedID
		acceptedSelected = acceptedSelected || selected == acceptedID
	}
	if !rejectedSelected || acceptedSelected {
		t.Fatalf("delta lost consumption distinction: rejected=%v accepted=%v preview=%+v", rejectedSelected, acceptedSelected, back)
	}
	if _, err := h.service.Deliver(ctx, h.source.ConversationID, switchMessage(back, "rejected-delta-A-message")); err != nil {
		t.Fatal(err)
	}
	a := h.waitStatus(back.ID, "inline")
	waitIdle(t, h.hybrids, a.RuntimeID)
	restored, err := h.hybrids.Record(a.RuntimeID)
	if err != nil || !a.NativeContinue || a.RuntimeID != original.ID || restored.AgentSessionID != original.AgentSessionID {
		t.Fatalf("A native identity changed: execution=%+v record=%+v error=%v", a, restored, err)
	}
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

func TestInterruptedSourceQuiescesWithoutWaitingForExecutionTimeout(t *testing.T) {
	h := deliveryTest(t, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	ids, err := h.store.RuntimeIDs(ctx, h.source.ConversationID)
	if err != nil {
		t.Fatal(err)
	}
	id := ids[0]
	if _, err := h.hybrids.Switch(ctx, id, hybrid.SwitchInput{ClientRequestID: "source-to-terminal", To: hybrid.SideTUI}); err != nil {
		t.Fatal(err)
	}
	if _, err := h.hybrids.Input(ctx, id, hybrid.InputRequest{ClientRequestID: "terminal-exit-command", Text: "/exit", Submit: true}); err != nil {
		t.Fatal(err)
	}
	for {
		rec, err := h.hybrids.Record(id)
		if err == nil && rec.State == hybrid.StateInterrupted && rec.PID == 0 {
			break
		}
		select {
		case <-ctx.Done():
			t.Fatalf("source did not interrupt: %+v %v", rec, err)
		case <-time.After(10 * time.Millisecond):
		}
	}
	p, err := h.service.Preview(ctx, h.source.ConversationID, Request{ClientRequestID: "interrupted-source-preview", ConfigID: h.config.ID, UserText: "continue after terminal exit"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.service.Deliver(ctx, h.source.ConversationID, switchMessage(p, "interrupted-source-message")); err != nil {
		t.Fatal(err)
	}
	e := h.waitStatus(p.ID, "inline")
	if e.RuntimeID == id {
		t.Fatal("new provider reused source projection")
	}
	if _, err := h.manager.Get(id); err == nil {
		t.Fatal("interrupted shell was not retired")
	}
	rec, err := h.hybrids.Record(id)
	if err != nil || rec.State != hybrid.StateInterrupted {
		t.Fatal("source history record changed", rec, err)
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
	if err := h.store.AddGap(ctx, h.source.ProviderThreadID, "tui"); err != nil {
		t.Fatal(err)
	}
	full, err := h.service.Preview(ctx, h.source.ConversationID, Request{ClientRequestID: "gap-full-preview-B", ConfigID: h.config.ID, UserText: "new B task after gap"})
	if err != nil || full.Strategy != "full_thread_summary" || full.TargetProviderThreadID != b.TargetProviderThreadID {
		t.Fatal("gap did not use the existing B thread", full, err)
	}
	for _, prior := range p.SelectedItemIDs {
		for _, next := range full.SelectedItemIDs {
			if prior == next {
				t.Fatal("full strategy re-imported already delivered history", prior)
			}
		}
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

func TestPreRetirementFailureAllowsUnchangedSourceResume(t *testing.T) {
	for _, mode := range []string{"archive-active-turn", "restart-paused-queue"} {
		t.Run(mode, func(t *testing.T) {
			h := deliveryTestWithSourceEnv(t, nil, map[string]string{"FAKEAGENT_TURN_DELAY": "10s"})
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			ids, err := h.store.RuntimeIDs(ctx, h.source.ConversationID)
			if err != nil || len(ids) != 1 {
				t.Fatal(ids, err)
			}
			id := ids[0]
			original, err := h.hybrids.Record(id)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := h.hybrids.Prompt(ctx, id, hybrid.PromptInput{ClientRequestID: "source-active-turn", Text: "source still working"}); err != nil {
				t.Fatal(err)
			}
			if mode == "restart-paused-queue" {
				if result, err := h.hybrids.Prompt(ctx, id, hybrid.PromptInput{ClientRequestID: "source-queued-work", Text: "source queued work"}); err != nil || !result.Queued {
					t.Fatalf("work did not queue: %+v %v", result, err)
				}
				if err := h.hybrids.Interrupt(ctx, id, "pause-source-queue", "script"); err != nil {
					t.Fatal(err)
				}
				waitIdle(t, h.hybrids, id)
				sub, err := h.hybrids.Subscribe(id, nil)
				if err != nil {
					t.Fatal(err)
				}
				queue := sub.Snapshot.State.Queue
				sub.Close()
				if !queue.Paused || len(queue.Items) != 1 {
					t.Fatalf("missing paused source work: %+v", queue)
				}
			}
			p, err := h.service.Preview(ctx, h.source.ConversationID, Request{ClientRequestID: "pre-retirement-preview", ConfigID: h.config.ID, UserText: "switch to B"})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := h.service.Deliver(ctx, h.source.ConversationID, switchMessage(p, "pre-retirement-message")); err != nil {
				t.Fatal(err)
			}
			failed := h.waitStatus(p.ID, "failed")
			source, err := h.hybrids.Record(id)
			if err != nil || failed.ErrorClass != "unfinished_work" || failed.RuntimeID == id || source.State != hybrid.StateActive {
				t.Fatalf("switch retired source: execution=%+v source=%+v error=%v", failed, source, err)
			}
			if _, err := h.hybrids.Record(failed.RuntimeID); !errors.Is(err, hybrid.ErrNotFound) {
				t.Fatalf("pre-retirement failure created target: %v", err)
			}
			if mode == "restart-paused-queue" {
				h.close()
				h.open()
			} else {
				if err := h.manager.Close(id); err != nil {
					t.Fatal(err)
				}
				for {
					source, err := h.hybrids.Record(id)
					_, shellErr := h.manager.Get(id)
					if err == nil && source.State == hybrid.StateClosed && errors.Is(shellErr, session.ErrNotFound) {
						break
					}
					select {
					case <-ctx.Done():
						t.Fatal("source did not archive")
					case <-time.After(10 * time.Millisecond):
					}
				}
			}
			before := h.promptCalls()
			if _, err := h.workspace.ResumeHybrid(ctx, id, workspace.HybridResumeInput{ClientRequestID: "resume-unchanged-source", Side: "acp"}); err != nil {
				t.Fatalf("failed target prevented source resume: %v", err)
			}
			resumed, err := h.hybrids.Record(id)
			if err != nil || resumed.State != hybrid.StateActive || resumed.AgentSessionID != original.AgentSessionID || h.promptCalls() != before {
				t.Fatalf("source resume changed native identity or dispatched work: %+v %v", resumed, err)
			}
			if _, err := h.hybrids.Prompt(ctx, id, hybrid.PromptInput{ClientRequestID: "ordinary-source-after-resume", Text: "continue A"}); err != nil {
				t.Fatalf("unchanged source inherited failed target gate: %v", err)
			}
		})
	}
}

func TestUnresolvedSwitchCannotResumeSource(t *testing.T) {
	for _, status := range []string{"pending", "unknown"} {
		t.Run(status, func(t *testing.T) {
			env := map[string]string{}
			if status == "unknown" {
				env = map[string]string{"FAKEAGENT_CRASH": "prompt-saved", "FAKEAGENT_FAIL": "session/load=-32603"}
			}
			h := deliveryTest(t, env)
			ctx := context.Background()
			ids, err := h.store.RuntimeIDs(ctx, h.source.ConversationID)
			if err != nil || len(ids) != 1 {
				t.Fatal(ids, err)
			}
			if status == "pending" {
				h.service.pauseAfterPhase = func(phase string) bool { return phase == "session_created" }
			}
			p, err := h.service.Preview(ctx, h.source.ConversationID, Request{ClientRequestID: "unresolved-source-preview", ConfigID: h.config.ID, UserText: "B work"})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := h.service.Deliver(ctx, h.source.ConversationID, switchMessage(p, "unresolved-source-message")); err != nil {
				t.Fatal(err)
			}
			h.service.workers.Wait()
			h.waitStatus(p.ID, status)
			h.close()
			h.open()
			for _, side := range []string{"acp", "tui"} {
				if _, err := h.workspace.ResumeHybrid(ctx, ids[0], workspace.HybridResumeInput{ClientRequestID: "unresolved-source-" + side, Side: side}); !errors.Is(err, hybrid.ErrSwitching) {
					t.Fatalf("%s source bypassed owner on %s: %v", status, side, err)
				}
			}
			if _, err := h.manager.Get(ids[0]); err == nil {
				t.Fatal("unresolved source started a shell")
			}
		})
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
