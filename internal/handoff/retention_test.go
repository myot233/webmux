package handoff

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/rainchan/webmux/internal/conversation"
	"github.com/rainchan/webmux/internal/hybrid"
)

func TestReleaseRebuildConfigDeletionAndForgetRetainTheRightDraftData(t *testing.T) {
	h := deliveryTest(t, nil)
	ctx := context.Background()
	p, _ := h.preview(conversation.PreviewLocation{})
	if len(p.SelectedItemIDs) == 0 {
		t.Fatal("source history missing")
	}
	original, err := h.store.Activity(ctx, p.SourceConversationID, p.SelectedItemIDs[0], 0, 4000)
	if err != nil {
		t.Fatal(err)
	}
	ids, err := h.store.RuntimeIDs(ctx, p.SourceConversationID)
	if err != nil || len(ids) != 1 {
		t.Fatal(ids, err)
	}
	if err := h.hybrids.QuiesceProvider(ctx, ids[0]); err != nil {
		t.Fatal(err)
	}
	if err := h.hybrids.MarkReleased(ctx, ids, hybrid.Release{ID: "retention-release", RepositoryID: "repo", Path: h.root, Commit: "snapshot", ReleasedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	if _, err := h.hybrids.ClearReleased(ctx, "retention-release"); err != nil {
		t.Fatal(err)
	}
	if err := h.launches.Delete(h.config.ID); err != nil {
		t.Fatal(err)
	}
	h.close()
	h.open()
	restored, err := h.service.Get(ctx, p.ID)
	if err != nil || !restored.Stale || restored.RenderedText != p.RenderedText || restored.Request.UserText != p.Request.UserText {
		t.Fatal("release or config deletion discarded draft", restored, err)
	}
	body, err := h.store.Activity(ctx, p.SourceConversationID, p.SelectedItemIDs[0], 0, 4000)
	if err != nil || body.Activity.Text != original.Activity.Text {
		t.Fatal("release discarded activity", err)
	}
	if err := h.hybrids.Forget(ctx, ids[0]); err != nil {
		t.Fatal(err)
	}
	if _, err := h.service.Get(ctx, p.ID); !errors.Is(err, conversation.ErrNotFound) {
		t.Fatal("forget kept source draft", err)
	}
	if _, err := h.store.Conversation(ctx, p.TargetConversationID); !errors.Is(err, conversation.ErrNotFound) {
		t.Fatal("forget kept abandoned fork", err)
	}
}
