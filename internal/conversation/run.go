package conversation

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// StartRun allocates an ordinal in the same transaction as the run. Repeating
// a prompt ID returns its existing run and never consumes a second ordinal.
func (s *Service) StartRun(ctx context.Context, threadID, promptID string) (Run, error) {
	var r Run
	if threadID == "" || promptID == "" {
		return r, ErrInvalid
	}
	err := s.transact(ctx, func(tx *sql.Tx) error {
		var created, updated int64
		err := tx.QueryRowContext(ctx, `SELECT id,conversation_id,ordinal,provider_thread_id,prompt_id,status,created_at,updated_at FROM run WHERE provider_thread_id=? AND prompt_id=?`, threadID, promptID).Scan(&r.ID, &r.ConversationID, &r.Ordinal, &r.ProviderThreadID, &r.PromptID, &r.Status, &created, &updated)
		if err == nil {
			r.CreatedAt = time.Unix(0, created).UTC()
			r.UpdatedAt = time.Unix(0, updated).UTC()
			return nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return storageError(ctx, err)
		}
		if err := tx.QueryRowContext(ctx, `SELECT c.id,c.next_run_ordinal FROM conversation c JOIN provider_thread p ON p.conversation_id=c.id WHERE p.id=?`, threadID).Scan(&r.ConversationID, &r.Ordinal); err != nil {
			return storageError(ctx, err)
		}
		r.ID, err = s.allocateID(ctx, tx, "run")
		if err != nil {
			return err
		}
		r.ProviderThreadID, r.PromptID, r.Status = threadID, promptID, "pending"
		r.CreatedAt = time.Now().UTC()
		r.UpdatedAt = r.CreatedAt
		_, err = tx.ExecContext(ctx, "INSERT INTO run(id,conversation_id,ordinal,provider_thread_id,prompt_id,status,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?)", r.ID, r.ConversationID, r.Ordinal, r.ProviderThreadID, r.PromptID, r.Status, r.CreatedAt.UnixNano(), r.UpdatedAt.UnixNano())
		if err != nil {
			return storageError(ctx, err)
		}
		_, err = tx.ExecContext(ctx, "UPDATE conversation SET next_run_ordinal=next_run_ordinal+1,updated_at=? WHERE id=?", r.UpdatedAt.UnixNano(), r.ConversationID)
		return storageError(ctx, err)
	})
	return r, err
}
