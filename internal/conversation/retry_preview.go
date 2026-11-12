package conversation

import (
	"context"
	"database/sql"
)

// DeliveryPreview retains the reviewed snapshot. Only history belonging to this
// handoff's unconsumed failed attempts may explain a same-Conversation change.
// Unrelated runs, activity and gaps still invalidate an explicit retry.
func (s *Service) DeliveryPreview(ctx context.Context, id string) (Preview, error) {
	var p Preview
	err := s.transact(ctx, func(tx *sql.Tx) error { return readDeliveryPreview(ctx, tx, id, &p) })
	return p, err
}

func readDeliveryPreview(ctx context.Context, tx *sql.Tx, id string, p *Preview) error {
	if err := readPreview(ctx, tx, id, p); err != nil {
		return err
	}
	if p.SourceConversationID != p.TargetConversationID {
		return nil
	}
	var failed bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM handoff_execution e JOIN handoff_delivery d ON d.id=e.delivery_id WHERE d.handoff_id=? AND e.status='failed')`, id).Scan(&failed); err != nil {
		return storageError(ctx, err)
	}
	if !failed {
		return nil
	}
	var unrelated bool
	err := tx.QueryRowContext(ctx, `WITH owned_run AS (
		SELECT r.id FROM run r JOIN handoff_execution e ON e.id=r.prompt_id
		JOIN handoff_delivery d ON d.id=e.delivery_id
		WHERE d.handoff_id=? AND e.status IN ('failed','pending')
		AND r.provider_thread_id=d.target_provider_thread_id AND r.consumed=0
	)
	SELECT
	 EXISTS(SELECT 1 FROM run WHERE conversation_id=? AND ordinal>? AND id NOT IN (SELECT id FROM owned_run))
	 OR EXISTS(SELECT 1 FROM activity WHERE conversation_id=? AND (position>? OR (position IS NULL AND created_at>=(SELECT created_at FROM context_handoff WHERE id=?))) AND (run_id IS NULL OR run_id NOT IN (SELECT id FROM owned_run)))
	 OR EXISTS(SELECT 1 FROM history_gap WHERE conversation_id=? AND created_at>=(SELECT created_at FROM context_handoff WHERE id=?))
	 OR EXISTS(SELECT 1 FROM conversation WHERE id=? AND (history_origin!=? OR reconcile_required!=?))`,
		id, p.SourceConversationID, p.SourceRunOrdinal, p.SourceConversationID, p.SourcePosition, id,
		p.SourceConversationID, id, p.SourceConversationID, p.HistoryOrigin, p.ReconcileRequired).Scan(&unrelated)
	if err != nil {
		return storageError(ctx, err)
	}
	p.Stale = unrelated
	return nil
}
