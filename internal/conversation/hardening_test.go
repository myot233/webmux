package conversation

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestSourceForgetRetainsTargetUnknownGateAndInlineOccupancy(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	s := openTest(t, dir)
	b := bindTest(t, s, "source")
	_, e := executionTest(t, s, b)
	if err := s.BindRuntime(ctx, e.RuntimeID, e.TargetProviderThreadID, "config", "omp"); err != nil {
		t.Fatal(err)
	}
	text := "historical packet\nUser message:\noriginal"
	e.Phase = "sending"
	e.NativeThreadID = "native"
	e.PromptHash = TextHash(text)
	if err := s.CheckpointExecution(ctx, e); err != nil {
		t.Fatal(err)
	}
	if err := s.ClaimExecutionText(ctx, e.ID, e.PromptHash, e.PromptHash); err != nil {
		t.Fatal(err)
	}
	if err := s.ForgetRuntime(ctx, "source"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Execution(ctx, e.ID); !errors.Is(err, ErrNotFound) {
		t.Fatal("source operation survived", err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s = openTest(t, dir)
	gate, held, err := s.RuntimeDispatch(ctx, e.RuntimeID)
	if err != nil || !held || gate.ID != e.ID || gate.Status != "unknown" {
		t.Fatal("unknown gate lost", gate, held, err)
	}
	if _, matched, err := s.NormalizeInlineUser(ctx, e.TargetProviderThreadID, "other-native", text); err != nil || matched {
		t.Fatal("wrong identity admitted", err)
	}
	if used, err := s.EstimatedNativeUsage(ctx, e.TargetProviderThreadID); err != nil || used != 100 {
		t.Fatal("source forget lost occupancy", used, err)
	}
	p, err := s.ReservePreview(ctx, PreviewReservation{SourceConversationID: e.TargetConversationID, ClientRequestID: "new-target-preview", RequestHash: "preview", Request: PreviewRequest{ConfigID: "config", AgentName: "omp", TargetProviderThreadID: e.TargetProviderThreadID}})
	if err != nil {
		t.Fatal(err)
	}
	p, err = s.CompletePreview(ctx, p.ID, PreviewResult{Budget: PreviewBudget{History: 16000}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.BeginExecution(ctx, ExecutionInput{SourceConversationID: e.TargetConversationID, HandoffID: p.ID, ClientRequestID: "bypass-unknown", RequestHash: "new", Revision: p.Revision}); !errors.Is(err, ErrConflict) {
		t.Fatal("new command bypassed forgotten-source unknown", err)
	}
	if _, matched, err := s.NormalizeInlineUser(ctx, e.TargetProviderThreadID, "native", text, "stable-user"); err != nil || !matched {
		t.Fatal("exact target proof failed", err)
	}
	if gate, held, err = s.RuntimeDispatch(ctx, e.RuntimeID); err != nil || held {
		t.Fatal("proof did not release gate", gate, held, err)
	}
	if used, err := s.EstimatedNativeUsage(ctx, e.TargetProviderThreadID); err != nil || used != 100 {
		t.Fatal("confirmed occupancy lost", used, err)
	}
}

func TestForgottenSourceNativeUnknownNeedsWholeProofButNeverSendsUserTask(t *testing.T) {
	ctx := context.Background()
	s := openTest(t, t.TempDir())
	_, _, e, items := injectionTest(t, s)
	if err := s.BindRuntime(ctx, e.RuntimeID, e.TargetProviderThreadID, "config", "omp"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ReserveInjection(ctx, e.ID, "native", items); err != nil {
		t.Fatal(err)
	}
	if err := s.ClaimInjection(ctx, e.DeliveryID); err != nil {
		t.Fatal(err)
	}
	if err := s.ForgetRuntime(ctx, "source"); err != nil {
		t.Fatal(err)
	}
	if _, held, err := s.RuntimeDispatch(ctx, e.RuntimeID); err != nil || !held {
		t.Fatal("history unknown lost", held, err)
	}
	if err := s.ReconcileInjection(ctx, e.DeliveryID, "native", items[:1]); !errors.Is(err, ErrConflict) {
		t.Fatal("partial proof", err)
	}
	if err := s.ReconcileInjection(ctx, e.DeliveryID, "native", items); err != nil {
		t.Fatal(err)
	}
	gate, held, err := s.RuntimeDispatch(ctx, e.RuntimeID)
	if err != nil || held || gate.CallStarted {
		t.Fatal("proof did not cancel unsent deleted operation", gate, held, err)
	}
	if _, err := s.Execution(ctx, e.ID); !errors.Is(err, ErrNotFound) {
		t.Fatal("deleted operation recreated", err)
	}
}

func TestPublishedManagedRuntimePrunesButReservationSurvives(t *testing.T) {
	ctx := context.Background()
	s := openTest(t, t.TempDir())
	b := bindTest(t, s, "source")
	_, e := executionTest(t, s, b)
	if err := s.BindRuntime(ctx, e.RuntimeID, e.TargetProviderThreadID, "config", "omp"); err != nil {
		t.Fatal(err)
	}
	if err := s.PruneRuntimes(ctx, []string{"source"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Binding(ctx, e.RuntimeID); err != nil {
		t.Fatal("unpublished reservation pruned", err)
	}
	if err := s.SyncRuntime(ctx, RuntimeInput{RuntimeID: e.RuntimeID, Cwd: "/workspace", ConfigID: "config", AgentName: "omp", Status: "closed"}); err != nil {
		t.Fatal(err)
	}
	if err := s.PruneRuntimes(ctx, []string{"source"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Binding(ctx, e.RuntimeID); !errors.Is(err, ErrNotFound) {
		t.Fatal("published managed orphan retained", err)
	}
}

func TestForgettingUnpublishedRuntimeLeavesRecoverableDeletionIntent(t *testing.T) {
	ctx := context.Background()
	s := openTest(t, t.TempDir())
	b := bindTest(t, s, "source")
	_, e := executionTest(t, s, b)
	if err := s.BindRuntime(ctx, e.RuntimeID, e.TargetProviderThreadID, "config", "omp"); err != nil {
		t.Fatal(err)
	}
	if err := s.MarkRuntimeForgetting(ctx, e.RuntimeID); err != nil {
		t.Fatal(err)
	}
	// Simulate successful JSON removal and a crash before SQL deletion.
	if err := s.PruneRuntimes(ctx, []string{"source"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Binding(ctx, e.RuntimeID); !errors.Is(err, ErrNotFound) {
		t.Fatal("forget intent was confused with unpublished creation", err)
	}
}

func TestForgottenSourceRemovesUnexecutedForkDraftMetadata(t *testing.T) {
	ctx := context.Background()
	s := openTest(t, t.TempDir())
	b := bindTest(t, s, "source")
	p, _ := executionTest(t, s, b)
	if err := s.ForgetRuntime(ctx, "source"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Conversation(ctx, p.TargetConversationID); !errors.Is(err, ErrNotFound) {
		t.Fatal("abandoned fork metadata survived", err)
	}
}

func TestSQLiteReadOnlyAndFullAbortBeforeActivityPublication(t *testing.T) {
	for _, fault := range []string{"read-only", "full"} {
		t.Run(fault, func(t *testing.T) {
			ctx := context.Background()
			s := openTest(t, t.TempDir())
			b := bindTest(t, s, "source")
			statement := "PRAGMA query_only=ON"
			if fault == "full" {
				var pages int
				if err := s.db.QueryRow("PRAGMA page_count").Scan(&pages); err != nil {
					t.Fatal(err)
				}
				statement = fmt.Sprintf("PRAGMA max_page_count=%d", pages)
			}
			if _, err := s.db.Exec(statement); err != nil {
				t.Fatal(err)
			}
			_, err := s.WriteActivity(ctx, ActivityInput{ProviderThreadID: b.ProviderThreadID, LocalKey: "cannot-commit", Kind: "message", Role: "user", Source: "local", Status: "completed", Text: strings.Repeat("private not logged ", 10000)})
			if !errors.Is(err, ErrUnavailable) {
				t.Fatal("storage failure did not fail closed", err)
			}
			var count int
			if err := s.db.QueryRow("SELECT count(*) FROM activity WHERE position IS NOT NULL").Scan(&count); err != nil || count != 0 {
				t.Fatal("partial activity published", count, err)
			}
		})
	}
}

func TestProcessCrashAfterClaimBeforeConsumptionCommitStaysUnknown(t *testing.T) {
	if dir := os.Getenv("WEBMUX_CONSUMPTION_COMMIT_CRASH"); dir != "" {
		s := Open(Options{StateDir: dir})
		if s.Unavailable() != nil {
			os.Exit(40)
		}
		b, err := s.Binding(context.Background(), "source")
		if err != nil {
			os.Exit(41)
		}
		r, err := s.Receipt(context.Background(), b.ConversationID, "message-command")
		if err != nil {
			os.Exit(41)
		}
		s.beforeCommit = func() error { os.Exit(42); return nil }
		_ = s.FinishExecution(context.Background(), r.ExecutionID, "inline", "")
		os.Exit(43)
	}
	ctx := context.Background()
	dir := t.TempDir()
	s := openTest(t, dir)
	b := bindTest(t, s, "source")
	_, e := executionTest(t, s, b)
	e.Phase = "sending"
	e.NativeThreadID = "native"
	e.PromptHash = TextHash("packet")
	if err := s.CheckpointExecution(ctx, e); err != nil {
		t.Fatal(err)
	}
	if err := s.ClaimExecutionText(ctx, e.ID, e.PromptHash, e.PromptHash); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(exe, "-test.run=^TestProcessCrashAfterClaimBeforeConsumptionCommitStaysUnknown$")
	cmd.Env = append(os.Environ(), "WEBMUX_CONSUMPTION_COMMIT_CRASH="+dir)
	var exit *exec.ExitError
	if err := cmd.Run(); !errors.As(err, &exit) || exit.ExitCode() != 42 {
		t.Fatal("unexpected child crash", err)
	}
	s = openTest(t, dir)
	got, err := s.Execution(ctx, e.ID)
	if err != nil || got.Status != "unknown" {
		t.Fatal("uncommitted success became known", got, err)
	}
	if err := s.ClaimExecutionText(ctx, e.ID, e.PromptHash, e.PromptHash); !errors.Is(err, ErrConflict) {
		t.Fatal("claimed call repeated after crash", err)
	}
	receipt, err := s.Receipt(ctx, b.ConversationID, "message-command")
	if err != nil || receipt.ExecutionID != e.ID {
		t.Fatal("new attempt allocated", receipt, err)
	}
}

func TestEstimatedNativeUsageIncludesPreviouslyConsumedAttachments(t *testing.T) {
	s := openTest(t, t.TempDir())
	ctx := context.Background()
	b := bindTest(t, s, "source")
	_, err := s.WriteActivity(ctx, ActivityInput{ProviderThreadID: b.ProviderThreadID, LocalKey: "with-files", Kind: "message", Role: "user", Source: "local", Status: "completed", Text: "body", Attachments: []ActivityAttachment{{Name: "image.png", MimeType: "image/png", Reference: "target-owned:image"}, {Name: "file.txt", MimeType: "text/plain", Reference: "target-owned:file"}}})
	if err != nil {
		t.Fatal(err)
	}
	used, err := s.EstimatedNativeUsage(ctx, b.ProviderThreadID)
	if err != nil || used != escapedTextBytes("body")+256+8192+4096 {
		t.Fatal("prior attachment occupancy was omitted", used, err)
	}
}
