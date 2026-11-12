package hybrid

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/rainchan/webmux/internal/agentprofile"
	"github.com/rainchan/webmux/internal/agentprofile/omp"
)

type heldProviderRegistry struct{ agentprofile.Agent }

func (heldProviderRegistry) RegisteredSession(agentprofile.ProcessFacts, func(string) string, string) (string, bool) {
	return "", false
}
func (heldProviderRegistry) ExternallyHeld(sid string, current agentprofile.ProcessFacts, _ func(int) (agentprofile.ProcessFacts, bool), getenv func(string) string, _ string) bool {
	return sid == "external-native" && current.Cwd != "" && getenv("REGISTRY_SCOPE") == "custom"
}

func TestArchivedProviderChecksExternalWriterInLaunchEnvironment(t *testing.T) {
	h := newHarness(t, 0)
	h.svc.agents = agentprofile.NewRegistry(heldProviderRegistry{Agent: omp.Agent})
	rec := Record{ID: "abcdefgh", Cmd: []string{"omp"}, Cwd: t.TempDir(), Agent: omp.Name, Side: SideACP, CreatedAt: time.Now(), State: StateClosed, AgentSessionID: "external-native"}
	if _, err := h.svc.records.add(rec, nil); err != nil {
		t.Fatal(err)
	}
	if err := h.svc.QuiesceProvider(context.Background(), rec.ID, func() (map[string]string, error) { return map[string]string{"REGISTRY_SCOPE": "custom"}, nil }); !errors.Is(err, ErrExternalWriter) {
		t.Fatalf("archived writer admitted: %v", err)
	}
	if err := h.svc.QuiesceProvider(context.Background(), rec.ID, func() (map[string]string, error) { return map[string]string{"REGISTRY_SCOPE": "other"}, nil }); err != nil {
		t.Fatalf("unoccupied scope rejected: %v", err)
	}
}

func TestProviderClaimRejectsPausedQueueWithoutBlockingQueueCommands(t *testing.T) {
	h := newHarness(t, 0)
	id := h.create(h.input("omp", SideACP)).Info().ID
	a, err := h.svc.actor(id)
	if err != nil {
		t.Fatal(err)
	}
	if !a.sync(func() { a.setQueue([]QueueItem{{ID: "queued", Text: "existing task"}}, true) }) {
		t.Fatal("actor stopped")
	}
	if err := h.svc.ClaimProvider(context.Background(), id, "handoff-owner"); !errors.Is(err, ErrBusy) {
		t.Fatalf("queue was claimed: %v", err)
	}
	if err := h.svc.ClearQueue(context.Background(), id, "clear-refused-queue"); err != nil {
		t.Fatalf("failed claim stranded queue: %v", err)
	}
	if !a.sync(func() {
		if a.contextExecutionID != "" {
			t.Error("refused claim installed owner")
		}
	}) {
		t.Fatal("actor stopped")
	}
}

func TestQuiesceOwnedOrUnregisteredProviderDoesNotReloadLaunch(t *testing.T) {
	for _, archived := range []bool{false, true} {
		t.Run(map[bool]string{false: "owned", true: "archived"}[archived], func(t *testing.T) {
			h := newHarness(t, 0)
			var id string
			if archived {
				rec := Record{ID: "abcdefgh", Agent: omp.Name, Cmd: []string{"omp"}, Cwd: t.TempDir(), Side: SideACP, State: StateClosed, CreatedAt: time.Now(), AgentSessionID: "archived-native"}
				if _, err := h.svc.records.add(rec, nil); err != nil {
					t.Fatal(err)
				}
				id = rec.ID
			} else {
				id = h.create(h.input("omp", SideACP)).Info().ID
			}
			queried := false
			err := h.svc.QuiesceProvider(context.Background(), id, func() (map[string]string, error) {
				queried = true
				return nil, errors.New("source launch was removed")
			})
			if err != nil || queried {
				t.Fatalf("unnecessary launch reload: queried=%v error=%v", queried, err)
			}
		})
	}
}
