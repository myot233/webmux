package conversation

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

const dispatchSchema = `
ALTER TABLE provider_thread ADD COLUMN last_delivered_position INTEGER NOT NULL DEFAULT 0;
ALTER TABLE conversation ADD COLUMN dispatch_execution_id TEXT NOT NULL DEFAULT '';
ALTER TABLE run ADD COLUMN consumed INTEGER NOT NULL DEFAULT 0;
ALTER TABLE handoff_preview ADD COLUMN target_seen_position INTEGER NOT NULL DEFAULT 0;
ALTER TABLE handoff_execution ADD COLUMN native_continue INTEGER NOT NULL DEFAULT 0;
ALTER TABLE handoff_execution ADD COLUMN replacement_handoff_id TEXT NOT NULL DEFAULT '';
PRAGMA user_version=7;
`

func (s *Service) ProviderThreads(ctx context.Context, id string) ([]ProviderThread, error) {
	if s.broken != nil {
		return nil, s.broken
	}
	rows, err := s.db.QueryContext(ctx, "SELECT id FROM provider_thread WHERE conversation_id=? ORDER BY created_at DESC,id", id)
	if err != nil {
		return nil, storageError(ctx, err)
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			_ = rows.Close()
			return nil, storageError(ctx, err)
		}
		ids = append(ids, id)
	}
	err = rows.Err()
	closeErr := rows.Close()
	if err != nil {
		return nil, storageError(ctx, err)
	}
	if closeErr != nil {
		return nil, storageError(ctx, closeErr)
	}
	out := make([]ProviderThread, 0, len(ids))
	for _, id := range ids {
		p, err := s.ProviderThread(ctx, id)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, nil
}

func (s *Service) ThreadRuntime(ctx context.Context, id string) (string, error) {
	if s.broken != nil {
		return "", s.broken
	}
	var runtime string
	err := s.db.QueryRowContext(ctx, "SELECT runtime_id FROM runtime_binding WHERE provider_thread_id=? ORDER BY rowid DESC LIMIT 1", id).Scan(&runtime)
	return runtime, storageError(ctx, err)
}

// AcknowledgeRun advances coverage only after actual foreground consumption.
// The byte-position cursor also captures late items from older Run ordinals.
func (s *Service) AcknowledgeRun(ctx context.Context, run string) error {
	return s.transact(ctx, func(tx *sql.Tx) error {
		var thread, conversation string
		var ordinal int64
		if err := tx.QueryRowContext(ctx, "SELECT provider_thread_id,conversation_id,ordinal FROM run WHERE id=?", run).Scan(&thread, &conversation, &ordinal); err != nil {
			return storageError(ctx, err)
		}
		if _, err := tx.ExecContext(ctx, "UPDATE run SET consumed=1 WHERE id=?", run); err != nil {
			return storageError(ctx, err)
		}
		_, err := tx.ExecContext(ctx, "UPDATE provider_thread SET last_delivered_run_ordinal=max(last_delivered_run_ordinal,?),last_delivered_position=max(last_delivered_position,(SELECT next_activity_position-1 FROM conversation WHERE id=?)) WHERE id=?", ordinal, conversation, thread)
		return storageError(ctx, err)
	})
}

func (s *Service) ActivateProvider(ctx context.Context, id string) error {
	return s.transact(ctx, func(tx *sql.Tx) error {
		result, err := tx.ExecContext(ctx, "UPDATE conversation SET active_provider_thread_id=?,updated_at=? WHERE id=(SELECT conversation_id FROM provider_thread WHERE id=?)", id, time.Now().UTC().UnixNano(), id)
		if err != nil {
			return storageError(ctx, err)
		}
		n, err := result.RowsAffected()
		if err != nil {
			return storageError(ctx, err)
		}
		if n == 0 {
			return ErrNotFound
		}
		return nil
	})
}

