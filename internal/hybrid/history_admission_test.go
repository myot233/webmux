package hybrid

import (
	"context"
	"errors"
	"syscall"
	"testing"
	"time"

	"github.com/rainchan/webmux/internal/agentprofile"
	"github.com/rainchan/webmux/internal/conversation"
)

func TestHistoryWriterRejectsUnreportedChildWork(t *testing.T) {
	h := newHarness(t, 0)
	shell := h.create(h.codexInput(SideACP, ""))
	id := shell.Info().ID
	stream := h.follow(id)
	prompt(t, h, id, "hi /background")
	stream.until("turn ended with surviving process", hasKind(EventTurnEnd))
	a, err := h.svc.actor(id)
	if err != nil {
		t.Fatal(err)
	}
	if !a.sync(func() {
		// Reproduce an agent that omits background-work updates. Only the
		// process-tree evidence remains after the normal turn tracker clears.
		a.chat.Turn.Unfinished = false
		a.work.endTurn()
		a.stopBackgroundWatch()
		if !a.canSendNow() {
			t.Error("fixture is not otherwise idle")
			return
		}
		before := a.link
		if !hasNewProcess(before.sidecar.PID(), a.bgBase) {
			t.Error("background child missing")
			return
		}
		if err := a.admitHistoryWriter(); !errors.Is(err, ErrBusy) {
			t.Errorf("admitted surviving child: %v", err)
		}
		if a.link != before || !hasNewProcess(before.sidecar.PID(), a.bgBase) {
			t.Error("refusal replaced or killed the existing writer")
		}
	}) {
		t.Fatal("actor stopped")
	}
}

type blockingHistoryInjector struct {
	agentprofile.Agent
	agentprofile.HistoryInjector
	entered chan struct{}
	resume  chan struct{}
}

func (i blockingHistoryInjector) SupportsHistoryVersion(version string) bool {
	close(i.entered)
	<-i.resume
	return i.HistoryInjector.SupportsHistoryVersion(version)
}

func historyCancellationTest(t *testing.T) (*harness, *actor, conversation.Execution, agentprofile.HistoricalContext) {
	t.Helper()
	ctx := context.Background()
	h := newHarness(t, 0)
	db := conversation.Open(conversation.Options{StateDir: h.state})
	h.svc.conversations = db
	t.Cleanup(func() { h.svc.Close(); _ = db.Close() })
	in := h.codexInput(SideACP, "")
	in.Env["FAKEAGENT_DELAY"] = "session/close=200ms"
	shell := h.create(in)
	id := shell.Info().ID
	b, err := db.Binding(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	p, err := db.ReservePreview(ctx, conversation.PreviewReservation{
		SourceConversationID: b.ConversationID, ClientRequestID: "history-preview", RequestHash: "preview-hash",
		Request: conversation.PreviewRequest{ConfigID: in.ConfigID, AgentName: "codex", TargetProviderThreadID: b.ProviderThreadID, UserText: "unsent request"},
	})
	if err != nil {
		t.Fatal(err)
	}
	p, err = db.CompletePreview(ctx, p.ID, conversation.PreviewResult{Budget: conversation.PreviewBudget{History: 16000, Used: 100, Source: "estimated"}})
	if err != nil {
		t.Fatal(err)
	}
	e, err := db.BeginExecution(ctx, conversation.ExecutionInput{SourceConversationID: b.ConversationID, HandoffID: p.ID, ClientRequestID: "history-message", RequestHash: "message-hash", Revision: p.Revision})
	if err != nil || e.RuntimeID != id {
		t.Fatal("history execution runtime", e, err)
	}
	a, err := h.svc.actor(id)
	if err != nil {
		t.Fatal(err)
	}
	var sid string
	if !a.sync(func() { sid = a.agentSessionID }) {
		t.Fatal("actor stopped")
	}
	history := agentprofile.HistoricalContext{Items: []agentprofile.HistoricalItem{{ID: "coverage", Role: "user", Text: "historical coverage"}}}
	if _, err := db.ReserveInjection(ctx, e.ID, sid, []conversation.InjectionItem{{ID: "coverage", Role: "user", TextHash: conversation.TextHash("historical coverage")}}); err != nil {
		t.Fatal(err)
	}
	return h, a, e, history
}

func TestHistoryAdmissionCancellationWaitsForActorResult(t *testing.T) {
	h, a, e, history := historyCancellationTest(t)
	gate := blockingHistoryInjector{entered: make(chan struct{}), resume: make(chan struct{})}
	var old *acpLink
	if !a.sync(func() {
		old = a.link
		gate.Agent = a.cmd.Agent
		gate.HistoryInjector = a.cmd.Agent.(agentprofile.HistoryInjector)
		a.cmd.Agent = gate
	}) {
		t.Fatal("actor stopped")
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		_, err := h.svc.InjectHistory(ctx, a.id, e.ID, history)
		done <- err
	}()
	select {
	case <-gate.entered:
	case <-time.After(testTimeout):
		t.Fatal("admission did not enter actor")
	}
	cancel()
	select {
	case err := <-done:
		close(gate.resume)
		t.Fatalf("cancelled caller abandoned admission before actor replied: %v", err)
	case <-time.After(50 * time.Millisecond):
	}
	close(gate.resume)
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("cancelled admission succeeded")
		}
	case <-time.After(testTimeout):
		t.Fatal("cancelled admission did not finish")
	}
	if !a.sync(func() {
		a.cmd.Agent = gate.Agent
		if a.link != old || a.opening {
			t.Error("cancelled admission lost ownership of original writer")
		}
	}) {
		t.Fatal("actor stopped")
	}
	h.closeSession(a.id)
}

func TestHistoryCancellationAfterAdmissionJoinsWriterAndAllowsClose(t *testing.T) {
	h, a, e, history := historyCancellationTest(t)
	pid := h.record(a.id).PID
	if pid <= 0 {
		t.Fatal("missing original writer")
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		_, err := h.svc.InjectHistory(ctx, a.id, e.ID, history)
		done <- err
	}()
	waitFor(t, "admitted writer release", func() bool { return countCalls(t, h.store, "session/close") > 0 })
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("cancelled injection outcome: %v", err)
		}
	case <-time.After(testTimeout):
		t.Fatal("cancelled injection did not join writer")
	}
	if err := syscall.Kill(pid, 0); !errors.Is(err, syscall.ESRCH) {
		t.Fatalf("original writer %d survived cancellation: %v", pid, err)
	}
	if !a.sync(func() {
		if a.opening || a.link != nil {
			t.Error("cancelled transition left actor opening or owned a new writer")
		}
	}) {
		t.Fatal("actor stopped")
	}
	receipt, err := h.svc.conversations.Injection(context.Background(), e.DeliveryID)
	if err != nil || receipt.CallStarted || receipt.State != "pending" {
		t.Fatal("cancellation dispatched native history", receipt, err)
	}
	h.closeSession(a.id)
	if rec := h.record(a.id); rec.State != StateInterrupted || rec.PID != 0 {
		t.Fatal("closing cancelled transition changed its interruption record", rec)
	}
}
