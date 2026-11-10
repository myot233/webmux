package conversation

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/rainchan/webmux/internal/pathutil"
	"github.com/rainchan/webmux/internal/statefile"

	_ "modernc.org/sqlite"
)

type Options struct{ StateDir string }

type Service struct {
	storage *statefile.Database
	db      *sql.DB
	broken  error
	// beforeCommit injects a crash/failure into tests, before SQLite publication.
	beforeCommit func() error
	id           func() (string, error)
}

// Open fails closed for conversations only. Callers may continue serving
// ordinary terminal sessions and inspect Unavailable without logging bodies.
func Open(o Options) *Service {
	s := &Service{id: newID}
	dir, err := pathutil.Expand(o.StateDir)
	if err == nil {
		s.storage, err = statefile.OpenDatabase(context.Background(), dir, statefile.Conversation)
	}
	if err == nil {
		s.db = s.storage.SQL
		err = migrate(context.Background(), s.db)
		if err == nil {
			err = s.recoverActivity(context.Background())
		}
		if err == nil {
			err = s.transact(context.Background(), func(tx *sql.Tx) error { return recoverExecutions(context.Background(), tx) })
		}
	}
	if err != nil {
		if s.storage != nil {
			_ = s.storage.Close()
			s.storage = nil
		}
		// SQL errors are not returned verbatim: constraints and drivers may include
		// user values. No prompt, config value or database page reaches application logs.
		s.broken = fmt.Errorf("%w: cannot open or migrate database", ErrUnavailable)
	}
	return s
}

func (s *Service) Unavailable() error { return s.broken }
func (s *Service) Close() error       { return s.storage.Close() }

func newID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}

func (s *Service) transact(ctx context.Context, f func(*sql.Tx) error) error {
	if s.broken != nil {
		return s.broken
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return storageError(ctx, err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := f(tx); err != nil {
		return err
	}
	if s.beforeCommit != nil {
		if err := s.beforeCommit(); err != nil {
			return err
		}
	}
	return storageError(ctx, tx.Commit())
}

func storageError(ctx context.Context, err error) error {
	if err == nil {
		return nil
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	return ErrUnavailable
}

// EnsureRuntime imports or creates a binding atomically. A unique runtime key
// makes migration safe across retries, concurrent calls and process restarts.
func (s *Service) EnsureRuntime(ctx context.Context, in RuntimeInput) (Binding, error) {
	var out Binding
	if in.RuntimeID == "" || in.Cwd == "" || in.AgentName == "" {
		return out, ErrInvalid
	}
	origin := in.HistoryOrigin
	if origin == "" {
		origin = "unknown"
	}
	if origin != "unknown" && origin != "complete" && origin != "bounded_import" {
		return out, ErrInvalid
	}
	err := s.transact(ctx, func(tx *sql.Tx) error {
		err := tx.QueryRowContext(ctx, "SELECT conversation_id,provider_thread_id FROM runtime_binding WHERE runtime_id=?", in.RuntimeID).Scan(&out.ConversationID, &out.ProviderThreadID)
		if err == nil {
			return nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return storageError(ctx, err)
		}
		out.ConversationID, err = s.allocateID(ctx, tx, "conversation")
		if err != nil {
			return err
		}
		out.ProviderThreadID, err = s.allocateID(ctx, tx, "provider_thread")
		if err != nil {
			return err
		}
		now := time.Now().UTC().UnixNano()
		created := in.CreatedAt.UnixNano()
		if in.CreatedAt.IsZero() {
			created = now
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO conversation(id,cwd,repository_id,worktree_id,active_provider_thread_id,history_origin,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?)`, out.ConversationID, in.Cwd, in.RepositoryID, in.WorktreeID, out.ProviderThreadID, origin, created, now)
		if err != nil {
			return storageError(ctx, err)
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO provider_thread(id,conversation_id,config_id,agent_name,agent_session_id,status,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?)`, out.ProviderThreadID, out.ConversationID, in.ConfigID, in.AgentName, in.AgentSessionID, in.Status, created, now)
		if err != nil {
			return storageError(ctx, err)
		}
		if err := putConfig(ctx, tx, out.ProviderThreadID, in.Config); err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, "INSERT INTO runtime_binding VALUES(?,?,?)", in.RuntimeID, out.ConversationID, out.ProviderThreadID)
		return storageError(ctx, err)
	})
	return out, err
}

func putConfig(ctx context.Context, tx *sql.Tx, id string, values map[string]string) error {
	if _, err := tx.ExecContext(ctx, "DELETE FROM provider_config WHERE provider_thread_id=?", id); err != nil {
		return storageError(ctx, err)
	}
	for name, value := range values {
		if _, err := tx.ExecContext(ctx, "INSERT INTO provider_config VALUES(?,?,?)", id, name, value); err != nil {
			return storageError(ctx, err)
		}
	}
	return nil
}

// SyncRuntime updates native identity and location without changing durable IDs.
// Releasing a worktree never calls ForgetRuntime; rebuilding may change cwd.
func (s *Service) SyncRuntime(ctx context.Context, in RuntimeInput) error {
	return s.transact(ctx, func(tx *sql.Tx) error {
		var b Binding
		if err := tx.QueryRowContext(ctx, "SELECT conversation_id,provider_thread_id FROM runtime_binding WHERE runtime_id=?", in.RuntimeID).Scan(&b.ConversationID, &b.ProviderThreadID); err != nil {
			return storageError(ctx, err)
		}
		var native string
		if err := tx.QueryRowContext(ctx, "SELECT agent_session_id FROM provider_thread WHERE id=?", b.ProviderThreadID).Scan(&native); err != nil {
			return storageError(ctx, err)
		}
		changed := false
		rows, err := tx.QueryContext(ctx, "SELECT name,value FROM provider_config WHERE provider_thread_id=?", b.ProviderThreadID)
		if err != nil {
			return storageError(ctx, err)
		}
		count := 0
		for rows.Next() {
			var key, value string
			if err := rows.Scan(&key, &value); err != nil {
				_ = rows.Close()
				return storageError(ctx, err)
			}
			count++
			if next, ok := in.Config[key]; !ok || next != value {
				changed = true
			}
		}
		err = rows.Err()
		closeErr := rows.Close()
		if err != nil {
			return storageError(ctx, err)
		}
		if closeErr != nil {
			return storageError(ctx, closeErr)
		}
		changed = changed || count != len(in.Config)
		if changed || native != in.AgentSessionID {
			if _, err := tx.ExecContext(ctx, "UPDATE provider_thread SET context_window=0,auto_compact_threshold=0 WHERE id=?", b.ProviderThreadID); err != nil {
				return storageError(ctx, err)
			}
		}
		if native != in.AgentSessionID {
			if _, err := tx.ExecContext(ctx, "UPDATE provider_thread SET context_usage=0,usage_measured=0 WHERE id=?", b.ProviderThreadID); err != nil {
				return storageError(ctx, err)
			}
		}
		now := time.Now().UTC().UnixNano()
		if _, err := tx.ExecContext(ctx, "UPDATE conversation SET cwd=?,repository_id=?,worktree_id=?,updated_at=? WHERE id=?", in.Cwd, in.RepositoryID, in.WorktreeID, now, b.ConversationID); err != nil {
			return storageError(ctx, err)
		}
		if _, err := tx.ExecContext(ctx, "UPDATE provider_thread SET agent_session_id=?,status=?,updated_at=? WHERE id=?", in.AgentSessionID, in.Status, now, b.ProviderThreadID); err != nil {
			return storageError(ctx, err)
		}
		return putConfig(ctx, tx, b.ProviderThreadID, in.Config)
	})
}

func (s *Service) Binding(ctx context.Context, runtimeID string) (Binding, error) {
	var b Binding
	if s.broken != nil {
		return b, s.broken
	}
	err := s.db.QueryRowContext(ctx, "SELECT conversation_id,provider_thread_id FROM runtime_binding WHERE runtime_id=?", runtimeID).Scan(&b.ConversationID, &b.ProviderThreadID)
	return b, storageError(ctx, err)
}

// ForgetRuntime cascades through identities, runs, handoffs and receipts. SQLite
// secure_delete overwrites deleted cells; rollback journals disappear at commit.
func (s *Service) ForgetRuntime(ctx context.Context, runtimeID string) error {
	return s.transact(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, "DELETE FROM conversation WHERE id IN (SELECT conversation_id FROM runtime_binding WHERE runtime_id=?)", runtimeID)
		return storageError(ctx, err)
	})
}

