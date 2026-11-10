package conversation

import (
	"context"
	"database/sql"
	"errors"
)

// BindRuntime attaches a reserved runtime to an existing provider identity. It
// never repurposes an existing binding or creates a second Conversation.
func (s *Service) BindRuntime(ctx context.Context, runtimeID, threadID, configID, agent string) error {
	return s.transact(ctx, func(tx *sql.Tx) error {
		var existing Binding
		err := tx.QueryRowContext(ctx, "SELECT conversation_id,provider_thread_id FROM runtime_binding WHERE runtime_id=?", runtimeID).Scan(&existing.ConversationID, &existing.ProviderThreadID)
		if err == nil {
			if existing.ProviderThreadID != threadID {
				return ErrConflict
			}
			return nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return storageError(ctx, err)
		}
		var conversationID string
		if err := tx.QueryRowContext(ctx, "SELECT conversation_id FROM provider_thread WHERE id=? AND config_id=? AND agent_name=?", threadID, configID, agent).Scan(&conversationID); err != nil {
			return storageError(ctx, err)
		}
		_, err = tx.ExecContext(ctx, "INSERT INTO runtime_binding VALUES(?,?,?)", runtimeID, conversationID, threadID)
		if err != nil {
			return storageError(ctx, err)
		}
		_, err = tx.ExecContext(ctx, "UPDATE conversation SET active_provider_thread_id=? WHERE id=? AND active_provider_thread_id=''", threadID, conversationID)
		return storageError(ctx, err)
	})
}

// UnbindRuntime undoes an unpublished runtime without deleting a reserved draft.
func (s *Service) UnbindRuntime(ctx context.Context, runtimeID string) error {
	return s.transact(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, "DELETE FROM runtime_binding WHERE runtime_id=?", runtimeID)
		return storageError(ctx, err)
	})
}
