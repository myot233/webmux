package conversation

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base32"
	"errors"
	"strings"
	"time"
)

const executionSchema = `
ALTER TABLE command_receipt ADD COLUMN execution_id TEXT NOT NULL DEFAULT '';
ALTER TABLE handoff_delivery ADD COLUMN runtime_id TEXT NOT NULL DEFAULT '';
CREATE TABLE handoff_execution (
 id TEXT PRIMARY KEY,
 delivery_id TEXT NOT NULL REFERENCES handoff_delivery(id) ON DELETE CASCADE,
 phase TEXT NOT NULL DEFAULT 'reserved', status TEXT NOT NULL DEFAULT 'pending',
 step TEXT NOT NULL DEFAULT 'precheck', error_class TEXT NOT NULL DEFAULT '',
 worktree_path TEXT NOT NULL DEFAULT '', worktree_id TEXT NOT NULL DEFAULT '',
 worktree_branch TEXT NOT NULL DEFAULT '', worktree_base TEXT NOT NULL DEFAULT '',
 native_thread_id TEXT NOT NULL DEFAULT '', prompt_hash TEXT NOT NULL DEFAULT '',
 call_started INTEGER NOT NULL DEFAULT 0,
 updated_at INTEGER NOT NULL
);
CREATE INDEX execution_delivery ON handoff_execution(delivery_id);
PRAGMA user_version=4;
`

type Execution struct {
	ClientRequestID                                                                               string
	Retry                                                                                         bool
	ID, DeliveryID, HandoffID, SourceConversationID, TargetConversationID, TargetProviderThreadID string
	RuntimeID, Phase, Status, Step, ErrorClass                                                    string
	WorktreePath, WorktreeID, WorktreeBranch, WorktreeBase                                        string
	NativeThreadID, PromptHash                                                                    string
	CallStarted, NativeContinue                                                                   bool
	ReplacementHandoffID, HistoryStatus                                                           string
	Attachments                                                                                   []DeliveryAttachment
	UpdatedAt                                                                                     time.Time
}

type ExecutionInput struct {
	SourceConversationID, HandoffID, ClientRequestID, RequestHash string
	Revision                                                      int64
	Retry                                                         bool
}

func readExecution(ctx context.Context, tx *sql.Tx, id string) (Execution, error) {
	var e Execution
	var updated int64
	err := tx.QueryRowContext(ctx, `SELECT e.id,e.delivery_id,d.handoff_id,h.source_conversation_id,h.target_conversation_id,d.target_provider_thread_id,d.runtime_id,e.phase,e.status,e.step,e.error_class,e.worktree_path,e.worktree_id,e.worktree_branch,e.worktree_base,e.native_thread_id,e.prompt_hash,e.call_started,e.native_continue,e.replacement_handoff_id,coalesce((SELECT state FROM native_injection WHERE delivery_id=e.delivery_id),''),coalesce((SELECT client_request_id FROM command_receipt WHERE execution_id=e.id LIMIT 1),''),e.rowid!=(SELECT min(first.rowid) FROM handoff_execution first WHERE first.delivery_id=e.delivery_id),e.updated_at FROM handoff_execution e JOIN handoff_delivery d ON d.id=e.delivery_id JOIN context_handoff h ON h.id=d.handoff_id WHERE e.id=?`, id).Scan(&e.ID, &e.DeliveryID, &e.HandoffID, &e.SourceConversationID, &e.TargetConversationID, &e.TargetProviderThreadID, &e.RuntimeID, &e.Phase, &e.Status, &e.Step, &e.ErrorClass, &e.WorktreePath, &e.WorktreeID, &e.WorktreeBranch, &e.WorktreeBase, &e.NativeThreadID, &e.PromptHash, &e.CallStarted, &e.NativeContinue, &e.ReplacementHandoffID, &e.HistoryStatus, &e.ClientRequestID, &e.Retry, &updated)
	e.UpdatedAt = time.Unix(0, updated).UTC()
	if err != nil {
		return e, storageError(ctx, err)
	}
	e.Attachments, err = readExecutionAttachments(ctx, tx, e.DeliveryID)
	return e, err
}

func (s *Service) Execution(ctx context.Context, id string) (Execution, error) {
	var e Execution
	err := s.transact(ctx, func(tx *sql.Tx) error { var err error; e, err = readExecution(ctx, tx, id); return err })
	return e, err
}

