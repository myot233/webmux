package conversation

import (
	"context"
	"database/sql"
	"errors"
)

// The target keeps a small dispatch fact after its source is forgotten. It has
// no source body, source identifiers or command arguments. SQL triggers keep it
// atomic with every attempt transition, including cascade deletion.
const dispatchReceiptSchema = `
ALTER TABLE runtime_binding ADD COLUMN published INTEGER NOT NULL DEFAULT 1;
UPDATE runtime_binding SET published=0 WHERE runtime_id IN
 (SELECT d.runtime_id FROM handoff_delivery d JOIN handoff_execution e ON e.delivery_id=d.id
  WHERE e.status='pending' AND e.phase IN ('reserved','worktree','session'));
CREATE TABLE dispatch_receipt (
 execution_id TEXT PRIMARY KEY,
 target_provider_thread_id TEXT NOT NULL REFERENCES provider_thread(id) ON DELETE CASCADE,
 runtime_id TEXT NOT NULL, native_thread_id TEXT NOT NULL,
 delivery_id TEXT NOT NULL, status TEXT NOT NULL, phase TEXT NOT NULL,
 call_started INTEGER NOT NULL, estimated_cost INTEGER NOT NULL
);
CREATE INDEX dispatch_receipt_target ON dispatch_receipt(target_provider_thread_id,delivery_id);
CREATE INDEX delivery_target ON handoff_delivery(target_provider_thread_id);
INSERT INTO dispatch_receipt
 SELECT e.id,d.target_provider_thread_id,d.runtime_id,e.native_thread_id,e.delivery_id,
 e.status,e.phase,e.call_started,coalesce(CAST(json_extract(CAST(p.budget AS TEXT),'$.Used') AS INTEGER),0)
 FROM handoff_execution e JOIN handoff_delivery d ON d.id=e.delivery_id
 JOIN handoff_preview p ON p.handoff_id=d.handoff_id;
CREATE TRIGGER dispatch_receipt_insert AFTER INSERT ON handoff_execution BEGIN
 INSERT INTO dispatch_receipt
 SELECT NEW.id,d.target_provider_thread_id,d.runtime_id,NEW.native_thread_id,NEW.delivery_id,
 NEW.status,NEW.phase,NEW.call_started,coalesce(CAST(json_extract(CAST(p.budget AS TEXT),'$.Used') AS INTEGER),0)
 FROM handoff_delivery d JOIN handoff_preview p ON p.handoff_id=d.handoff_id WHERE d.id=NEW.delivery_id;
END;
CREATE TRIGGER dispatch_receipt_update AFTER UPDATE ON handoff_execution BEGIN
 UPDATE dispatch_receipt SET status=NEW.status,phase=NEW.phase,
 native_thread_id=NEW.native_thread_id,call_started=NEW.call_started WHERE execution_id=NEW.id;
END;
CREATE TRIGGER dispatch_receipt_delete BEFORE DELETE ON handoff_execution BEGIN
 UPDATE dispatch_receipt SET status=CASE
 WHEN OLD.status='inline' THEN 'inline'
 WHEN OLD.status='failed' THEN 'cancelled'
 WHEN OLD.call_started=0 AND NOT EXISTS
 (SELECT 1 FROM native_injection WHERE delivery_id=OLD.delivery_id AND call_started=1 AND state IN ('pending','unknown'))
 THEN 'cancelled' ELSE 'unknown' END WHERE execution_id=OLD.id;
END;
CREATE TRIGGER dispatch_receipt_history AFTER UPDATE OF state ON native_injection
 WHEN NEW.state IN ('injected','unsupported') BEGIN
 UPDATE dispatch_receipt SET status='cancelled' WHERE delivery_id=NEW.delivery_id
 AND call_started=0 AND status='unknown'
 AND NOT EXISTS(SELECT 1 FROM handoff_execution WHERE id=execution_id);
END;
PRAGMA user_version=11;
`

func readDispatch(ctx context.Context, tx *sql.Tx, id string) (Execution, error) {
	e, err := readExecution(ctx, tx, id)
	if !errors.Is(err, ErrNotFound) {
		return e, err
	}
	err = tx.QueryRowContext(ctx, `SELECT r.execution_id,r.delivery_id,p.conversation_id,
 r.target_provider_thread_id,r.runtime_id,r.native_thread_id,r.status,r.phase,r.call_started
 FROM dispatch_receipt r JOIN provider_thread p ON p.id=r.target_provider_thread_id WHERE r.execution_id=?`, id).
		Scan(&e.ID, &e.DeliveryID, &e.TargetConversationID, &e.TargetProviderThreadID, &e.RuntimeID, &e.NativeThreadID, &e.Status, &e.Phase, &e.CallStarted)
	return e, storageError(ctx, err)
}

