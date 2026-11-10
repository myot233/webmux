package conversation

import (
	"context"
	"database/sql"
	"encoding/json"
)

const inlineReceiptSchema = `
CREATE TABLE inline_receipt (
 execution_id TEXT PRIMARY KEY,
 target_provider_thread_id TEXT NOT NULL REFERENCES provider_thread(id) ON DELETE CASCADE,
 native_thread_id TEXT NOT NULL, prompt_hash TEXT NOT NULL,
 user_text TEXT NOT NULL, attachments BLOB NOT NULL,
 provider_item_id TEXT NOT NULL DEFAULT ''
);
CREATE INDEX inline_receipt_match ON inline_receipt(target_provider_thread_id,native_thread_id,prompt_hash);
PRAGMA user_version=6;
`

func encodeInlineAttachments(in []DeliveryAttachment) ([]byte, error) {
	return json.Marshal(in)
}

// The target owns only the user's new request and its own attachment references,
// never a copy of the source history. Forgetting the source can remove previews
// and operations without exposing the historical envelope on the child's load.
func saveInlineReceipt(ctx context.Context, tx *sql.Tx, executionID string) error {
	e, err := readExecution(ctx, tx, executionID)
	if err != nil {
		return err
	}
	if e.Phase != "sending" || e.NativeThreadID == "" || len(e.PromptHash) != 64 {
		return nil
	}
	var p Preview
	if err := readPreview(ctx, tx, e.HandoffID, &p); err != nil {
		return err
	}
	attachments := append([]DeliveryAttachment{}, e.Attachments...)
	for n := range attachments {
		attachments[n].SourceReference = ""
	}
	body, err := encodeInlineAttachments(attachments)
	if err != nil {
		return ErrUnavailable
	}
	_, err = tx.ExecContext(ctx, "INSERT OR IGNORE INTO inline_receipt(execution_id,target_provider_thread_id,native_thread_id,prompt_hash,user_text,attachments,provider_item_id) SELECT ?,?,?,?,?,?,replay_user_item_id FROM handoff_execution WHERE id=?", e.ID, e.TargetProviderThreadID, e.NativeThreadID, e.PromptHash, p.Request.UserText, body, e.ID)
	return storageError(ctx, err)
}

func backfillInlineReceipts(ctx context.Context, tx *sql.Tx) error {
	rows, err := tx.QueryContext(ctx, "SELECT id FROM handoff_execution WHERE phase='sending'")
	if err != nil {
		return err
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			_ = rows.Close()
			return err
		}
		ids = append(ids, id)
	}
	err = rows.Err()
	closeErr := rows.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	for _, id := range ids {
		if err := saveInlineReceipt(ctx, tx, id); err != nil {
			return err
		}
	}
	return nil
}
