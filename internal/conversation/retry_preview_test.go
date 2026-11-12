package conversation

import (
	"context"
	"errors"
	"testing"
)

func failedSameConversationAttempt(t *testing.T, dir string) (*Service, Binding, Preview, Execution) {
	t.Helper()
	ctx := context.Background()
	s := openTest(t, dir)
	b := bindTest(t, s, "source")
	p, err := s.ReservePreview(ctx, PreviewReservation{SourceConversationID: b.ConversationID, ClientRequestID: "same-conversation-preview", RequestHash: "reviewed", Request: PreviewRequest{ConfigID: "target", AgentName: "omp", UserText: "request"}})
	if err != nil {
		t.Fatal(err)
	}
	p, err = s.CompletePreview(ctx, p.ID, PreviewResult{RenderedText: "frozen reviewed context", Budget: PreviewBudget{History: 16000, Source: "estimated"}})
	if err != nil {
		t.Fatal(err)
	}
	e, err := s.BeginExecution(ctx, ExecutionInput{SourceConversationID: b.ConversationID, HandoffID: p.ID, ClientRequestID: "same-conversation-initial", RequestHash: "initial", Revision: p.Revision})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.BindRuntime(ctx, e.RuntimeID, p.TargetProviderThreadID, "target", "omp"); err != nil {
		t.Fatal(err)
	}
	e.Phase, e.NativeThreadID = "sending", "native-target"
	if err := s.CheckpointExecution(ctx, e); err != nil {
		t.Fatal(err)
	}
	r, err := s.StartRun(ctx, p.TargetProviderThreadID, e.ID)
	if err != nil {
		t.Fatal(err)
	}
	activityTest(t, s, Binding{ConversationID: b.ConversationID, ProviderThreadID: p.TargetProviderThreadID}, r.ID, "rejected-request", "", "rejected request")
	if err := s.FinishRun(ctx, r.ID, "auth_required"); err != nil {
		t.Fatal(err)
	}
	if err := s.FinishExecution(ctx, e.ID, "failed", "provider_refused"); err != nil {
		t.Fatal(err)
	}
	return s, b, p, e
}

func TestSameConversationRetryUsesOnlyFailedAttemptProvenance(t *testing.T) {
	ctx := context.Background()
	s, b, p, failed := failedSameConversationAttempt(t, t.TempDir())
	ordinary, err := s.Preview(ctx, p.ID)
	if err != nil || !ordinary.Stale {
		t.Fatalf("attempt did not advance source history: %+v %v", ordinary, err)
	}
	reviewed, err := s.DeliveryPreview(ctx, p.ID)
	if err != nil || reviewed.Stale || reviewed.SourcePosition != p.SourcePosition || reviewed.SourceRunOrdinal != p.SourceRunOrdinal || reviewed.Revision != p.Revision || reviewed.RenderedText != p.RenderedText {
		t.Fatalf("retry changed frozen context: %+v %v", reviewed, err)
	}
	retry, err := s.BeginExecution(ctx, ExecutionInput{SourceConversationID: b.ConversationID, HandoffID: p.ID, ClientRequestID: "same-conversation-retry", RequestHash: "retry", Revision: p.Revision, Retry: true})
	if err != nil || retry.ID == failed.ID || retry.RuntimeID != failed.RuntimeID {
		t.Fatalf("own failed attempt prevented retry: %+v %v", retry, err)
	}
	ordinary, err = s.Preview(ctx, p.ID)
	if err != nil || !ordinary.Stale || ordinary.SourcePosition != p.SourcePosition {
		t.Fatalf("retry mutated persisted preview snapshot: %+v %v", ordinary, err)
	}
}

func TestSameConversationRetryRejectsUnrelatedChanges(t *testing.T) {
	for _, change := range []string{"run", "activity", "gap", "tui-gap", "revision"} {
		t.Run(change, func(t *testing.T) {
			ctx := context.Background()
			s, b, p, _ := failedSameConversationAttempt(t, t.TempDir())
			switch change {
			case "run":
				if _, err := s.StartRun(ctx, b.ProviderThreadID, "unrelated-request"); err != nil {
					t.Fatal(err)
				}
			case "activity":
				activityTest(t, s, b, "", "unrelated-marker", "", "unrelated activity")
			case "gap", "tui-gap":
				reason := "storage_failure"
				if change == "tui-gap" {
					reason = "tui"
				}
				if err := s.AddGap(ctx, b.ProviderThreadID, reason); err != nil {
					t.Fatal(err)
				}
			case "revision":
				if _, err := s.EditPreview(ctx, p.ID, p.Revision, "newly reviewed context", p.Budget); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := s.BeginExecution(ctx, ExecutionInput{SourceConversationID: b.ConversationID, HandoffID: p.ID, ClientRequestID: "unrelated-retry", RequestHash: "retry", Revision: p.Revision, Retry: true}); !errors.Is(err, ErrConflict) {
				t.Fatalf("%s was incorrectly attributed to failed attempt: %v", change, err)
			}
		})
	}
}

func TestFailedRuntimeDispatchSurvivesReopen(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	s, _, _, failed := failedSameConversationAttempt(t, dir)
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s = openTest(t, dir)
	e, held, err := s.RuntimeDispatch(ctx, failed.RuntimeID)
	if err != nil || !held || e.ID != failed.ID || e.Status != "failed" {
		t.Fatalf("failed owner lost after restart: %+v held=%v error=%v", e, held, err)
	}
	if e, held, err := s.RuntimeDispatch(ctx, "source"); err != nil || held || e.ID != failed.ID {
		t.Fatalf("failed target claimed unchanged source: %+v held=%v error=%v", e, held, err)
	}
}

func TestUnresolvedRuntimeDispatchStillOwnsSourceAndTarget(t *testing.T) {
	for _, status := range []string{"pending", "unknown"} {
		t.Run(status, func(t *testing.T) {
			ctx := context.Background()
			s, source, p, _ := failedSameConversationAttempt(t, t.TempDir())
			e, err := s.BeginExecution(ctx, ExecutionInput{SourceConversationID: source.ConversationID, HandoffID: p.ID, ClientRequestID: "unresolved-retry", RequestHash: "retry", Revision: p.Revision, Retry: true})
			if err != nil {
				t.Fatal(err)
			}
			if status == "unknown" {
				e.Phase = "sending"
				if err := s.CheckpointExecution(ctx, e); err != nil {
					t.Fatal(err)
				}
				if err := s.FinishExecution(ctx, e.ID, status, "disconnected"); err != nil {
					t.Fatal(err)
				}
			}
			for _, runtime := range []string{"source", e.RuntimeID} {
				got, held, err := s.RuntimeDispatch(ctx, runtime)
				if err != nil || !held || got.ID != e.ID || got.Status != status {
					t.Fatalf("%s runtime %s escaped owner: %+v held=%v error=%v", status, runtime, got, held, err)
				}
			}
		})
	}
}
