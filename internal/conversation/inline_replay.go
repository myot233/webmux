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
		var attachments []byte
		err := tx.QueryRowContext(ctx, `SELECT execution_id,user_text,attachments FROM inline_receipt WHERE target_provider_thread_id=? AND native_thread_id=? AND prompt_hash=? AND (provider_item_id='' OR provider_item_id=?) ORDER BY rowid DESC LIMIT 1`, thread, nativeID, hex.EncodeToString(digest[:]), providerID).Scan(&executionID, &result.Text, &attachments)
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		if err != nil {
			return storageError(ctx, err)
		}
		if err := decodePreview(attachments, &result.Attachments); err != nil {
			return err
		}
		matched = true
		if providerID != "" {
			if _, err := tx.ExecContext(ctx, "UPDATE inline_receipt SET provider_item_id=? WHERE execution_id=? AND provider_item_id=''", providerID, executionID); err != nil {
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
	if err := s.FinishExecution(ctx, executionID, "inline", ""); err != nil && !errors.Is(err, ErrNotFound) {
		return result, false, err
	}
	return result, true, nil
}
