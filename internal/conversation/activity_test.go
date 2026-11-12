package conversation

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"
)

func activityTest(t *testing.T, s *Service, b Binding, run, key, provider, text string) string {
	t.Helper()
	id, err := s.WriteActivity(context.Background(), ActivityInput{ProviderThreadID: b.ProviderThreadID, RunID: run, LocalKey: key, ProviderItemID: provider, Kind: "message", Role: "agent", Status: "in_progress", Source: "live", Text: text})
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func runTest(t *testing.T, s *Service, b Binding, key string) Run {
	t.Helper()
	r, err := s.StartRun(context.Background(), b.ProviderThreadID, key)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestLongActivitySurvivesRestartAndUTF8Pagination(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	s := openTest(t, dir)
	b := bindTest(t, s, "runtime")
	r := runTest(t, s, b, "prompt")
	give := strings.Repeat("中文🙂\n", 60000)
	id := activityTest(t, s, b, r.ID, "message", "provider-id", give[:1232])
	activityTest(t, s, b, r.ID, "message", "provider-id", give[1232:])
	page, err := s.Activities(ctx, b.ConversationID, 0, 0, 0)
	if err != nil || len(page.Items) != 0 {
		t.Fatalf("draft published: %+v %v", page, err)
	}
	if err := s.FinishRun(ctx, r.ID, "end_turn"); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s = openTest(t, dir)
	var body strings.Builder
	var offset int64
	for {
		part, err := s.Activity(ctx, b.ConversationID, id, offset, 4001)
		if err != nil {
			t.Fatal(err)
		}
		if !utf8.ValidString(part.Activity.Text) || part.NextTextOffset < offset {
			t.Fatal("invalid text boundary")
		}
		body.WriteString(part.Activity.Text)
		offset = part.NextTextOffset
		if !part.Truncated {
			break
		}
	}
	if body.String() != give {
		t.Fatalf("lost long text: %d/%d", body.Len(), len(give))
	}
	if _, err := s.Activity(ctx, b.ConversationID, id, 1, 4000); !errors.Is(err, ErrInvalid) {
		t.Fatalf("mid-rune offset: %v", err)
	}
}

func TestActivityCommitRollbackAndCrashTail(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	s := openTest(t, dir)
	b := bindTest(t, s, "runtime")
	r := runTest(t, s, b, "prompt")
	s.beforeCommit = func() error { return errors.New("injected crash") }
	if _, err := s.WriteActivity(ctx, ActivityInput{ProviderThreadID: b.ProviderThreadID, RunID: r.ID, LocalKey: "tail", Kind: "message", Role: "agent", Status: "in_progress", Source: "live", Text: "not committed"}); err == nil {
		t.Fatal("chunk committed")
	}
	s.beforeCommit = nil
	id := activityTest(t, s, b, r.ID, "tail", "", "partial")
	s.beforeCommit = func() error { return errors.New("injected position failure") }
	if err := s.FinishRun(ctx, r.ID, "end_turn"); err == nil {
		t.Fatal("positions committed")
	}
	s.beforeCommit = nil
	if _, err := s.Activity(ctx, b.ConversationID, id, 0, 4000); !errors.Is(err, ErrNotFound) {
		t.Fatal("draft visible", err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s = openTest(t, dir)
	page, err := s.Activities(ctx, b.ConversationID, 0, 0, 0)
	if err != nil || len(page.Items) != 0 || !page.ReconcileRequired || len(page.Gaps) != 1 || page.Gaps[0].Reason != "crash" {
		t.Fatalf("crash tail accepted: %+v %v", page, err)
	}
	r = runTest(t, s, b, "next")
	activityTest(t, s, b, r.ID, "complete", "", "next")
	if err := s.FinishRun(ctx, r.ID, "end_turn"); err != nil {
		t.Fatal(err)
	}
	page, err = s.Activities(ctx, b.ConversationID, 0, 0, 0)
	if err != nil || len(page.Items) != 1 || page.Items[0].Activity.Position != 1 {
		t.Fatalf("position hole: %+v %v", page, err)
	}
}

func TestOrderedReplayDeduplicatesRepeatedText(t *testing.T) {
	for _, mode := range []string{"id", "ordered", "missing", "reordered", "unknown", "id_changed"} {
		t.Run(mode, func(t *testing.T) {
			ctx := context.Background()
			s := openTest(t, t.TempDir())
			b := bindTest(t, s, "runtime")
			r := runTest(t, s, b, "live")
			ids := []string{"a", "b", "c"}
			texts := []string{"same", "same", "different"}
			for i, key := range ids {
				provider := ""
				if mode == "id" || mode == "id_changed" {
					provider = key
				}
				activityTest(t, s, b, r.ID, key, provider, texts[i])
			}
			if err := s.FinishRun(ctx, r.ID, "end_turn"); err != nil {
				t.Fatal(err)
			}
			old, err := s.Activities(ctx, b.ConversationID, 0, 0, 0)
			if err != nil {
				t.Fatal(err)
			}
			replay := runTest(t, s, b, "replay")
			order := []int{0, 1, 2}
			if mode == "missing" {
				order = []int{0, 2}
			}
			if mode == "reordered" {
				order = []int{2, 0, 1}
			}
			for _, i := range order {
				provider := ""
				if mode == "id" {
					provider = ids[i]
				}
				if mode == "id_changed" {
					provider = "changed-" + ids[i]
				}
				activityTest(t, s, b, replay.ID, "replay-"+ids[i], provider, texts[i])
			}
			if mode == "unknown" {
				activityTest(t, s, b, replay.ID, "extra", "", "same")
			}
			if err := s.ReconcileReplay(ctx, b.ProviderThreadID, replay.ID); err != nil {
				t.Fatal(err)
			}
			page, err := s.Activities(ctx, b.ConversationID, 0, 0, 0)
			if err != nil || len(page.Items) != 3 {
				t.Fatalf("replay duplicated: %+v %v", page, err)
			}
			for i, item := range page.Items {
				if item.Activity.ID != old.Items[i].Activity.ID {
					t.Fatal("replay changed identity")
				}
			}
			wantGap := mode != "id" && mode != "ordered"
			if page.ReconcileRequired != wantGap {
				t.Fatalf("reconcile=%v want %v", page.ReconcileRequired, wantGap)
			}
		})
	}
}

func TestSnapshotWritesOnlyChangedChunks(t *testing.T) {
	ctx := context.Background()
	s := openTest(t, t.TempDir())
	b := bindTest(t, s, "runtime")
	r := runTest(t, s, b, "prompt")
	in := ActivityInput{ProviderThreadID: b.ProviderThreadID, RunID: r.ID, LocalKey: "tool", ProviderItemID: "tool", Kind: "tool", Role: "agent", Source: "live", Status: "in_progress", Replace: true, Text: strings.Repeat("a", MaxActivityBytes)}
	id, err := s.WriteActivity(ctx, in)
	if err != nil {
		t.Fatal(err)
	}
	var first int64
	if err := s.db.QueryRow("SELECT rowid FROM activity_chunk WHERE activity_id=? AND byte_offset=0", id).Scan(&first); err != nil {
		t.Fatal(err)
	}
	in.Text += "more🙂"
	in.Status = "completed"
	if _, err := s.WriteActivity(ctx, in); err != nil {
		t.Fatal(err)
	}
	var after int64
	if err := s.db.QueryRow("SELECT rowid FROM activity_chunk WHERE activity_id=? AND byte_offset=0", id).Scan(&after); err != nil || after != first {
		t.Fatal("unchanged prefix rewritten", err)
	}
	if err := s.FinishRun(ctx, r.ID, "end_turn"); err != nil {
		t.Fatal(err)
	}
	part, err := s.Activity(ctx, b.ConversationID, id, MaxActivityBytes, 4000)
	if err != nil || part.Activity.Text != "more🙂" {
		t.Fatalf("snapshot suffix: %+v %v", part, err)
	}
}

func TestReasoningAndRawStateAreNotActivityKinds(t *testing.T) {
	s := openTest(t, t.TempDir())
	b := bindTest(t, s, "runtime")
	for _, kind := range []string{"thought", "reasoning", "raw_acp", "native_tool_state"} {
		if _, err := s.WriteActivity(context.Background(), ActivityInput{ProviderThreadID: b.ProviderThreadID, LocalKey: kind, Kind: kind, Source: "live", Text: "private"}); !errors.Is(err, ErrInvalid) {
			t.Fatalf("persisted %s: %v", kind, err)
		}
	}
}

func BenchmarkLongActivityStream(b *testing.B) {
	s := Open(Options{StateDir: b.TempDir()})
	defer func() { _ = s.Close() }()
	if err := s.Unavailable(); err != nil {
		b.Fatal(err)
	}
	ctx := context.Background()
	binding, err := s.EnsureRuntime(ctx, RuntimeInput{RuntimeID: "runtime", Cwd: "/workspace", AgentName: "test"})
	if err != nil {
		b.Fatal(err)
	}
	chunk := strings.Repeat("x", 4096)
	b.SetBytes(int64(len(chunk)))
	b.ReportAllocs()
	b.ResetTimer()
	r, err := s.StartRun(ctx, binding.ProviderThreadID, "benchmark")
	if err != nil {
		b.Fatal(err)
	}
	for range b.N {
		if _, err := s.WriteActivity(ctx, ActivityInput{ProviderThreadID: binding.ProviderThreadID, RunID: r.ID, LocalKey: "stream", Kind: "message", Role: "agent", Source: "live", Status: "in_progress", Text: chunk}); err != nil {
			b.Fatal(err)
		}
	}
}

func TestActivitySnapshotsRetainPrefixAndReplaceShorterText(t *testing.T) {
	ctx := context.Background()
	s := openTest(t, t.TempDir())
	b := bindTest(t, s, "runtime")
	r := runTest(t, s, b, "snapshot")
	in := ActivityInput{ProviderThreadID: b.ProviderThreadID, RunID: r.ID, LocalKey: "tool", Kind: "tool", Role: "agent", Source: "live", Status: "in_progress", Replace: true, Text: strings.Repeat("a", MaxActivityBytes)}
	id, err := s.WriteActivity(ctx, in)
	if err != nil {
		t.Fatal(err)
	}
	var before, after int
	if err := s.db.QueryRow("SELECT total_changes()").Scan(&before); err != nil {
		t.Fatal(err)
	}
	in.Text += "new tail"
	if _, err := s.WriteActivity(ctx, in); err != nil {
		t.Fatal(err)
	}
	if err := s.db.QueryRow("SELECT total_changes()").Scan(&after); err != nil {
		t.Fatal(err)
	}
	if after-before != 2 {
		t.Fatalf("append snapshot wrote %d rows, want chunk+metadata", after-before)
	}
	in.Text = "short🙂"
	in.Status = "completed"
	if _, err := s.WriteActivity(ctx, in); err != nil {
		t.Fatal(err)
	}
	if err := s.FinishRun(ctx, r.ID, "end_turn"); err != nil {
		t.Fatal(err)
	}
	part, err := s.Activity(ctx, b.ConversationID, id, 0, 4000)
	if err != nil || part.Truncated || part.Activity.Text != in.Text {
		t.Fatalf("snapshot replace: %+v %v", part, err)
	}
}

func TestCommittedPositionsDoNotSkipConcurrentDrafts(t *testing.T) {
	ctx := context.Background()
	s := openTest(t, t.TempDir())
	b := bindTest(t, s, "runtime")
	first, second := runTest(t, s, b, "first"), runTest(t, s, b, "second")
	activityTest(t, s, b, first.ID, "draft-a", "", "first")
	activityTest(t, s, b, second.ID, "draft-b", "", "second")
	if err := s.FinishRun(ctx, second.ID, "end_turn"); err != nil {
		t.Fatal(err)
	}
	page, err := s.Activities(ctx, b.ConversationID, 0, 1, 4000)
	if err != nil || len(page.Items) != 1 || page.Items[0].Activity.Text != "second" {
		t.Fatalf("page: %+v %v", page, err)
	}
	if err := s.FinishRun(ctx, first.ID, "end_turn"); err != nil {
		t.Fatal(err)
	}
	next, err := s.Activities(ctx, b.ConversationID, page.NextPosition, 1, 4000)
	if err != nil || len(next.Items) != 1 || next.Items[0].Activity.Text != "first" || next.NextPosition != 2 {
		t.Fatalf("draft skipped: %+v %v", next, err)
	}
}

func TestStableReplayImportsOnlyNewItemsAndRetainsTUIGap(t *testing.T) {
	ctx := context.Background()
	s := openTest(t, t.TempDir())
	b := bindTest(t, s, "runtime")
	r := runTest(t, s, b, "first")
	oldID := activityTest(t, s, b, r.ID, "a", "provider-a", "original")
	if err := s.FinishRun(ctx, r.ID, "end_turn"); err != nil {
		t.Fatal(err)
	}
	if err := s.AddGap(ctx, b.ProviderThreadID, "tui"); err != nil {
		t.Fatal(err)
	}
	for i := range 2 {
		key := "replay-one"
		if i > 0 {
			key = "replay-two"
		}
		replay := runTest(t, s, b, key)
		activityTest(t, s, b, replay.ID, key+"-a", "provider-a", "original")
		activityTest(t, s, b, replay.ID, key+"-b", "provider-b", "new TUI reply")
		if err := s.ReconcileReplay(ctx, b.ProviderThreadID, replay.ID); err != nil {
			t.Fatal(err)
		}
	}
	page, err := s.Activities(ctx, b.ConversationID, 0, 0, 0)
	if err != nil || len(page.Items) != 2 || page.Items[0].Activity.ID != oldID || page.Items[1].Activity.Source != "live" || !page.ReconcileRequired {
		t.Fatalf("replay import: %+v %v", page, err)
	}
	// These test writes use Source=live; the hybrid recorder explicitly tags its
	// staged replay writes Source=replay. The store never rewrites provenance.
}

func TestVersionOneUpgradeKeepsIdentityAndOrdinals(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	db, err := sql.Open("sqlite", filepath.Join(dir, "conversations.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(schema); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("INSERT INTO conversation(id,cwd,history_origin,next_run_ordinal,created_at,updated_at) VALUES('existing','/workspace','unknown',23,1,1)"); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	s := openTest(t, dir)
	c, err := s.Conversation(ctx, "existing")
	if err != nil || c.NextRunOrdinal != 23 || c.HistoryOrigin != "unknown" {
		t.Fatalf("upgrade changed identity: %+v %v", c, err)
	}
	var version int
	if err := s.db.QueryRow("PRAGMA user_version").Scan(&version); err != nil || version != schemaVersion {
		t.Fatalf("schema version: %d %v", version, err)
	}
}

func TestHeadingPatchDoesNotRewriteAccumulatedOutput(t *testing.T) {
	ctx := context.Background()
	s := openTest(t, t.TempDir())
	b := bindTest(t, s, "runtime")
	r := runTest(t, s, b, "heading")
	heading := "original\n"
	in := ActivityInput{ProviderThreadID: b.ProviderThreadID, RunID: r.ID, LocalKey: "tool", Kind: "tool", Role: "agent", Source: "live", Status: "in_progress", Text: strings.Repeat("x", MaxActivityBytes*5), Heading: &heading}
	id, err := s.WriteActivity(ctx, in)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec("CREATE TEMP TABLE changed_output(n INTEGER); CREATE TEMP TRIGGER output_deleted AFTER DELETE ON activity_chunk BEGIN INSERT INTO changed_output VALUES(1); END; CREATE TEMP TRIGGER output_inserted AFTER INSERT ON activity_chunk BEGIN INSERT INTO changed_output VALUES(1); END;"); err != nil {
		t.Fatal(err)
	}
	heading = "updated command\n"
	in.Text = ""
	in.Status = "completed"
	if _, err := s.WriteActivity(ctx, in); err != nil {
		t.Fatal(err)
	}
	var changes int
	if err := s.db.QueryRow("SELECT count(*) FROM changed_output").Scan(&changes); err != nil || changes != 0 {
		t.Fatalf("metadata rewrote output: %d %v", changes, err)
	}
	if err := s.FinishRun(ctx, r.ID, "end_turn"); err != nil {
		t.Fatal(err)
	}
	part, err := s.Activity(ctx, b.ConversationID, id, 0, 4000)
	if err != nil || !strings.HasPrefix(part.Activity.Text, heading) {
		t.Fatalf("updated heading missing: %+v %v", part, err)
	}
	part, err = s.Activity(ctx, b.ConversationID, id, int64(len(heading)+MaxActivityBytes*5-1), 4000)
	if err != nil || part.Activity.Text != "x" || part.Truncated {
		t.Fatalf("output corrupted: %+v %v", part, err)
	}
}
