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
