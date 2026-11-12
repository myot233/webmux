package conversation

import (
	"context"
	"errors"
	"testing"
)

func TestDeltaIncludesLateActivityFromCoveredRun(t *testing.T) {
	ctx := context.Background()
	s := openTest(t, t.TempDir())
	a, err := s.EnsureRuntime(ctx, RuntimeInput{RuntimeID: "runtime-a", Cwd: "/workspace", ConfigID: "A", AgentName: "omp", AgentSessionID: "native-a", HistoryOrigin: "complete"})
	if err != nil {
		t.Fatal(err)
	}
	runA := runTest(t, s, a, "A")
	activityTest(t, s, a, runA.ID, "A-user", "", "A text")
	if err := s.FinishRun(ctx, runA.ID, "end_turn"); err != nil {
		t.Fatal(err)
	}
	if err := s.AcknowledgeRun(ctx, runA.ID); err != nil {
		t.Fatal(err)
	}
	p, err := s.ReservePreview(ctx, PreviewReservation{SourceConversationID: a.ConversationID, ClientRequestID: "preview-B", RequestHash: "B", Request: PreviewRequest{ConfigID: "B", AgentName: "omp"}})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.BindRuntime(ctx, "runtime-b", p.TargetProviderThreadID, "B", "omp"); err != nil {
		t.Fatal(err)
	}
	b := Binding{ConversationID: a.ConversationID, ProviderThreadID: p.TargetProviderThreadID}
	runB := runTest(t, s, b, "B")
	activityTest(t, s, b, runB.ID, "B-agent", "", "B text")
	late, err := s.WriteActivity(ctx, ActivityInput{ProviderThreadID: b.ProviderThreadID, RunID: runB.ID, LocalKey: "late", Kind: "tool", Role: "agent", Status: "in_progress", Source: "live", Text: "background"})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.FinishRun(ctx, runB.ID, "end_turn"); err != nil {
		t.Fatal(err)
	}
	back, err := s.ReservePreview(ctx, PreviewReservation{SourceConversationID: a.ConversationID, ClientRequestID: "preview-A", RequestHash: "A", Request: PreviewRequest{ConfigID: "A", AgentName: "omp", TargetProviderThreadID: a.ProviderThreadID}})
	if err != nil {
		t.Fatal(err)
	}
	back, err = s.CompletePreview(ctx, back.ID, PreviewResult{Budget: PreviewBudget{History: 16000, Source: "estimated"}})
	if err != nil {
		t.Fatal(err)
	}
	e, err := s.BeginExecution(ctx, ExecutionInput{SourceConversationID: a.ConversationID, HandoffID: back.ID, ClientRequestID: "message-A", RequestHash: "A", Revision: 1})
	if err != nil {
		t.Fatal(err)
	}
	e.Phase = "sending"
	e.NativeThreadID = "native-a"
	if err := s.CheckpointExecution(ctx, e); err != nil {
		t.Fatal(err)
	}
	if err := s.FinishExecution(ctx, e.ID, "inline", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := s.WriteActivity(ctx, ActivityInput{ProviderThreadID: b.ProviderThreadID, RunID: runB.ID, LocalKey: "late", Kind: "tool", Role: "agent", Status: "completed", Source: "live", Text: " done"}); err != nil {
		t.Fatal(err)
	}
	next, err := s.ReservePreview(ctx, PreviewReservation{SourceConversationID: a.ConversationID, ClientRequestID: "preview-A-next", RequestHash: "A-next", Request: PreviewRequest{ConfigID: "A", AgentName: "omp", TargetProviderThreadID: a.ProviderThreadID}})
	if err != nil {
		t.Fatal(err)
	}
	candidates, err := s.Candidates(ctx, a.ConversationID, next.SourcePosition, next.TargetSeenPosition)
	if err != nil || next.Strategy != "delta_since_target_last_seen" || next.FromRunOrdinal != runB.Ordinal || len(candidates) != 1 || candidates[0].Message.ActivityID != late {
		t.Fatalf("lost late activity: %+v %+v %v", next, candidates, err)
	}
}

func TestAcknowledgementKeepsUnconsumedRunsBeforeLaterAcceptedRun(t *testing.T) {
	for _, status := range []string{"failed", "cancelled"} {
		t.Run(status, func(t *testing.T) {
			ctx := context.Background()
			s := openTest(t, t.TempDir())
			a, err := s.EnsureRuntime(ctx, RuntimeInput{RuntimeID: "runtime-a", Cwd: "/workspace", ConfigID: "A", AgentName: "omp", AgentSessionID: "native-a", HistoryOrigin: "complete"})
			if err != nil {
				t.Fatal(err)
			}
			seen := runTest(t, s, a, "seen")
			activityTest(t, s, a, seen.ID, "seen-user", "", "already seen")
			if err := s.FinishRun(ctx, seen.ID, "end_turn"); err != nil {
				t.Fatal(err)
			}
			if err := s.AcknowledgeRun(ctx, seen.ID); err != nil {
				t.Fatal(err)
			}
			unconsumed := runTest(t, s, a, "unconsumed")
			unseenID := activityTest(t, s, a, unconsumed.ID, "unseen-user", "", "unconsumed request")
			if err := s.FinishRun(ctx, unconsumed.ID, status); err != nil {
				t.Fatal(err)
			}
			accepted := runTest(t, s, a, "accepted")
			acceptedID := activityTest(t, s, a, accepted.ID, "accepted-user", "", "accepted request")
			if err := s.FinishRun(ctx, accepted.ID, "end_turn"); err != nil {
				t.Fatal(err)
			}
			if err := s.AcknowledgeRun(ctx, accepted.ID); err != nil {
				t.Fatal(err)
			}
			p, err := s.ReservePreview(ctx, PreviewReservation{SourceConversationID: a.ConversationID, ClientRequestID: "preview-unseen", RequestHash: "unseen", Request: PreviewRequest{ConfigID: "A", AgentName: "omp", TargetProviderThreadID: a.ProviderThreadID}})
			if err != nil {
				t.Fatal(err)
			}
			candidates, err := s.Candidates(ctx, a.ConversationID, p.SourcePosition, p.TargetSeenPosition)
			if err != nil || p.Strategy != "delta_since_target_last_seen" || p.FromRunOrdinal != unconsumed.Ordinal || len(candidates) != 2 {
				t.Fatalf("lost unconsumed run: preview=%+v candidates=%+v error=%v", p, candidates, err)
			}
			if candidates[0].Message.ActivityID != unseenID || candidates[0].Consumed || candidates[1].Message.ActivityID != acceptedID || !candidates[1].Consumed {
				t.Fatalf("incorrect consumption provenance: %+v", candidates)
			}
			p, err = s.CompletePreview(ctx, p.ID, PreviewResult{SelectedItemIDs: []string{unseenID}, Budget: PreviewBudget{History: 16000, Source: "estimated"}})
			if err != nil {
				t.Fatal(err)
			}
			e, err := s.BeginExecution(ctx, ExecutionInput{SourceConversationID: a.ConversationID, HandoffID: p.ID, ClientRequestID: "deliver-unseen", RequestHash: "unseen", Revision: p.Revision})
			if err != nil {
				t.Fatal(err)
			}
			e.Phase, e.NativeThreadID = "sending", "native-a"
			if err := s.CheckpointExecution(ctx, e); err != nil {
				t.Fatal(err)
			}
			if err := s.FinishExecution(ctx, e.ID, "inline", ""); err != nil {
				t.Fatal(err)
			}
			next := runTest(t, s, a, "next")
			activityTest(t, s, a, next.ID, "next-user", "", "next accepted request")
			if err := s.FinishRun(ctx, next.ID, "end_turn"); err != nil {
				t.Fatal(err)
			}
			if err := s.AcknowledgeRun(ctx, next.ID); err != nil {
				t.Fatal(err)
			}
			thread, err := s.ProviderThread(ctx, a.ProviderThreadID)
			if err != nil || thread.LastDeliveredPosition <= p.SourcePosition {
				t.Fatalf("delivered hole pinned later coverage: %+v %v", thread, err)
			}
			candidates, err = s.Candidates(ctx, a.ConversationID, thread.LastDeliveredPosition, thread.LastDeliveredPosition)
			if err != nil || len(candidates) != 0 {
				t.Fatalf("already delivered history was repeated: %+v %v", candidates, err)
			}
		})
	}
}

func TestForgettingOneProviderRuntimeKeepsSharedConversation(t *testing.T) {
	ctx := context.Background()
	s := openTest(t, t.TempDir())
	a := bindTest(t, s, "runtime-a")
	p, err := s.ReservePreview(ctx, PreviewReservation{SourceConversationID: a.ConversationID, ClientRequestID: "preview-B", RequestHash: "B", Request: PreviewRequest{ConfigID: "B", AgentName: "omp"}})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.BindRuntime(ctx, "runtime-b", p.TargetProviderThreadID, "B", "omp"); err != nil {
		t.Fatal(err)
	}
	if err := s.ForgetRuntime(ctx, "runtime-a"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Conversation(ctx, a.ConversationID); err != nil {
		t.Fatal("deleting archived provider removed live provider", err)
	}
	if err := s.ForgetRuntime(ctx, "runtime-b"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Conversation(ctx, a.ConversationID); !errors.Is(err, ErrNotFound) {
		t.Fatal("last runtime retained private history", err)
	}
}
