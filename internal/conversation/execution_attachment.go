package conversation

import (
	"context"
	"database/sql"
)

const attachmentSchema = `
CREATE TABLE handoff_attachment (
 delivery_id TEXT NOT NULL REFERENCES handoff_delivery(id) ON DELETE CASCADE,
 position INTEGER NOT NULL, source_reference TEXT NOT NULL, reference TEXT NOT NULL,
 name TEXT NOT NULL,mime_type TEXT NOT NULL,kind TEXT NOT NULL,digest TEXT NOT NULL,size INTEGER NOT NULL,
 PRIMARY KEY(delivery_id,position)
);
ALTER TABLE handoff_execution ADD COLUMN replay_user_item_id TEXT NOT NULL DEFAULT '';
PRAGMA user_version=5;
`

type DeliveryAttachment struct {
	SourceReference, Reference, Name, MimeType, Kind, Digest string
	Size                                                     int64
}

func readExecutionAttachments(ctx context.Context, tx *sql.Tx, id string) ([]DeliveryAttachment, error) {
	out := []DeliveryAttachment{}
	rows, err := tx.QueryContext(ctx, "SELECT source_reference,reference,name,mime_type,kind,digest,size FROM handoff_attachment WHERE delivery_id=? ORDER BY position", id)
	if err != nil {
		return nil, storageError(ctx, err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var a DeliveryAttachment
		if err := rows.Scan(&a.SourceReference, &a.Reference, &a.Name, &a.MimeType, &a.Kind, &a.Digest, &a.Size); err != nil {
			return nil, storageError(ctx, err)
		}
		out = append(out, a)
	}
	return out, storageError(ctx, rows.Err())
}

// SaveExecutionAttachments records copy intentions before touching target files.
// The canonical delivery owns them, so explicit retries reuse the same copies.
func (s *Service) SaveExecutionAttachments(ctx context.Context, id string, attachments []DeliveryAttachment) error {
	if len(attachments) > 8 {
		return ErrInvalid
	}
	return s.transact(ctx, func(tx *sql.Tx) error {
		e, err := readExecution(ctx, tx, id)
		if err != nil {
			return err
		}
		if e.Status != "pending" || e.Phase == "sending" {
			return ErrConflict
		}
		existing, err := readExecutionAttachments(ctx, tx, e.DeliveryID)
		if err != nil {
			return err
		}
		if len(existing) > 0 {
			return ErrConflict
		}
		for i, a := range attachments {
			if len(a.Digest) != 64 || a.Reference == "" || a.SourceReference == "" || a.Size < 0 || a.Size > 10<<20 {
				return ErrInvalid
			}
			if _, err := tx.ExecContext(ctx, "INSERT INTO handoff_attachment VALUES(?,?,?,?,?,?,?,?,?)", e.DeliveryID, i, a.SourceReference, a.Reference, a.Name, a.MimeType, a.Kind, a.Digest, a.Size); err != nil {
				return storageError(ctx, err)
			}
		}
		return nil
	})
}
