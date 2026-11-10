package conversation

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"time"
)

func encodePreviewRequest(in PreviewRequest) ([]byte, error) { return json.Marshal(in) }
func encodePreviewBudget(in PreviewBudget) ([]byte, error)   { return json.Marshal(in) }
func decodePreview(data []byte, out any) error {
	if len(data) > 2<<20 {
		return ErrUnavailable
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(out); err != nil {
		return ErrUnavailable
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return ErrUnavailable
	}
	return nil
}

// Receipt exposes only the durable outcome, never grants permission to repeat a
// provider mutation whose first outcome is pending or unknown.
func (s *Service) Receipt(ctx context.Context, conversationID, requestID string) (CommandReceipt, error) {
	var receipt CommandReceipt
	err := s.transact(ctx, func(tx *sql.Tx) error {
		var err error
		receipt, err = readReceipt(ctx, tx, conversationID, requestID)
		return err
	})
	return receipt, err
}

// ReservePreview allocates the handoff and dormant target in one transaction.
// No runtime, worktree, agent process or native writer is created here.
func (s *Service) ReservePreview(ctx context.Context, in PreviewReservation) (Preview, error) {
	var id string
	if in.ClientRequestID == "" || in.RequestHash == "" || in.Request.ConfigID == "" || in.Request.AgentName == "" {
		return Preview{}, ErrInvalid
	}
	err := s.transact(ctx, func(tx *sql.Tx) error {
		receipt, err := readReceipt(ctx, tx, in.SourceConversationID, in.ClientRequestID)
		if err == nil {
			if receipt.RequestHash != in.RequestHash {
				return ErrConflict
			}
			id = receipt.HandoffID
			return nil
		}
		if !errors.Is(err, ErrNotFound) {
			return err
		}
		var cwd, repo, worktree, origin string
		var nextPosition, nextRun int64
		var reconcile bool
		if err := tx.QueryRowContext(ctx, "SELECT cwd,repository_id,worktree_id,history_origin,reconcile_required,next_activity_position,next_run_ordinal FROM conversation WHERE id=?", in.SourceConversationID).Scan(&cwd, &repo, &worktree, &origin, &reconcile, &nextPosition, &nextRun); err != nil {
			return storageError(ctx, err)
		}
		targetConversation := in.SourceConversationID
		if in.Request.Fork && in.Request.TargetProviderThreadID != "" {
			return ErrInvalid
		}
		if in.Request.Location.Cwd == "" && in.Request.Location.NewWorktreeBranch == "" {
			in.Request.Location.Cwd = cwd
		}
		if in.Request.Location.RepositoryID == "" && in.Request.Location.NewWorktreeBranch == "" {
			in.Request.Location.RepositoryID = repo
			in.Request.Location.WorktreeID = worktree
		}
		now := time.Now().UTC().UnixNano()
		if in.Request.Fork {
			targetConversation, err = s.allocateID(ctx, tx, "conversation")
			if err != nil {
				return err
			}
			_, err = tx.ExecContext(ctx, "INSERT INTO conversation(id,parent_id,relationship,cwd,repository_id,worktree_id,history_origin,created_at,updated_at) VALUES(?,?,'fork',?,?,?,'complete',?,?)", targetConversation, in.SourceConversationID, in.Request.Location.Cwd, in.Request.Location.RepositoryID, in.Request.Location.WorktreeID, now, now)
			if err != nil {
				return storageError(ctx, err)
			}
		} else if in.Request.Location.Cwd != cwd || in.Request.Location.RepositoryID != repo || in.Request.Location.WorktreeID != worktree || in.Request.Location.NewWorktreeBranch != "" {
			return ErrConflict
		}
		target := in.Request.TargetProviderThreadID
		from := int64(1)
		seenPosition := int64(0)
		strategy := "full_thread_summary"
		if target == "" {
			target, err = s.allocateID(ctx, tx, "provider_thread")
			if err != nil {
				return err
			}
			_, err = tx.ExecContext(ctx, "INSERT INTO provider_thread(id,conversation_id,config_id,agent_name,status,created_at,updated_at) VALUES(?,?,?,?,'draft',?,?)", target, targetConversation, in.Request.ConfigID, in.Request.AgentName, now, now)
			if err != nil {
				return storageError(ctx, err)
			}
			if err := putConfig(ctx, tx, target, in.Request.AgentConfig); err != nil {
				return err
			}
			if in.Request.Fork {
				if _, err := tx.ExecContext(ctx, "UPDATE conversation SET active_provider_thread_id=? WHERE id=?", target, targetConversation); err != nil {
					return storageError(ctx, err)
				}
			}
		} else {
			var delivered int64
			if err := tx.QueryRowContext(ctx, "SELECT last_delivered_run_ordinal,last_delivered_position FROM provider_thread WHERE id=? AND conversation_id=? AND config_id=? AND agent_name=?", target, targetConversation, in.Request.ConfigID, in.Request.AgentName).Scan(&delivered, &seenPosition); err != nil {
				return storageError(ctx, err)
			}
			if delivered > 0 && !reconcile && origin == "complete" {
				from = delivered + 1
				strategy = "delta_since_target_last_seen"
				var earliest sql.NullInt64
				if err := tx.QueryRowContext(ctx, `SELECT min(r.ordinal) FROM activity a JOIN run r ON r.id=a.run_id WHERE a.conversation_id=? AND a.position>? AND a.position<=? AND NOT(a.provider_thread_id=? AND r.consumed=1)`, targetConversation, seenPosition, nextPosition-1, target).Scan(&earliest); err != nil {
					return storageError(ctx, err)
				}
				if earliest.Valid {
					from = min(from, earliest.Int64)
				}
			}
		}
		id, err = s.allocateID(ctx, tx, "context_handoff")
		if err != nil {
			return err
		}
		to := nextRun - 1
		if from > to+1 {
			from = to + 1
		}
		_, err = tx.ExecContext(ctx, "INSERT INTO context_handoff VALUES(?,?,?,?,?,?,?,?,?)", id, in.SourceConversationID, targetConversation, target, strategy, from, to, 1, now)
		if err != nil {
			return storageError(ctx, err)
		}
		request, err := encodePreviewRequest(in.Request)
		if err != nil {
			return err
		}
		if len(request) > 2<<20 {
			return ErrInvalid
		}
		budget, err := encodePreviewBudget(PreviewBudget{Source: "estimated"})
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, "INSERT INTO handoff_preview(handoff_id,source_position,source_run_ordinal,history_origin,reconcile_required,request,budget,target_seen_position) VALUES(?,?,?,?,?,?,?,?)", id, nextPosition-1, nextRun-1, origin, reconcile, request, budget, seenPosition)
		if err != nil {
			return storageError(ctx, err)
		}
		_, err = tx.ExecContext(ctx, "INSERT INTO command_receipt(conversation_id,client_request_id,request_hash,handoff_id,result,created_at) VALUES(?,?,?,?,'preview',?)", in.SourceConversationID, in.ClientRequestID, in.RequestHash, id, now)
		return storageError(ctx, err)
	})
	if err != nil {
		return Preview{}, err
	}
	return s.Preview(ctx, id)
}