// v10 could delete a source while leaving its target's dispatch owner behind.
// Historical receipts own the target identity even after SyncRuntime moves its
// live binding. Recover the runtime separately; never recreate the source or
// infer that a replacement native thread consumed the old call.
func backfillOrphanDispatch(ctx context.Context, tx *sql.Tx) error {
	rows, err := tx.QueryContext(ctx, `SELECT c.dispatch_execution_id,p.id,b.runtime_id,p.agent_session_id
 FROM conversation c JOIN provider_thread p ON p.conversation_id=c.id AND p.id=coalesce(
 (SELECT i.target_provider_thread_id FROM inline_receipt i WHERE i.execution_id=c.dispatch_execution_id),
 (SELECT i.target_provider_thread_id FROM native_injection i JOIN provider_thread ip ON ip.id=i.target_provider_thread_id
  WHERE ip.conversation_id=c.id AND i.call_started=1 AND i.state IN ('pending','unknown') ORDER BY i.rowid DESC LIMIT 1),
 nullif(c.active_provider_thread_id,''),
 (SELECT rb.provider_thread_id FROM runtime_binding rb WHERE rb.conversation_id=c.id ORDER BY rb.rowid DESC LIMIT 1))
 JOIN runtime_binding b ON b.conversation_id=c.id AND b.runtime_id=coalesce(
 (SELECT rb.runtime_id FROM runtime_binding rb WHERE rb.conversation_id=c.id AND rb.provider_thread_id=p.id ORDER BY rb.rowid DESC LIMIT 1),
 (SELECT rb.runtime_id FROM runtime_binding rb WHERE rb.conversation_id=c.id AND rb.provider_thread_id=c.active_provider_thread_id ORDER BY rb.rowid DESC LIMIT 1),
 (SELECT rb.runtime_id FROM runtime_binding rb WHERE rb.conversation_id=c.id ORDER BY rb.rowid DESC LIMIT 1))
 WHERE c.dispatch_execution_id!=''
 AND NOT EXISTS(SELECT 1 FROM handoff_execution WHERE id=c.dispatch_execution_id)
 AND (NOT EXISTS(SELECT 1 FROM dispatch_receipt WHERE execution_id=c.dispatch_execution_id)
 OR EXISTS(SELECT 1 FROM dispatch_receipt r WHERE r.execution_id=c.dispatch_execution_id
 AND r.status='cancelled' AND r.call_started=0 AND r.target_provider_thread_id!=p.id))`)
	if err != nil {
		return err
	}
	var orphaned []Execution
	for rows.Next() {
		var e Execution
		if err := rows.Scan(&e.ID, &e.TargetProviderThreadID, &e.RuntimeID, &e.NativeThreadID); err != nil {
			_ = rows.Close()
			return err
		}
		orphaned = append(orphaned, e)
	}
	err = rows.Err()
	closeErr := rows.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	for _, e := range orphaned {
		e.Status, e.Phase, e.DeliveryID = "cancelled", "ready", e.ID
		cost := 0
		var providerID string
		err := tx.QueryRowContext(ctx, "SELECT native_thread_id,provider_item_id FROM inline_receipt WHERE execution_id=? AND target_provider_thread_id=?", e.ID, e.TargetProviderThreadID).Scan(&e.NativeThreadID, &providerID)
		if err == nil {
			e.CallStarted, e.Status, e.Phase = true, "unknown", "sending"
			// The deleted envelope cannot be measured again. Reserve the maximum
			// history allowance rather than invent a smaller legacy occupancy.
			cost = 64000
			var consumed bool
			if err := tx.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM run WHERE provider_thread_id=? AND prompt_id=? AND consumed=1)", e.TargetProviderThreadID, e.ID).Scan(&consumed); err != nil {
				return err
			}
			if consumed || providerID != "" {
				e.Status = "inline"
			}
		} else if errors.Is(err, sql.ErrNoRows) {
			err = tx.QueryRowContext(ctx, `SELECT delivery_id,native_thread_id FROM native_injection
 WHERE target_provider_thread_id=? AND call_started=1 AND state IN ('pending','unknown') ORDER BY rowid DESC LIMIT 1`, e.TargetProviderThreadID).Scan(&e.DeliveryID, &e.NativeThreadID)
			if err == nil {
				e.Status = "unknown"
			} else if !errors.Is(err, sql.ErrNoRows) {
				return err
			}
		} else {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO dispatch_receipt VALUES(?,?,?,?,?,?,?,?,?)
 ON CONFLICT(execution_id) DO UPDATE SET target_provider_thread_id=excluded.target_provider_thread_id,
 runtime_id=excluded.runtime_id,native_thread_id=excluded.native_thread_id,delivery_id=excluded.delivery_id,
 status=excluded.status,phase=excluded.phase,call_started=excluded.call_started,estimated_cost=excluded.estimated_cost`,
			e.ID, e.TargetProviderThreadID, e.RuntimeID, e.NativeThreadID, e.DeliveryID, e.Status, e.Phase, e.CallStarted, cost); err != nil {
			return err
		}
	}
	return nil
}

// DispatchExecution is an internal writer-admission fact. Unlike Execution it
// can survive source deletion and must never be exposed as a source operation.
func (s *Service) DispatchExecution(ctx context.Context, id string) (Execution, error) {
	var e Execution
	err := s.transact(ctx, func(tx *sql.Tx) error { var err error; e, err = readDispatch(ctx, tx, id); return err })
	return e, err
}
