package conversation

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"testing"
)

func executionTest(t *testing.T, s *Service, b Binding) (Preview, Execution) {
	t.Helper()
	ctx := context.Background()
	p, err := s.ReservePreview(ctx, PreviewReservation{SourceConversationID: b.ConversationID, ClientRequestID: "preview-command", RequestHash: "preview-hash", Request: PreviewRequest{ConfigID: "config", AgentName: "omp", Fork: true, UserText: "original", AgentConfig: map[string]string{}}})
	if err != nil {
		t.Fatal(err)
	}
	p, err = s.CompletePreview(ctx, p.ID, PreviewResult{Budget: PreviewBudget{History: 16000, Used: 100, Source: "estimated"}})
	if err != nil {
		t.Fatal(err)
	}
	e, err := s.BeginExecution(ctx, ExecutionInput{SourceConversationID: b.ConversationID, HandoffID: p.ID, ClientRequestID: "message-command", RequestHash: "message-hash", Revision: p.Revision})
	if err != nil {
		t.Fatal(err)
	}
	return p, e
}

func TestExecutionCrashPhasesAndDurableReceipt(t *testing.T) {
	for _, phase := range []string{"reserved", "worktree", "session", "ready", "sending"} {
		t.Run(phase, func(t *testing.T) {
			ctx := context.Background()
			dir := t.TempDir()
			s := openTest(t, dir)
			b := bindTest(t, s, "source")
			p, e := executionTest(t, s, b)
			e.Phase, e.Step = phase, "prompt"
			if err := s.CheckpointExecution(ctx, e); err != nil {
				t.Fatal(err)
			}
			if phase == "sending" {
				if err := s.ClaimExecution(ctx, e.ID); err != nil {
					t.Fatal(err)
				}
			}
			if err := s.Close(); err != nil {
				t.Fatal(err)
			}
			reopened := openTest(t, dir)
			got, err := reopened.BeginExecution(ctx, ExecutionInput{SourceConversationID: b.ConversationID, HandoffID: p.ID, ClientRequestID: "message-command", RequestHash: "message-hash", Revision: 1})
			want := "pending"
			if phase == "sending" {
				want = "unknown"
			}
			if err != nil || got.ID != e.ID || got.RuntimeID != e.RuntimeID || got.Status != want {
				t.Fatalf("crash %s: %+v %v", phase, got, err)
			}
			if _, err := reopened.BeginExecution(ctx, ExecutionInput{SourceConversationID: b.ConversationID, HandoffID: p.ID, ClientRequestID: "message-command", RequestHash: "changed", Revision: 1}); !errors.Is(err, ErrConflict) {
				t.Fatal(err)
			}
			if _, err := reopened.BeginExecution(ctx, ExecutionInput{SourceConversationID: b.ConversationID, HandoffID: p.ID, ClientRequestID: "other-command", RequestHash: "message-hash", Revision: 1, Retry: true}); !errors.Is(err, ErrConflict) {
				t.Fatal("pending/unknown implicitly retried", err)
			}
		})
	}
}

func TestExplicitRetryPreservesFirstFailedAttempt(t *testing.T) {
	ctx := context.Background()
	s := openTest(t, t.TempDir())
	b := bindTest(t, s, "source")
	p, e := executionTest(t, s, b)
	if err := s.FinishExecution(ctx, e.ID, "failed", "connector_missing"); err != nil {
		t.Fatal(err)
	}
	retry, err := s.BeginExecution(ctx, ExecutionInput{SourceConversationID: b.ConversationID, HandoffID: p.ID, ClientRequestID: "retry-command", RequestHash: "retry-hash", Revision: 1, Retry: true})
	if err != nil || retry.ID == e.ID || retry.RuntimeID != e.RuntimeID || retry.DeliveryID != e.DeliveryID {
		t.Fatalf("retry: %+v %v", retry, err)
	}
	old, err := s.BeginExecution(ctx, ExecutionInput{SourceConversationID: b.ConversationID, HandoffID: p.ID, ClientRequestID: "message-command", RequestHash: "message-hash", Revision: 1})
	if err != nil || old.Status != "failed" || old.ErrorClass != "connector_missing" {
		t.Fatalf("old receipt changed: %+v %v", old, err)
	}
}

func TestInlineReplayRequiresExactNativePayloadAndNeverUndoesConsumption(t *testing.T) {
	ctx := context.Background()
	s := openTest(t, t.TempDir())
	b := bindTest(t, s, "source")
	_, e := executionTest(t, s, b)
	text := "WebMux handoff marker\n\nUser message:\noriginal"
	digest := sha256.Sum256([]byte(text))
	e.Phase, e.NativeThreadID, e.PromptHash = "sending", "native", hex.EncodeToString(digest[:])
	if err := s.CheckpointExecution(ctx, e); err != nil {
		t.Fatal(err)
	}
	if err := s.FinishExecution(ctx, e.ID, "unknown", "transport"); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct{ native, text string }{{"wrong", text}, {"native", text + "changed"}} {
		if _, matched, err := s.NormalizeInlineReplay(ctx, e.TargetProviderThreadID, test.native, test.text); err != nil || matched {
			t.Fatalf("unproven match: %t %v", matched, err)
		}
	}
	normalized, matched, err := s.NormalizeInlineReplay(ctx, e.TargetProviderThreadID, "native", text)
	if err != nil || !matched || normalized != "original" {
		t.Fatalf("proof: %q %t %v", normalized, matched, err)
	}
	if err := s.FinishExecution(ctx, e.ID, "failed", "late_auth"); err != nil {
		t.Fatal(err)
	}
	got, err := s.Execution(ctx, e.ID)
	if err != nil || got.Status != "inline" {
		t.Fatalf("consumption overwritten: %+v %v", got, err)
	}
	p, err := s.ProviderThread(ctx, e.TargetProviderThreadID)
	if err != nil || p.LastDeliveredRunOrdinal != 0 {
		t.Fatalf("fork advanced unrelated ordinal: %+v %v", p, err)
	}
}

func TestInlineReplayDoesNotRewriteDifferentStableUserItem(t *testing.T) {
	ctx := context.Background()
	s := openTest(t, t.TempDir())
	b := bindTest(t, s, "source")
	_, e := executionTest(t, s, b)
	text := "WebMux handoff known exact payload"
	hash := sha256.Sum256([]byte(text))
	e.Phase, e.NativeThreadID, e.PromptHash = "sending", "native", hex.EncodeToString(hash[:])
	if err := s.CheckpointExecution(ctx, e); err != nil {
		t.Fatal(err)
	}
	if _, matched, err := s.NormalizeInlineUser(ctx, e.TargetProviderThreadID, "native", text, "original-item"); err != nil || !matched {
		t.Fatal(matched, err)
	}
	result, matched, err := s.NormalizeInlineUser(ctx, e.TargetProviderThreadID, "native", text, "ordinary-copy")
	if err != nil || matched || result.Text != text {
		t.Fatal("rewrote a different user item", result, matched, err)
	}
}
