package conversation

import (
	"context"
	"database/sql"
)

// LatestPreview restores the newest draft, including unresolved delivery state.
// The ID query and LIMIT avoid materializing a conversation's whole history.
func (s *Service) LatestPreview(ctx context.Context, source string, fork bool) (Preview, error) {
	var out Preview
	err := s.transact(ctx, func(tx *sql.Tx) error {
		var id string
		if err := tx.QueryRowContext(ctx, `SELECT id FROM context_handoff WHERE source_conversation_id=? AND (target_conversation_id!=source_conversation_id)=? ORDER BY created_at DESC,rowid DESC LIMIT 1`, source, fork).Scan(&id); err != nil {
			return storageError(ctx, err)
		}
		return readPreview(ctx, tx, id, &out)
	})
	return out, err
}
