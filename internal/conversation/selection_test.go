package conversation

import (
	"context"
	"errors"
	"testing"
)

func TestCompletePreviewBulkSelectionPreservesFrozenScopeAndAtomicity(t *testing.T) {
	ctx := context.Background()
	s := openTest(t, t.TempDir())
	source := bindTest(t, s, "source")
	run := runTest(t, s, source, "history")
	first := activityTest(t, s, source, run.ID, "first", "", "first message")
	second := activityTest(t, s, source, run.ID, "second", "", "second message")
	if err := s.FinishRun(ctx, run.ID, "end_turn"); err != nil {
		t.Fatal(err)
	}
	p, err := s.ReservePreview(ctx, PreviewReservation{SourceConversationID: source.ConversationID, ClientRequestID: "preview-command", RequestHash: "preview-hash", Request: PreviewRequest{ConfigID: "config", AgentName: "omp", Fork: true}})
	if err != nil {
		t.Fatal(err)
	}
	futureRun := runTest(t, s, source, "future")
	future := activityTest(t, s, source, futureRun.ID, "future", "", "later message")
	if err := s.FinishRun(ctx, futureRun.ID, "end_turn"); err != nil {
		t.Fatal(err)
	}
	other := bindTest(t, s, "other")
	otherRun := runTest(t, s, other, "other-history")
	foreign := activityTest(t, s, other, otherRun.ID, "other", "", "another conversation")
	if err := s.FinishRun(ctx, otherRun.ID, "end_turn"); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, selected string
		want           error
	}{
		{"future", future, ErrConflict},
		{"foreign", foreign, ErrConflict},
		{"duplicate", first, ErrInvalid},
		{"empty", "", ErrInvalid},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := s.CompletePreview(ctx, p.ID, PreviewResult{OmittedItemIDs: []string{first}, SelectedItemIDs: []string{tc.selected}})
			if !errors.Is(err, tc.want) {
				t.Fatalf("CompletePreview error=%v, want %v", err, tc.want)
			}
			saved, err := s.Preview(ctx, p.ID)
			if err != nil || saved.State != "preparing" || len(saved.SelectedItemIDs)+len(saved.OmittedItemIDs) != 0 {
				t.Fatalf("partial selection committed: %+v, %v", saved, err)
			}
			page, err := s.Selection(ctx, p.ID, 0, 100, "")
			if err != nil || page.SelectedCount != 0 || page.OmittedCount != 0 {
				t.Fatalf("partial selection counts committed: %+v, %v", page, err)
			}
		})
	}
	saved, err := s.CompletePreview(ctx, p.ID, PreviewResult{OmittedItemIDs: []string{first}, SelectedItemIDs: []string{second}})
	if err != nil || saved.State != "ready" || len(saved.SelectedItemIDs) != 1 || saved.SelectedItemIDs[0] != second || len(saved.OmittedItemIDs) != 1 || saved.OmittedItemIDs[0] != first {
		t.Fatalf("bulk selection lost classification: %+v, %v", saved, err)
	}
	t.Run("upgrade_v8_selection_counts", func(t *testing.T) {
		if _, err := s.db.ExecContext(ctx, `DROP INDEX handoff_item_position; ALTER TABLE handoff_preview DROP COLUMN selected_count; ALTER TABLE handoff_preview DROP COLUMN omitted_count; PRAGMA user_version=8;`); err != nil {
			t.Fatal(err)
		}
		if err := migrate(ctx, s.db); err != nil {
			t.Fatal(err)
		}
		page, err := s.Selection(ctx, p.ID, 0, 1, "")
		if err != nil || page.SelectedCount != 1 || page.OmittedCount != 1 || len(page.Items) != 1 || page.Done {
			t.Fatalf("selection migration lost counts/page: %+v, %v", page, err)
		}
	})
	if _, err := s.EditPreview(ctx, p.ID, saved.Revision, "manual context", saved.Budget); err != nil {
		t.Fatal(err)
	}
	page, err := s.Selection(ctx, p.ID, 0, 100, "")
	if err != nil || page.SelectedCount != 0 || page.OmittedCount != 2 || !page.Done {
		t.Fatalf("manual edit lost selection counts: %+v, %v", page, err)
	}
}
