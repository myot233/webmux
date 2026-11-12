package hybrid

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/rainchan/webmux/internal/conversation"
	"github.com/rainchan/webmux/internal/session"
)

func TestLegacyConversationMigrationAndRetention(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	r := openRecords(dir)
	legacy := Record{ID: "abcdefgh", Agent: "codex", ConfigID: "config", Cwd: t.TempDir(), Cmd: []string{"codex", "--model", "unchanged"}, Side: SideACP, AgentSessionID: "native", State: StateClosed, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
	if _, err := r.add(legacy, nil); err != nil {
		t.Fatal(err)
	}
	r.close()
	db := conversation.Open(conversation.Options{StateDir: dir})
	defer func() { _ = db.Close() }()
	manager := session.NewManager(session.Options{})
	s := New(Options{StateDir: dir, Manager: manager, Conversations: db})
	if err := s.Unavailable(); err != nil {
		t.Fatal(err)
	}
	b, err := db.Binding(ctx, legacy.ID)
	if err != nil {
		t.Fatal(err)
	}
	actual, err := s.Record(legacy.ID)
	if err != nil || !reflect.DeepEqual(actual.Cmd, legacy.Cmd) || actual.AgentSessionID != "native" {
		t.Fatalf("legacy changed: %+v %v", actual, err)
	}
	c, err := db.Conversation(ctx, b.ConversationID)
	if err != nil || c.HistoryOrigin != "unknown" {
		t.Fatalf("fabricated history: %+v %v", c, err)
	}
	if len(manager.List()) != 0 {
		t.Fatal("migration launched runtime")
	}
	if err := s.MarkReleased(ctx, []string{legacy.ID}, Release{ID: "release", RepositoryID: "repo", Path: legacy.Cwd, Commit: "commit", ReleasedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	if after, err := db.Binding(ctx, legacy.ID); err != nil || after != b {
		t.Fatal("release deleted identity", err)
	}
	if _, err := s.ClearReleased(ctx, "release"); err != nil {
		t.Fatal(err)
	}
	s.Close()
	s = New(Options{StateDir: dir, Manager: manager, Conversations: db})
	defer s.Close()
	if after, err := db.Binding(ctx, legacy.ID); err != nil || after != b {
		t.Fatal("restart changed identity", err)
	}
	if err := s.RemoveRecord(ctx, legacy.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Conversation(ctx, b.ConversationID); !errors.Is(err, conversation.ErrNotFound) {
		t.Fatalf("forget retained identity: %v", err)
	}
}

func TestBrokenConversationDatabaseLeavesOrdinaryTerminalsAvailable(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "conversations.sqlite"), []byte("corrupt"), 0600); err != nil {
		t.Fatal(err)
	}
	db := conversation.Open(conversation.Options{StateDir: dir})
	defer func() { _ = db.Close() }()
	manager := session.NewManager(session.Options{DefaultCwd: t.TempDir()})
	s := New(Options{StateDir: dir, Manager: manager, Conversations: db})
	defer s.Close()
	if s.Unavailable() == nil {
		t.Fatal("corrupt database accepted")
	}
	shell, err := manager.Create(session.CreateInput{Cmd: []string{"/bin/sh", "-c", "exit 0"}})
	if err != nil {
		t.Fatal("ordinary terminal disabled", err)
	}
	defer func() { _ = manager.Remove(shell.Info().ID) }()
}

func TestEvictionAlsoForgetsConversation(t *testing.T) {
	h := &harness{t: t, state: t.TempDir(), store: t.TempDir(), logs: &syncBuffer{}}
	r := openRecords(h.state)
	seeded := make([]Record, 0, MaxRecords)
	for i := range MaxRecords {
		rec := testRecord(limitID(i))
		rec.State = StateClosed
		rec.UpdatedAt = time.Unix(int64(i), 0)
		seeded = append(seeded, rec)
	}
	seedRecords(t, r, seeded)
	r.close()
	db := conversation.Open(conversation.Options{StateDir: h.state})
	h.open(0)
	h.svc.conversations = db
	t.Cleanup(func() { h.svc.Close(); _ = db.Close() })
	if err := h.svc.migrateConversations(); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	old, err := db.Binding(ctx, limitID(0))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.svc.Create(ctx, h.input("omp", SideTUI)); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Conversation(ctx, old.ConversationID); !errors.Is(err, conversation.ErrNotFound) {
		t.Fatalf("eviction retained conversation: %v", err)
	}
}
