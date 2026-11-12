package conversation

import (
	"context"
	"database/sql"
	"errors"
	"time"
	"unicode/utf8"
)

var activityKinds = map[string]bool{"message": true, "command": true, "tool": true, "error": true, "interrupt": true, "file_change": true, "plan": true, "compaction": true, "turn_start": true, "turn_end": true, "permission": true, "marker": true}

// WriteActivity records normalized text before any UI cap or lossy projection.
// Only IDs and normalized fields cross this boundary; no raw ACP or reasoning.
// A message delta writes one chunk, never the prefix accumulated so far.
func (s *Service) WriteActivity(ctx context.Context, in ActivityInput) (string, error) {
	if in.ProviderThreadID == "" || in.LocalKey == "" || len(in.LocalKey) > 8192 || len(in.ProviderItemID) > 4096 || !activityKinds[in.Kind] || (!utf8.ValidString(in.Text) || (in.Heading != nil && !utf8.ValidString(*in.Heading))) {
		return "", ErrInvalid
	}
	if in.Role != "" && in.Role != "user" && in.Role != "agent" && in.Role != "system" {
		return "", ErrInvalid
	}
	if in.Source != "live" && in.Source != "local" && in.Source != "replay" {
		return "", ErrInvalid
	}
	var id string
	err := s.transact(ctx, func(tx *sql.Tx) error {
		var bytes, jsonBytes int64
		var position sql.NullInt64
		err := tx.QueryRowContext(ctx, "SELECT id,byte_length,json_bytes,position FROM activity WHERE provider_thread_id=? AND local_key=?", in.ProviderThreadID, in.LocalKey).Scan(&id, &bytes, &jsonBytes, &position)
		if errors.Is(err, sql.ErrNoRows) {
			id, err = s.allocateID(ctx, tx, "activity")
			if err != nil {
				return err
			}
			var conversationID string
			if err := tx.QueryRowContext(ctx, "SELECT conversation_id FROM provider_thread WHERE id=?", in.ProviderThreadID).Scan(&conversationID); err != nil {
				return storageError(ctx, err)
			}
			var runID *string
			if in.RunID != "" {
				var owner, status string
				if err := tx.QueryRowContext(ctx, "SELECT provider_thread_id,status FROM run WHERE id=?", in.RunID).Scan(&owner, &status); err != nil {
					return storageError(ctx, err)
				}
				if owner != in.ProviderThreadID || status == "interrupted" {
					return ErrConflict
				}
				runID = &in.RunID
			}
			_, err = tx.ExecContext(ctx, `INSERT INTO activity(id,conversation_id,provider_thread_id,run_id,kind,role,status,source,provider_item_id,local_key,created_at) VALUES(?,?,?,?,?,?,?,?,?,?,?)`, id, conversationID, in.ProviderThreadID, runID, in.Kind, in.Role, in.Status, in.Source, in.ProviderItemID, in.LocalKey, time.Now().UTC().UnixNano())
			if err != nil {
				return storageError(ctx, err)
			}
		} else if err != nil {
			return storageError(ctx, err)
		}
		// Committed history cannot be mutated by later replay or stale connections.
		if position.Valid {
			return ErrConflict
		}
		if in.Replace {
			jsonBytes = escapedTextBytes(in.Text)
		} else {
			jsonBytes += escapedTextBytes(in.Text)
		}
		if in.Replace {
			prefix, err := snapshotPrefix(ctx, tx, "activity_chunk", id, in.Text)
			if err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx, "DELETE FROM activity_chunk WHERE activity_id=? AND byte_offset>=?", id, prefix); err != nil {
				return storageError(ctx, err)
			}
			bytes = prefix
			in.Text = in.Text[prefix:]
		}
		if in.Text != "" {
			for start := 0; start < len(in.Text); start += MaxActivityBytes {
				end := min(len(in.Text), start+MaxActivityBytes)
				if _, err := tx.ExecContext(ctx, "INSERT INTO activity_chunk VALUES(?,?,?,?)", id, bytes+int64(start), end-start, []byte(in.Text[start:end])); err != nil {
					return storageError(ctx, err)
				}
			}
		}
		if _, err := tx.ExecContext(ctx, "UPDATE activity SET byte_length=?,json_bytes=?,kind=?,status=CASE WHEN ?='' THEN status ELSE ? END WHERE id=?", bytes+int64(len(in.Text)), jsonBytes, in.Kind, in.Status, in.Status, id); err != nil {
			return storageError(ctx, err)
		}
		if in.Heading != nil {
			prefix, err := snapshotPrefix(ctx, tx, "activity_heading_chunk", id, *in.Heading)
			if err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx, "DELETE FROM activity_heading_chunk WHERE activity_id=? AND byte_offset>=?", id, prefix); err != nil {
				return storageError(ctx, err)
			}
			for start := int(prefix); start < len(*in.Heading); start += MaxActivityBytes {
				end := min(len(*in.Heading), start+MaxActivityBytes)
				if _, err := tx.ExecContext(ctx, "INSERT INTO activity_heading_chunk VALUES(?,?,?,?)", id, start, end-start, []byte((*in.Heading)[start:end])); err != nil {
					return storageError(ctx, err)
				}
			}
			if _, err := tx.ExecContext(ctx, "UPDATE activity SET heading_length=?,heading_json_bytes=? WHERE id=?", len(*in.Heading), escapedTextBytes(*in.Heading), id); err != nil {
				return storageError(ctx, err)
			}
		}
		if len(in.Attachments) > 0 {
			var start, metadataBytes int
			if err := tx.QueryRowContext(ctx, "SELECT count(*),COALESCE(sum(length(CAST(name AS BLOB))+length(CAST(mime_type AS BLOB))+length(CAST(reference AS BLOB))),0) FROM activity_attachment WHERE activity_id=?", id).Scan(&start, &metadataBytes); err != nil {
				return storageError(ctx, err)
			}
			if start+len(in.Attachments) > 64 {
				return ErrInvalid
			}
			for i, a := range in.Attachments {
				metadataBytes += len(a.Name) + len(a.MimeType) + len(a.Reference)
				if metadataBytes > 8192 {
					return ErrInvalid
				}
				if _, err := tx.ExecContext(ctx, "INSERT INTO activity_attachment VALUES(?,?,?,?,?,?)", id, start+i, a.Name, a.MimeType, a.Reference, a.Size); err != nil {
					return storageError(ctx, err)
				}
			}
		}
		if in.RunID != "" && (in.Status == "completed" || in.Status == "failed") {
			var status string
			if err := tx.QueryRowContext(ctx, "SELECT status FROM run WHERE id=?", in.RunID).Scan(&status); err != nil {
				return storageError(ctx, err)
			}
			if status != "pending" {
				return commitItems(ctx, tx, "id", id)
			}
		}
		if in.RunID == "" {
			return commitItems(ctx, tx, "id", id)
		}
		return nil
	})
	return id, err
}