// PruneRuntimes reconciles a successfully read legacy record inventory. A failed
// JSON removal/SQL cleanup can therefore be retried after restart. Unbound fork
// conversations are not considered orphaned runtime records.
func (s *Service) PruneRuntimes(ctx context.Context, retained []string) error {
	return s.transact(ctx, func(tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, "SELECT runtime_id FROM runtime_binding WHERE runtime_id NOT IN (SELECT runtime_id FROM handoff_delivery WHERE runtime_id != '')")
		if err != nil {
			return storageError(ctx, err)
		}
		keep := make(map[string]bool, len(retained))
		for _, id := range retained {
			keep[id] = true
		}
		var drop []string
		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err != nil {
				_ = rows.Close()
				return storageError(ctx, err)
			}
			if !keep[id] {
				drop = append(drop, id)
			}
		}
		err = rows.Err()
		closeErr := rows.Close()
		if err != nil {
			return storageError(ctx, err)
		}
		if closeErr != nil {
			return storageError(ctx, closeErr)
		}
		for _, id := range drop {
			if _, err := tx.ExecContext(ctx, "DELETE FROM conversation WHERE id IN (SELECT conversation_id FROM runtime_binding WHERE runtime_id=?)", id); err != nil {
				return storageError(ctx, err)
			}
		}
		return nil
	})
}

// Table names are internal constants; caller-supplied IDs never enter SQL text.
func (s *Service) allocateID(ctx context.Context, tx *sql.Tx, table string) (string, error) {
	for range 8 {
		id, err := s.id()
		if err != nil {
			return "", ErrUnavailable
		}
		var found int
		err = tx.QueryRowContext(ctx, "SELECT 1 FROM "+table+" WHERE id=?", id).Scan(&found)
		if errors.Is(err, sql.ErrNoRows) {
			return id, nil
		}
		if err != nil {
			return "", storageError(ctx, err)
		}
	}
	return "", ErrConflict
}