func (s *Service) Preview(ctx context.Context, id string) (Preview, error) {
	var out Preview
	err := s.transact(ctx, func(tx *sql.Tx) error { return readPreview(ctx, tx, id, &out) })
	return out, err
}

func readPreview(ctx context.Context, tx *sql.Tx, id string, out *Preview) error {
	var request, budget []byte
	var manual sql.NullString
	var nextPosition, nextRun int64
	var currentOrigin string
	var currentReconcile bool
	err := tx.QueryRowContext(ctx, `SELECT h.id,h.source_conversation_id,h.target_conversation_id,h.target_provider_thread_id,h.strategy,h.from_run_ordinal,h.to_run_ordinal,h.revision,p.source_position,p.source_run_ordinal,p.history_origin,p.reconcile_required,p.request,p.budget,p.rendered_text,p.manual_text,p.state,p.target_seen_position,c.next_activity_position,c.next_run_ordinal,c.history_origin,c.reconcile_required FROM context_handoff h JOIN handoff_preview p ON p.handoff_id=h.id JOIN conversation c ON c.id=h.source_conversation_id WHERE h.id=?`, id).Scan(&out.ID, &out.SourceConversationID, &out.TargetConversationID, &out.TargetProviderThreadID, &out.Strategy, &out.FromRunOrdinal, &out.ToRunOrdinal, &out.Revision, &out.SourcePosition, &out.SourceRunOrdinal, &out.HistoryOrigin, &out.ReconcileRequired, &request, &budget, &out.RenderedText, &manual, &out.State, &out.TargetSeenPosition, &nextPosition, &nextRun, &currentOrigin, &currentReconcile)
	if err != nil {
		return storageError(ctx, err)
	}
	if err := decodePreview(request, &out.Request); err != nil {
		return err
	}
	if err := decodePreview(budget, &out.Budget); err != nil {
		return err
	}
	out.Stale = nextPosition-1 != out.SourcePosition || currentOrigin != out.HistoryOrigin || currentReconcile != out.ReconcileRequired
	if nextRun-1 != out.SourceRunOrdinal {
		var newWork bool
		if err := tx.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM run WHERE conversation_id=? AND ordinal>? AND status!='replay')", out.SourceConversationID, out.SourceRunOrdinal).Scan(&newWork); err != nil {
			return storageError(ctx, err)
		}
		out.Stale = out.Stale || newWork
	}
	if manual.Valid {
		out.ManualText = &manual.String
		out.RenderedText = manual.String
	}
	out.SelectedItemIDs = []string{}
	out.OmittedItemIDs = []string{}
	rows, err := tx.QueryContext(ctx, "SELECT item_id,selected FROM handoff_item WHERE handoff_id=? ORDER BY position", id)
	if err != nil {
		return storageError(ctx, err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var item string
		var selected bool
		if err := rows.Scan(&item, &selected); err != nil {
			return storageError(ctx, err)
		}
		if selected {
			out.SelectedItemIDs = append(out.SelectedItemIDs, item)
		} else {
			out.OmittedItemIDs = append(out.OmittedItemIDs, item)
		}
	}
	return storageError(ctx, rows.Err())
}

