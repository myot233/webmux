package conversation

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"time"
)

const injectionSchema = `
CREATE TABLE native_injection (
 delivery_id TEXT PRIMARY KEY,
 target_provider_thread_id TEXT NOT NULL REFERENCES provider_thread(id) ON DELETE CASCADE,
 native_thread_id TEXT NOT NULL,
 state TEXT NOT NULL DEFAULT 'pending',
 estimated_cost INTEGER NOT NULL,
 call_started INTEGER NOT NULL DEFAULT 0,
 updated_at INTEGER NOT NULL
);
CREATE TABLE native_injection_item (
 delivery_id TEXT NOT NULL REFERENCES native_injection(delivery_id) ON DELETE CASCADE,
 ordinal INTEGER NOT NULL, item_id TEXT NOT NULL,
 role TEXT NOT NULL, text_hash TEXT NOT NULL,
 provider_item_id TEXT NOT NULL DEFAULT '',
 PRIMARY KEY(delivery_id,ordinal)
);
CREATE INDEX native_injection_thread ON native_injection(target_provider_thread_id,native_thread_id);
CREATE INDEX native_injection_start ON native_injection_item(role,text_hash) WHERE ordinal=0;
CREATE INDEX native_injection_provider ON native_injection_item(provider_item_id) WHERE provider_item_id!='';
PRAGMA user_version=8;
`

type InjectionItem struct {
	ID, Role, TextHash, ProviderItemID string
}

type Injection struct {
	DeliveryID, NativeThreadID, State string
	CallStarted                       bool
	EstimatedCost                     int64
	Items                             []InjectionItem
}

func TextHash(text string) string {
	digest := sha256.Sum256([]byte(text))
	return hex.EncodeToString(digest[:])
}

func readInjection(ctx context.Context, tx *sql.Tx, deliveryID string) (Injection, error) {
	i := Injection{DeliveryID: deliveryID, Items: []InjectionItem{}}
	err := tx.QueryRowContext(ctx, "SELECT native_thread_id,state,call_started,estimated_cost FROM native_injection WHERE delivery_id=?", deliveryID).Scan(&i.NativeThreadID, &i.State, &i.CallStarted, &i.EstimatedCost)
	if err != nil {
		return i, storageError(ctx, err)
	}
	rows, err := tx.QueryContext(ctx, "SELECT item_id,role,text_hash,provider_item_id FROM native_injection_item WHERE delivery_id=? ORDER BY ordinal", deliveryID)
	if err != nil {
		return i, storageError(ctx, err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var item InjectionItem
		if err := rows.Scan(&item.ID, &item.Role, &item.TextHash, &item.ProviderItemID); err != nil {
			return i, storageError(ctx, err)
		}
		i.Items = append(i.Items, item)
	}
	if len(i.Items) == 0 {
		return i, ErrUnavailable
	}
	return i, storageError(ctx, rows.Err())
}

func (s *Service) Injection(ctx context.Context, deliveryID string) (Injection, error) {
	var i Injection
	err := s.transact(ctx, func(tx *sql.Tx) error { var err error; i, err = readInjection(ctx, tx, deliveryID); return err })
	return i, err
}

// ReserveInjection freezes the native identity and exact ordered digests before
// opening the mutation gate. A failed later user prompt never deletes this fact.
func (s *Service) ReserveInjection(ctx context.Context, executionID, nativeID string, items []InjectionItem) (Injection, error) {
	var out Injection
	if nativeID == "" || len(items) == 0 || len(items) > 10001 {
		return out, ErrInvalid
	}
	err := s.transact(ctx, func(tx *sql.Tx) error {
		e, err := readExecution(ctx, tx, executionID)
		if err != nil {
			return err
		}
		out, err = readInjection(ctx, tx, e.DeliveryID)
		if err == nil {
			if out.NativeThreadID != nativeID || len(out.Items) != len(items) {
				return ErrConflict
			}
			for n, item := range items {
				if item.ID != out.Items[n].ID || item.Role != out.Items[n].Role || item.TextHash != out.Items[n].TextHash {
					return ErrConflict
				}
			}
			return nil
		}
		if !errors.Is(err, ErrNotFound) {
			return err
		}
		if e.Status != "pending" || e.CallStarted {
			return ErrConflict
		}
		var preview Preview
		if err := readPreview(ctx, tx, e.HandoffID, &preview); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, "INSERT INTO native_injection(delivery_id,target_provider_thread_id,native_thread_id,estimated_cost,updated_at) VALUES(?,?,?,?,?)", e.DeliveryID, e.TargetProviderThreadID, nativeID, preview.Budget.Used, time.Now().UTC().UnixNano()); err != nil {
			return storageError(ctx, err)
		}
		seen := map[string]bool{}
		for n, item := range items {
			if item.ID == "" || len(item.ID) > 128 || seen[item.ID] || (item.Role != "user" && item.Role != "assistant") || len(item.TextHash) != 64 {
				return ErrInvalid
			}
			seen[item.ID] = true
			if _, err := tx.ExecContext(ctx, "INSERT INTO native_injection_item(delivery_id,ordinal,item_id,role,text_hash) VALUES(?,?,?,?,?)", e.DeliveryID, n, item.ID, item.Role, item.TextHash); err != nil {
				return storageError(ctx, err)
			}
		}
		out, err = readInjection(ctx, tx, e.DeliveryID)
		return err
	})
	return out, err
}