func (s *Service) Delivery(ctx context.Context, handoffID string) (Execution, error) {
	var e Execution
	err := s.transact(ctx, func(tx *sql.Tx) error {
		var id string
		if err := tx.QueryRowContext(ctx, `SELECT e.id FROM handoff_execution e JOIN handoff_delivery d ON d.id=e.delivery_id WHERE d.handoff_id=? ORDER BY e.rowid DESC LIMIT 1`, handoffID).Scan(&id); err != nil {
			return storageError(ctx, err)
		}
		var err error
		e, err = readExecution(ctx, tx, id)
		return err
	})
	return e, err
}

// BeginExecution commits the receipt and pending delivery before any external
// mutation. Retry is explicit and permits only a definitively failed attempt.
// An old command key always points to its own attempt's original outcome.
func (s *Service) BeginExecution(ctx context.Context, in ExecutionInput) (Execution, error) {
	var out Execution
	if in.ClientRequestID == "" || len(in.ClientRequestID) > 64 || in.RequestHash == "" || in.Revision < 1 {
		return out, ErrInvalid
	}
	err := s.transact(ctx, func(tx *sql.Tx) error {
		receipt, err := readReceipt(ctx, tx, in.SourceConversationID, in.ClientRequestID)
		if err == nil {
			if receipt.RequestHash != in.RequestHash || receipt.ExecutionID == "" {
				return ErrConflict
			}
			out, err = readExecution(ctx, tx, receipt.ExecutionID)
			return err
		}
		if !errors.Is(err, ErrNotFound) {
			return err
		}
		var p Preview
		if err := readPreview(ctx, tx, in.HandoffID, &p); err != nil {
			return err
		}
		if p.SourceConversationID != in.SourceConversationID || p.Revision != in.Revision || p.Stale || p.Budget.OverBudget || (p.State != "ready" && p.State != "failed") {
			return ErrConflict
		}
		{
			var owner string
			if err := tx.QueryRowContext(ctx, "SELECT dispatch_execution_id FROM conversation WHERE id=?", p.TargetConversationID).Scan(&owner); err != nil {
				return storageError(ctx, err)
			}
			if owner != "" {
				previous, err := readDispatch(ctx, tx, owner)
				if err != nil {
					return ErrUnavailable
				}
				if err == nil && (previous.Status == "pending" || previous.Status == "unknown") {
					return ErrConflict
				}
			}
		}
		var deliveryID, runtimeID string
		var prior Execution
		err = tx.QueryRowContext(ctx, "SELECT id,runtime_id FROM handoff_delivery WHERE handoff_id=?", p.ID).Scan(&deliveryID, &runtimeID)
		if err == nil {
			var latest string
			if err := tx.QueryRowContext(ctx, "SELECT id FROM handoff_execution WHERE delivery_id=? ORDER BY rowid DESC LIMIT 1", deliveryID).Scan(&latest); err != nil {
				return storageError(ctx, err)
			}
			prior, err = readExecution(ctx, tx, latest)
			if err != nil {
				return err
			}
			if !in.Retry || prior.Status != "failed" {
				return ErrConflict
			}
		} else if errors.Is(err, sql.ErrNoRows) {
			if in.Retry || p.State != "ready" {
				return ErrConflict
			}
			deliveryID, err = s.allocateID(ctx, tx, "handoff_delivery")
			if err != nil {
				return err
			}
			// Runtime IDs keep the manager's eight-character base32 form. Reserve
			// against every durable binding and delivery before publishing one.
			if !p.Request.Fork {
				err := tx.QueryRowContext(ctx, "SELECT runtime_id FROM runtime_binding WHERE provider_thread_id=? ORDER BY rowid DESC LIMIT 1", p.TargetProviderThreadID).Scan(&runtimeID)
				if err != nil && !errors.Is(err, sql.ErrNoRows) {
					return storageError(ctx, err)
				}
			}
			for attempt := 0; runtimeID == "" && attempt < 8; attempt++ {
				var bytes [5]byte
				if _, err := rand.Read(bytes[:]); err != nil {
					return ErrUnavailable
				}
				runtimeID = strings.ToLower(base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(bytes[:]))
				var used int
				if err := tx.QueryRowContext(ctx, "SELECT (SELECT count(*) FROM runtime_binding WHERE runtime_id=?)+(SELECT count(*) FROM handoff_delivery WHERE runtime_id=?)", runtimeID, runtimeID).Scan(&used); err != nil {
					return storageError(ctx, err)
				}
				if used == 0 {
					break
				}
				runtimeID = ""
			}
			if runtimeID == "" {
				return ErrUnavailable
			}
			_, err = tx.ExecContext(ctx, "INSERT INTO handoff_delivery(id,handoff_id,target_provider_thread_id,status,runtime_id,updated_at) VALUES(?,?,?,'pending',?,?)", deliveryID, p.ID, p.TargetProviderThreadID, runtimeID, time.Now().UTC().UnixNano())
			if err != nil {
				return storageError(ctx, err)
			}
		} else {
			return storageError(ctx, err)
		}
		id, err := s.allocateID(ctx, tx, "handoff_execution")
		if err != nil {
			return err
		}
		now := time.Now().UTC()
		phase := prior.Phase
		if phase == "" {
			phase = "reserved"
		}
		if phase == "sending" {
			phase = "ready"
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO handoff_execution(id,delivery_id,phase,worktree_path,worktree_id,worktree_branch,worktree_base,updated_at) VALUES(?,?,?,?,?,?,?,?)`, id, deliveryID, phase, prior.WorktreePath, prior.WorktreeID, prior.WorktreeBranch, prior.WorktreeBase, now.UnixNano())
		if err != nil {
			return storageError(ctx, err)
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO command_receipt(conversation_id,client_request_id,request_hash,handoff_id,delivery_id,runtime_id,execution_id,created_at) VALUES(?,?,?,?,?,?,?,?)`, in.SourceConversationID, in.ClientRequestID, in.RequestHash, p.ID, deliveryID, runtimeID, id, now.UnixNano())
		if err != nil {
			return storageError(ctx, err)
		}
		if _, err := tx.ExecContext(ctx, "UPDATE handoff_preview SET state='delivering' WHERE handoff_id=?", p.ID); err != nil {
			return storageError(ctx, err)
		}
		if _, err := tx.ExecContext(ctx, "UPDATE handoff_delivery SET status=CASE WHEN EXISTS(SELECT 1 FROM native_injection WHERE delivery_id=? AND state='injected') THEN 'injected' ELSE 'pending' END,error_class='',updated_at=? WHERE id=?", deliveryID, now.UnixNano(), deliveryID); err != nil {
			return storageError(ctx, err)
		}
		{
			if _, err := tx.ExecContext(ctx, "UPDATE conversation SET dispatch_execution_id=? WHERE id=?", id, p.TargetConversationID); err != nil {
				return storageError(ctx, err)
			}
		}
		out, err = readExecution(ctx, tx, id)
		return err
	})
	return out, err
}

