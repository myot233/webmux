package conversation

import (
	"context"
	"database/sql"
	"time"
)

type ActivityPart struct {
	Exportable      bool
	ExclusionReason string
	Activity        Activity
	NextTextOffset  int64
	Truncated       bool
}

type ActivityPage struct {
	Items             []ActivityPart
	NextPosition      int64
	Gaps              []HistoryGap
	ReconcileRequired bool
	HistoryOrigin     string
}

func pageBounds(limit, maxBytes int) (int, int, error) {
	if limit == 0 {
		limit = 20
	}
	if maxBytes == 0 {
		maxBytes = 4000
	}
	if limit < 1 || limit > 100 || maxBytes < 4 || maxBytes > MaxActivityBytes || limit*maxBytes > 256<<10 {
		return 0, 0, ErrInvalid
	}
	return limit, maxBytes, nil
}

// Activities reads only committed items from a transaction snapshot; archived
// conversations need no actor or provider process. New writes appear on a
// subsequent page and never renumber an already returned position.
func (s *Service) Activities(ctx context.Context, conversationID string, after int64, limit, maxBytes int) (ActivityPage, error) {
	out := ActivityPage{Items: []ActivityPart{}, Gaps: []HistoryGap{}, NextPosition: after}
	limit, maxBytes, err := pageBounds(limit, maxBytes)
	if err != nil || after < 0 {
		return out, ErrInvalid
	}
	err = s.transact(ctx, func(tx *sql.Tx) error {
		if err := tx.QueryRowContext(ctx, "SELECT history_origin,reconcile_required FROM conversation WHERE id=?", conversationID).Scan(&out.HistoryOrigin, &out.ReconcileRequired); err != nil {
			return storageError(ctx, err)
		}
		rows, err := tx.QueryContext(ctx, "SELECT id FROM activity WHERE conversation_id=? AND position>? ORDER BY position LIMIT ?", conversationID, after, limit)
		if err != nil {
			return storageError(ctx, err)
		}
		var ids []string
		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err != nil {
				_ = rows.Close()
				return storageError(ctx, err)
			}
			ids = append(ids, id)
		}
		err = rows.Err()
		closeErr := rows.Close()
		if err != nil {
			return storageError(ctx, err)
		}
		if closeErr != nil {
			return storageError(ctx, closeErr)
		}
		for _, id := range ids {
			part, err := readActivity(ctx, tx, conversationID, id, 0, maxBytes)
			if err != nil {
				return err
			}
			out.Items = append(out.Items, part)
			out.NextPosition = part.Activity.Position
		}
		rows, err = tx.QueryContext(ctx, "SELECT id,provider_thread_id,reason,created_at FROM history_gap WHERE conversation_id=? AND resolved=0 ORDER BY created_at LIMIT 100", conversationID)
		if err != nil {
			return storageError(ctx, err)
		}
		defer func() { _ = rows.Close() }()
		for rows.Next() {
			var gap HistoryGap
			var created int64
			if err := rows.Scan(&gap.ID, &gap.ProviderThreadID, &gap.Reason, &created); err != nil {
				return storageError(ctx, err)
			}
			gap.CreatedAt = time.Unix(0, created).UTC()
			out.Gaps = append(out.Gaps, gap)
		}
		return storageError(ctx, rows.Err())
	})
	return out, err
}

func (s *Service) Activity(ctx context.Context, conversationID, id string, offset int64, maxBytes int) (ActivityPart, error) {
	var out ActivityPart
	_, maxBytes, err := pageBounds(1, maxBytes)
	if err != nil {
		return out, err
	}
	err = s.transact(ctx, func(tx *sql.Tx) error {
		var err error
		out, err = readActivity(ctx, tx, conversationID, id, offset, maxBytes)
		return err
	})
	return out, err
}

func readActivity(ctx context.Context, tx *sql.Tx, conversationID, id string, offset int64, maxBytes int) (ActivityPart, error) {
	var part ActivityPart
	a := &part.Activity
	var created int64
	err := tx.QueryRowContext(ctx, `SELECT a.id,a.conversation_id,a.provider_thread_id,COALESCE(a.run_id,''),a.position,COALESCE(r.ordinal,0),a.kind,a.role,a.status,a.source,a.provider_item_id,a.byte_length+a.heading_length,a.created_at FROM activity a LEFT JOIN run r ON r.id=a.run_id WHERE a.conversation_id=? AND a.id=? AND a.position IS NOT NULL`, conversationID, id).Scan(&a.ID, &a.ConversationID, &a.ProviderThreadID, &a.RunID, &a.Position, &a.RunOrdinal, &a.Kind, &a.Role, &a.Status, &a.Source, &a.ProviderItemID, &a.ByteLength, &created)
	if err != nil {
		return part, storageError(ctx, err)
	}
	a.CreatedAt = time.Unix(0, created).UTC()
	a.Text, part.NextTextOffset, err = readText(ctx, tx, id, offset, maxBytes)
	if err != nil {
		return part, err
	}
	part.Truncated = part.NextTextOffset < a.ByteLength
	a.Attachments = []ActivityAttachment{}
	rows, err := tx.QueryContext(ctx, "SELECT name,mime_type,reference,size FROM activity_attachment WHERE activity_id=? ORDER BY position", id)
	if err != nil {
		return part, storageError(ctx, err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var v ActivityAttachment
		if err := rows.Scan(&v.Name, &v.MimeType, &v.Reference, &v.Size); err != nil {
			return part, storageError(ctx, err)
		}
		a.Attachments = append(a.Attachments, v)
	}
	projection := ProjectActivity(*a)
	part.Exportable = projection.Exportable
	part.ExclusionReason = projection.ExclusionReason
	if !part.Exportable {
		a.Text = ""
		a.Attachments = []ActivityAttachment{}
	}
	return part, storageError(ctx, rows.Err())
}
