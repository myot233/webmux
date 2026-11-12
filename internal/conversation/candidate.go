package conversation

import (
	"context"
	"database/sql"
)

// Candidate contains only bounded metadata and the precomputed text cost. A
// selector can inspect ten thousand items without loading their full bodies.
type Candidate struct {
	Message               HistoricalMessage
	ByteLength, JSONBytes int64
	Consumed              bool
}

func (s *Service) Candidates(ctx context.Context, id string, sourcePoint, afterPosition int64) ([]Candidate, error) {
	var out []Candidate
	err := s.transact(ctx, func(tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, `SELECT a.id,a.conversation_id,a.provider_thread_id,COALESCE(a.run_id,''),a.position,COALESCE(r.ordinal,0),a.kind,a.role,a.status,a.source,a.byte_length+a.heading_length,a.json_bytes+a.heading_json_bytes,COALESCE(r.consumed,0) FROM activity a LEFT JOIN run r ON r.id=a.run_id WHERE a.conversation_id=? AND a.position IS NOT NULL AND a.position<=? AND a.position>? ORDER BY a.position`, id, sourcePoint, afterPosition)
		if err != nil {
			return storageError(ctx, err)
		}
		defer func() { _ = rows.Close() }()
		for rows.Next() {
			var c Candidate
			m := &c.Message
			if err := rows.Scan(&m.ActivityID, &m.ConversationID, &m.ProviderThreadID, &m.RunID, &m.Position, &m.RunOrdinal, &m.Kind, &m.Role, &m.Status, &m.Source, &c.ByteLength, &c.JSONBytes, &c.Consumed); err != nil {
				return storageError(ctx, err)
			}
			p := ProjectActivity(Activity{Kind: m.Kind, Role: m.Role})
			m.Role = p.Role
			m.Exportable = p.Exportable
			m.ExclusionReason = p.ExclusionReason
			if m.Exportable {
				out = append(out, c)
			}
		}
		return storageError(ctx, rows.Err())
	})
	return out, err
}

// ObserveUsage accepts telemetry only for the exact native identity currently
// associated with this provider thread. Late results cannot revive old usage.
func (s *Service) ObserveUsage(ctx context.Context, thread, nativeID string, used, window int64) error {
	if used < 0 || window < 0 || used > 1<<40 || window > 1<<40 || nativeID == "" {
		return ErrInvalid
	}
	return s.transact(ctx, func(tx *sql.Tx) error {
		result, err := tx.ExecContext(ctx, "UPDATE provider_thread SET context_usage=?,context_window=?,usage_measured=1 WHERE id=? AND agent_session_id=?", used, window, thread, nativeID)
		if err != nil {
			return storageError(ctx, err)
		}
		n, err := result.RowsAffected()
		if err != nil {
			return storageError(ctx, err)
		}
		if n == 0 {
			return ErrConflict
		}
		return nil
	})
}

// RuntimeIDs supplies only bindings of this Conversation for attachment scope.
func (s *Service) RuntimeIDs(ctx context.Context, id string) ([]string, error) {
	out := []string{}
	err := s.transact(ctx, func(tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, "SELECT runtime_id FROM runtime_binding WHERE conversation_id=?", id)
		if err != nil {
			return storageError(ctx, err)
		}
		defer func() { _ = rows.Close() }()
		for rows.Next() {
			var runtime string
			if err := rows.Scan(&runtime); err != nil {
				return storageError(ctx, err)
			}
			out = append(out, runtime)
		}
		return storageError(ctx, rows.Err())
	})
	return out, err
}

// EstimatedNativeUsage counts saved visible work and already delivered history
// when provider telemetry is absent. It is an estimate: private reasoning and
// unobserved TUI intervals remain represented by explicit gaps, not invented text.
func (s *Service) EstimatedNativeUsage(ctx context.Context, thread string) (int64, error) {
	var total int64
	err := s.transact(ctx, func(tx *sql.Tx) error {
		if err := tx.QueryRowContext(ctx, "SELECT COALESCE(sum(json_bytes+heading_json_bytes+256),0) FROM activity WHERE provider_thread_id=? AND position IS NOT NULL", thread).Scan(&total); err != nil {
			return storageError(ctx, err)
		}
		var delivered int64
		if err := tx.QueryRowContext(ctx, `SELECT COALESCE(sum(CAST(json_extract(CAST(p.budget AS TEXT),'$.Used') AS INTEGER)),0) FROM handoff_delivery d JOIN handoff_preview p ON p.handoff_id=d.handoff_id WHERE d.target_provider_thread_id=? AND d.status IN ('inline','injected')`, thread).Scan(&delivered); err != nil {
			return storageError(ctx, err)
		}
		total += delivered
		return nil
	})
	return total, err
}
