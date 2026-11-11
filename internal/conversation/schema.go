package conversation

import (
	"context"
	"database/sql"
	"fmt"
)

const schemaVersion = 10

const schema = `
CREATE TABLE conversation (
 id TEXT PRIMARY KEY, parent_id TEXT REFERENCES conversation(id) ON DELETE SET NULL,
 relationship TEXT NOT NULL DEFAULT '', cwd TEXT NOT NULL,
 repository_id TEXT NOT NULL DEFAULT '', worktree_id TEXT NOT NULL DEFAULT '',
 active_provider_thread_id TEXT NOT NULL DEFAULT '',
 next_run_ordinal INTEGER NOT NULL DEFAULT 1 CHECK(next_run_ordinal > 0),
 history_origin TEXT NOT NULL CHECK(history_origin IN ('unknown','bounded_import','complete')),
 created_at INTEGER NOT NULL, updated_at INTEGER NOT NULL
);
CREATE TABLE provider_thread (
 id TEXT PRIMARY KEY, conversation_id TEXT NOT NULL REFERENCES conversation(id) ON DELETE CASCADE,
 config_id TEXT NOT NULL, agent_name TEXT NOT NULL, agent_session_id TEXT NOT NULL DEFAULT '',
 status TEXT NOT NULL, last_delivered_run_ordinal INTEGER NOT NULL DEFAULT 0,
 context_usage INTEGER NOT NULL DEFAULT 0, context_window INTEGER NOT NULL DEFAULT 0,
 auto_compact_threshold INTEGER NOT NULL DEFAULT 0,
 created_at INTEGER NOT NULL, updated_at INTEGER NOT NULL,
 UNIQUE(id, conversation_id)
);
CREATE INDEX provider_conversation ON provider_thread(conversation_id);
CREATE TABLE provider_config (
 provider_thread_id TEXT NOT NULL REFERENCES provider_thread(id) ON DELETE CASCADE,
 name TEXT NOT NULL, value TEXT NOT NULL, PRIMARY KEY(provider_thread_id, name)
);
CREATE TABLE runtime_binding (
 runtime_id TEXT PRIMARY KEY, conversation_id TEXT NOT NULL REFERENCES conversation(id) ON DELETE CASCADE,
 provider_thread_id TEXT NOT NULL,
 FOREIGN KEY(provider_thread_id,conversation_id) REFERENCES provider_thread(id,conversation_id) ON DELETE CASCADE
);
CREATE TABLE run (
 id TEXT PRIMARY KEY, conversation_id TEXT NOT NULL REFERENCES conversation(id) ON DELETE CASCADE,
 ordinal INTEGER NOT NULL CHECK(ordinal > 0), provider_thread_id TEXT NOT NULL,
 prompt_id TEXT NOT NULL, status TEXT NOT NULL, created_at INTEGER NOT NULL, updated_at INTEGER NOT NULL,
 UNIQUE(conversation_id, ordinal), UNIQUE(provider_thread_id, prompt_id),
 FOREIGN KEY(provider_thread_id,conversation_id) REFERENCES provider_thread(id,conversation_id) ON DELETE CASCADE
);
CREATE TABLE context_handoff (
 id TEXT PRIMARY KEY, source_conversation_id TEXT NOT NULL REFERENCES conversation(id) ON DELETE CASCADE,
 target_conversation_id TEXT NOT NULL REFERENCES conversation(id) ON DELETE CASCADE,
 target_provider_thread_id TEXT NOT NULL,
 strategy TEXT NOT NULL CHECK(strategy IN ('full_thread_summary','delta_since_target_last_seen','manual_context')),
 from_run_ordinal INTEGER NOT NULL, to_run_ordinal INTEGER NOT NULL,
 revision INTEGER NOT NULL CHECK(revision > 0), created_at INTEGER NOT NULL,
 FOREIGN KEY(target_provider_thread_id,target_conversation_id) REFERENCES provider_thread(id,conversation_id) ON DELETE CASCADE
);
CREATE TABLE handoff_item (
 handoff_id TEXT NOT NULL REFERENCES context_handoff(id) ON DELETE CASCADE,
 item_id TEXT NOT NULL, selected INTEGER NOT NULL, position INTEGER NOT NULL,
 PRIMARY KEY(handoff_id,item_id)
);
CREATE TABLE handoff_delivery (
 id TEXT PRIMARY KEY, handoff_id TEXT NOT NULL UNIQUE REFERENCES context_handoff(id) ON DELETE CASCADE,
 target_provider_thread_id TEXT NOT NULL REFERENCES provider_thread(id) ON DELETE CASCADE,
 native_thread_id TEXT NOT NULL DEFAULT '', status TEXT NOT NULL
 CHECK(status IN ('pending','injected','inline','unknown','failed')),
 error_class TEXT NOT NULL DEFAULT '', updated_at INTEGER NOT NULL
);
CREATE TABLE delivery_item (
 delivery_id TEXT NOT NULL REFERENCES handoff_delivery(id) ON DELETE CASCADE,
 item_id TEXT NOT NULL, PRIMARY KEY(delivery_id,item_id)
);
CREATE TABLE command_receipt (
 conversation_id TEXT NOT NULL REFERENCES conversation(id) ON DELETE CASCADE,
 client_request_id TEXT NOT NULL, request_hash TEXT NOT NULL,
 handoff_id TEXT REFERENCES context_handoff(id) ON DELETE CASCADE,
 delivery_id TEXT REFERENCES handoff_delivery(id) ON DELETE CASCADE,
 runtime_id TEXT NOT NULL DEFAULT '', result TEXT NOT NULL DEFAULT '', error_class TEXT NOT NULL DEFAULT '',
 created_at INTEGER NOT NULL, PRIMARY KEY(conversation_id,client_request_id)
);
PRAGMA user_version = 1;
`

