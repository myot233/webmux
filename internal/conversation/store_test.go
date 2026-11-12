package conversation

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
)

func openTest(t *testing.T, dir string) *Service {
	t.Helper()
	s := Open(Options{StateDir: dir})
	if err := s.Unavailable(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := s.Close(); err != nil {
			t.Error(err)
		}
	})
	return s
}

func bindTest(t *testing.T, s *Service, id string) Binding {
	t.Helper()
	b, err := s.EnsureRuntime(context.Background(), RuntimeInput{RuntimeID: id, Cwd: "/workspace", AgentName: "codex", ConfigID: "cfg", AgentSessionID: "native-id", Status: "closed", Config: map[string]string{"model": "test"}})
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestIdentityAndOrdinalsSurviveConcurrencyAndRestart(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	s := openTest(t, dir)
	b := bindTest(t, s, "runtime")
	const count = 32
	results := make(chan Run, count)
	failures := make(chan error, count)
	var wg sync.WaitGroup
	for i := range count {
		wg.Go(func() {
			r, err := s.StartRun(ctx, b.ProviderThreadID, fmt.Sprintf("prompt-%d", i))
			if err != nil {
				failures <- err
			}
			results <- r
		})
	}
	wg.Wait()
	close(results)
	close(failures)
	for err := range failures {
		t.Error(err)
	}
	ordinals := map[int64]bool{}
	ids := map[string]bool{}
	for r := range results {
		if r.Ordinal < 1 || r.Ordinal > count || ordinals[r.Ordinal] || ids[r.ID] {
			t.Fatalf("duplicate/invalid run: %+v", r)
		}
		ordinals[r.Ordinal] = true
		ids[r.ID] = true
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s = openTest(t, dir)
	if again := bindTest(t, s, "runtime"); again != b {
		t.Fatalf("identity changed: %+v %+v", b, again)
	}
	r, err := s.StartRun(ctx, b.ProviderThreadID, "prompt-0")
	if err != nil || !ids[r.ID] {
		t.Fatalf("retry duplicated run: %+v %v", r, err)
	}
	r, err = s.StartRun(ctx, b.ProviderThreadID, "next")
	if err != nil || r.Ordinal != count+1 {
		t.Fatalf("ordinal reused: %+v %v", r, err)
	}
	c, err := s.Conversation(ctx, b.ConversationID)
	if err != nil || c.HistoryOrigin != "unknown" || c.NextRunOrdinal != count+2 {
		t.Fatalf("conversation: %+v %v", c, err)
	}
	p, err := s.ProviderThread(ctx, b.ProviderThreadID)
	if err != nil || p.Config["model"] != "test" || p.AgentSessionID != "native-id" {
		t.Fatalf("thread: %+v %v", p, err)
	}
}

func TestConcurrentMigrationCreatesOneIdentity(t *testing.T) {
	s := openTest(t, t.TempDir())
	var wg sync.WaitGroup
	bindings := make(chan Binding, 16)
	for range 16 {
		wg.Go(func() { bindings <- bindTest(t, s, "legacy") })
	}
	wg.Wait()
	close(bindings)
	var first Binding
	for b := range bindings {
		if first.ConversationID == "" {
			first = b
		}
		if b != first {
			t.Fatalf("duplicate: %+v %+v", first, b)
		}
	}
}

func handoffInput(source, target Binding) HandoffInput {
	return HandoffInput{SourceConversationID: source.ConversationID, TargetConversationID: target.ConversationID, TargetProviderThreadID: target.ProviderThreadID, Strategy: "full_thread_summary", FromRunOrdinal: 1, ToRunOrdinal: 2, SelectedItemIDs: []string{"a", "b"}, OmittedItemIDs: []string{"c"}, ClientRequestID: "request", RequestHash: "hash"}
}

func TestHandoffAtomicityAndDurableReceipts(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	s := openTest(t, dir)
	source, target := bindTest(t, s, "source"), bindTest(t, s, "target")
	in := handoffInput(source, target)
	injected := errors.New("injected crash")
	s.beforeCommit = func() error { return injected }
	if _, err := s.CommitHandoff(ctx, in); !errors.Is(err, injected) {
		t.Fatal(err)
	}
	for _, table := range []string{"context_handoff", "handoff_item", "handoff_delivery", "command_receipt"} {
		var count int
		if err := s.db.QueryRow("SELECT count(*) FROM " + table).Scan(&count); err != nil || count != 0 {
			t.Fatalf("partial %s: %d %v", table, count, err)
		}
	}
	s.beforeCommit = nil
	r, err := s.CommitHandoff(ctx, in)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s = openTest(t, dir)
	again, err := s.CommitHandoff(ctx, in)
	if err != nil || !reflect.DeepEqual(r, again) {
		t.Fatalf("receipt changed: %+v %+v %v", r, again, err)
	}
	in.RequestHash = "different"
	if _, err := s.CommitHandoff(ctx, in); !errors.Is(err, ErrConflict) {
		t.Fatalf("payload conflict: %v", err)
	}
	// A receipt cannot outlive history the user explicitly forgot, including
	// when the forgotten conversation was the source of another thread's handoff.
	if err := s.ForgetRuntime(ctx, "source"); err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"context_handoff", "handoff_item", "handoff_delivery", "command_receipt"} {
		var count int
		if err := s.db.QueryRow("SELECT count(*) FROM " + table).Scan(&count); err != nil || count != 0 {
			t.Fatalf("retained %s: %d %v", table, count, err)
		}
	}
	if _, err := s.Conversation(ctx, target.ConversationID); err != nil {
		t.Fatal("deleted target identity", err)
	}
}

func TestRunRollbackAndThreadOwnership(t *testing.T) {
	ctx := context.Background()
	s := openTest(t, t.TempDir())
	source, target := bindTest(t, s, "source"), bindTest(t, s, "target")
	s.beforeCommit = func() error { return errors.New("crash") }
	if _, err := s.StartRun(ctx, source.ProviderThreadID, "p"); err == nil {
		t.Fatal("committed crash")
	}
	s.beforeCommit = nil
	r, err := s.StartRun(ctx, source.ProviderThreadID, "p")
	if err != nil || r.Ordinal != 1 {
		t.Fatalf("partial ordinal: %+v %v", r, err)
	}
	in := handoffInput(source, target)
	in.TargetProviderThreadID = source.ProviderThreadID
	if _, err := s.CommitHandoff(ctx, in); err == nil {
		t.Fatal("cross-conversation thread accepted")
	}
}

func TestMetadataSyncAndOrphanPruning(t *testing.T) {
	ctx := context.Background()
	s := openTest(t, t.TempDir())
	a, b := bindTest(t, s, "a"), bindTest(t, s, "b")
	if err := s.SyncRuntime(ctx, RuntimeInput{RuntimeID: "a", Cwd: "/rebuilt", RepositoryID: "repo", WorktreeID: "new", AgentSessionID: "native-id", Status: "interrupted", Config: map[string]string{"model": "next"}}); err != nil {
		t.Fatal(err)
	}
	if again, err := s.Binding(ctx, "a"); err != nil || again != a {
		t.Fatalf("rebuild changed ID: %+v %v", again, err)
	}
	c, err := s.Conversation(ctx, a.ConversationID)
	if err != nil || c.Cwd != "/rebuilt" {
		t.Fatalf("location: %+v %v", c, err)
	}
	if err := s.PruneRuntimes(ctx, []string{"a"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Conversation(ctx, b.ConversationID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("orphan retained: %v", err)
	}
}

func TestStoreVersionsCorruptionAndLockIsolation(t *testing.T) {
	for _, mode := range []string{"empty", "current", "future", "corrupt", "incomplete"} {
		t.Run(mode, func(t *testing.T) {
			ctx := context.Background()
			dir := t.TempDir()
			path := filepath.Join(dir, "conversations.sqlite")
			switch mode {
			case "current":
				s := openTest(t, dir)
				bindTest(t, s, "legacy")
				if err := s.Close(); err != nil {
					t.Fatal(err)
				}
			case "future", "incomplete":
				db, err := sql.Open("sqlite", path)
				if err != nil {
					t.Fatal(err)
				}
				version := 1
				if mode == "future" {
					version = 999
				}
				if _, err := db.Exec(fmt.Sprintf("PRAGMA user_version=%d", version)); err != nil {
					t.Fatal(err)
				}
				if err := db.Close(); err != nil {
					t.Fatal(err)
				}
			case "corrupt":
				if err := os.WriteFile(path, []byte("damaged"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			before, _ := os.ReadFile(path)
			s := Open(Options{StateDir: dir})
			defer func() { _ = s.Close() }()
			if mode == "future" || mode == "corrupt" || mode == "incomplete" {
				if !errors.Is(s.Unavailable(), ErrUnavailable) {
					t.Fatal("invalid store accepted")
				}
				after, _ := os.ReadFile(path)
				if !reflect.DeepEqual(before, after) {
					t.Fatal("invalid database modified")
				}
				if _, err := s.EnsureRuntime(ctx, RuntimeInput{RuntimeID: "a", Cwd: "/a", AgentName: "codex"}); !errors.Is(err, ErrUnavailable) {
					t.Fatal(err)
				}
			} else {
				if err := s.Unavailable(); err != nil {
					t.Fatal(err)
				}
				other := Open(Options{StateDir: dir})
				defer func() { _ = other.Close() }()
				if other.Unavailable() == nil {
					t.Fatal("lock conflict accepted")
				}
			}
		})
	}
}

func TestMigrationRollback(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	db, err := sql.Open("sqlite", filepath.Join(dir, "conversations.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	// Force schema DDL to fail after its first table has been created.
	if _, err := db.Exec("CREATE TABLE provider_thread (existing TEXT)"); err != nil {
		t.Fatal(err)
	}
	if err := migrate(ctx, db); err == nil {
		t.Fatal("migration should fail")
	}
	var count, version int
	if err := db.QueryRow("SELECT count(*) FROM sqlite_master WHERE name='conversation'").Scan(&count); err != nil || count != 0 {
		t.Fatalf("migration partially committed: %d %v", count, err)
	}
	if err := db.QueryRow("PRAGMA user_version").Scan(&version); err != nil || version != 0 {
		t.Fatalf("version committed: %d %v", version, err)
	}
}

func TestIDCollisionRetriesWithoutReusingIdentity(t *testing.T) {
	s := openTest(t, t.TempDir())
	first := bindTest(t, s, "first")
	calls := 0
	s.id = func() (string, error) {
		calls++
		if calls == 1 {
			return first.ConversationID, nil
		}
		return newID()
	}
	second := bindTest(t, s, "second")
	if second.ConversationID == first.ConversationID || calls != 3 {
		t.Fatalf("collision not retried: %+v %d", second, calls)
	}
	s.id = func() (string, error) { return first.ConversationID, nil }
	if _, err := s.EnsureRuntime(context.Background(), RuntimeInput{RuntimeID: "third", Cwd: "/a", AgentName: "codex"}); !errors.Is(err, ErrConflict) {
		t.Fatalf("collision exhaustion: %v", err)
	}
}

func TestProcessCrashRollsBackHandoff(t *testing.T) {
	if dir := os.Getenv("WEBMUX_CONVERSATION_CRASH_TEST"); dir != "" {
		s := Open(Options{StateDir: dir})
		if s.Unavailable() != nil {
			os.Exit(40)
		}
		source, err := s.Binding(context.Background(), "source")
		if err != nil {
			os.Exit(41)
		}
		target, err := s.Binding(context.Background(), "target")
		if err != nil {
			os.Exit(41)
		}
		s.beforeCommit = func() error { os.Exit(42); return nil }
		_, _ = s.CommitHandoff(context.Background(), handoffInput(source, target))
		os.Exit(43)
	}
	dir := t.TempDir()
	s := openTest(t, dir)
	bindTest(t, s, "source")
	bindTest(t, s, "target")
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(exe, "-test.run=^TestProcessCrashRollsBackHandoff$")
	cmd.Env = append(os.Environ(), "WEBMUX_CONVERSATION_CRASH_TEST="+dir)
	var exit *exec.ExitError
	if err := cmd.Run(); !errors.As(err, &exit) || exit.ExitCode() != 42 {
		t.Fatalf("unexpected child outcome: %v", err)
	}
	s = openTest(t, dir)
	for _, table := range []string{"context_handoff", "handoff_delivery", "command_receipt"} {
		var count int
		if err := s.db.QueryRow("SELECT count(*) FROM " + table).Scan(&count); err != nil || count != 0 {
			t.Fatalf("crash partially committed %s: %d %v", table, count, err)
		}
	}
}
