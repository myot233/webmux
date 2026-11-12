package conversation

import (
	"context"
	"errors"
	"testing"
)

func injectionTest(t *testing.T, s *Service) (Binding, Preview, Execution, []InjectionItem) {
	t.Helper()
	b := bindTest(t, s, "source")
	p, e := executionTest(t, s, b)
	items := []InjectionItem{{ID: "coverage", Role: "user", TextHash: TextHash("scoped coverage")}, {ID: "activity", Role: "assistant", TextHash: TextHash("original historical result")}}
	return b, p, e, items
}

func TestInjectionCrashRetainsUnknownAndRequiresWholeOrderedNativeProof(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	s := openTest(t, dir)
	b, p, e, items := injectionTest(t, s)
	if _, err := s.ReserveInjection(ctx, e.ID, "native", items); err != nil {
		t.Fatal(err)
	}
	if err := s.ClaimInjection(ctx, e.DeliveryID); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s = openTest(t, dir)
	i, err := s.Injection(ctx, e.DeliveryID)
	if err != nil || i.State != "unknown" {
		t.Fatal(i, err)
	}
	got, err := s.Execution(ctx, e.ID)
	if err != nil || got.Status != "unknown" || got.CallStarted {
		t.Fatalf("history and user dispatch were conflated: %+v %v", got, err)
	}
	if err := s.ClaimInjection(ctx, e.DeliveryID); !errors.Is(err, ErrConflict) {
		t.Fatal("unknown injection claimed again", err)
	}
	if _, err := s.BeginExecution(ctx, ExecutionInput{SourceConversationID: b.ConversationID, HandoffID: p.ID, ClientRequestID: "retry-unknown", RequestHash: "new", Revision: 1, Retry: true}); !errors.Is(err, ErrConflict) {
		t.Fatal("unknown bypassed by new command", err)
	}
	for _, native := range []string{"other-native", "native"} {
		proof := items
		if native == "native" {
			proof = items[:1]
		}
		if err := s.ReconcileInjection(ctx, e.DeliveryID, native, proof); !errors.Is(err, ErrConflict) {
			t.Fatal("partial or wrong native proof accepted", err)
		}
	}
	if err := s.ReconcileInjection(ctx, e.DeliveryID, "native", items); err != nil {
		t.Fatal(err)
	}
	got, err = s.Execution(ctx, e.ID)
	if err != nil || got.HistoryStatus != "injected" || got.Status != "pending" || got.CallStarted {
		t.Fatal("proof dispatched user prompt", got, err)
	}
}

func TestInjectedHistoryFactSurvivesUserFailureAndSourceDeletion(t *testing.T) {
	ctx := context.Background()
	s := openTest(t, t.TempDir())
	_, _, e, items := injectionTest(t, s)
	if _, err := s.ReserveInjection(ctx, e.ID, "native", items); err != nil {
		t.Fatal(err)
	}
	if err := s.ClaimInjection(ctx, e.DeliveryID); err != nil {
		t.Fatal(err)
	}
	if err := s.FinishInjection(ctx, e.DeliveryID, "injected"); err != nil {
		t.Fatal(err)
	}
	if err := s.FinishExecution(ctx, e.ID, "failed", "user_prompt_refused"); err != nil {
		t.Fatal(err)
	}
	got, err := s.Execution(ctx, e.ID)
	if err != nil || got.Status != "failed" || got.HistoryStatus != "injected" {
		t.Fatal("user outcome erased injected history", got, err)
	}
	var status string
	if err := s.db.QueryRow("SELECT status FROM handoff_delivery WHERE id=?", e.DeliveryID).Scan(&status); err != nil || status != "injected" {
		t.Fatal("canonical delivery lost injection", status, err)
	}
	if err := s.ForgetRuntime(ctx, "source"); err != nil {
		t.Fatal(err)
	}
	if used, err := s.EstimatedNativeUsage(ctx, e.TargetProviderThreadID); err != nil || used != 100 {
		t.Fatal("source deletion lost native occupancy estimate", used, err)
	}
	if err := s.ReconcileInjection(ctx, e.DeliveryID, "native", items); err != nil {
		t.Fatal("target-owned replay receipt lost", err)
	}
	if _, err := s.Execution(ctx, e.ID); !errors.Is(err, ErrNotFound) {
		t.Fatal("replay resurrected source operation", err)
	}
}

