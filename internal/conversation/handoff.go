package conversation

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// CommitHandoff reserves all three objects atomically. Reusing a command key
// with a different hash is a conflict, even after restart. A pending receipt
// is returned as pending; it does not authorize repeating a provider call.
func (s *Service) CommitHandoff(ctx context.Context, in HandoffInput) (CommandReceipt, error) {
	var receipt CommandReceipt
	if in.ClientRequestID == "" || in.RequestHash == "" || in.FromRunOrdinal < 0 || in.ToRunOrdinal < in.FromRunOrdinal {
		return receipt, ErrInvalid
	}
	if in.Strategy != "full_thread_summary" && in.Strategy != "delta_since_target_last_seen" && in.Strategy != "manual_context" {
		return receipt, ErrInvalid
	}
	seen := map[string]bool{}
	for _, ids := range [][]string{in.SelectedItemIDs, in.OmittedItemIDs} {
		for _, id := range ids {
			if id == "" || seen[id] {
				return receipt, ErrInvalid
			}
			seen[id] = true
		}
	}
	err := s.transact(ctx, func(tx *sql.Tx) error {
		var err error
		receipt, err = readReceipt(ctx, tx, in.TargetConversationID, in.ClientRequestID)
		if err == nil {
			if receipt.RequestHash != in.RequestHash {
				return ErrConflict
			}
			return nil
		}
		if !errors.Is(err, ErrNotFound) {
			return err
		}
		hID, err := s.allocateID(ctx, tx, "context_handoff")
		if err != nil {
			return err
		}
		dID, err := s.allocateID(ctx, tx, "handoff_delivery")
		if err != nil {
			return err
		}
		now := time.Now().UTC()
		_, err = tx.ExecContext(ctx, "INSERT INTO context_handoff VALUES(?,?,?,?,?,?,?,?,?)", hID, in.SourceConversationID, in.TargetConversationID, in.TargetProviderThreadID, in.Strategy, in.FromRunOrdinal, in.ToRunOrdinal, 1, now.UnixNano())
		if err != nil {
			return storageError(ctx, err)
		}
		for selected, ids := range [][]string{in.OmittedItemIDs, in.SelectedItemIDs} {
			for position, id := range ids {
				if _, err := tx.ExecContext(ctx, "INSERT INTO handoff_item VALUES(?,?,?,?)", hID, id, selected, position); err != nil {
					return storageError(ctx, err)
				}
			}
		}
		_, err = tx.ExecContext(ctx, "INSERT INTO handoff_delivery(id,handoff_id,target_provider_thread_id,status,updated_at) VALUES(?,?,?,'pending',?)", dID, hID, in.TargetProviderThreadID, now.UnixNano())
		if err != nil {
			return storageError(ctx, err)
		}
		_, err = tx.ExecContext(ctx, "INSERT INTO command_receipt(conversation_id,client_request_id,request_hash,handoff_id,delivery_id,created_at) VALUES(?,?,?,?,?,?)", in.TargetConversationID, in.ClientRequestID, in.RequestHash, hID, dID, now.UnixNano())
		if err != nil {
			return storageError(ctx, err)
		}
		receipt = CommandReceipt{ConversationID: in.TargetConversationID, ClientRequestID: in.ClientRequestID, RequestHash: in.RequestHash, HandoffID: hID, DeliveryID: dID, CreatedAt: now}
		return nil
	})
	return receipt, err
}

func readReceipt(ctx context.Context, tx *sql.Tx, conversationID, requestID string) (CommandReceipt, error) {
	r := CommandReceipt{ConversationID: conversationID, ClientRequestID: requestID}
	var created int64
	err := tx.QueryRowContext(ctx, "SELECT request_hash,COALESCE(handoff_id,''),COALESCE(delivery_id,''),execution_id,runtime_id,result,error_class,created_at FROM command_receipt WHERE conversation_id=? AND client_request_id=?", conversationID, requestID).Scan(&r.RequestHash, &r.HandoffID, &r.DeliveryID, &r.ExecutionID, &r.RuntimeID, &r.Result, &r.ErrorClass, &created)
	r.CreatedAt = time.Unix(0, created).UTC()
	return r, storageError(ctx, err)
}
