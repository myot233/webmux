package conversation

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
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
		legacy, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "v8.sqlite"))
		if err != nil {
			t.Fatal(err)
		}
		defer legacy.Close()
		if _, err := legacy.ExecContext(ctx, schema+activitySchema+previewSchema+budgetSchema+executionSchema+attachmentSchema+inlineReceiptSchema+dispatchSchema+injectionSchema); err != nil {
			t.Fatal(err)
		}
		if _, err := legacy.ExecContext(ctx, `
INSERT INTO conversation(id,cwd,history_origin,created_at,updated_at) VALUES('source','/source','complete',1,1);
INSERT INTO provider_thread(id,conversation_id,config_id,agent_name,status,created_at,updated_at) VALUES('thread','source','config','omp','active',1,1);
INSERT INTO context_handoff VALUES('preview','source','source','thread','full_thread_summary',1,1,1,1);
INSERT INTO handoff_preview(handoff_id,source_position,source_run_ordinal,history_origin,reconcile_required,request,budget,state) VALUES('preview',2,1,'complete',0,'{}','{}','ready');
INSERT INTO handoff_item VALUES('preview','first',0,1),('preview','second',1,2);
`); err != nil {
			t.Fatal(err)
		}
		if err := migrate(ctx, legacy); err != nil {
			t.Fatal(err)
		}
		var selected, omitted int
		if err := legacy.QueryRowContext(ctx, "SELECT selected_count,omitted_count FROM handoff_preview WHERE handoff_id='preview'").Scan(&selected, &omitted); err != nil || selected != 1 || omitted != 1 {
			t.Fatalf("selection migration lost counts: %d/%d, %v", selected, omitted, err)
		}
	})
	saved, err = s.EditDraft(ctx, p.ID, saved.Revision, nil, new("task-only edit"), saved.Budget)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.ExecContext(ctx, "UPDATE conversation SET cwd='/moved' WHERE id=?", source.ConversationID); err != nil {
		t.Fatal(err)
	}
	selection, err := s.Selection(ctx, p.ID, 0, 100, "")
	if err != nil || selection.SelectedCount != 1 || selection.OmittedCount != 1 || selection.SourceLocation != saved.Request.SourceLocation {
		t.Fatalf("task edit changed selection or frozen source: %+v, %v", selection, err)
	}
	if _, err := s.EditDraft(ctx, p.ID, saved.Revision, new("manual context"), nil, saved.Budget); err != nil {
		t.Fatal(err)
	}
	page, err := s.Selection(ctx, p.ID, 0, 100, "")
	if err != nil || page.SelectedCount != 0 || page.OmittedCount != 2 || !page.Done {
		t.Fatalf("manual edit lost selection counts: %+v, %v", page, err)
	}
}
