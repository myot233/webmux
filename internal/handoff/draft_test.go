package handoff

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/rainchan/webmux/internal/conversation"
)

func TestUserTaskEditPreservesNativeSelectionAndRestoresAfterRestart(t *testing.T) {
	h := nativeTest(t, nil)
	ctx := context.Background()
	p, _ := h.preview(conversation.PreviewLocation{})
	if p.Request.SourceLocation.Cwd != h.root {
		t.Fatal("missing frozen source location", p.Request.SourceLocation)
	}
	p.RenderedText = "frozen coverage from a previous formatter\n\n[History role=user]body"
	native, _, err := h.service.nativeContext(ctx, p)
	if err != nil || native.Items[0].Text != "frozen coverage from a previous formatter" {
		t.Fatal("native coverage was regenerated", err)
	}
	p, _ = h.service.Get(ctx, p.ID)
	original := p
	task := "new independent task 中文🙂"
	p, err = h.service.EditDraft(ctx, p.ID, p.Revision, nil, &task)
	if err != nil {
		t.Fatal(err)
	}
	if !p.Request.NativePlanned || p.ManualText != nil || p.Strategy != original.Strategy || p.RenderedText != original.RenderedText || !slices.Equal(p.SelectedItemIDs, original.SelectedItemIDs) || p.SourcePosition != original.SourcePosition || p.Request.UserText != task || p.Revision != original.Revision+1 {
		t.Fatal("task edit changed historical context")
	}
	if _, err := h.service.EditDraft(ctx, p.ID, original.Revision, nil, new("other device")); !errors.Is(err, conversation.ErrConflict) {
		t.Fatal("stale editor accepted", err)
	}
	huge := strings.Repeat("字", 24000)
	p, err = h.service.EditDraft(ctx, p.ID, p.Revision, nil, &huge)
	if err != nil || !p.Budget.OverBudget || p.Request.UserText != huge || p.ManualText != nil {
		t.Fatal("over-budget task was lost or became manual", err)
	}
	h.close()
	h.open()
	p, err = h.service.Get(ctx, p.ID)
	if err != nil || p.Request.UserText != huge || !p.Budget.OverBudget || p.ManualText != nil || !slices.Equal(p.SelectedItemIDs, original.SelectedItemIDs) {
		t.Fatal("draft restart", err)
	}
	latest, err := h.store.LatestPreview(ctx, h.source.ConversationID, true)
	if err != nil || latest.ID != p.ID {
		t.Fatal("draft discovery", err)
	}
	p, err = h.service.EditDraft(ctx, p.ID, p.Revision, nil, &task)
	if err != nil || p.Budget.OverBudget || p.Request.UserText != task {
		t.Fatal("task reduction", err)
	}
	manual := "preserve this edited historical constraint"
	p, err = h.service.EditDraft(ctx, p.ID, p.Revision, &manual, nil)
	if err != nil || p.ManualText == nil || *p.ManualText != manual || p.Request.UserText != task || len(p.SelectedItemIDs) != 0 {
		t.Fatal("separate manual draft", err)
	}
}
