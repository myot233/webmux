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

func TestPreviewFreshKeepsKnownSmallWindowWithoutNativeOccupancy(t *testing.T) {
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
	if p.Budget.Window != 8000 || p.Budget.Source != "estimated" || p.Budget.NativeUsed != 0 || !p.Request.ForceFresh || p.TargetProviderThreadID == h.source.ProviderThreadID || p.State != "over_budget" || p.Request.AgentConfig["model"] != "small" {
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
	retry, err := h.service.Preview(ctx, h.source.ConversationID, Request{ClientRequestID: "target-changed", ConfigID: h.config.ID, TargetProviderThreadID: h.source.ProviderThreadID, UserText: "task"})
	if err != nil || !retry.Stale || retry.ID != p.ID {
		t.Fatalf("retry bypassed freshness: %+v %v", retry, err)
	}
	got, err := h.service.Get(ctx, p.ID)
	if err != nil || !got.Stale || got.Budget.Window != p.Budget.Window || got.RenderedText != p.RenderedText {
		t.Fatalf("changed target overwrote conclusion: %+v %v", got, err)
	}
}

func TestPreviewIncludesCommittedIdleCompaction(t *testing.T) {
	ctx := context.Background()
	h := previewTest(t)
	id, err := h.store.WriteActivity(ctx, conversation.ActivityInput{ProviderThreadID: h.source.ProviderThreadID, LocalKey: "idle-compaction", Kind: "compaction", Role: "agent", Status: "completed", Source: "live", Text: "preserve this idle summary"})
	if err != nil {
		t.Fatal(err)
	}
	p, err := h.service.Preview(ctx, h.source.ConversationID, Request{ClientRequestID: "idle-preview", ConfigID: h.config.ID, Fork: true, UserText: "task"})
	if err != nil || len(p.SelectedItemIDs) != 1 || p.SelectedItemIDs[0] != id || !strings.Contains(p.RenderedText, "preserve this idle summary") {
		t.Fatalf("idle activity vanished: %+v %v", p, err)
	}
}

func TestPreviewEditRetainsTargetConfigStaleness(t *testing.T) {
	ctx := context.Background()
	h := previewTest(t)
	p, err := h.service.Preview(ctx, h.source.ConversationID, Request{ClientRequestID: "edit-deleted-target", ConfigID: h.config.ID, Fork: true, UserText: "task"})
	if err != nil {
		t.Fatal(err)
	}
	if err := h.launch.Delete(h.config.ID); err != nil {
		t.Fatal(err)
	}
	updated, err := h.service.Edit(ctx, p.ID, 1, "keep this draft")
	if err != nil || !updated.Stale || updated.RenderedText != "keep this draft" || updated.Revision != 2 {
		t.Fatalf("edit bypassed target freshness: %+v %v", updated, err)
	}
}

func TestPreviewDisablingHybridInvalidatesDraftWithoutReplacingText(t *testing.T) {
	h := previewTest(t)
	ctx := context.Background()
	in := Request{ClientRequestID: "disabled-hybrid-preview", ConfigID: h.config.ID, Fork: true, UserText: "task"}
	p, err := h.service.Preview(ctx, h.source.ConversationID, in)
	if err != nil {
		t.Fatal(err)
	}
	p, err = h.service.Edit(ctx, p.ID, p.Revision, "keep my edit")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.launch.Update(h.config.ID, launch.Input{Name: h.config.Name, Cmd: h.config.Cmd, Env: h.config.Env, Hybrid: false}); err != nil {
		t.Fatal(err)
	}
	got, err := h.service.Preview(ctx, h.source.ConversationID, in)
	if err != nil || !got.Stale || got.ID != p.ID || got.RenderedText != "keep my edit" {
		t.Fatalf("disabled hybrid kept ready: %+v %v", got, err)
	}
}

func TestPreviewTargetConfigDriftWithUnknownWindow(t *testing.T) {
	h := previewTest(t)
	ctx := context.Background()
	update := func(model string) {
		t.Helper()
		if err := h.store.SyncRuntime(ctx, conversation.RuntimeInput{RuntimeID: "source", ConfigID: h.config.ID, AgentName: "omp", AgentSessionID: "native", Status: "closed", Config: map[string]string{"model": model}}); err != nil {
			t.Fatal(err)
		}
	}
	update("model-a")
	in := Request{ClientRequestID: "target-baseline-preview", ConfigID: h.config.ID, TargetProviderThreadID: h.source.ProviderThreadID, AgentConfig: map[string]string{"model": "model-b"}, UserText: "task"}
	p, err := h.service.Preview(ctx, h.source.ConversationID, in)
	if err != nil || p.Stale {
		t.Fatal("intentional override invalidated baseline", p.Stale, err)
	}
	update("model-b")
	if got, err := h.service.Get(ctx, p.ID); err != nil || got.Stale {
		t.Fatal("requested override invalidated draft", got.Stale, err)
	}
	update("model-c")
	got, err := h.service.Preview(ctx, h.source.ConversationID, in)
	if err != nil || !got.Stale || got.Budget != p.Budget || got.RenderedText != p.RenderedText {
		t.Fatalf("target drift hidden by same fallback budget: %+v %v", got, err)
	}
}