func migrate(ctx context.Context, db *sql.DB) error {
	var version int
	if err := db.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil {
		return err
	}
	if version > schemaVersion {
		return fmt.Errorf("unsupported conversation schema version %d", version)
	}
	if version == schemaVersion {
		var result string
		if err := db.QueryRowContext(ctx, "PRAGMA quick_check").Scan(&result); err != nil {
			return err
		}
		if result != "ok" {
			return ErrUnavailable
		}
		// Fail on malformed current-version databases too.
		for _, table := range []string{"conversation", "provider_thread", "run", "context_handoff", "handoff_delivery", "command_receipt", "activity", "activity_chunk", "activity_heading_chunk", "activity_attachment", "history_gap", "handoff_preview", "handoff_execution", "handoff_attachment", "inline_receipt", "native_injection", "native_injection_item", "dispatch_receipt"} {
			if _, err := db.ExecContext(ctx, "SELECT 1 FROM "+table+" LIMIT 0"); err != nil {
				return err
			}
		}
		var triggers int
		if err := db.QueryRowContext(ctx, "SELECT count(*) FROM sqlite_schema WHERE type='trigger' AND name IN ('dispatch_receipt_insert','dispatch_receipt_update','dispatch_receipt_delete','dispatch_receipt_history')").Scan(&triggers); err != nil || triggers != 4 {
			return ErrUnavailable
		}
		return nil
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if version < 1 {
		if _, err := tx.ExecContext(ctx, schema); err != nil {
			return err
		}
	}
	if version < 2 {
		if _, err := tx.ExecContext(ctx, activitySchema); err != nil {
			return err
		}
	}
	if version < 3 {
		if _, err := tx.ExecContext(ctx, previewSchema+budgetSchema); err != nil {
			return err
		}
		if err := backfillJSONCosts(ctx, tx); err != nil {
			return err
		}
	}
	if version < 4 {
		if _, err := tx.ExecContext(ctx, executionSchema); err != nil {
			return err
		}
	}
	if version < 5 {
		if _, err := tx.ExecContext(ctx, attachmentSchema); err != nil {
			return err
		}
	}
	if version < 6 {
		if _, err := tx.ExecContext(ctx, inlineReceiptSchema); err != nil {
			return err
		}
	}
	if version < 7 {
		if _, err := tx.ExecContext(ctx, dispatchSchema); err != nil {
			return err
		}
	}
	if version < 8 {
		if _, err := tx.ExecContext(ctx, injectionSchema); err != nil {
			return err
		}
	}
	if version < 9 {
		if _, err := tx.ExecContext(ctx, `CREATE INDEX handoff_source_created ON context_handoff(source_conversation_id,created_at DESC); CREATE INDEX receipt_execution ON command_receipt(execution_id); PRAGMA user_version=9;`); err != nil {
			return err
		}
	}
	if version < 10 {
		if _, err := tx.ExecContext(ctx, dispatchReceiptSchema); err != nil {
			return err
		}
	}
	if version < 6 {
		if err := backfillInlineReceipts(ctx, tx); err != nil {
			return err
		}
	}
	if version < 10 {
		if err := backfillOrphanDispatch(ctx, tx); err != nil {
			return err
		}
	}
	return tx.Commit()
}
