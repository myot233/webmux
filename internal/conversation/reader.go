package conversation

import (
	"context"
	"database/sql"
)

// ReaderScope identifies one frozen source, not the target's unrestricted local
// environment. It is supplied to a provider only after execution was reserved.
type ReaderScope struct{ ConversationID, HandoffID string }

func (s *Service) ReaderScope(ctx context.Context, runtimeID string) (ReaderScope, error) {
	var out ReaderScope
	err := s.transact(ctx, func(tx *sql.Tx) error {
		err := tx.QueryRowContext(ctx, `SELECT h.source_conversation_id,h.id FROM runtime_binding b JOIN context_handoff h ON h.target_provider_thread_id=b.provider_thread_id JOIN handoff_delivery d ON d.handoff_id=h.id AND d.runtime_id=b.runtime_id WHERE b.runtime_id=? ORDER BY d.rowid DESC LIMIT 1`, runtimeID).Scan(&out.ConversationID, &out.HandoffID)
		return storageError(ctx, err)
	})
	return out, err
}