func TestUnknownInjectionReconcilesDurablyAfterSourceDeletion(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	s := openTest(t, dir)
	_, _, e, items := injectionTest(t, s)
	if _, err := s.ReserveInjection(ctx, e.ID, "native", items); err != nil {
		t.Fatal(err)
	}
	if err := s.ClaimInjection(ctx, e.DeliveryID); err != nil {
		t.Fatal(err)
	}
	if err := s.FinishInjection(ctx, e.DeliveryID, "unknown"); err != nil {
		t.Fatal(err)
	}
	if err := s.ForgetRuntime(ctx, "source"); err != nil {
		t.Fatal(err)
	}
	var deliveries int
	if err := s.db.QueryRow("SELECT count(*) FROM handoff_delivery WHERE id=?", e.DeliveryID).Scan(&deliveries); err != nil || deliveries != 0 {
		t.Fatal("source delivery survived forgetting", deliveries, err)
	}
	items[0].ProviderItemID, items[1].ProviderItemID = "native-coverage", "native-history"
	if err := s.ReconcileInjection(ctx, e.DeliveryID, "native", items[:1]); !errors.Is(err, ErrConflict) {
		t.Fatal("source deletion admitted partial proof", err)
	}
	if err := s.ReconcileInjection(ctx, e.DeliveryID, "native", items); err != nil {
		t.Fatal("retained unknown receipt could not reconcile", err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s = openTest(t, dir)
	i, err := s.Injection(ctx, e.DeliveryID)
	if err != nil || i.State != "injected" || len(i.Items) != len(items) {
		t.Fatal("reconciliation was not durable", i, err)
	}
	for n, item := range i.Items {
		if item.ProviderItemID != items[n].ProviderItemID {
			t.Fatal("provider identity was not committed with receipt", item)
		}
	}
	if err := s.ReconcileInjection(ctx, e.DeliveryID, "native", items); err != nil {
		t.Fatal("complete replay was not idempotent", err)
	}
	if err := s.ClaimInjection(ctx, e.DeliveryID); !errors.Is(err, ErrConflict) {
		t.Fatal("reconciled history could be injected again", err)
	}
	if used, err := s.EstimatedNativeUsage(ctx, e.TargetProviderThreadID); err != nil || used != 100 {
		t.Fatal("reconciliation lost target occupancy estimate", used, err)
	}
	if _, err := s.Execution(ctx, e.ID); !errors.Is(err, ErrNotFound) {
		t.Fatal("reconciliation resurrected source operation", err)
	}
	if err := s.db.QueryRow("SELECT count(*) FROM handoff_delivery WHERE id=?", e.DeliveryID).Scan(&deliveries); err != nil || deliveries != 0 {
		t.Fatal("reconciliation recreated source delivery", deliveries, err)
	}
}

func TestReconciledReplayWithoutNewWorkDoesNotInvalidateFrozenSource(t *testing.T) {
	ctx := context.Background()
	s := openTest(t, t.TempDir())
	b, p, _, _ := injectionTest(t, s)
	replay, err := s.StartRun(ctx, b.ProviderThreadID, "load-replay")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.ReconcileReplay(ctx, b.ProviderThreadID, replay.ID); err != nil {
		t.Fatal(err)
	}
	got, err := s.Preview(ctx, p.ID)
	if err != nil || got.Stale {
		t.Fatal("empty reconciled replay counted as a new user turn", got.Stale, err)
	}
	if _, err := s.StartRun(ctx, b.ProviderThreadID, "actual-new-user-work"); err != nil {
		t.Fatal(err)
	}
	got, err = s.Preview(ctx, p.ID)
	if err != nil || !got.Stale {
		t.Fatal("new user turn failed to invalidate source", got.Stale, err)
	}
}