// ClaimInjection is one way. Process death after this point requires replay
// reconciliation; neither a new attempt nor an inline fallback can bypass it.
func (s *Service) ClaimInjection(ctx context.Context, deliveryID string) error {
	return s.transact(ctx, func(tx *sql.Tx) error {
		result, err := tx.ExecContext(ctx, "UPDATE native_injection SET call_started=1 WHERE delivery_id=? AND state='pending' AND call_started=0", deliveryID)
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

func finishInjection(ctx context.Context, tx *sql.Tx, deliveryID, state string) error {
	i, err := readInjection(ctx, tx, deliveryID)
	if err != nil {
		return err
	}
	if i.State == "injected" {
		return nil
	}
	if (!i.CallStarted && state != "unsupported") || (state != "injected" && state != "unsupported" && state != "unknown") {
		return ErrConflict
	}
	now := time.Now().UTC().UnixNano()
	if _, err := tx.ExecContext(ctx, "UPDATE native_injection SET state=?,updated_at=? WHERE delivery_id=?", state, now, deliveryID); err != nil {
		return storageError(ctx, err)
	}
	if state != "injected" {
		return nil
	}
	if _, err := tx.ExecContext(ctx, "UPDATE handoff_delivery SET status='injected',native_thread_id=?,error_class='',updated_at=? WHERE id=?", i.NativeThreadID, now, deliveryID); err != nil {
		return storageError(ctx, err)
	}
	if _, err := tx.ExecContext(ctx, "INSERT OR IGNORE INTO delivery_item SELECT ?,item_id FROM native_injection_item WHERE delivery_id=? AND ordinal>0", deliveryID, deliveryID); err != nil {
		return storageError(ctx, err)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE provider_thread SET last_delivered_run_ordinal=max(last_delivered_run_ordinal,(SELECT h.to_run_ordinal FROM context_handoff h JOIN handoff_delivery d ON d.handoff_id=h.id WHERE d.id=?)),last_delivered_position=max(last_delivered_position,(SELECT p.source_position FROM handoff_preview p JOIN handoff_delivery d ON d.handoff_id=p.handoff_id WHERE d.id=?)) WHERE id=(SELECT d.target_provider_thread_id FROM handoff_delivery d JOIN context_handoff h ON h.id=d.handoff_id WHERE d.id=? AND h.source_conversation_id=h.target_conversation_id)`, deliveryID, deliveryID, deliveryID); err != nil {
		return storageError(ctx, err)
	}
	return nil
}

func (s *Service) FinishInjection(ctx context.Context, deliveryID, state string) error {
	return s.transact(ctx, func(tx *sql.Tx) error { return finishInjection(ctx, tx, deliveryID, state) })
}

func (s *Service) HasInjections(ctx context.Context, thread, nativeID string) (bool, error) {
	if s.broken != nil {
		return false, s.broken
	}
	var found bool
	err := s.db.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM native_injection WHERE target_provider_thread_id=? AND native_thread_id=? AND call_started=1 AND state!='unsupported')", thread, nativeID).Scan(&found)
	return found, storageError(ctx, err)
}

// InjectionReplay finds only the matching bounded receipt; opening replay does
// not load every historical delivery or materialize historical message bodies.
func (s *Service) InjectionReplay(ctx context.Context, thread, nativeID, role, hash, providerID string) (Injection, bool, error) {
	var out Injection
	known := false
	err := s.transact(ctx, func(tx *sql.Tx) error {
		if providerID != "" {
			if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM native_injection_item m JOIN native_injection i ON i.delivery_id=m.delivery_id WHERE i.target_provider_thread_id=? AND i.native_thread_id=? AND i.call_started=1 AND i.state!='unsupported' AND m.provider_item_id=? AND m.role=? AND m.text_hash=?)`, thread, nativeID, providerID, role, hash).Scan(&known); err != nil {
				return storageError(ctx, err)
			}
		}
		var id string
		err := tx.QueryRowContext(ctx, `SELECT i.delivery_id FROM native_injection i JOIN native_injection_item m ON m.delivery_id=i.delivery_id WHERE i.target_provider_thread_id=? AND i.native_thread_id=? AND i.call_started=1 AND i.state!='unsupported' AND m.ordinal=0 AND m.role=? AND m.text_hash=? AND (m.provider_item_id='' OR m.provider_item_id=?) ORDER BY i.rowid DESC LIMIT 1`, thread, nativeID, role, hash, providerID).Scan(&id)
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		if err != nil {
			return storageError(ctx, err)
		}
		out, err = readInjection(ctx, tx, id)
		return err
	})
	return out, known, err
}

// ReconcileInjection requires the entire recorded sequence in the exact native
// thread. Neither a marker nor one matching message can confirm an unknown call.
func (s *Service) ReconcileInjection(ctx context.Context, deliveryID, nativeID string, observed []InjectionItem) error {
	return s.transact(ctx, func(tx *sql.Tx) error {
		i, err := readInjection(ctx, tx, deliveryID)
		if err != nil {
			return err
		}
		if i.NativeThreadID != nativeID || !i.CallStarted || len(i.Items) != len(observed) {
			return ErrConflict
		}
		for n, item := range observed {
			want := i.Items[n]
			if item.Role != want.Role || item.TextHash != want.TextHash || (want.ProviderItemID != "" && want.ProviderItemID != item.ProviderItemID) {
				return ErrConflict
			}
			if _, err := tx.ExecContext(ctx, "UPDATE native_injection_item SET provider_item_id=? WHERE delivery_id=? AND ordinal=?", item.ProviderItemID, deliveryID, n); err != nil {
				return storageError(ctx, err)
			}
		}
		if err := finishInjection(ctx, tx, deliveryID, "injected"); err != nil {
			return err
		}
		// Only history was sent. A subsequent explicit repetition of the user
		// command may continue its still-unsent prompt, without reinjecting.
		_, err = tx.ExecContext(ctx, "UPDATE handoff_execution SET status='pending',error_class='',phase='ready' WHERE delivery_id=? AND status='unknown' AND call_started=0", deliveryID)
		return storageError(ctx, err)
	})
}
