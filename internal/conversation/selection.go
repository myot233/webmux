package conversation

import (
	"context"
	"database/sql"
)

const selectionSchema = `
CREATE INDEX handoff_item_position ON handoff_item(handoff_id,position);
ALTER TABLE handoff_preview ADD COLUMN selected_count INTEGER NOT NULL DEFAULT 0;
ALTER TABLE handoff_preview ADD COLUMN omitted_count INTEGER NOT NULL DEFAULT 0;
UPDATE handoff_preview SET
 selected_count=(SELECT count(*) FROM handoff_item WHERE handoff_id=handoff_preview.handoff_id AND selected=1),
 omitted_count=(SELECT count(*) FROM handoff_item WHERE handoff_id=handoff_preview.handoff_id AND selected=0);
PRAGMA user_version=9;
`

// SelectionPage is bounded independently of preview text and request metadata.
// Positions use the same immutable activity cursor as Activities.
type SelectionPage struct {
	SourceConversationID, HistoryOrigin    string
	SourcePosition, Revision, NextPosition int64
	SelectedCount, OmittedCount            int
	ReconcileRequired, Manual, Done        bool
	Items                                  []SelectionItem
	SourceLocation                         PreviewLocation
}

type SelectionItem struct {
	ActivityID string
	Position   int64
	Selected   bool
}

// Selection reads only scope and a bounded selection page, never the full draft.
// itemID optionally narrows the page to one activity within the frozen source.
func (s *Service) Selection(ctx context.Context, id string, after int64, limit int, itemID string) (SelectionPage, error) {
	out := SelectionPage{Items: []SelectionItem{}, NextPosition: after}
	if after < 0 || limit < 1 || limit > 100 {
		return out, ErrInvalid
	}
	err := s.transact(ctx, func(tx *sql.Tx) error {
		err := tx.QueryRowContext(ctx, `SELECT h.source_conversation_id,h.revision,p.source_position,p.history_origin,p.reconcile_required,p.manual_text IS NOT NULL,p.selected_count,p.omitted_count,coalesce(json_extract(CAST(p.request AS TEXT),'$.SourceLocation.Cwd'),''),coalesce(json_extract(CAST(p.request AS TEXT),'$.SourceLocation.RepositoryID'),''),coalesce(json_extract(CAST(p.request AS TEXT),'$.SourceLocation.WorktreeID'),'') FROM context_handoff h JOIN handoff_preview p ON p.handoff_id=h.id JOIN conversation c ON c.id=h.source_conversation_id WHERE h.id=?`, id).Scan(&out.SourceConversationID, &out.Revision, &out.SourcePosition, &out.HistoryOrigin, &out.ReconcileRequired, &out.Manual, &out.SelectedCount, &out.OmittedCount, &out.SourceLocation.Cwd, &out.SourceLocation.RepositoryID, &out.SourceLocation.WorktreeID)
		if err != nil {
			return storageError(ctx, err)
		}
		if after > out.SourcePosition {
			return ErrInvalid
		}
		query := `SELECT item_id,position,selected FROM handoff_item WHERE handoff_id=? AND position>? AND position<=?`
		args := []any{id, after, out.SourcePosition}
		if itemID != "" {
			query += " AND item_id=?"
			args = append(args, itemID)
		}
		query += " ORDER BY position LIMIT ?"
		args = append(args, limit+1)
		rows, err := tx.QueryContext(ctx, query, args...)
		if err != nil {
			return storageError(ctx, err)
		}
		defer func() { _ = rows.Close() }()
		out.Done = true
		for rows.Next() {
			var item SelectionItem
			if err := rows.Scan(&item.ActivityID, &item.Position, &item.Selected); err != nil {
				return storageError(ctx, err)
			}
			if len(out.Items) == limit {
				out.Done = false
				break
			}
			out.Items = append(out.Items, item)
			out.NextPosition = item.Position
		}
		return storageError(ctx, rows.Err())
	})
	return out, err
}