func (s *Service) CompletePreview(ctx context.Context, id string, result PreviewResult) (Preview, error) {
	err := s.transact(ctx, func(tx *sql.Tx) error {
		var preview Preview
		if err := readPreview(ctx, tx, id, &preview); err != nil {
			return err
		}
		if preview.State != "preparing" {
			return nil
		}
		state := "ready"
		if result.Budget.OverBudget {
			state = "over_budget"
		}
		budget, err := encodePreviewBudget(result.Budget)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, "UPDATE handoff_preview SET rendered_text=?,budget=?,state=? WHERE handoff_id=?", result.RenderedText, budget, state, id); err != nil {
			return storageError(ctx, err)
		}
		seen := map[string]bool{}
		for selected, items := range [][]string{result.OmittedItemIDs, result.SelectedItemIDs} {
			for _, item := range items {
				if item == "" || seen[item] {
					return ErrInvalid
				}
				seen[item] = true
				result, err := tx.ExecContext(ctx, "INSERT INTO handoff_item SELECT ?,id,?,position FROM activity WHERE id=? AND conversation_id=? AND position<=?", id, selected, item, preview.SourceConversationID, preview.SourcePosition)
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
			}
		}
		return nil
	})
	if err != nil {
		return Preview{}, err
	}
	return s.Preview(ctx, id)
}

func (s *Service) EditPreview(ctx context.Context, id string, revision int64, text string, budget PreviewBudget) (Preview, error) {
	if len(text) > 1<<20 {
		return Preview{}, ErrInvalid
	}
	err := s.transact(ctx, func(tx *sql.Tx) error {
		var p Preview
		if err := readPreview(ctx, tx, id, &p); err != nil {
			return err
		}
		if p.Revision != revision || p.State == "preparing" || p.State == "superseded" || p.State == "delivering" || p.State == "delivered" || p.State == "unknown" {
			return ErrConflict
		}
		encoded, err := encodePreviewBudget(budget)
		if err != nil {
			return err
		}
		state := "ready"
		if budget.OverBudget {
			state = "over_budget"
		}
		if _, err := tx.ExecContext(ctx, "UPDATE context_handoff SET revision=revision+1,strategy='manual_context' WHERE id=?", id); err != nil {
			return storageError(ctx, err)
		}
		if _, err := tx.ExecContext(ctx, "UPDATE handoff_preview SET manual_text=?,budget=?,state=? WHERE handoff_id=?", text, encoded, state, id); err != nil {
			return storageError(ctx, err)
		}
		// An edited free-form context no longer promises any original item was
		// reproduced intact; all source references remain recoverable as omitted.
		_, err = tx.ExecContext(ctx, "UPDATE handoff_item SET selected=0 WHERE handoff_id=?", id)
		return storageError(ctx, err)
	})
	if err != nil {
		return Preview{}, err
	}
	return s.Preview(ctx, id)
}
