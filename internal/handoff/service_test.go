package handoff

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/rainchan/webmux/internal/conversation"
	"github.com/rainchan/webmux/internal/launch"
)

type previewHarness struct {
	service *Service
	store   *conversation.Service
	launch  *launch.Service
	source  conversation.Binding
	config  launch.Config
	dir     string
}

func previewTest(t *testing.T) previewHarness {
	t.Helper()
	ctx := context.Background()
	dir := t.TempDir()
	store := conversation.Open(conversation.Options{StateDir: dir})
	if err := store.Unavailable(); err != nil {
		t.Fatal(err)
	}
	launches := launch.Open(t.TempDir())
	t.Cleanup(launches.Close)
	config, err := launches.Create(launch.Input{Name: "test", Cmd: []string{"omp"}, Hybrid: true})
	if err != nil {
		t.Fatal(err)
	}
	source, err := store.EnsureRuntime(ctx, conversation.RuntimeInput{RuntimeID: "source", Cwd: t.TempDir(), AgentName: "omp", ConfigID: config.ID, HistoryOrigin: "complete", Status: "closed"})
	if err != nil {
		t.Fatal(err)
	}
	service := New(Options{Conversations: store, Launch: launches, TokenCap: 16000, MaxPromptBytes: 64 << 10, MaxPromptWireBytes: 14 << 20})
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Error(err)
		}
	})
	return previewHarness{service: service, store: store, launch: launches, source: source, config: config, dir: dir}
}

func (h previewHarness) add(t *testing.T, text string) {
	t.Helper()
	ctx := context.Background()
	r, err := h.store.StartRun(ctx, h.source.ProviderThreadID, "prompt-"+text)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.store.WriteActivity(ctx, conversation.ActivityInput{ProviderThreadID: h.source.ProviderThreadID, RunID: r.ID, LocalKey: r.ID, Kind: "message", Role: "user", Status: "completed", Source: "local", Text: text}); err != nil {
		t.Fatal(err)
	}
	if err := h.store.FinishRun(ctx, r.ID, "end_turn"); err != nil {
		t.Fatal(err)
	}
}

func TestPreviewSelectsIntactHistoryAndPersistsRevision(t *testing.T) {
	ctx := context.Background()
	h := previewTest(t)
	h.add(t, "original 中文🙂")
	h.add(t, "latest request")
	in := Request{ClientRequestID: "preview-request", ConfigID: h.config.ID, Fork: true, UserText: "new task"}
	p, err := h.service.Preview(ctx, h.source.ConversationID, in)
	if err != nil {
		t.Fatal(err)
	}
	if p.State != "ready" || p.Revision != 1 || len(p.SelectedItemIDs) != 2 || p.Stale || !strings.Contains(p.RenderedText, "original 中文🙂") || strings.Contains(p.RenderedText, "User message:") {
		t.Fatalf("preview: %+v", p)
	}
	if !strings.HasSuffix(InlinePrompt(p), "User message:\nnew task") {
		t.Fatal("new request was modified")
	}
	again, err := h.service.Preview(ctx, h.source.ConversationID, in)
	if err != nil || again.ID != p.ID || again.TargetConversationID != p.TargetConversationID {
		t.Fatalf("duplicate draft: %+v %v", again, err)
	}
	in.UserText = "different"
	if _, err := h.service.Preview(ctx, h.source.ConversationID, in); !errors.Is(err, conversation.ErrConflict) {
		t.Fatalf("payload conflict: %v", err)
	}
	edited, err := h.service.Edit(ctx, p.ID, p.Revision, "my manual context")
	if err != nil {
		t.Fatal(err)
	}
	if edited.Revision != 2 || edited.Strategy != "manual_context" || edited.RenderedText != "my manual context" || len(edited.SelectedItemIDs) != 0 || len(edited.OmittedItemIDs) != 2 {
		t.Fatalf("manual edit: %+v", edited)
	}
	if _, err := h.service.Edit(ctx, p.ID, p.Revision, "stale editor"); !errors.Is(err, conversation.ErrConflict) {
		t.Fatalf("revision conflict: %v", err)
	}
	if err := h.store.Close(); err != nil {
		t.Fatal(err)
	}
	reopened := conversation.Open(conversation.Options{StateDir: h.dir})
	defer func() { _ = reopened.Close() }()
	saved, err := reopened.Preview(ctx, p.ID)
	if err != nil || saved.RenderedText != "my manual context" || saved.Revision != 2 {
		t.Fatalf("restart lost edit: %+v %v", saved, err)
	}
}