// Checkpoint saves only resource identities and phase. Pending work before
// sending can safely resume; sending may only advance via an observed outcome.
func (s *Service) CheckpointExecution(ctx context.Context, e Execution) error {
	return s.transact(ctx, func(tx *sql.Tx) error {
		prior, err := readExecution(ctx, tx, e.ID)
		if err != nil {
			return err
		}
		if prior.Status != "pending" || prior.Phase == "sending" {
			return ErrConflict
		}
		switch e.Phase {
		case "reserved", "worktree", "session", "ready", "sending":
		default:
			return ErrInvalid
		}
		_, err = tx.ExecContext(ctx, `UPDATE handoff_execution SET phase=?,step=?,worktree_path=?,worktree_id=?,worktree_branch=?,worktree_base=?,native_thread_id=?,prompt_hash=?,native_continue=?,replacement_handoff_id=?,updated_at=? WHERE id=?`, e.Phase, e.Step, e.WorktreePath, e.WorktreeID, e.WorktreeBranch, e.WorktreeBase, e.NativeThreadID, e.PromptHash, e.NativeContinue, e.ReplacementHandoffID, time.Now().UTC().UnixNano(), e.ID)
		return storageError(ctx, err)
	})
}

// FinishExecution never overwrites confirmed consumption. Inline confirmation
// commits coverage and item receipts in the same transaction as the outcome.
func (s *Service) FinishExecution(ctx context.Context, id, status, errorClass string) error {
	if status != "inline" && status != "unknown" && status != "failed" {
		return ErrInvalid
	}
	return s.transact(ctx, func(tx *sql.Tx) error {
		e, err := readExecution(ctx, tx, id)
		if err != nil {
			return err
		}
		if e.Status == "inline" || (e.Status == "failed" && status != "inline") {
			return nil
		}
		if status == "inline" && e.NativeThreadID == "" {
			return ErrInvalid
		}
		historyUnknown := status == "unknown" && (e.HistoryStatus == "unknown" || e.HistoryStatus == "pending")
		if e.Phase != "sending" && status != "failed" && !historyUnknown {
			return ErrConflict
		}
		now := time.Now().UTC().UnixNano()
		_, err = tx.ExecContext(ctx, "UPDATE handoff_execution SET status=?,error_class=?,updated_at=? WHERE id=?", status, errorClass, now, id)
		if err != nil {
			return storageError(ctx, err)
		}
		deliveryStatus := status
		if e.HistoryStatus == "injected" {
			deliveryStatus = "injected"
		}
		_, err = tx.ExecContext(ctx, "UPDATE handoff_delivery SET status=?,native_thread_id=?,error_class=?,updated_at=? WHERE id=?", deliveryStatus, e.NativeThreadID, errorClass, now, e.DeliveryID)
		if err != nil {
			return storageError(ctx, err)
		}
		state := status
		if status == "inline" {
			state = "delivered"
			if _, err := tx.ExecContext(ctx, "INSERT OR IGNORE INTO delivery_item SELECT ?,item_id FROM handoff_item WHERE handoff_id=? AND selected=1", e.DeliveryID, e.HandoffID); err != nil {
				return storageError(ctx, err)
			}
			// Fork coverage is source-specific and must not become a Run cursor in
			// the child Conversation, whose ordinal sequence starts independently.
			if e.SourceConversationID == e.TargetConversationID {
				if _, err := tx.ExecContext(ctx, "UPDATE provider_thread SET last_delivered_run_ordinal=max(last_delivered_run_ordinal,(SELECT to_run_ordinal FROM context_handoff WHERE id=?)),last_delivered_position=max(last_delivered_position,(SELECT source_position FROM handoff_preview WHERE handoff_id=?)) WHERE id=?", e.HandoffID, e.HandoffID, e.TargetProviderThreadID); err != nil {
					return storageError(ctx, err)
				}
			}
		}
		_, err = tx.ExecContext(ctx, "UPDATE handoff_preview SET state=? WHERE handoff_id=?", state, e.HandoffID)
		return storageError(ctx, err)
	})
}