// RuntimeDispatch returns the persisted provider-switch owner. Resuming the
// intended runtime for replay is safe; other runtimes must not become writers.
func (s *Service) RuntimeDispatch(ctx context.Context, runtime string) (Execution, bool, error) {
	var out Execution
	found := false
	err := s.transact(ctx, func(tx *sql.Tx) error {
		var id string
		err := tx.QueryRowContext(ctx, "SELECT c.dispatch_execution_id FROM conversation c JOIN runtime_binding b ON b.conversation_id=c.id WHERE b.runtime_id=?", runtime).Scan(&id)
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		if err != nil {
			return storageError(ctx, err)
		}
		if id == "" {
			return nil
		}
		out, err = readDispatch(ctx, tx, id)
		if errors.Is(err, ErrNotFound) {
			return ErrUnavailable
		}
		if err != nil {
			return err
		}
		found = out.Status == "pending" || out.Status == "unknown"
		return nil
	})
	return out, found, err
}

// LinkFreshPreview prepares recovery without issuing another user prompt. The
// original attempt remains queryable and the superseded preview cannot execute.
func (s *Service) LinkFreshPreview(ctx context.Context, executionID, replacementID string) error {
	return s.transact(ctx, func(tx *sql.Tx) error {
		e, err := readExecution(ctx, tx, executionID)
		if err != nil {
			return err
		}
		if e.Status != "pending" || e.Phase == "sending" {
			return ErrConflict
		}
		var replacement Preview
		if err := readPreview(ctx, tx, replacementID, &replacement); err != nil {
			return err
		}
		if replacement.SourceConversationID != e.SourceConversationID || replacement.TargetConversationID != e.TargetConversationID || !replacement.Request.ForceFresh {
			return ErrConflict
		}
		now := time.Now().UTC().UnixNano()
		if _, err := tx.ExecContext(ctx, "UPDATE handoff_execution SET status='failed',error_class='fresh_preview_required',replacement_handoff_id=?,updated_at=? WHERE id=?", replacementID, now, executionID); err != nil {
			return storageError(ctx, err)
		}
		if _, err := tx.ExecContext(ctx, "UPDATE handoff_delivery SET status='failed',error_class='fresh_preview_required',updated_at=? WHERE id=?", now, e.DeliveryID); err != nil {
			return storageError(ctx, err)
		}
		_, err = tx.ExecContext(ctx, "UPDATE handoff_preview SET state='superseded' WHERE handoff_id=?", e.HandoffID)
		return storageError(ctx, err)
	})
}

// FreshenPreview drops an exhausted reusable native context before any runtime
// exists. It retains the command receipt and draft ID, then selects full history.
func (s *Service) FreshenPreview(ctx context.Context, id string) (Preview, error) {
	err := s.transact(ctx, func(tx *sql.Tx) error {
		var p Preview
		if err := readPreview(ctx, tx, id, &p); err != nil {
			return err
		}
		if p.State != "preparing" || p.Request.Fork {
			return ErrConflict
		}
		target, err := s.allocateID(ctx, tx, "provider_thread")
		if err != nil {
			return err
		}
		now := time.Now().UTC().UnixNano()
		if _, err := tx.ExecContext(ctx, "INSERT INTO provider_thread(id,conversation_id,config_id,agent_name,status,created_at,updated_at) VALUES(?,?,?,?,'draft',?,?)", target, p.TargetConversationID, p.Request.ConfigID, p.Request.AgentName, now, now); err != nil {
			return storageError(ctx, err)
		}
		if err := putConfig(ctx, tx, target, p.Request.AgentConfig); err != nil {
			return err
		}
		p.Request.TargetProviderThreadID = ""
		p.Request.ForceFresh = true
		request, err := encodePreviewRequest(p.Request)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, "UPDATE context_handoff SET target_provider_thread_id=?,strategy='full_thread_summary',from_run_ordinal=1 WHERE id=?", target, id); err != nil {
			return storageError(ctx, err)
		}
		_, err = tx.ExecContext(ctx, "UPDATE handoff_preview SET target_seen_position=0,request=? WHERE handoff_id=?", request, id)
		return storageError(ctx, err)
	})
	if err != nil {
		return Preview{}, err
	}
	return s.Preview(ctx, id)
}
