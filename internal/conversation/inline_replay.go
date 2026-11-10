package conversation

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
)

// NormalizeInlineReplay recognizes only the exact payload recorded before a
// call to this exact native thread. A marker alone never proves delivery and
// arbitrary user text is never stripped. It also reconciles an unknown call.
type ReplayedUser struct {
	Text        string
	Attachments []DeliveryAttachment
}

func (s *Service) NormalizeInlineReplay(ctx context.Context, thread, nativeID, text string) (string, bool, error) {
	result, matched, err := s.NormalizeInlineUser(ctx, thread, nativeID, text)
	return result.Text, matched, err
}

func (s *Service) NormalizeInlineUser(ctx context.Context, thread, nativeID, text string, providerIDs ...string) (ReplayedUser, bool, error) {
	result := ReplayedUser{Text: text}
	providerID := ""
	if len(providerIDs) > 0 {
		providerID = providerIDs[0]
	}
	if len(text) > 64<<10 || nativeID == "" {
		return result, false, nil
	}
	digest := sha256.Sum256([]byte(text))
	var executionID string
	matched := false
	err := s.transact(ctx, func(tx *sql.Tx) error {
		var handoffID string
		err := tx.QueryRowContext(ctx, `SELECT e.id,d.handoff_id FROM handoff_execution e JOIN handoff_delivery d ON d.id=e.delivery_id WHERE d.target_provider_thread_id=? AND e.native_thread_id=? AND e.prompt_hash=? AND e.phase='sending' AND (e.replay_user_item_id='' OR e.replay_user_item_id=?) ORDER BY e.updated_at DESC LIMIT 1`, thread, nativeID, hex.EncodeToString(digest[:]), providerID).Scan(&executionID, &handoffID)
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		if err != nil {
			return storageError(ctx, err)
		}
		var p Preview
		if err := readPreview(ctx, tx, handoffID, &p); err != nil {
			return err
		}
		result.Text, matched = p.Request.UserText, true
		execution, err := readExecution(ctx, tx, executionID)
		if err != nil {
			return err
		}
		result.Attachments = execution.Attachments
		if providerID != "" {
			if _, err := tx.ExecContext(ctx, "UPDATE handoff_execution SET replay_user_item_id=? WHERE id=? AND replay_user_item_id=''", providerID, executionID); err != nil {
				return storageError(ctx, err)
			}
		}
		return nil
	})
	if err != nil {
		return result, false, err
	}
	if !matched {
		return result, false, nil
	}
	if err := s.FinishExecution(ctx, executionID, "inline", ""); err != nil {
		return result, false, err
	}
	return result, true, nil
}