func TestPreviewBecomesStaleWithoutOverwritingEditedText(t *testing.T) {
	ctx := context.Background()
	h := previewTest(t)
	h.add(t, "old")
	p, err := h.service.Preview(ctx, h.source.ConversationID, Request{ClientRequestID: "stale-preview", ConfigID: h.config.ID, UserText: "task"})
	if err != nil {
		t.Fatal(err)
	}
	p, err = h.service.Edit(ctx, p.ID, 1, "preserve this edit")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.store.StartRun(ctx, h.source.ProviderThreadID, "started-on-another-device"); err != nil {
		t.Fatal(err)
	}
	saved, err := h.service.Get(ctx, p.ID)
	if err != nil || !saved.Stale || saved.RenderedText != "preserve this edit" || saved.SourcePosition != p.SourcePosition {
		t.Fatalf("stale draft overwritten: %+v %v", saved, err)
	}
}

func TestPreviewConcurrentEditorsAndOverBudgetKeepDraft(t *testing.T) {
	ctx := context.Background()
	h := previewTest(t)
	h.add(t, "history")
	p, err := h.service.Preview(ctx, h.source.ConversationID, Request{ClientRequestID: "concurrent-preview", ConfigID: h.config.ID, Fork: true, UserText: "task"})
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for _, text := range []string{"one", "two"} {
		wg.Go(func() { _, err := h.service.Edit(ctx, p.ID, 1, text); results <- err })
	}
	wg.Wait()
	close(results)
	success, conflict := 0, 0
	for err := range results {
		if err == nil {
			success++
		} else if errors.Is(err, conversation.ErrConflict) {
			conflict++
		} else {
			t.Fatal(err)
		}
	}
	if success != 1 || conflict != 1 {
		t.Fatalf("lost update: success=%d conflict=%d", success, conflict)
	}
	manual := strings.Repeat("大🙂", 20000)
	saved, err := h.service.Edit(ctx, p.ID, 2, manual)
	if err != nil || saved.State != "over_budget" || saved.RenderedText != manual {
		t.Fatalf("oversized edit lost: state=%s error=%v", saved.State, err)
	}
}

func TestPreviewReusesObservedWindowForUnchangedTargetSelection(t *testing.T) {
	ctx := context.Background()
	h := previewTest(t)
	h.add(t, "history")
	if err := h.store.SyncRuntime(ctx, conversation.RuntimeInput{RuntimeID: "source", Cwd: "/workspace", ConfigID: h.config.ID, AgentName: "omp", AgentSessionID: "native", Status: "closed", Config: map[string]string{"model": "small"}}); err != nil {
		t.Fatal(err)
	}
	if err := h.store.ObserveUsage(ctx, h.source.ProviderThreadID, "native", 1000, 8000); err != nil {
		t.Fatal(err)
	}
	p, err := h.service.Preview(ctx, h.source.ConversationID, Request{ClientRequestID: "observed-preview", ConfigID: h.config.ID, TargetProviderThreadID: h.source.ProviderThreadID, UserText: "task"})
	if err != nil {
		t.Fatal(err)
	}
	if p.Budget.Window != 8000 || p.Budget.Source != "provider" || p.State != "over_budget" || p.Request.AgentConfig["model"] != "small" {
		t.Fatalf("lost observed capacity: %+v", p.Budget)
	}
}

func TestPreviewMarksChangedTargetBudgetStale(t *testing.T) {
	ctx := context.Background()
	h := previewTest(t)
	if err := h.store.SyncRuntime(ctx, conversation.RuntimeInput{RuntimeID: "source", ConfigID: h.config.ID, AgentName: "omp", AgentSessionID: "native", Status: "closed"}); err != nil {
		t.Fatal(err)
	}
	p, err := h.service.Preview(ctx, h.source.ConversationID, Request{ClientRequestID: "target-changed", ConfigID: h.config.ID, TargetProviderThreadID: h.source.ProviderThreadID, UserText: "task"})
	if err != nil {
		t.Fatal(err)
	}
	if err := h.store.ObserveUsage(ctx, h.source.ProviderThreadID, "native", 1000, 8000); err != nil {
		t.Fatal(err)
	}
	got, err := h.service.Get(ctx, p.ID)
	if err != nil || !got.Stale || got.Budget.Window != p.Budget.Window || got.RenderedText != p.RenderedText {
		t.Fatalf("changed target overwrote conclusion: %+v %v", got, err)
	}
}