// commitItems assigns visible positions only when drafts are complete. Readers
// can never advance a cursor past an invisible draft that later fills a hole.
func commitItems(ctx context.Context, tx *sql.Tx, column, id string) error {
	rows, err := tx.QueryContext(ctx, "SELECT id,conversation_id FROM activity WHERE "+column+"=? AND position IS NULL AND status NOT IN ('pending','in_progress') ORDER BY rowid", id)
	if err != nil {
		return storageError(ctx, err)
	}
	type entry struct{ id, conversation string }
	var items []entry
	for rows.Next() {
		var e entry
		if err := rows.Scan(&e.id, &e.conversation); err != nil {
			_ = rows.Close()
			return storageError(ctx, err)
		}
		items = append(items, e)
	}
	err = rows.Err()
	closeErr := rows.Close()
	if err != nil {
		return storageError(ctx, err)
	}
	if closeErr != nil {
		return storageError(ctx, closeErr)
	}
	for _, item := range items {
		var pos int64
		if err := tx.QueryRowContext(ctx, "UPDATE conversation SET next_activity_position=next_activity_position+1 WHERE id=? RETURNING next_activity_position-1", item.conversation).Scan(&pos); err != nil {
			return storageError(ctx, err)
		}
		if _, err := tx.ExecContext(ctx, "UPDATE activity SET position=? WHERE id=?", pos, item.id); err != nil {
			return storageError(ctx, err)
		}
	}
	return nil
}

