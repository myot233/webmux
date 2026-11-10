package conversation

import (
	"context"
	"time"
)

func (s *Service) Conversation(ctx context.Context, id string) (Conversation, error) {
	var c Conversation
	if s.broken != nil {
		return c, s.broken
	}
	var parent *string
	var created, updated int64
	err := s.db.QueryRowContext(ctx, `SELECT id,parent_id,relationship,cwd,repository_id,worktree_id,active_provider_thread_id,next_run_ordinal,next_activity_position,history_origin,reconcile_required,created_at,updated_at FROM conversation WHERE id=?`, id).Scan(&c.ID, &parent, &c.Relationship, &c.Cwd, &c.RepositoryID, &c.WorktreeID, &c.ActiveProviderThreadID, &c.NextRunOrdinal, &c.NextActivityPosition, &c.HistoryOrigin, &c.ReconcileRequired, &created, &updated)
	if parent != nil {
		c.ParentID = *parent
	}
	c.CreatedAt, c.UpdatedAt = time.Unix(0, created).UTC(), time.Unix(0, updated).UTC()
	return c, storageError(ctx, err)
}

func (s *Service) ProviderThread(ctx context.Context, id string) (ProviderThread, error) {
	var p ProviderThread
	p.Config = map[string]string{}
	if s.broken != nil {
		return p, s.broken
	}
	var created, updated int64
	err := s.db.QueryRowContext(ctx, `SELECT id,conversation_id,config_id,agent_name,agent_session_id,status,last_delivered_run_ordinal,context_usage,context_window,auto_compact_threshold,usage_measured,created_at,updated_at FROM provider_thread WHERE id=?`, id).Scan(&p.ID, &p.ConversationID, &p.ConfigID, &p.AgentName, &p.AgentSessionID, &p.Status, &p.LastDeliveredRunOrdinal, &p.ContextUsage, &p.ContextWindow, &p.AutoCompactThreshold, &p.UsageMeasured, &created, &updated)
	if err != nil {
		return p, storageError(ctx, err)
	}
	rows, err := s.db.QueryContext(ctx, "SELECT name,value FROM provider_config WHERE provider_thread_id=?", id)
	if err != nil {
		return p, storageError(ctx, err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var name, value string
		if err := rows.Scan(&name, &value); err != nil {
			return p, storageError(ctx, err)
		}
		p.Config[name] = value
	}
	p.CreatedAt, p.UpdatedAt = time.Unix(0, created).UTC(), time.Unix(0, updated).UTC()
	return p, storageError(ctx, rows.Err())
}
