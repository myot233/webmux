package hybrid

import (
	"bytes"
	"context"
	"errors"
	"io"
	"syscall"
	"testing"
	"time"

	"github.com/rainchan/webmux/internal/acp"
	"github.com/rainchan/webmux/internal/conversation"
)

func TestHistoryReaderRefreshDetectsConnectionLostBeforeInstallation(t *testing.T) {
	h := newHarness(t, 0)
	id := h.create(h.input("omp", SideACP)).Info().ID
	a, err := h.svc.actor(id)
	if err != nil {
		t.Fatal(err)
	}
	reader, sink := io.Pipe()
	defer func() { _ = reader.Close() }()
	disconnected := acp.NewConn(bytes.NewReader(nil), sink, acp.Options{})
	defer func() { _ = disconnected.Close() }()
	<-disconnected.Done()
	if !a.sync(func() {
		old := a.link
		replacement := *old
		replacement.conn = disconnected
		a.link = nil
		a.opening = true
		// The real disconnect callback may run while the replacement is still
		// outside the actor. It is correctly ignored until installation.
		a.linkLost(&replacement)
		a.finishHistoryReaderRefresh(openResult{link: &replacement}, nil)
		if a.link == &replacement || a.reconnects != 1 || !a.opening {
			t.Errorf("installed dead connection stayed active: link=%v reconnects=%d opening=%v", a.link == &replacement, a.reconnects, a.opening)
		}
	}) {
		t.Fatal("actor stopped")
	}
}

func readerRefreshCancellationTest(t *testing.T) (*harness, *actor, conversation.Execution) {
	t.Helper()
	ctx := context.Background()
	h := newHarness(t, 0)
	db := conversation.Open(conversation.Options{StateDir: h.state})
	h.svc.conversations = db
	t.Cleanup(func() { h.svc.Close(); _ = db.Close() })
	in := h.codexInput(SideACP, "")
	in.Env["FAKEAGENT_DELAY"] = "session/close=200ms"
	id := h.create(in).Info().ID
	binding, err := db.Binding(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	preview, err := db.ReservePreview(ctx, conversation.PreviewReservation{
		SourceConversationID: binding.ConversationID, ClientRequestID: "reader-preview", RequestHash: "preview-hash",
		Request: conversation.PreviewRequest{ConfigID: in.ConfigID, AgentName: "codex", TargetProviderThreadID: binding.ProviderThreadID, UserText: "unsent reader request"},
	})
	if err != nil {
		t.Fatal(err)
	}
	preview, err = db.CompletePreview(ctx, preview.ID, conversation.PreviewResult{Budget: conversation.PreviewBudget{History: 16000, Used: 100, Source: "estimated"}})
	if err != nil {
		t.Fatal(err)
	}
	execution, err := db.BeginExecution(ctx, conversation.ExecutionInput{SourceConversationID: binding.ConversationID, HandoffID: preview.ID, ClientRequestID: "reader-message", RequestHash: "message-hash", Revision: preview.Revision})
	if err != nil || execution.RuntimeID != id {
		t.Fatal("reader execution runtime", execution, err)
	}
	// Scope discovery is real; this case cancels before reopening the provider,
	// so no replacement MCP process should be started.
	h.svc.historyReader = func(_ context.Context, scope conversation.ReaderScope) []acp.MCPServer {
		if scope.HandoffID != preview.ID {
			return nil
		}
		return []acp.MCPServer{{Name: "webmux-history", Command: "/bin/false"}}
	}
	if err := h.svc.ClaimProvider(ExecutionContext(ctx, execution.ID), id, execution.ID); err != nil {
		t.Fatal(err)
	}
	a, err := h.svc.actor(id)
	if err != nil {
		t.Fatal(err)
	}
	return h, a, execution
}

func TestHistoryReaderRefreshQueuedCancellationRetainsWriter(t *testing.T) {
	h, a, execution := readerRefreshCancellationTest(t)
	var old *acpLink
	blocked, resume := make(chan struct{}), make(chan struct{})
	if !a.post(func() {
		old = a.link
		close(blocked)
		<-resume
	}) {
		t.Fatal("actor stopped")
	}
	<-blocked
	ctx, cancel := context.WithCancel(ExecutionContext(context.Background(), execution.ID))
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- h.svc.RefreshHistoryReader(ctx, a.id, execution.ID) }()
	waitFor(t, "queued reader admission", func() bool { return len(a.inbox) > 0 })
	cancel()
	select {
	case err := <-done:
		close(resume)
		t.Fatalf("cancelled caller abandoned queued reader admission: %v", err)
	case <-time.After(50 * time.Millisecond):
	}
	close(resume)
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("queued cancellation outcome: %v", err)
		}
	case <-time.After(testTimeout):
		t.Fatal("cancelled reader admission did not finish")
	}
	if !a.sync(func() {
		if a.link != old || a.opening {
			t.Error("queued cancellation detached the original reader writer")
		}
	}) {
		t.Fatal("actor stopped")
	}
	if countCalls(t, h.store, acp.MethodSessionClose) != 0 {
		t.Fatal("queued cancellation released the untouched writer")
	}
	h.closeSession(a.id)
}

func TestHistoryReaderRefreshCancellationJoinsWriterAndAllowsClose(t *testing.T) {
	h, a, execution := readerRefreshCancellationTest(t)
	pid := h.record(a.id).PID
	if pid <= 0 {
		t.Fatal("missing original reader writer")
	}
	ctx, cancel := context.WithCancel(ExecutionContext(context.Background(), execution.ID))
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- h.svc.RefreshHistoryReader(ctx, a.id, execution.ID) }()
	waitFor(t, "reader writer release", func() bool { return countCalls(t, h.store, acp.MethodSessionClose) > 0 })
	cancel()
	if err := h.manager.Close(a.id); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("reader cancellation outcome: %v", err)
		}
	case <-time.After(testTimeout):
		t.Fatal("cancelled reader refresh did not join writer")
	}
	if err := syscall.Kill(pid, 0); !errors.Is(err, syscall.ESRCH) {
		t.Fatalf("original reader writer %d survived cancellation: %v", pid, err)
	}
	// Close was requested while opening. Its completion proves the refresh
	// cleared opening and ran afterTransition rather than leaving close queued.
	waitFor(t, "reader shell closed after cancellation", func() bool { _, err := h.manager.Get(a.id); return err != nil })
	if rec := h.record(a.id); rec.State != StateInterrupted || rec.PID != 0 {
		t.Fatal("cancelled reader close changed interruption record", rec)
	}
	if countCalls(t, h.store, acp.MethodSessionResume) != 0 || countCalls(t, h.store, acp.MethodSessionPrompt) != 0 {
		t.Fatal("cancelled refresh reopened the reader or dispatched a user turn")
	}
}
