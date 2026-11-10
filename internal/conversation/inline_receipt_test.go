package conversation

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"
)

func TestTargetInlineReceiptSurvivesSourceForgetAndBackfill(t *testing.T) {
	for _, upgrade := range []bool{false, true} {
		t.Run(map[bool]string{false: "current", true: "backfill"}[upgrade], func(t *testing.T) {
			ctx := context.Background()
			dir := t.TempDir()
			s := openTest(t, dir)
			b := bindTest(t, s, "source")
			_, e := executionTest(t, s, b)
			text := "full historical envelope\nUser message:\noriginal"
			digest := sha256.Sum256([]byte(text))
			e.Phase, e.NativeThreadID, e.PromptHash = "sending", "native", hex.EncodeToString(digest[:])
			if err := s.CheckpointExecution(ctx, e); err != nil {
				t.Fatal(err)
			}
			if err := s.ClaimExecutionText(ctx, e.ID, e.PromptHash, e.PromptHash); err != nil {
				t.Fatal(err)
			}
			if upgrade {
				tx, err := s.db.BeginTx(ctx, nil)
				if err != nil {
					t.Fatal(err)
				}
				defer func() { _ = tx.Rollback() }()
				if _, err := tx.Exec("DROP TABLE inline_receipt;" + strings.ReplaceAll(inlineReceiptSchema, "PRAGMA user_version=6;", "")); err != nil {
					t.Fatal(err)
				}
				if err := backfillInlineReceipts(ctx, tx); err != nil {
					t.Fatal(err)
				}
				if err := tx.Commit(); err != nil {
					t.Fatal(err)
				}
			}
			if err := s.ForgetRuntime(ctx, "source"); err != nil {
				t.Fatal(err)
			}
			if err := s.Close(); err != nil {
				t.Fatal(err)
			}
			s = openTest(t, dir)
			got, matched, err := s.NormalizeInlineUser(ctx, e.TargetProviderThreadID, "native", text, "provider-user")
			if err != nil || !matched || got.Text != "original" {
				t.Fatalf("target normalization lost: %+v %t %v", got, matched, err)
			}
			var count int
			if err := s.db.QueryRow("SELECT count(*) FROM inline_receipt").Scan(&count); err != nil || count != 1 {
				t.Fatal("target-owned receipt missing", count, err)
			}
			if _, err := s.db.Exec("DELETE FROM conversation WHERE id=?", e.TargetConversationID); err != nil {
				t.Fatal(err)
			}
			if err := s.db.QueryRow("SELECT count(*) FROM inline_receipt").Scan(&count); err != nil || count != 0 {
				t.Fatal("target receipt survived target removal", count, err)
			}
		})
	}
}