func (s *Service) FinishRun(ctx context.Context, id, status string) error {
	if status == "" {
		return ErrInvalid
	}
	return s.transact(ctx, func(tx *sql.Tx) error {
		var previous string
		if err := tx.QueryRowContext(ctx, "SELECT status FROM run WHERE id=?", id).Scan(&previous); err != nil {
			return storageError(ctx, err)
		}
		if previous == status {
			return nil
		}
		if previous != "pending" {
			return ErrConflict
		}
		result, err := tx.ExecContext(ctx, "UPDATE run SET status=?,updated_at=? WHERE id=?", status, time.Now().UTC().UnixNano(), id)
		if err != nil {
			return storageError(ctx, err)
		}
		count, err := result.RowsAffected()
		if err != nil {
			return storageError(ctx, err)
		}
		if count == 0 {
			return ErrNotFound
		}
		activityStatus := "interrupted"
		if status == "end_turn" || status == "max_tokens" || status == "max_turn_requests" {
			activityStatus = "completed"
		}
		if _, err := tx.ExecContext(ctx, "UPDATE activity SET status=? WHERE run_id=? AND ((kind='message' AND status='in_progress') OR (kind='compaction' AND status IN ('','in_progress'))) AND position IS NULL", activityStatus, id); err != nil {
			return storageError(ctx, err)
		}
		return commitItems(ctx, tx, "run_id", id)
	})
}

// AddGap records uncertainty without deriving invented chat content from PTY
// bytes. Reconciliation must positively resolve a gap before delta is complete.
func (s *Service) AddGap(ctx context.Context, threadID, reason string) error {
	if reason != "tui" && reason != "replay_ambiguous" && reason != "crash" && reason != "storage_failure" && reason != "unconfirmed_prompt" {
		return ErrInvalid
	}
	return s.transact(ctx, func(tx *sql.Tx) error {
		id, err := s.allocateID(ctx, tx, "history_gap")
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `INSERT OR IGNORE INTO history_gap(id,conversation_id,provider_thread_id,reason,created_at) SELECT ?,conversation_id,id,?,? FROM provider_thread WHERE id=?`, id, reason, time.Now().UTC().UnixNano(), threadID)
		if err != nil {
			return storageError(ctx, err)
		}
		_, err = tx.ExecContext(ctx, "UPDATE conversation SET reconcile_required=1 WHERE id=(SELECT conversation_id FROM provider_thread WHERE id=?)", threadID)
		return storageError(ctx, err)
	})
}

// RecoverActivity keeps crash tails uncommitted, records the missing interval,
// and leaves committed positions untouched. It never launches a provider.
func (s *Service) recoverActivity(ctx context.Context) error {
	return s.transact(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO history_gap(id,conversation_id,provider_thread_id,reason,created_at) SELECT lower(hex(randomblob(16))),conversation_id,provider_thread_id,'crash',? FROM run WHERE status!='interrupted' AND EXISTS(SELECT 1 FROM activity WHERE run_id=run.id AND position IS NULL)`, time.Now().UTC().UnixNano())
		if err != nil {
			return storageError(ctx, err)
		}
		if _, err := tx.ExecContext(ctx, "UPDATE conversation SET reconcile_required=1 WHERE id IN (SELECT conversation_id FROM history_gap WHERE resolved=0)"); err != nil {
			return storageError(ctx, err)
		}
		_, err = tx.ExecContext(ctx, "UPDATE run SET status='interrupted' WHERE status!='interrupted' AND EXISTS(SELECT 1 FROM activity WHERE run_id=run.id AND position IS NULL)")
		return storageError(ctx, err)
	})
}

// ACP snapshot patches often repeat the entire output so far. Preserve every
// identical stored chunk and write only the changed suffix, keeping disk write
// amplification bounded by new content rather than accumulated output size.
func snapshotPrefix(ctx context.Context, tx *sql.Tx, table, id, text string) (int64, error) {
	rows, err := tx.QueryContext(ctx, "SELECT byte_offset,text FROM "+table+" WHERE activity_id=? ORDER BY byte_offset", id)
	if err != nil {
		return 0, storageError(ctx, err)
	}
	defer func() { _ = rows.Close() }()
	var prefix int64
	for rows.Next() {
		var offset int64
		var body []byte
		if err := rows.Scan(&offset, &body); err != nil {
			return 0, storageError(ctx, err)
		}
		end := offset + int64(len(body))
		if end > int64(len(text)) || string(body) != text[offset:end] {
			break
		}
		prefix = end
	}
	return prefix, storageError(ctx, rows.Err())
}

// DiscardDraftActivity removes a steering attempt the provider explicitly did
// not consume. Committed activity is never silently erased through this path.
func (s *Service) DiscardDraftActivity(ctx context.Context, thread, key string) error {
	return s.transact(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, "DELETE FROM activity WHERE provider_thread_id=? AND local_key=? AND position IS NULL", thread, key)
		return storageError(ctx, err)
	})
}
