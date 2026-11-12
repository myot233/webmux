package conversation

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"slices"
	"time"
)

type replayItem struct {
	id, provider, kind, role string
	digest                   [32]byte
}

func replayItems(ctx context.Context, tx *sql.Tx, thread, run string, committed bool) ([]replayItem, error) {
	query := "SELECT id,provider_item_id,kind,role FROM activity WHERE provider_thread_id=? AND position IS NOT NULL AND kind IN ('message','command','tool','file_change','plan','compaction') ORDER BY position"
	args := []any{thread}
	if !committed {
		query = "SELECT id,provider_item_id,kind,role FROM activity WHERE run_id=? AND position IS NULL ORDER BY rowid"
		args = []any{run}
	}
	rows, err := tx.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, storageError(ctx, err)
	}
	var items []replayItem
	for rows.Next() {
		var item replayItem
		if err := rows.Scan(&item.id, &item.provider, &item.kind, &item.role); err != nil {
			_ = rows.Close()
			return nil, storageError(ctx, err)
		}
		items = append(items, item)
	}
	err = rows.Err()
	closeErr := rows.Close()
	if err != nil {
		return nil, storageError(ctx, err)
	}
	if closeErr != nil {
		return nil, storageError(ctx, closeErr)
	}
	for i := range items {
		rows, err := tx.QueryContext(ctx, "SELECT text FROM (SELECT 0 section,byte_offset,text FROM activity_heading_chunk WHERE activity_id=? UNION ALL SELECT 1,byte_offset,text FROM activity_chunk WHERE activity_id=?) ORDER BY section,byte_offset", items[i].id, items[i].id)
		if err != nil {
			return nil, storageError(ctx, err)
		}
		digest := sha256.New()
		for rows.Next() {
			var chunk []byte
			if err := rows.Scan(&chunk); err != nil {
				_ = rows.Close()
				return nil, storageError(ctx, err)
			}
			if _, err := digest.Write(chunk); err != nil {
				_ = rows.Close()
				return nil, storageError(ctx, err)
			}
		}
		err = rows.Err()
		closeErr := rows.Close()
		if err != nil {
			return nil, storageError(ctx, err)
		}
		if closeErr != nil {
			return nil, storageError(ctx, closeErr)
		}
		copy(items[i].digest[:], digest.Sum(nil))
	}
	return items, nil
}

// ReconcileReplay compares complete items, never independent chunk hashes. A
// provider ID is scoped to the thread, role and kind. ID-less messages match
// only the next item of the full ordered prefix, so legitimate repeated text
// remains distinct. A missing, reordered or unknown ID-less item leaves an
// explicit gap; it is never appended as another copy of old work.
func (s *Service) ReconcileReplay(ctx context.Context, thread, run string) error {
	return s.transact(ctx, func(tx *sql.Tx) error {
		var owner, status string
		if err := tx.QueryRowContext(ctx, "SELECT provider_thread_id,status FROM run WHERE id=?", run).Scan(&owner, &status); err != nil {
			return storageError(ctx, err)
		}
		if owner != thread {
			return ErrConflict
		}
		if status == "replay" {
			return nil
		}
		if status != "pending" {
			return ErrConflict
		}
		old, err := replayItems(ctx, tx, thread, run, true)
		if err != nil {
			return err
		}
		incoming, err := replayItems(ctx, tx, thread, run, false)
		if err != nil {
			return err
		}
		next := 0
		ambiguous := false
		for _, item := range incoming {
			matched := -1
			if item.provider != "" {
				matched = slices.IndexFunc(old, func(o replayItem) bool {
					return o.provider == item.provider && o.kind == item.kind && o.role == item.role
				})
				// A locally allocated prompt does not know the provider's later item ID.
				// Bind it only at the exact ordered prefix with identical normalized text.
				if matched < 0 && next < len(old) && old[next].provider == "" && item.kind == old[next].kind && item.role == old[next].role && item.digest == old[next].digest {
					matched = next
					if _, err := tx.ExecContext(ctx, "UPDATE activity SET provider_item_id=? WHERE id=?", item.provider, old[next].id); err != nil {
						return storageError(ctx, err)
					}
					old[next].provider = item.provider
				}
			} else if next < len(old) && item.kind == old[next].kind && item.role == old[next].role && item.digest == old[next].digest {
				matched = next
			}
			discard := false
			if matched >= 0 {
				if matched != next || item.digest != old[matched].digest {
					ambiguous = true
				}
				if matched == next {
					next++
				}
				discard = true
			} else if item.provider == "" || next < len(old) {
				// A changed ID or missing prefix cannot be treated as new work. Preserve
				// the original identity and require reconciliation rather than duplicating
				// a transcript whose provider IDs turned out to be unstable.
				if next < len(old) && item.kind == old[next].kind && item.role == old[next].role && item.digest == old[next].digest {
					next++
				}
				ambiguous = true
				discard = true
			}
			if discard {
				if _, err := tx.ExecContext(ctx, "DELETE FROM activity WHERE id=?", item.id); err != nil {
					return storageError(ctx, err)
				}
			}
		}
		if next < len(old) {
			ambiguous = true
		}
		if ambiguous {
			id, err := s.allocateID(ctx, tx, "history_gap")
			if err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO history_gap(id,conversation_id,provider_thread_id,reason,created_at) SELECT ?,conversation_id,id,'replay_ambiguous',? FROM provider_thread WHERE id=?`, id, time.Now().UTC().UnixNano(), thread); err != nil {
				return storageError(ctx, err)
			}
			if _, err := tx.ExecContext(ctx, "UPDATE conversation SET reconcile_required=1 WHERE id=(SELECT conversation_id FROM provider_thread WHERE id=?)", thread); err != nil {
				return storageError(ctx, err)
			}
		}
		// Stable-ID imports remain explicitly replay provenance; pre-upgrade history
		// retains unknown origin. An unprovable TUI gap is deliberately not erased.
		if _, err := tx.ExecContext(ctx, "UPDATE activity SET status='completed' WHERE run_id=? AND kind='message' AND position IS NULL", run); err != nil {
			return storageError(ctx, err)
		}
		if err := commitItems(ctx, tx, "run_id", run); err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, "UPDATE run SET status='replay',updated_at=? WHERE id=? AND provider_thread_id=?", time.Now().UTC().UnixNano(), run, thread)
		return storageError(ctx, err)
	})
}