// RecoverExecutions is run during database open. No provider side effect is
// retried merely because the process died before its acknowledgement committed.
func recoverExecutions(ctx context.Context, tx *sql.Tx) error {
	if _, err := tx.ExecContext(ctx, "UPDATE native_injection SET state='unknown' WHERE state='pending' AND call_started=1"); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "UPDATE handoff_execution SET status='unknown',error_class='history_unknown' WHERE status='pending' AND delivery_id IN (SELECT delivery_id FROM native_injection WHERE state='unknown')"); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "UPDATE handoff_execution SET phase='ready' WHERE status='pending' AND phase='sending' AND call_started=0"); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "UPDATE handoff_execution SET status='unknown',error_class='interrupted' WHERE status='pending' AND phase='sending'"); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "UPDATE handoff_delivery SET status='unknown',error_class='interrupted' WHERE id IN (SELECT delivery_id FROM handoff_execution WHERE status='unknown') AND status='pending'"); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, "UPDATE handoff_preview SET state='unknown' WHERE handoff_id IN (SELECT d.handoff_id FROM handoff_delivery d JOIN handoff_execution e ON e.delivery_id=d.id WHERE e.status='unknown')")
	return err
}

// ClaimExecution is a one-way dispatch gate owned by the actor. Even an
// accidental duplicate internal command cannot issue a second provider call.
func (s *Service) ClaimExecution(ctx context.Context, id string) error {
	return s.transact(ctx, func(tx *sql.Tx) error {
		result, err := tx.ExecContext(ctx, "UPDATE handoff_execution SET call_started=1 WHERE id=? AND phase='sending' AND status='pending' AND call_started=0", id)
		if err != nil {
			return storageError(ctx, err)
		}
		n, err := result.RowsAffected()
		if err != nil {
			return storageError(ctx, err)
		}
		if n != 1 {
			return ErrConflict
		}
		return nil
	})
}

// ClaimExecutionText persists the actual provider text after capability fallback
// in the same one-way gate that admits the provider call.
func (s *Service) ClaimExecutionText(ctx context.Context, id, expectedHash, wireHash string) error {
	if len(expectedHash) != 64 || len(wireHash) != 64 {
		return ErrInvalid
	}
	return s.transact(ctx, func(tx *sql.Tx) error {
		result, err := tx.ExecContext(ctx, "UPDATE handoff_execution SET call_started=1,prompt_hash=? WHERE id=? AND phase='sending' AND status='pending' AND call_started=0 AND prompt_hash=?", wireHash, id, expectedHash)
		if err != nil {
			return storageError(ctx, err)
		}
		n, err := result.RowsAffected()
		if err != nil {
			return storageError(ctx, err)
		}
		if n != 1 {
			return ErrConflict
		}
		return saveInlineReceipt(ctx, tx, id)
	})
}
