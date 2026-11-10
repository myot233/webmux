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
func (s *Service) NormalizeInlineReplay(ctx context.Context, thread, nativeID, text string) (string, bool, error) {
	if len(text) > 64<<10 || nativeID == "" {
		return text, false, nil
	}
	digest := sha256.Sum256([]byte(text))
	var executionID, original string
	matched := false
	err := s.transact(ctx, func(tx *sql.Tx) error {
		var handoffID string
		err := tx.QueryRowContext(ctx, `SELECT e.id,d.handoff_id FROM handoff_execution e JOIN handoff_delivery d ON d.id=e.delivery_id WHERE d.target_provider_thread_id=? AND e.native_thread_id=? AND e.prompt_hash=? AND e.phase='sending' ORDER BY e.updated_at DESC LIMIT 1`, thread, nativeID, hex.EncodeToString(digest[:])).Scan(&executionID, &handoffID)
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
		original, matched = p.Request.UserText, true
		return nil
	})
	if err != nil {
		return text, false, err
	}
	if !matched {
		return text, false, nil
	}
	if err := s.FinishExecution(ctx, executionID, "inline", ""); err != nil {
		return text, false, err
	}
	return original, true, nil
}
